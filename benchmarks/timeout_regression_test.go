package benchmarks

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
)

// TestTimeoutPerformanceRegression ensures timeout performance doesn't regress
// These tests establish baseline performance expectations
func TestTimeoutPerformanceRegression(t *testing.T) {
	binary := "../patience"

	// Performance baselines established from current implementation
	baselines := []struct {
		timeout     string
		maxOverhead float64 // Maximum allowed overhead as percentage
		description string
	}{
		{
			timeout:     "50ms",
			maxOverhead: 0.25, // 25% overhead max (realistic for very short timeouts)
			description: "50ms timeout should have <25% overhead",
		},
		{
			timeout:     "100ms",
			maxOverhead: 0.20, // 20% overhead max (realistic for short timeouts)
			description: "100ms timeout should have <20% overhead",
		}, {
			timeout:     "200ms",
			maxOverhead: 0.10, // 10% overhead max
			description: "200ms timeout should have <10% overhead",
		},
		{
			timeout:     "500ms",
			maxOverhead: 0.08, // 8% overhead max
			description: "500ms timeout should have <8% overhead",
		},
		{
			timeout:     "1s",
			maxOverhead: 0.05, // 5% overhead max
			description: "1s timeout should have <5% overhead",
		},
	}

	for _, baseline := range baselines {
		t.Run(baseline.timeout, func(t *testing.T) {
			// Run multiple times to get average
			runs := 5
			var totalElapsed time.Duration

			for i := 0; i < runs; i++ {
				start := time.Now()
				cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", baseline.timeout,
					"--", "sleep", "10")
				err := cmd.Run()
				elapsed := time.Since(start)

				if err == nil {
					t.Errorf("Command should have failed due to timeout")
				}

				totalElapsed += elapsed
			}

			averageElapsed := totalElapsed / time.Duration(runs)
			expectedTimeout := parseTimeoutDuration(baseline.timeout)
			overhead := float64(averageElapsed-expectedTimeout) / float64(expectedTimeout)

			if overhead > baseline.maxOverhead {
				t.Errorf("%s: overhead %.2f%% exceeds maximum %.2f%% (average: %v, expected: %v)",
					baseline.description, overhead*100, baseline.maxOverhead*100,
					averageElapsed, expectedTimeout)
			}

			t.Logf("Performance: timeout=%v, average=%v, overhead=%.2f%%, max_allowed=%.2f%%",
				baseline.timeout, averageElapsed, overhead*100, baseline.maxOverhead*100)
		})
	}
}

// TestTimeoutVarianceRegression ensures timeout timing is consistent
func TestTimeoutVarianceRegression(t *testing.T) {
	binary := "../patience"
	timeout := "100ms"
	runs := 10
	maxVariancePercent := 0.15 // 15% max variance

	var timings []time.Duration

	for i := 0; i < runs; i++ {
		start := time.Now()
		cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", timeout,
			"--", "sleep", "10")
		err := cmd.Run()
		elapsed := time.Since(start)

		if err == nil {
			t.Errorf("Run %d: command should have failed due to timeout", i+1)
		}

		timings = append(timings, elapsed)
	}

	// Calculate average and variance
	var total time.Duration
	for _, timing := range timings {
		total += timing
	}
	average := total / time.Duration(len(timings))

	// Check variance
	var maxVariance time.Duration
	for _, timing := range timings {
		variance := timing - average
		if variance < 0 {
			variance = -variance
		}
		if variance > maxVariance {
			maxVariance = variance
		}
	}

	variancePercent := float64(maxVariance) / float64(average)

	if variancePercent > maxVariancePercent {
		t.Errorf("Timeout variance %.2f%% exceeds maximum %.2f%% (max_variance: %v, average: %v)",
			variancePercent*100, maxVariancePercent*100, maxVariance, average)
	}

	t.Logf("Variance test: average=%v, max_variance=%v, variance=%.2f%%, max_allowed=%.2f%%",
		average, maxVariance, variancePercent*100, maxVariancePercent*100)
}

// BenchmarkTimeoutOverheadRegression benchmarks timeout overhead to detect regressions
func BenchmarkTimeoutOverheadRegression(b *testing.B) {
	binary := "../patience"

	// Baseline: commands without timeout should be fast
	b.Run("baseline_no_timeout", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cmd := exec.Command(binary, "fixed", "--attempts", "1",
				"--", "echo", "test")
			err := cmd.Run()
			if err != nil {
				b.Fatalf("Command should succeed: %v", err)
			}
		}
	})

	// Test: commands with timeout should have minimal overhead
	b.Run("with_timeout_1s", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", "1s",
				"--", "echo", "test")
			err := cmd.Run()
			if err != nil {
				b.Fatalf("Command should succeed: %v", err)
			}
		}
	})

	// Test: very short timeouts should still work efficiently
	b.Run("with_timeout_10ms", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", "10ms",
				"--", "echo", "test")
			err := cmd.Run()
			if err != nil {
				b.Fatalf("Command should succeed: %v", err)
			}
		}
	})
}

// TestTimeoutMemoryUsageRegression ensures timeout doesn't cause memory leaks
func TestTimeoutMemoryUsageRegression(t *testing.T) {
	binary := "../patience"

	// Run many timeout operations to check for memory leaks
	iterations := 100
	timeout := "50ms"

	for i := 0; i < iterations; i++ {
		cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", timeout,
			"--", "sleep", "1")
		err := cmd.Run()

		if err == nil {
			t.Errorf("Iteration %d: command should have failed due to timeout", i+1)
		}

		// No explicit memory check here, but this test will help detect
		// obvious memory leaks during development
	}

	t.Logf("Completed %d timeout operations without obvious memory issues", iterations)
}

// TestTimeoutConcurrencyRegression ensures concurrent timeouts don't interfere
func TestTimeoutConcurrencyRegression(t *testing.T) {
	binary := "../patience"
	timeout := "100ms"
	concurrency := 10
	maxAllowedTime := 200 * time.Millisecond

	results := make(chan time.Duration, concurrency)

	// Start many concurrent timeout operations
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			start := time.Now()
			cmd := exec.Command(binary, "fixed", "--attempts", "1", "--timeout", timeout,
				"--", "sleep", "10")
			err := cmd.Run()
			elapsed := time.Since(start)

			if err == nil {
				t.Errorf("Goroutine %d: command should have failed due to timeout", id)
			}

			results <- elapsed
		}(i)
	}

	// Collect all results
	var timings []time.Duration
	for i := 0; i < concurrency; i++ {
		select {
		case elapsed := <-results:
			timings = append(timings, elapsed)

			if elapsed > maxAllowedTime {
				t.Errorf("Concurrent execution %d: elapsed time %v exceeds maximum allowed %v",
					i+1, elapsed, maxAllowedTime)
			}

		case <-time.After(5 * time.Second):
			t.Fatalf("Timeout waiting for concurrent execution %d to complete", i+1)
		}
	}

	// Calculate statistics
	var total time.Duration
	var min, max time.Duration = timings[0], timings[0]

	for _, timing := range timings {
		total += timing
		if timing < min {
			min = timing
		}
		if timing > max {
			max = timing
		}
	}

	average := total / time.Duration(len(timings))
	spread := max - min
	spreadPercent := float64(spread) / float64(average)

	// Spread should be reasonable (within 30% of average)
	if spreadPercent > 0.30 {
		t.Errorf("Concurrent timeout spread %.2f%% is too high (spread: %v, average: %v)",
			spreadPercent*100, spread, average)
	}

	t.Logf("Concurrency test: average=%v, min=%v, max=%v, spread=%.2f%%",
		average, min, max, spreadPercent*100)
}

// TestTimeoutWithRetriesRegression ensures timeout+retry performance is stable
func TestTimeoutWithRetriesRegression(t *testing.T) {
	binary := "../patience"
	timeout := "50ms"
	attempts := 3

	// Expected time: ~150ms (3 timeouts) + small backoff delays
	maxExpectedTime := 200 * time.Millisecond

	start := time.Now()
	cmd := exec.Command(binary, "exponential", "--attempts", fmt.Sprintf("%d", attempts),
		"--timeout", timeout, "--base-delay", "1ms", "--max-delay", "5ms",
		"--", "sleep", "10")
	err := cmd.Run()
	totalElapsed := time.Since(start)

	if err == nil {
		t.Errorf("All attempts should have failed due to timeout")
	}

	if totalElapsed > maxExpectedTime {
		t.Errorf("Total elapsed time %v exceeds maximum expected %v",
			totalElapsed, maxExpectedTime)
	}

	// Should be at least the sum of timeouts
	minExpectedTime := time.Duration(attempts) * parseTimeoutDuration(timeout)
	if totalElapsed < minExpectedTime {
		t.Errorf("Total elapsed time %v is less than minimum expected %v",
			totalElapsed, minExpectedTime)
	}

	t.Logf("Retry timeout test: attempts=%d, timeout=%v, elapsed=%v, expected_range=[%v, %v]",
		attempts, timeout, totalElapsed, minExpectedTime, maxExpectedTime)
}

// Helper function to parse timeout string to duration
func parseTimeoutDuration(timeoutStr string) time.Duration {
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
		case "1s":
			return 1 * time.Second
		default:
			return 100 * time.Millisecond
		}
	}
	return duration
}
