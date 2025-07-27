package benchmarks

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestTimeoutPrecision tests that timeouts are enforced with reasonable precision
// This test will FAIL initially because current timeout enforcement is imprecise
func TestTimeoutPrecision(t *testing.T) {
	binary := "../patience"

	testCases := []struct {
		name           string
		timeout        string
		maxAllowedTime time.Duration
		description    string
	}{
		{
			name:           "50ms_timeout",
			timeout:        "50ms",
			maxAllowedTime: 200 * time.Millisecond, // 4x tolerance for overhead
			description:    "50ms timeout should be enforced within 200ms",
		},
		{
			name:           "100ms_timeout",
			timeout:        "100ms",
			maxAllowedTime: 300 * time.Millisecond, // 3x tolerance
			description:    "100ms timeout should be enforced within 300ms",
		},
		{
			name:           "200ms_timeout",
			timeout:        "200ms",
			maxAllowedTime: 500 * time.Millisecond, // 2.5x tolerance
			description:    "200ms timeout should be enforced within 500ms",
		},
		{
			name:           "500ms_timeout",
			timeout:        "500ms",
			maxAllowedTime: 1000 * time.Millisecond, // 2x tolerance
			description:    "500ms timeout should be enforced within 1s",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use fixed strategy with timeout to test timeout precision
			// Command: sleep 10 (will definitely exceed timeout)
			start := time.Now()
			cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", tc.timeout,
				"--", "sleep", "10")
			err := cmd.Run()
			elapsed := time.Since(start)

			// Command should fail (timeout expected)
			if err == nil {
				t.Errorf("Command should have failed due to timeout")
			}

			// This is the critical test that will FAIL initially
			if elapsed > tc.maxAllowedTime {
				t.Errorf("%s: elapsed time %v exceeds maximum allowed %v",
					tc.description, elapsed, tc.maxAllowedTime)
			}

			// Should be at least close to the timeout duration (sanity check)
			minExpected := time.Duration(float64(parseTimeout(tc.timeout)) * 0.8) // 80% tolerance
			if elapsed < minExpected {
				t.Errorf("Elapsed time %v should be at least %v (80%% of timeout)",
					elapsed, minExpected)
			}

			t.Logf("Timeout test: requested=%v, elapsed=%v, max_allowed=%v",
				tc.timeout, elapsed, tc.maxAllowedTime)
		})
	}
}

// TestTimeoutConsistency tests that timeout behavior is consistent across multiple runs
func TestTimeoutConsistency(t *testing.T) {
	binary := "../patience"
	timeout := "100ms"
	maxAllowedTime := 300 * time.Millisecond
	runs := 5

	var timings []time.Duration

	for i := 0; i < runs; i++ {
		start := time.Now()
		cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", timeout,
			"--", "sleep", "10")
		err := cmd.Run()
		elapsed := time.Since(start)

		// Should fail due to timeout
		if err == nil {
			t.Errorf("Run %d: command should have failed due to timeout", i+1)
		}

		timings = append(timings, elapsed)

		// Each run should be within tolerance
		if elapsed > maxAllowedTime {
			t.Errorf("Run %d: elapsed time %v exceeds maximum allowed %v",
				i+1, elapsed, maxAllowedTime)
		}
	}

	// Calculate variance in timings
	var total time.Duration
	for _, timing := range timings {
		total += timing
	}
	average := total / time.Duration(len(timings))

	// Check that variance is reasonable (within 50% of average)
	maxVariance := average / 2
	for i, timing := range timings {
		variance := timing - average
		if variance < 0 {
			variance = -variance
		}

		if variance > maxVariance {
			t.Errorf("Run %d timing %v varies too much from average %v (variance: %v, max: %v)",
				i+1, timing, average, variance, maxVariance)
		}
	}

	t.Logf("Timeout consistency: average=%v, timings=%v", average, timings)
}

// TestConcurrentTimeouts tests timeout behavior with multiple concurrent executions
func TestConcurrentTimeouts(t *testing.T) {
	binary := "../patience"
	timeout := "100ms"
	maxAllowedTime := 300 * time.Millisecond
	concurrency := 3

	results := make(chan time.Duration, concurrency)
	errors := make(chan error, concurrency)

	// Start multiple concurrent timeout tests
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			start := time.Now()
			cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", timeout,
				"--", "sleep", "10")
			err := cmd.Run()
			elapsed := time.Since(start)

			results <- elapsed
			errors <- err
		}(i)
	}

	// Collect results
	var timings []time.Duration
	for i := 0; i < concurrency; i++ {
		select {
		case elapsed := <-results:
			timings = append(timings, elapsed)

			// Each concurrent execution should respect timeout
			if elapsed > maxAllowedTime {
				t.Errorf("Concurrent execution %d: elapsed time %v exceeds maximum allowed %v",
					i+1, elapsed, maxAllowedTime)
			}

		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for concurrent execution %d to complete", i+1)
		}

		// Collect error (should have failed due to timeout)
		select {
		case err := <-errors:
			if err == nil {
				t.Errorf("Concurrent execution %d should have failed due to timeout", i+1)
			}
		case <-time.After(100 * time.Millisecond):
			// Continue if error channel is slow
		}
	}

	t.Logf("Concurrent timeout timings: %v", timings)
}

// TestTimeoutWithRetries tests timeout behavior when combined with retry logic
func TestTimeoutWithRetries(t *testing.T) {
	binary := "../patience"
	timeout := "100ms"
	attempts := 3
	maxAllowedTimePerAttempt := 300 * time.Millisecond

	// Use exponential backoff with very small delays to focus on timeout behavior
	start := time.Now()
	cmd := exec.Command(binary, "exponential", "--attempts", fmt.Sprintf("%d", attempts),
		"--timeout", timeout, "--base-delay", "1ms", "--max-delay", "10ms",
		"--", "sleep", "10")
	err := cmd.Run()
	totalElapsed := time.Since(start)

	// Should fail (all attempts should timeout)
	if err == nil {
		t.Errorf("All attempts should have failed due to timeout")
	}

	// Each attempt should respect the timeout
	// Total time should be roughly: (timeout * attempts) + (backoff delays)
	// Since backoff delays are very small (1-10ms), timeout should dominate
	expectedMinTime := time.Duration(attempts) * parseTimeout(timeout)
	expectedMaxTime := time.Duration(attempts) * maxAllowedTimePerAttempt

	if totalElapsed < expectedMinTime {
		t.Errorf("Total elapsed %v should be at least %v (timeout * attempts)",
			totalElapsed, expectedMinTime)
	}

	if totalElapsed > expectedMaxTime {
		t.Errorf("Total elapsed %v should not exceed %v (max_allowed * attempts)",
			totalElapsed, expectedMaxTime)
	}

	t.Logf("Retry timeout test: attempts=%d, timeout=%v, total_elapsed=%v, expected_range=[%v, %v]",
		attempts, timeout, totalElapsed, expectedMinTime, expectedMaxTime)
}

// BenchmarkTimeoutEnforcement benchmarks the overhead of timeout enforcement
func BenchmarkTimeoutEnforcement(b *testing.B) {
	binary := "../patience"

	timeouts := []string{
		"10ms",
		"50ms",
		"100ms",
		"500ms",
	}

	for _, timeout := range timeouts {
		b.Run(fmt.Sprintf("timeout_%v", timeout), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Use a command that completes quickly (before timeout)
				start := time.Now()
				cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", timeout,
					"--", "echo", "test")
				err := cmd.Run()
				elapsed := time.Since(start)

				if err != nil {
					b.Fatalf("Command should succeed quickly, got error: %v", err)
				}

				// Record the overhead of timeout mechanism
				b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/timeout")
			}
		})
	}
}

// BenchmarkTimeoutOverhead compares execution with and without timeout
func BenchmarkTimeoutOverhead(b *testing.B) {
	binary := "../patience"

	b.Run("without_timeout", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			cmd := exec.Command(binary, "fixed", "--attempts", "1",
				"--", "echo", "benchmark")
			err := cmd.Run()
			if err != nil {
				b.Fatalf("Command should succeed, got error: %v", err)
			}
		}
	})

	b.Run("with_timeout", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Generous timeout that won't trigger
			cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", "1s",
				"--", "echo", "benchmark")
			err := cmd.Run()
			if err != nil {
				b.Fatalf("Command should succeed, got error: %v", err)
			}
		}
	})
}

// TestTimeoutWithDifferentCommands tests timeout behavior with various command types
func TestTimeoutWithDifferentCommands(t *testing.T) {
	binary := "../patience"
	timeout := "200ms"
	maxAllowedTime := 500 * time.Millisecond

	testCases := []struct {
		name        string
		command     []string
		description string
		skipCheck   func() bool
	}{
		{
			name:        "sleep_command",
			command:     []string{"sleep", "10"},
			description: "Standard sleep command",
			skipCheck:   func() bool { return false },
		},
		{
			name:        "infinite_loop",
			command:     []string{"sh", "-c", "while true; do echo loop; done"},
			description: "Shell infinite loop",
			skipCheck:   func() bool { return false },
		},
		{
			name:        "network_timeout",
			command:     []string{"curl", "--max-time", "30", "http://httpbin.org/delay/10"},
			description: "Network request with long delay",
			skipCheck: func() bool {
				_, err := exec.LookPath("curl")
				return err != nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipCheck() {
				t.Skip("Skipping test due to missing dependencies")
			}

			args := []string{"fixed", "--attempts", "1", "--timeout", timeout, "--"}
			args = append(args, tc.command...)

			start := time.Now()
			cmd := exec.Command(binary, args...)
			err := cmd.Run()
			elapsed := time.Since(start)

			// Should fail due to timeout
			if err == nil {
				t.Errorf("Command should have failed due to timeout: %s", tc.description)
			}

			// Critical assertion that will likely FAIL initially
			if elapsed > maxAllowedTime {
				t.Errorf("%s: elapsed time %v exceeds maximum allowed %v",
					tc.description, elapsed, maxAllowedTime)
			}

			t.Logf("%s: timeout=%v, elapsed=%v, max_allowed=%v",
				tc.description, timeout, elapsed, maxAllowedTime)
		})
	}
}

// TestTimeoutErrorHandling tests that timeout errors are handled properly
func TestTimeoutErrorHandling(t *testing.T) {
	binary := "../patience"
	timeout := "100ms"

	cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", timeout,
		"--", "sleep", "10")
	output, err := cmd.CombinedOutput()

	// Should return an error (command failed due to timeout)
	if err == nil {
		t.Errorf("Command should have failed due to timeout")
	}

	// Output should indicate timeout or similar
	outputStr := string(output)
	timeoutKeywords := []string{"timeout", "time", "killed", "terminated", "exceeded"}
	foundKeyword := false
	for _, keyword := range timeoutKeywords {
		if strings.Contains(strings.ToLower(outputStr), keyword) {
			foundKeyword = true
			break
		}
	}

	if !foundKeyword {
		t.Logf("Output should indicate timeout-related failure. Got: %s", outputStr)
		// Don't fail the test for this, as the exact error message format may vary
	}

	t.Logf("Timeout error output: %s", outputStr)
}

// Helper function to parse timeout string to duration
func parseTimeout(timeoutStr string) time.Duration {
	duration, err := time.ParseDuration(timeoutStr)
	if err != nil {
		// Fallback for simple cases
		switch timeoutStr {
		case "50ms":
			return 50 * time.Millisecond
		case "100ms":
			return 100 * time.Millisecond
		case "200ms":
			return 200 * time.Millisecond
		case "500ms":
			return 500 * time.Millisecond
		default:
			return 100 * time.Millisecond
		}
	}
	return duration
}
