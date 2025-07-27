package benchmarks

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestNetworkTimeoutStress tests patience with various timeout scenarios
func TestNetworkTimeoutStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network timeout stress test in short mode")
	}

	binary := "../patience"
	testDuration := 10 * time.Minute

	t.Logf("Starting 10-minute network timeout stress test")

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	timeoutScenarios := []struct {
		timeout     string
		attempts    string
		description string
	}{
		{"1s", "2", "Fast timeout, few attempts"},
		{"3s", "3", "Medium timeout, medium attempts"},
		{"5s", "2", "Slow timeout, few attempts"},
		{"2s", "4", "Medium timeout, many attempts"},
	}

	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	baselineMemMB := float64(baseline.Alloc) / 1024 / 1024

	cycleCount := 0
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto analysis
		case <-ticker.C:
			for _, scenario := range timeoutScenarios {
				cycleCount++

				// Test with a command that will timeout
				// Use httpbin delay endpoint that takes longer than our timeout
				cmd := exec.CommandContext(ctx, binary, "exponential",
					"--timeout", scenario.timeout,
					"--attempts", scenario.attempts,
					"--base-delay", "500ms",
					"--", "curl", "-s", "https://httpbin.org/delay/10") // 10 second delay

				start := time.Now()
				err := cmd.Run()
				duration := time.Since(start)

				// Check memory
				var current runtime.MemStats
				runtime.ReadMemStats(&current)
				currentMemMB := float64(current.Alloc) / 1024 / 1024
				memGrowth := currentMemMB - baselineMemMB

				t.Logf("Timeout test %d (%s): duration=%v, memory=%.2f MB (+%.2f), err=%v",
					cycleCount, scenario.description, duration, currentMemMB, memGrowth, err != nil)

				// Validate timeout behavior - should complete within reasonable time
				maxExpectedDuration := 30 * time.Second // Conservative upper bound
				if duration > maxExpectedDuration {
					t.Fatalf("Timeout test took too long: %v > %v for %s", duration, maxExpectedDuration, scenario.description)
				}

				// Memory should not grow excessively
				if memGrowth > 100 {
					t.Fatalf("Memory growth too high: %.2f MB for %s", memGrowth, scenario.description)
				}
			}
		}
	}

analysis:
	var final runtime.MemStats
	runtime.ReadMemStats(&final)
	finalMemMB := float64(final.Alloc) / 1024 / 1024
	totalGrowth := finalMemMB - baselineMemMB

	t.Logf("Network timeout stress test completed:")
	t.Logf("  Total cycles: %d", cycleCount)
	t.Logf("  Memory growth: %.2f MB", totalGrowth)

	if totalGrowth > 50 {
		t.Fatalf("Total memory growth too high: %.2f MB", totalGrowth)
	}

	if cycleCount < 10 {
		t.Fatalf("Too few cycles completed: %d", cycleCount)
	}

	t.Logf("✅ Network timeout stress test completed successfully")
}

func parseAttempts(s string) int {
	switch s {
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	default:
		return 1
	}
}

func parseInt(s string) int {
	switch s {
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	default:
		return 1
	}
}
