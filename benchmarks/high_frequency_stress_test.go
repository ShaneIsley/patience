package benchmarks

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// HighFrequencyMetrics tracks performance during high-frequency retry testing
type HighFrequencyMetrics struct {
	StartTime        time.Time
	EndTime          time.Time
	TotalCycles      int64
	SuccessfulCycles int64
	FailedCycles     int64
	AverageLatency   time.Duration
	MinLatency       time.Duration
	MaxLatency       time.Duration
	PeakMemoryMB     float64
	MemoryGrowthMB   float64
	BaselineMemoryMB float64
}

// TestHighFrequencyRetryStress runs rapid retry cycles for 10 minutes
func TestHighFrequencyRetryStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high-frequency stress test in short mode")
	}

	binary := "../patience"

	// Test parameters
	testDuration := 10 * time.Minute
	retryInterval := 100 * time.Millisecond               // Very fast retry cycles
	expectedCycles := int64(testDuration / retryInterval) // ~6000 cycles

	t.Logf("Starting 10-minute high-frequency retry stress test")
	t.Logf("Retry interval: %v", retryInterval)
	t.Logf("Expected cycles: ~%d", expectedCycles)

	// Initialize metrics
	metrics := &HighFrequencyMetrics{
		StartTime:  time.Now(),
		MinLatency: time.Hour, // Initialize to high value
		MaxLatency: 0,
	}

	// Get baseline memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	metrics.BaselineMemoryMB = float64(m.Alloc) / 1024 / 1024

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Start memory monitoring
	stopMonitoring := make(chan bool)
	go monitorHighFrequencyMemory(t, metrics, stopMonitoring)

	// High-frequency retry loop
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	var totalLatency time.Duration

	for {
		select {
		case <-ctx.Done():
			t.Logf("Test duration completed")
			goto analysis
		case <-ticker.C:
			// Measure latency of each retry cycle
			cycleStart := time.Now()

			// Run patience with a fast-failing command
			// Use a command that will retry quickly and fail predictably
			cmd := exec.CommandContext(ctx, binary, "fixed",
				"--attempts", "2",
				"--delay", "50ms",
				"--failure-pattern", "test-failure",
				"--", "echo", "test-failure-pattern")

			err := cmd.Run()

			cycleLatency := time.Since(cycleStart)
			totalLatency += cycleLatency

			// Update latency metrics
			if cycleLatency < metrics.MinLatency {
				metrics.MinLatency = cycleLatency
			}
			if cycleLatency > metrics.MaxLatency {
				metrics.MaxLatency = cycleLatency
			}

			metrics.TotalCycles++

			if err != nil {
				metrics.FailedCycles++
			} else {
				// This should fail due to pattern matching, so success is unexpected
				metrics.SuccessfulCycles++
			}

			// Log progress every 1000 cycles
			if metrics.TotalCycles%1000 == 0 {
				avgLatency := totalLatency / time.Duration(metrics.TotalCycles)
				t.Logf("Completed %d cycles, avg latency: %v", metrics.TotalCycles, avgLatency)
			}
		}
	}

analysis:
	// Stop monitoring
	stopMonitoring <- true

	// Calculate final metrics
	metrics.EndTime = time.Now()
	if metrics.TotalCycles > 0 {
		metrics.AverageLatency = totalLatency / time.Duration(metrics.TotalCycles)
	}

	// Get final memory usage
	runtime.ReadMemStats(&m)
	finalMemoryMB := float64(m.Alloc) / 1024 / 1024
	metrics.MemoryGrowthMB = finalMemoryMB - metrics.BaselineMemoryMB

	// Analyze results
	analyzeHighFrequencyResults(t, metrics, expectedCycles)
}

// monitorHighFrequencyMemory monitors memory usage during high-frequency testing
func monitorHighFrequencyMemory(t *testing.T, metrics *HighFrequencyMetrics, stop chan bool) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			currentMemMB := float64(m.Alloc) / 1024 / 1024

			if currentMemMB > metrics.PeakMemoryMB {
				metrics.PeakMemoryMB = currentMemMB
			}

			t.Logf("High-frequency memory check - Current: %.2f MB, Peak: %.2f MB, Cycles: %d",
				currentMemMB, metrics.PeakMemoryMB, metrics.TotalCycles)
		}
	}
}

// analyzeHighFrequencyResults analyzes and validates the high-frequency test results
func analyzeHighFrequencyResults(t *testing.T, metrics *HighFrequencyMetrics, expectedCycles int64) {
	duration := metrics.EndTime.Sub(metrics.StartTime)
	cyclesPerSecond := float64(metrics.TotalCycles) / duration.Seconds()

	t.Logf("High-frequency stress test results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Total cycles: %d (expected ~%d)", metrics.TotalCycles, expectedCycles)
	t.Logf("  Cycles per second: %.1f", cyclesPerSecond)
	t.Logf("  Successful cycles: %d", metrics.SuccessfulCycles)
	t.Logf("  Failed cycles: %d", metrics.FailedCycles)
	t.Logf("  Average latency: %v", metrics.AverageLatency)
	t.Logf("  Min latency: %v", metrics.MinLatency)
	t.Logf("  Max latency: %v", metrics.MaxLatency)
	t.Logf("  Baseline memory: %.2f MB", metrics.BaselineMemoryMB)
	t.Logf("  Peak memory: %.2f MB", metrics.PeakMemoryMB)
	t.Logf("  Memory growth: %.2f MB", metrics.MemoryGrowthMB)

	// Validation criteria

	// Should complete a reasonable number of cycles
	minExpectedCycles := expectedCycles / 2 // Allow for some overhead
	if metrics.TotalCycles < minExpectedCycles {
		t.Fatalf("Too few cycles completed: %d < %d (may indicate performance issues)",
			metrics.TotalCycles, minExpectedCycles)
	}

	// Most cycles should fail (due to failure pattern matching)
	failureRate := float64(metrics.FailedCycles) / float64(metrics.TotalCycles)
	if failureRate < 0.95 {
		t.Fatalf("Expected high failure rate due to pattern matching, got %.1f%%", failureRate*100)
	}

	// Average latency should be reasonable
	maxAcceptableLatency := 5 * time.Second // Allow for retry delays
	if metrics.AverageLatency > maxAcceptableLatency {
		t.Fatalf("Average latency too high: %v > %v", metrics.AverageLatency, maxAcceptableLatency)
	}

	// Memory growth should be minimal
	maxMemoryGrowth := 100.0 // 100MB
	if metrics.MemoryGrowthMB > maxMemoryGrowth {
		t.Fatalf("Memory growth too high: %.2f MB > %.2f MB", metrics.MemoryGrowthMB, maxMemoryGrowth)
	}

	// Peak memory should be reasonable
	maxPeakMemory := 500.0 // 500MB
	if metrics.PeakMemoryMB > maxPeakMemory {
		t.Fatalf("Peak memory usage too high: %.2f MB > %.2f MB", metrics.PeakMemoryMB, maxPeakMemory)
	}

	// Latency should be consistent (max shouldn't be too much higher than average)
	latencyRatio := float64(metrics.MaxLatency) / float64(metrics.AverageLatency)
	if latencyRatio > 10.0 {
		t.Fatalf("Latency too inconsistent: max/avg ratio %.1f > 10.0", latencyRatio)
	}

	t.Logf("✅ High-frequency stress test completed successfully")
	t.Logf("   Cycles: %d (%.1f/sec)", metrics.TotalCycles, cyclesPerSecond)
	t.Logf("   Avg latency: %v", metrics.AverageLatency)
	t.Logf("   Memory growth: %.2f MB", metrics.MemoryGrowthMB)
}
