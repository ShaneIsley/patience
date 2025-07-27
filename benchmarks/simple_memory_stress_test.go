package benchmarks

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestSimpleMemoryStress runs a simplified 60-minute memory leak test
func TestSimpleMemoryStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory stress test in short mode")
	}

	binary := "../patience"
	testDuration := 60 * time.Minute

	t.Logf("Starting 60-minute simple memory stress test")

	// Get baseline memory
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	baselineMemMB := float64(baseline.Alloc) / 1024 / 1024

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	cycleCount := 0
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto analysis
		case <-ticker.C:
			cycleCount++

			// Run a simple patience command that will retry and fail
			cmd := exec.CommandContext(ctx, binary, "fixed",
				"--attempts", "3",
				"--delay", "100ms",
				"--failure-pattern", "failure-marker",
				"--", "echo", "failure-marker-test")

			err := cmd.Run()

			// Check memory every 10 cycles (5 minutes)
			if cycleCount%10 == 0 {
				var current runtime.MemStats
				runtime.ReadMemStats(&current)
				currentMemMB := float64(current.Alloc) / 1024 / 1024
				memGrowth := currentMemMB - baselineMemMB

				t.Logf("Memory check cycle %d: %.2f MB (+%.2f), err=%v",
					cycleCount, currentMemMB, memGrowth, err != nil)

				// Fail early if memory growth is excessive
				if memGrowth > 50 {
					t.Fatalf("Memory growth too high: %.2f MB > 50 MB", memGrowth)
				}
			}
		}
	}

analysis:
	var final runtime.MemStats
	runtime.ReadMemStats(&final)
	finalMemMB := float64(final.Alloc) / 1024 / 1024
	totalGrowth := finalMemMB - baselineMemMB

	t.Logf("Simple memory stress test completed:")
	t.Logf("  Duration: %v", testDuration)
	t.Logf("  Total cycles: %d", cycleCount)
	t.Logf("  Baseline memory: %.2f MB", baselineMemMB)
	t.Logf("  Final memory: %.2f MB", finalMemMB)
	t.Logf("  Total growth: %.2f MB", totalGrowth)

	// Final validation
	if totalGrowth > 50 {
		t.Fatalf("Total memory growth too high: %.2f MB > 50 MB", totalGrowth)
	}

	if cycleCount < 100 {
		t.Fatalf("Too few cycles completed: %d < 100", cycleCount)
	}

	t.Logf("✅ Simple memory stress test completed successfully")
}
