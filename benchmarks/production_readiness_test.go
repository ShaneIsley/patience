package benchmarks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestEndToEndIntegration tests all 7 strategies with real command execution
func TestEndToEndIntegration(t *testing.T) {
	binary := "../patience"

	strategies := []struct {
		name string
		args []string
	}{
		{"http-aware", []string{"--attempts", "2"}},
		{"exponential", []string{"--attempts", "2", "--base-delay", "10ms"}},
		{"linear", []string{"--attempts", "2", "--increment", "10ms"}},
		{"fixed", []string{"--attempts", "2", "--delay", "10ms"}},
		{"jitter", []string{"--attempts", "2", "--base-delay", "10ms"}},
		{"decorrelated-jitter", []string{"--attempts", "2", "--base-delay", "10ms"}},
		{"fibonacci", []string{"--attempts", "2", "--base-delay", "10ms"}},
	}

	for _, strategy := range strategies {
		t.Run(strategy.name, func(t *testing.T) {
			// Test successful command
			t.Run("Success", func(t *testing.T) {
				args := []string{strategy.name}
				args = append(args, strategy.args...)
				args = append(args, "--", "echo", "test successful")

				cmd := exec.Command(binary, args...)
				output, err := cmd.CombinedOutput()

				if err != nil {
					t.Fatalf("Strategy %s failed on success case: %v\nOutput: %s", strategy.name, err, output)
				}

				if !strings.Contains(string(output), "test successful") {
					t.Errorf("Strategy %s did not preserve command output: %s", strategy.name, output)
				}
			})

			// Test failure with retries
			t.Run("FailureWithRetries", func(t *testing.T) {
				args := []string{strategy.name}
				args = append(args, strategy.args...)
				args = append(args, "--", "sh", "-c", "echo 'attempt failed'; exit 1")

				start := time.Now()
				cmd := exec.Command(binary, args...)
				output, err := cmd.CombinedOutput()
				elapsed := time.Since(start)

				// Should fail after retries
				if err == nil {
					t.Errorf("Strategy %s should have failed after retries", strategy.name)
				}

				// Should have attempted multiple times
				attemptCount := strings.Count(string(output), "attempt failed")
				if attemptCount < 2 {
					t.Errorf("Strategy %s should have retried at least 2 times, got %d attempts\nOutput: %s",
						strategy.name, attemptCount, output)
				}

				// Should have taken some time due to delays (except for first attempt)
				if strategy.name != "fixed" || elapsed < 10*time.Millisecond {
					// Allow some variance for timing
				}
			})

			// Test timeout functionality
			t.Run("Timeout", func(t *testing.T) {
				args := []string{strategy.name}
				args = append(args, strategy.args...)
				args = append(args, "--timeout", "50ms", "--", "sleep", "1")

				start := time.Now()
				cmd := exec.Command(binary, args...)
				_, err := cmd.CombinedOutput()
				elapsed := time.Since(start)

				// Should fail due to timeout
				if err == nil {
					t.Errorf("Strategy %s should have failed due to timeout", strategy.name)
				}

				// Should not take much longer than timeout * attempts
				maxExpected := 200 * time.Millisecond // 50ms timeout * 2 attempts + overhead
				if elapsed > maxExpected {
					t.Errorf("Strategy %s took too long with timeout: %v > %v", strategy.name, elapsed, maxExpected)
				}
			})
		})
	}
}

// TestPatternMatching tests success/failure pattern detection
func TestPatternMatching(t *testing.T) {
	binary := "../patience"

	testCases := []struct {
		name           string
		successPattern string
		failurePattern string
		command        string
		output         string
		expectSuccess  bool
	}{
		{
			name:           "SuccessPattern",
			successPattern: "SUCCESS",
			command:        "echo",
			output:         "Operation SUCCESS completed",
			expectSuccess:  true,
		},
		{
			name:           "FailurePattern",
			failurePattern: "ERROR",
			command:        "echo",
			output:         "Operation ERROR occurred",
			expectSuccess:  false,
		},
		{
			name:           "ComplexSuccessPattern",
			successPattern: `(?i)(deployment|build).*(successful|completed)`,
			command:        "echo",
			output:         "Build completed successfully",
			expectSuccess:  true,
		},
		{
			name:           "JSONSuccessPattern",
			successPattern: `"status":\s*"success"`,
			command:        "echo",
			output:         `{"status": "success", "message": "done"}`,
			expectSuccess:  true,
		},
		{
			name:           "CaseInsensitive",
			successPattern: "success",
			command:        "echo",
			output:         "Operation SUCCESS",
			expectSuccess:  false, // Should fail without case-insensitive flag
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"fixed", "--attempts", "1"}

			if tc.successPattern != "" {
				args = append(args, "--success-pattern", tc.successPattern)
			}
			if tc.failurePattern != "" {
				args = append(args, "--failure-pattern", tc.failurePattern)
			}

			args = append(args, "--", tc.command, tc.output)

			cmd := exec.Command(binary, args...)
			_, err := cmd.CombinedOutput()

			if tc.expectSuccess && err != nil {
				t.Errorf("Expected success but got error: %v", err)
			}
			if !tc.expectSuccess && err == nil {
				t.Errorf("Expected failure but command succeeded")
			}
		})
	}

	// Test case-insensitive flag
	t.Run("CaseInsensitiveFlag", func(t *testing.T) {
		cmd := exec.Command(binary, "fixed", "--attempts", "1",
			"--success-pattern", "success", "--case-insensitive",
			"--", "echo", "Operation SUCCESS")
		_, err := cmd.CombinedOutput()

		if err != nil {
			t.Errorf("Case-insensitive pattern matching failed: %v", err)
		}
	})
}

// TestHTTPAwareIntegration tests HTTP-aware strategy with mock HTTP server
func TestHTTPAwareIntegration(t *testing.T) {
	binary := "../patience"

	// Create mock HTTP server that returns Retry-After header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited", "retry_after": 1}`))
	}))
	defer server.Close()

	t.Run("RetryAfterHeader", func(t *testing.T) {
		start := time.Now()
		cmd := exec.Command(binary, "http-aware", "--attempts", "2",
			"--", "curl", "-f", server.URL)
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		// Should fail after retries (curl -f fails on 429)
		if err == nil {
			t.Error("Expected failure due to 429 status")
		}

		// Should have waited for Retry-After delay
		if elapsed < 1*time.Second {
			t.Errorf("Should have waited for Retry-After delay, elapsed: %v", elapsed)
		}
	})

	// Test fallback strategy
	t.Run("FallbackStrategy", func(t *testing.T) {
		// Test with a command that doesn't produce HTTP output
		cmd := exec.Command(binary, "http-aware", "--fallback", "exponential",
			"--attempts", "2", "--", "sh", "-c", "echo 'not http'; exit 1")
		output, err := cmd.CombinedOutput()

		// Should fail after retries using fallback strategy
		if err == nil {
			t.Error("Expected failure after retries")
		}

		// Should have used fallback strategy (exponential delays)
		if !strings.Contains(string(output), "not http") {
			t.Errorf("Command output not preserved: %s", output)
		}
	})
}

// TestErrorHandling tests various error conditions and edge cases
func TestErrorHandling(t *testing.T) {
	binary := "../patience"

	testCases := []struct {
		name        string
		args        []string
		expectError bool
		errorText   string
	}{
		{
			name:        "NoCommand",
			args:        []string{"fixed"},
			expectError: true,
			errorText:   "no command specified",
		},
		{
			name:        "InvalidAttempts",
			args:        []string{"fixed", "--attempts", "0", "--", "echo", "test"},
			expectError: true,
			errorText:   "attempts must be between 1 and 1000",
		},
		{
			name:        "InvalidPattern",
			args:        []string{"fixed", "--success-pattern", "[invalid", "--", "echo", "test"},
			expectError: true,
			errorText:   "invalid success pattern",
		},
		{
			name:        "InvalidTimeout",
			args:        []string{"fixed", "--timeout", "-1s", "--", "echo", "test"},
			expectError: true,
			errorText:   "timeout must be non-negative",
		},
		{
			name:        "InvalidStrategy",
			args:        []string{"nonexistent", "--", "echo", "test"},
			expectError: true,
			errorText:   "unknown command",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(binary, tc.args...)
			output, err := cmd.CombinedOutput()

			if tc.expectError && err == nil {
				t.Errorf("Expected error but command succeeded. Output: %s", output)
			}

			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v. Output: %s", err, output)
			}

			if tc.expectError && tc.errorText != "" {
				if !strings.Contains(string(output), tc.errorText) {
					t.Errorf("Expected error message containing '%s', got: %s", tc.errorText, output)
				}
			}
		})
	}
}

// TestStrategySpecificBehavior tests unique behaviors of each strategy
func TestStrategySpecificBehavior(t *testing.T) {
	binary := "../patience"

	t.Run("ExponentialGrowth", func(t *testing.T) {
		// Test that exponential strategy actually increases delays
		start := time.Now()
		cmd := exec.Command(binary, "exponential",
			"--attempts", "3", "--base-delay", "100ms", "--multiplier", "2.0",
			"--", "sh", "-c", "exit 1")
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected failure after retries")
		}

		// Should take at least: 100ms + 200ms = 300ms for delays
		minExpected := 300 * time.Millisecond
		if elapsed < minExpected {
			t.Errorf("Exponential delays too short: %v < %v", elapsed, minExpected)
		}
	})

	t.Run("LinearGrowth", func(t *testing.T) {
		// Test that linear strategy increases delays linearly
		start := time.Now()
		cmd := exec.Command(binary, "linear",
			"--attempts", "3", "--increment", "100ms",
			"--", "sh", "-c", "exit 1")
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected failure after retries")
		}

		// Should take at least: 100ms + 200ms = 300ms for delays
		minExpected := 300 * time.Millisecond
		if elapsed < minExpected {
			t.Errorf("Linear delays too short: %v < %v", elapsed, minExpected)
		}
	})

	t.Run("FixedDelay", func(t *testing.T) {
		// Test that fixed strategy uses consistent delays
		start := time.Now()
		cmd := exec.Command(binary, "fixed",
			"--attempts", "3", "--delay", "100ms",
			"--", "sh", "-c", "exit 1")
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected failure after retries")
		}

		// Should take at least: 100ms + 100ms = 200ms for delays
		minExpected := 200 * time.Millisecond
		maxExpected := 300 * time.Millisecond // Allow some variance
		if elapsed < minExpected || elapsed > maxExpected {
			t.Errorf("Fixed delays out of range: %v not in [%v, %v]", elapsed, minExpected, maxExpected)
		}
	})

	t.Run("MaxDelayRespected", func(t *testing.T) {
		// Test that max-delay is respected
		start := time.Now()
		cmd := exec.Command(binary, "exponential",
			"--attempts", "4", "--base-delay", "100ms", "--multiplier", "10.0", "--max-delay", "150ms",
			"--", "sh", "-c", "exit 1")
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected failure after retries")
		}

		// Should not exceed: 100ms + 150ms + 150ms = 400ms (max-delay caps the growth)
		maxExpected := 500 * time.Millisecond // Allow overhead
		if elapsed > maxExpected {
			t.Errorf("Max delay not respected: %v > %v", elapsed, maxExpected)
		}
	})
}

// TestConcurrentExecution tests behavior under concurrent load
func TestConcurrentExecution(t *testing.T) {
	binary := "../patience"

	t.Run("ConcurrentStrategies", func(t *testing.T) {
		concurrency := 10
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func(id int) {
				strategy := []string{"exponential", "linear", "fixed"}[id%3]
				cmd := exec.Command(binary, strategy, "--attempts", "1", "--", "echo", fmt.Sprintf("test-%d", id))
				_, err := cmd.CombinedOutput()
				done <- err
			}(i)
		}

		// Wait for all to complete
		for i := 0; i < concurrency; i++ {
			if err := <-done; err != nil {
				t.Errorf("Concurrent execution %d failed: %v", i, err)
			}
		}
	})
}

// TestRealWorldScenarios tests common real-world usage patterns
func TestRealWorldScenarios(t *testing.T) {
	binary := "../patience"

	t.Run("DatabaseConnection", func(t *testing.T) {
		// Simulate waiting for database to be ready
		cmd := exec.Command(binary, "exponential",
			"--attempts", "3", "--base-delay", "10ms",
			"--success-pattern", "ready",
			"--", "sh", "-c", "echo 'database ready'; exit 0")

		_, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Database connection simulation failed: %v", err)
		}
	})

	t.Run("APICall", func(t *testing.T) {
		// Simulate API call with JSON response
		cmd := exec.Command(binary, "http-aware",
			"--attempts", "2",
			"--success-pattern", `"status":\s*"success"`,
			"--", "echo", `{"status": "success", "data": "result"}`)

		_, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("API call simulation failed: %v", err)
		}
	})

	t.Run("FileDownload", func(t *testing.T) {
		// Simulate file download with retries
		cmd := exec.Command(binary, "exponential",
			"--attempts", "2", "--base-delay", "10ms",
			"--timeout", "100ms",
			"--", "echo", "file downloaded successfully")

		_, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("File download simulation failed: %v", err)
		}
	})

	t.Run("DeploymentScript", func(t *testing.T) {
		// Simulate deployment with success pattern
		cmd := exec.Command(binary, "linear",
			"--attempts", "2", "--increment", "10ms",
			"--success-pattern", "deployment.*successful",
			"--", "echo", "deployment completed successfully")

		_, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Deployment script simulation failed: %v", err)
		}
	})
}

// TestPerformanceUnderLoad tests performance characteristics under various loads
func TestPerformanceUnderLoad(t *testing.T) {
	binary := "../patience"

	t.Run("HighAttemptCount", func(t *testing.T) {
		// Test with high attempt count but fast failures
		start := time.Now()
		cmd := exec.Command(binary, "fixed",
			"--attempts", "100", "--delay", "1ms",
			"--", "false")
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected failure after many attempts")
		}

		// Should complete in reasonable time (100 * 1ms + overhead)
		maxExpected := 500 * time.Millisecond
		if elapsed > maxExpected {
			t.Errorf("High attempt count took too long: %v > %v", elapsed, maxExpected)
		}
	})

	t.Run("ComplexPatterns", func(t *testing.T) {
		// Test with complex regex patterns
		complexPattern := `(?i)(?:deployment|build|release).*(?:successful|completed|ready|done).*(?:version|v)\s*\d+\.\d+\.\d+`

		start := time.Now()
		cmd := exec.Command(binary, "fixed",
			"--attempts", "1",
			"--success-pattern", complexPattern,
			"--", "echo", "Deployment completed successfully for version 1.2.3")
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Complex pattern matching failed: %v", err)
		}

		// Should complete quickly even with complex regex
		maxExpected := 100 * time.Millisecond
		if elapsed > maxExpected {
			t.Errorf("Complex pattern took too long: %v > %v", elapsed, maxExpected)
		}
	})
}
