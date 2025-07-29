package backoff

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdaptiveSafeLocking tests the new safe locking mechanism
func TestAdaptiveSafeLocking(t *testing.T) {
	t.Run("ensureBucketsInitialized is thread-safe", func(t *testing.T) {
		strategy, err := NewAdaptive(&Fixed{Duration: time.Second}, 0.1, 100)
		require.NoError(t, err)

		// Test concurrent initialization
		const numGoroutines = 10
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				strategy.ensureBucketsInitialized()
			}()
		}

		wg.Wait()

		// Verify buckets are initialized exactly once
		strategy.mu.RLock()
		bucketsInitialized := len(strategy.delayBuckets) > 0
		strategy.mu.RUnlock()

		assert.True(t, bucketsInitialized, "Buckets should be initialized")
	})

	t.Run("findBucketIndexSafe does not deadlock", func(t *testing.T) {
		strategy, err := NewAdaptive(&Fixed{Duration: time.Second}, 0.1, 100)
		require.NoError(t, err)

		// Test concurrent access to findBucketIndexSafe
		const numGoroutines = 20
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		results := make([]int, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				defer wg.Done()
				results[index] = strategy.findBucketIndexSafe(time.Second)
			}(i)
		}

		// This should complete without deadlock
		done := make(chan bool)
		go func() {
			wg.Wait()
			done <- true
		}()

		select {
		case <-done:
			// Success - no deadlock
		case <-time.After(5 * time.Second):
			t.Fatal("Test timed out - likely deadlock detected")
		}

		// Verify all results are valid
		for i, result := range results {
			assert.GreaterOrEqual(t, result, 0, "Result %d should be non-negative", i)
		}
	})

	t.Run("getBucketSuccessRateSafe is thread-safe", func(t *testing.T) {
		strategy, err := NewAdaptive(&Fixed{Duration: time.Second}, 0.1, 100)
		require.NoError(t, err)

		// Record some outcomes first
		strategy.RecordOutcome(time.Second, true, 100*time.Millisecond)
		strategy.RecordOutcome(time.Second, false, 200*time.Millisecond)

		// Test concurrent access
		const numGoroutines = 15
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		results := make([]float64, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				defer wg.Done()
				results[index] = strategy.getBucketSuccessRateSafe(time.Second)
			}(i)
		}

		wg.Wait()

		// Verify all results are valid (between 0 and 1)
		for i, result := range results {
			assert.GreaterOrEqual(t, result, 0.0, "Result %d should be >= 0", i)
			assert.LessOrEqual(t, result, 1.0, "Result %d should be <= 1", i)
		}
	})

	t.Run("no double unlock with safe methods", func(t *testing.T) {
		strategy, err := NewAdaptive(&Fixed{Duration: time.Second}, 0.1, 100)
		require.NoError(t, err)

		// This test ensures the new safe methods don't have the double-unlock bug
		// If there's a double unlock, this will panic
		assert.NotPanics(t, func() {
			for i := 0; i < 100; i++ {
				strategy.findBucketIndexSafe(time.Duration(i) * time.Millisecond)
				strategy.getBucketSuccessRateSafe(time.Duration(i) * time.Millisecond)
			}
		})
	})
}

// TestAdaptiveRaceConditions tests for race conditions using Go's race detector
func TestAdaptiveRaceConditions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition test in short mode")
	}

	strategy, err := NewAdaptive(&Fixed{Duration: time.Second}, 0.1, 100)
	require.NoError(t, err)

	// Run concurrent operations that previously could cause races
	const numGoroutines = 50
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				delay := time.Duration(j%10+1) * time.Millisecond

				// Mix of read and write operations
				switch j % 4 {
				case 0:
					strategy.Delay(j % 5)
				case 1:
					strategy.RecordOutcome(delay, j%2 == 0, delay)
				case 2:
					strategy.findBucketIndexSafe(delay)
				case 3:
					strategy.getBucketSuccessRateSafe(delay)
				}
			}
		}(i)
	}

	wg.Wait()
}

// Benchmark the new safe locking mechanism
func BenchmarkAdaptiveSafeLocking(b *testing.B) {
	strategy, err := NewAdaptive(&Fixed{Duration: time.Second}, 0.1, 1000)
	require.NoError(b, err)

	b.Run("findBucketIndexSafe", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			strategy.findBucketIndexSafe(time.Duration(i%1000) * time.Millisecond)
		}
	})

	b.Run("getBucketSuccessRateSafe", func(b *testing.B) {
		// Pre-populate with some data
		for i := 0; i < 100; i++ {
			strategy.RecordOutcome(time.Duration(i)*time.Millisecond, i%2 == 0, time.Duration(i)*time.Millisecond)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			strategy.getBucketSuccessRateSafe(time.Duration(i%1000) * time.Millisecond)
		}
	})

	b.Run("concurrent_mixed_operations", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				delay := time.Duration(i%100) * time.Millisecond
				switch i % 3 {
				case 0:
					strategy.Delay(i % 5)
				case 1:
					strategy.findBucketIndexSafe(delay)
				case 2:
					strategy.getBucketSuccessRateSafe(delay)
				}
				i++
			}
		})
	})
}
