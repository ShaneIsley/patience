package benchmarks

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestLargeOutputStress tests patience with commands that generate large outputs
func TestLargeOutputStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large output stress test in short mode")
	}

	binary := "../patience"
	testDuration := 10 * time.Minute

	t.Logf("Starting 10-minute large output handling stress test")

	// Get baseline memory
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	baselineMemMB := float64(baseline.Alloc) / 1024 / 1024

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	testCases := []struct {
		name        string
		outputSize  string
		description string
	}{
		{"small", "1000", "1KB output"},
		{"medium", "100000", "100KB output"},
		{"large", "1000000", "1MB output"},
		{"xlarge", "5000000", "5MB output"},
	}

	cycleCount := 0
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto analysis
		case <-ticker.C:
			for _, tc := range testCases {
				cycleCount++

				// Create command that generates large output
				// Use printf to generate predictable large output
				outputCmd := fmt.Sprintf("printf '%%*s' %s ''", tc.outputSize)

				// Run patience with the large output command
				cmd := exec.CommandContext(ctx, binary, "fixed",
					"--attempts", "2",
					"--delay", "100ms",
					"--success-pattern", "success-marker-not-found", // Will fail and retry
					"--", "sh", "-c", outputCmd)

				start := time.Now()
				err := cmd.Run()
				duration := time.Since(start)

				// Check memory after processing large output
				var current runtime.MemStats
				runtime.ReadMemStats(&current)
				currentMemMB := float64(current.Alloc) / 1024 / 1024
				memGrowth := currentMemMB - baselineMemMB

				t.Logf("Cycle %d (%s): duration=%v, memory=%.2f MB (+%.2f), err=%v",
					cycleCount, tc.description, duration, currentMemMB, memGrowth, err != nil)

				// Validate memory usage doesn't grow excessively
				if memGrowth > 200 { // 200MB growth limit
					t.Fatalf("Memory growth too high after %s output: %.2f MB", tc.description, memGrowth)
				}

				// Validate reasonable processing time
				maxDuration := 30 * time.Second
				if duration > maxDuration {
					t.Fatalf("Processing took too long for %s output: %v > %v", tc.description, duration, maxDuration)
				}
			}

			// Force garbage collection to clean up
			runtime.GC()
		}
	}

analysis:
	// Final memory check
	var final runtime.MemStats
	runtime.ReadMemStats(&final)
	finalMemMB := float64(final.Alloc) / 1024 / 1024
	totalGrowth := finalMemMB - baselineMemMB

	t.Logf("Large output stress test completed:")
	t.Logf("  Total cycles: %d", cycleCount)
	t.Logf("  Baseline memory: %.2f MB", baselineMemMB)
	t.Logf("  Final memory: %.2f MB", finalMemMB)
	t.Logf("  Total growth: %.2f MB", totalGrowth)

	// Final validation
	if totalGrowth > 100 {
		t.Fatalf("Total memory growth too high: %.2f MB > 100 MB", totalGrowth)
	}

	if cycleCount < 10 {
		t.Fatalf("Too few test cycles completed: %d < 10", cycleCount)
	}

	t.Logf("✅ Large output stress test completed successfully")
}
