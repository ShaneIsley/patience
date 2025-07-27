package benchmarks

import (
	"context"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ConcurrentStressMetrics tracks metrics during concurrent execution
type ConcurrentStressMetrics struct {
	StartTime      time.Time
	EndTime        time.Time
	TotalProcesses int64
	SuccessfulRuns int64
	FailedRuns     int64
	TimeoutRuns    int64
	MaxConcurrent  int64
	CurrentRunning int64
	PeakMemoryMB   float64
	PeakGoroutines int
}

// TestConcurrentProcessStress runs 100 concurrent patience processes for 10 minutes
func TestConcurrentProcessStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent stress test in short mode")
	}

	binary := "../patience"

	// Test parameters
	testDuration := 10 * time.Minute
	maxConcurrentProcesses := int64(100)
	processSpawnInterval := 100 * time.Millisecond // Spawn new process every 100ms

	t.Logf("Starting 10-minute concurrent process stress test")
	t.Logf("Max concurrent processes: %d", maxConcurrentProcesses)
	t.Logf("Process spawn interval: %v", processSpawnInterval)

	// Initialize metrics
	metrics := &ConcurrentStressMetrics{
		StartTime: time.Now(),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Channel to control concurrent process count
	semaphore := make(chan struct{}, maxConcurrentProcesses)

	// WaitGroup to track all processes
	var wg sync.WaitGroup

	// Start memory monitoring
	stopMonitoring := make(chan bool)
	go monitorResourceUsage(t, metrics, stopMonitoring)

	// Process spawning loop
	spawnTicker := time.NewTicker(processSpawnInterval)
	defer spawnTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Logf("Test duration completed, waiting for processes to finish...")
			goto cleanup
		case <-spawnTicker.C:
			// Try to acquire semaphore (non-blocking)
			select {
			case semaphore <- struct{}{}:
				// Successfully acquired, spawn process
				wg.Add(1)
				atomic.AddInt64(&metrics.TotalProcesses, 1)
				atomic.AddInt64(&metrics.CurrentRunning, 1)

				// Update max concurrent if needed
				current := atomic.LoadInt64(&metrics.CurrentRunning)
				for {
					max := atomic.LoadInt64(&metrics.MaxConcurrent)
					if current <= max || atomic.CompareAndSwapInt64(&metrics.MaxConcurrent, max, current) {
						break
					}
				}

				go runConcurrentPatienceProcess(t, binary, semaphore, &wg, metrics)
			default:
				// Semaphore full, skip this spawn
				continue
			}
		}
	}

cleanup:
	// Stop spawning new processes and wait for existing ones
	spawnTicker.Stop()

	// Wait for all processes to complete (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("All processes completed successfully")
	case <-time.After(30 * time.Second):
		t.Logf("Timeout waiting for processes to complete")
	}

	// Stop monitoring
	stopMonitoring <- true

	// Final metrics
	metrics.EndTime = time.Now()

	// Analyze results
	analyzeStressTestResults(t, metrics)
}

// runConcurrentPatienceProcess runs a single patience process
func runConcurrentPatienceProcess(t *testing.T, binary string, semaphore chan struct{}, wg *sync.WaitGroup, metrics *ConcurrentStressMetrics) {
	defer func() {
		<-semaphore // Release semaphore
		wg.Done()
		atomic.AddInt64(&metrics.CurrentRunning, -1)
	}()

	// Create a context with timeout for this specific process
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run patience with a command that will retry and eventually fail
	// This exercises the full retry logic without overwhelming external services
	cmd := exec.CommandContext(ctx, binary, "exponential",
		"--attempts", "3",
		"--base-delay", "500ms",
		"--failure-pattern", "nonexistent",
		"--", "echo", "test-nonexistent-pattern")

	err := cmd.Run()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			atomic.AddInt64(&metrics.TimeoutRuns, 1)
		} else {
			atomic.AddInt64(&metrics.FailedRuns, 1)
		}
	} else {
		// This should actually fail due to the failure pattern, so success is unexpected
		atomic.AddInt64(&metrics.SuccessfulRuns, 1)
	}
}

// monitorResourceUsage monitors system resources during the test
func monitorResourceUsage(t *testing.T, metrics *ConcurrentStressMetrics, stop chan bool) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// Monitor memory usage
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			currentMemMB := float64(m.Alloc) / 1024 / 1024

			if currentMemMB > metrics.PeakMemoryMB {
				metrics.PeakMemoryMB = currentMemMB
			}

			// Monitor goroutines
			currentGoroutines := runtime.NumGoroutine()
			if currentGoroutines > metrics.PeakGoroutines {
				metrics.PeakGoroutines = currentGoroutines
			}

			running := atomic.LoadInt64(&metrics.CurrentRunning)
			total := atomic.LoadInt64(&metrics.TotalProcesses)

			t.Logf("Resource check - Running: %d, Total spawned: %d, Memory: %.2f MB, Goroutines: %d",
				running, total, currentMemMB, currentGoroutines)
		}
	}
}

// analyzeStressTestResults analyzes and validates the stress test results
func analyzeStressTestResults(t *testing.T, metrics *ConcurrentStressMetrics) {
	duration := metrics.EndTime.Sub(metrics.StartTime)

	t.Logf("Concurrent stress test results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Total processes spawned: %d", metrics.TotalProcesses)
	t.Logf("  Successful runs: %d", metrics.SuccessfulRuns)
	t.Logf("  Failed runs: %d", metrics.FailedRuns)
	t.Logf("  Timeout runs: %d", metrics.TimeoutRuns)
	t.Logf("  Max concurrent processes: %d", metrics.MaxConcurrent)
	t.Logf("  Peak memory usage: %.2f MB", metrics.PeakMemoryMB)
	t.Logf("  Peak goroutines: %d", metrics.PeakGoroutines)

	// Validation criteria
	if metrics.TotalProcesses <= 100 {
		t.Fatalf("Should have spawned at least 100 processes, got %d", metrics.TotalProcesses)
	}

	if metrics.MaxConcurrent > 100 {
		t.Fatalf("Should not exceed max concurrent limit of 100, got %d", metrics.MaxConcurrent)
	}

	// Most processes should complete (allowing for some timeouts under stress)
	completedProcesses := metrics.SuccessfulRuns + metrics.FailedRuns
	completionRate := float64(completedProcesses) / float64(metrics.TotalProcesses)
	if completionRate < 0.90 {
		t.Fatalf("At least 90%% of processes should complete (not timeout), got %.1f%%", completionRate*100)
	}

	// Memory usage should be reasonable
	if metrics.PeakMemoryMB > 500.0 {
		t.Fatalf("Peak memory usage should not exceed 500MB, got %.2f MB", metrics.PeakMemoryMB)
	}

	// Goroutine count should be reasonable
	if metrics.PeakGoroutines > 1000 {
		t.Fatalf("Peak goroutine count should not exceed 1000, got %d", metrics.PeakGoroutines)
	}

	// Timeout rate should be low
	timeoutRate := float64(metrics.TimeoutRuns) / float64(metrics.TotalProcesses)
	if timeoutRate > 0.05 {
		t.Fatalf("Timeout rate should be less than 5%%, got %.1f%%", timeoutRate*100)
	}
	t.Logf("✅ Concurrent stress test completed successfully")
	t.Logf("   Processes: %d (%.1f%% completion rate)", metrics.TotalProcesses, completionRate*100)
	t.Logf("   Peak concurrent: %d", metrics.MaxConcurrent)
	t.Logf("   Peak memory: %.2f MB", metrics.PeakMemoryMB)
}
