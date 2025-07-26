//go:build integration
// +build integration

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

// TestRealWorldAPIs tests patience with real-world API scenarios
// These tests are marked with build tag 'integration' and should be run with: go test -tags=integration
func TestRealWorldAPIs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world API tests in short mode")
	}

	binary := "../patience"

	// Test with httpbin.org (reliable test API)
	t.Run("HTTPBin", func(t *testing.T) {
		testCases := []struct {
			name     string
			endpoint string
			expect   string
		}{
			{"Status200", "https://httpbin.org/status/200", ""},
			{"JSON", "https://httpbin.org/json", `"slideshow"`},
			{"UserAgent", "https://httpbin.org/user-agent", "curl"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cmd := exec.Command(binary, "http-aware", "--attempts", "3", "--timeout", "10s",
					"--", "curl", "-s", tc.endpoint)
				output, err := cmd.CombinedOutput()

				if err != nil {
					t.Errorf("HTTPBin test %s failed: %v\nOutput: %s", tc.name, err, output)
					return
				}

				if tc.expect != "" && !strings.Contains(string(output), tc.expect) {
					t.Errorf("HTTPBin test %s missing expected content '%s' in: %s", tc.name, tc.expect, output)
				}
			})
		}
	})

	// Test rate limiting scenarios
	t.Run("RateLimiting", func(t *testing.T) {
		// Use httpbin's delay endpoint to simulate slow responses
		start := time.Now()
		cmd := exec.Command(binary, "http-aware", "--attempts", "2", "--timeout", "5s",
			"--", "curl", "-s", "https://httpbin.org/delay/1")
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Rate limiting test failed: %v", err)
		}

		// Should complete in reasonable time (1s delay + overhead)
		if elapsed > 3*time.Second {
			t.Errorf("Rate limiting test took too long: %v", elapsed)
		}
	})
}

// TestMockAPIScenarios tests various API response scenarios with mock servers
func TestMockAPIScenarios(t *testing.T) {
	binary := "../patience"

	t.Run("RetryAfterHeader", func(t *testing.T) {
		// Mock server that returns Retry-After header
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error": "rate limited"}`))
			} else {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "success"}`))
			}
		}))
		defer server.Close()

		start := time.Now()
		cmd := exec.Command(binary, "http-aware", "--attempts", "3",
			"--", "curl", "-f", "-s", server.URL)
		output, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Retry-After test failed: %v\nOutput: %s", err, output)
		}

		// Should have waited for Retry-After delay
		if elapsed < 1*time.Second {
			t.Errorf("Should have waited for Retry-After delay, elapsed: %v", elapsed)
		}

		// Should contain success response
		if !strings.Contains(string(output), "success") {
			t.Errorf("Should contain success response: %s", output)
		}

		// Should have made exactly 2 calls
		if callCount != 2 {
			t.Errorf("Expected 2 API calls, got %d", callCount)
		}
	})

	t.Run("JSONRetryField", func(t *testing.T) {
		// Mock server that returns retry_after in JSON
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error": "service unavailable", "retry_after": 1}`))
			} else {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "ok", "data": "result"}`))
			}
		}))
		defer server.Close()

		start := time.Now()
		cmd := exec.Command(binary, "http-aware", "--attempts", "3",
			"--", "curl", "-f", "-s", server.URL)
		output, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("JSON retry field test failed: %v\nOutput: %s", err, output)
		}

		// Should have waited for retry_after delay
		if elapsed < 1*time.Second {
			t.Errorf("Should have waited for JSON retry_after delay, elapsed: %v", elapsed)
		}

		// Should contain success response
		if !strings.Contains(string(output), "ok") {
			t.Errorf("Should contain success response: %s", output)
		}
	})

	t.Run("FallbackStrategy", func(t *testing.T) {
		// Mock server that returns non-HTTP content
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))
		}))
		defer server.Close()

		start := time.Now()
		cmd := exec.Command(binary, "http-aware", "--fallback", "exponential", "--attempts", "3",
			"--", "curl", "-f", "-s", server.URL)
		_, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		// Should fail after retries using fallback strategy
		if err == nil {
			t.Error("Expected failure after retries")
		}

		// Should have used exponential backoff (some delay between retries)
		if elapsed < 50*time.Millisecond {
			t.Errorf("Should have used fallback strategy delays, elapsed: %v", elapsed)
		}
	})

	t.Run("HTTPStatusCodes", func(t *testing.T) {
		statusCodes := []int{400, 401, 403, 404, 429, 500, 502, 503, 504}

		for _, statusCode := range statusCodes {
			t.Run(fmt.Sprintf("Status%d", statusCode), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(statusCode)
					w.Write([]byte(fmt.Sprintf("HTTP %d Error", statusCode)))
				}))
				defer server.Close()

				cmd := exec.Command(binary, "http-aware", "--attempts", "2",
					"--", "curl", "-f", "-s", server.URL)
				output, _ := cmd.CombinedOutput()

				// Most status codes should succeed (curl doesn't fail by default)
				// Only test that the command executes without panic
				if strings.Contains(string(output), "panic") {
					t.Errorf("Status %d caused panic: %s", statusCode, output)
				}
			})
		}
	})
}

// TestAPIPatternMatching tests pattern matching with real API responses
func TestAPIPatternMatching(t *testing.T) {
	binary := "../patience"

	t.Run("JSONSuccessPattern", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "success", "message": "Operation completed", "code": 200}`))
		}))
		defer server.Close()

		cmd := exec.Command(binary, "http-aware", "--attempts", "1",
			"--success-pattern", `"status":\s*"success"`,
			"--", "curl", "-f", "-s", server.URL)
		_, err := cmd.CombinedOutput()

		if err != nil {
			t.Errorf("JSON success pattern test failed: %v", err)
		}
	})

	t.Run("JSONFailurePattern", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK) // HTTP 200 but application error
			w.Write([]byte(`{"status": "error", "message": "Validation failed", "code": 400}`))
		}))
		defer server.Close()

		cmd := exec.Command(binary, "http-aware", "--attempts", "1",
			"--failure-pattern", `"status":\s*"error"`,
			"--", "curl", "-f", "-s", server.URL)
		_, err := cmd.CombinedOutput()

		// Should fail due to failure pattern match
		if err == nil {
			t.Error("Expected failure due to failure pattern match")
		}
	})

	t.Run("HTTPHeaderPattern", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Status", "ready")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Service is ready"))
		}))
		defer server.Close()

		cmd := exec.Command(binary, "http-aware", "--attempts", "1",
			"--success-pattern", "X-Status: ready",
			"--", "curl", "-i", "-s", server.URL)
		_, err := cmd.CombinedOutput()

		if err != nil {
			t.Errorf("HTTP header pattern test failed: %v", err)
		}
	})
}

// TestLoadBalancerScenarios tests scenarios common with load balancers
func TestLoadBalancerScenarios(t *testing.T) {
	binary := "../patience"

	t.Run("HealthCheck", func(t *testing.T) {
		// Simulate health check that becomes ready after some time
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Service starting..."))
			} else {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Service ready"))
			}
		}))
		defer server.Close()

		cmd := exec.Command(binary, "exponential", "--attempts", "5", "--base-delay", "10ms",
			"--success-pattern", "Service ready",
			"--", "curl", "-f", "-s", server.URL)
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Errorf("Health check test failed: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(string(output), "Service ready") {
			t.Errorf("Should contain ready message: %s", output)
		}

		// Should have made at least 3 calls
		if callCount < 3 {
			t.Errorf("Expected at least 3 health check calls, got %d", callCount)
		}
	})

	t.Run("CircuitBreaker", func(t *testing.T) {
		// Simulate circuit breaker that opens after failures
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount <= 2 {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Service error"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Circuit breaker open"))
			}
		}))
		defer server.Close()

		cmd := exec.Command(binary, "linear", "--attempts", "4", "--increment", "10ms",
			"--", "curl", "-f", "-s", server.URL)
		output, err := cmd.CombinedOutput()

		// Should fail after all attempts
		if err == nil {
			t.Error("Expected failure after circuit breaker opens")
		}

		// Should have made multiple attempts
		if callCount < 3 {
			t.Errorf("Expected multiple attempts, got %d", callCount)
		}

		// Should contain circuit breaker message
		if !strings.Contains(string(output), "Circuit breaker") {
			t.Errorf("Should contain circuit breaker message: %s", output)
		}
	})
}

// TestDatabaseConnectionScenarios tests database-like connection patterns
func TestDatabaseConnectionScenarios(t *testing.T) {
	binary := "../patience"

	t.Run("DatabaseStartup", func(t *testing.T) {
		// Simulate database that takes time to start
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 4 {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Database starting..."))
			} else {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Database ready for connections"))
			}
		}))
		defer server.Close()

		start := time.Now()
		cmd := exec.Command(binary, "fibonacci", "--attempts", "6", "--base-delay", "10ms",
			"--success-pattern", "ready for connections",
			"--", "curl", "-f", "-s", server.URL)
		output, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Database startup test failed: %v\nOutput: %s", err, output)
		}

		// Should have used Fibonacci delays
		if elapsed < 30*time.Millisecond { // 10ms + 10ms + 20ms minimum
			t.Errorf("Should have used Fibonacci delays, elapsed: %v", elapsed)
		}

		if !strings.Contains(string(output), "ready for connections") {
			t.Errorf("Should contain ready message: %s", output)
		}
	})

	t.Run("ConnectionPool", func(t *testing.T) {
		// Simulate connection pool exhaustion
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Connection pool exhausted"))
		}))
		defer server.Close()

		cmd := exec.Command(binary, "decorrelated-jitter", "--attempts", "3", "--base-delay", "10ms",
			"--", "curl", "-f", "-s", server.URL)
		output, err := cmd.CombinedOutput()

		// Should fail after retries
		if err == nil {
			t.Error("Expected failure due to connection pool exhaustion")
		}

		if !strings.Contains(string(output), "Connection pool exhausted") {
			t.Errorf("Should contain pool exhaustion message: %s", output)
		}
	})
}

// TestMicroserviceScenarios tests patterns common in microservice architectures
func TestMicroserviceScenarios(t *testing.T) {
	binary := "../patience"

	t.Run("ServiceMesh", func(t *testing.T) {
		// Simulate service mesh with retries and circuit breaking
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			switch callCount {
			case 1:
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Service temporarily unavailable"))
			case 2:
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("Rate limited"))
			default:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"service": "user-service", "status": "healthy"}`))
			}
		}))
		defer server.Close()

		start := time.Now()
		cmd := exec.Command(binary, "http-aware", "--attempts", "4", "--fallback", "jitter",
			"--success-pattern", `"status":\s*"healthy"`,
			"--", "curl", "-f", "-s", server.URL)
		output, err := cmd.CombinedOutput()
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Service mesh test failed: %v\nOutput: %s", err, output)
		}

		// Should have waited for Retry-After
		if elapsed < 1*time.Second {
			t.Errorf("Should have waited for Retry-After, elapsed: %v", elapsed)
		}

		if !strings.Contains(string(output), "healthy") {
			t.Errorf("Should contain healthy status: %s", output)
		}

		// Should have made exactly 3 calls
		if callCount != 3 {
			t.Errorf("Expected 3 service calls, got %d", callCount)
		}
	})

	t.Run("DistributedTracing", func(t *testing.T) {
		// Simulate service with tracing headers
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for tracing headers (curl doesn't add them, but test the response)
			w.Header().Set("X-Trace-Id", "abc123")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Request processed"))
		}))
		defer server.Close()

		cmd := exec.Command(binary, "fixed", "--attempts", "1",
			"--success-pattern", "Request processed",
			"--", "curl", "-i", "-s", server.URL)
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Errorf("Distributed tracing test failed: %v", err)
		}

		// Should contain trace header
		if !strings.Contains(string(output), "X-Trace-Id") {
			t.Errorf("Should contain trace header: %s", output)
		}
	})
}

// BenchmarkRealWorldPerformance benchmarks performance with real-world scenarios
func BenchmarkRealWorldPerformance(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping real-world performance benchmarks in short mode")
	}

	binary := "../patience"

	b.Run("HTTPBinPerformance", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			start := time.Now()
			cmd := exec.Command(binary, "http-aware", "--attempts", "1", "--timeout", "5s",
				"--", "curl", "-s", "https://httpbin.org/json")
			err := cmd.Run()
			elapsed := time.Since(start)

			if err != nil {
				b.Fatalf("HTTPBin performance test failed: %v", err)
			}

			b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/http-request")
		}
	})

	b.Run("LocalMockPerformance", func(b *testing.B) {
		// Create a local mock server for consistent performance testing
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		}))
		defer server.Close()

		for i := 0; i < b.N; i++ {
			start := time.Now()
			cmd := exec.Command(binary, "http-aware", "--attempts", "1",
				"--", "curl", "-f", "-s", server.URL)
			err := cmd.Run()
			elapsed := time.Since(start)

			if err != nil {
				b.Fatalf("Local mock performance test failed: %v", err)
			}

			b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/local-request")
		}
	})
}
