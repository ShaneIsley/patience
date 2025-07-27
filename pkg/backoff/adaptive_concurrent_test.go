package backoff

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdaptiveStrategy_ConcurrentAccess tests concurrent access to adaptive strategy
func TestAdaptiveStrategy_ConcurrentAccess(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 50)
	require.NoError(t, err)

	const numGoroutines = 10
	const operationsPerGoroutine = 100
	var wg sync.WaitGroup

	// Track any panics or errors
	errorChan := make(chan error, numGoroutines*operationsPerGoroutine)

	// Start multiple goroutines performing concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// Mix of RecordOutcome and Delay calls
				if j%2 == 0 {
					// Record outcome
					delay := time.Duration(goroutineID*100+j) * time.Millisecond
					success := j%3 == 0 // 33% success rate
					latency := time.Duration(j*10) * time.Millisecond

					func() {
						defer func() {
							if r := recover(); r != nil {
								errorChan <- assert.AnError
							}
						}()
						adaptive.RecordOutcome(delay, success, latency)
					}()
				} else {
					// Get delay
					attempt := j%5 + 1

					func() {
						defer func() {
							if r := recover(); r != nil {
								errorChan <- assert.AnError
							}
						}()
						delay := adaptive.Delay(attempt)
						if delay <= 0 {
							errorChan <- assert.AnError
						}
					}()
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errorChan)

	// Check for any errors or panics
	errorCount := 0
	for err := range errorChan {
		if err != nil {
			errorCount++
		}
	}

	assert.Equal(t, 0, errorCount, "Should have no panics or errors during concurrent access")

	// Verify the strategy is still functional after concurrent access
	delay := adaptive.Delay(1)
	assert.True(t, delay > 0, "Strategy should still be functional after concurrent access")
}

// TestAdaptiveStrategy_RaceConditions tests for race conditions using stress testing
func TestAdaptiveStrategy_RaceConditions(t *testing.T) {
	fallback := NewExponential(500*time.Millisecond, 2.0, 30*time.Second)
	adaptive, err := NewAdaptive(fallback, 0.3, 100)
	require.NoError(t, err)

	const numGoroutines = 20
	const operationsPerGoroutine = 200
	var wg sync.WaitGroup

	// Shared state to verify consistency
	var totalRecorded int64
	var totalDelayRequests int64
	var mu sync.Mutex

	// Start stress test
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			localRecorded := 0
			localDelayRequests := 0

			for j := 0; j < operationsPerGoroutine; j++ {
				switch j % 3 {
				case 0:
					// Record successful outcome
					delay := time.Duration(goroutineID*50+j*10) * time.Millisecond
					adaptive.RecordOutcome(delay, true, 50*time.Millisecond)
					localRecorded++

				case 1:
					// Record failed outcome
					delay := time.Duration(goroutineID*30+j*5) * time.Millisecond
					adaptive.RecordOutcome(delay, false, 0)
					localRecorded++

				case 2:
					// Request delay
					attempt := (j % 10) + 1
					delay := adaptive.Delay(attempt)
					assert.True(t, delay > 0, "Delay should always be positive")
					localDelayRequests++
				}
			}

			// Update shared counters
			mu.Lock()
			totalRecorded += int64(localRecorded)
			totalDelayRequests += int64(localDelayRequests)
			mu.Unlock()
		}(i)
	}

	// Wait for completion
	wg.Wait()

	// Verify operation counts are reasonable (allow for modulo distribution variance)
	totalOperations := int64(numGoroutines * operationsPerGoroutine)

	assert.True(t, totalRecorded > totalOperations/2, "Should have recorded substantial number of outcomes")
	assert.True(t, totalDelayRequests > 0, "Should have made some delay requests")
	assert.Equal(t, totalOperations, totalRecorded+totalDelayRequests, "Total operations should match")

	// Verify strategy is still responsive and consistent
	delay1 := adaptive.Delay(1)
	delay2 := adaptive.Delay(1)
	assert.Equal(t, delay1, delay2, "Same attempt should return same delay after stress test")
}

// TestAdaptiveStrategy_ConcurrentLearning tests that learning works correctly under concurrent access
func TestAdaptiveStrategy_ConcurrentLearning(t *testing.T) {
	fallback := NewFixed(5 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.4, 30)
	require.NoError(t, err)

	const numGoroutines = 5
	const recordsPerGoroutine = 50
	var wg sync.WaitGroup

	// All goroutines will record successful outcomes with short delays
	shortDelay := 800 * time.Millisecond

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < recordsPerGoroutine; j++ {
				adaptive.RecordOutcome(shortDelay, true, 100*time.Millisecond)
				// Small delay to create more realistic timing
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// After concurrent learning, strategy should prefer shorter delays
	learnedDelay := adaptive.Delay(1)
	fallbackDelay := fallback.Delay(1)

	assert.Less(t, learnedDelay, fallbackDelay,
		"After concurrent learning of successful short delays, should prefer shorter delays than fallback")
}

// TestAdaptiveStrategy_ConcurrentMemoryWindow tests memory window behavior under concurrent access
func TestAdaptiveStrategy_ConcurrentMemoryWindow(t *testing.T) {
	fallback := NewFixed(2 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.3, 10) // Small memory window for easier testing
	require.NoError(t, err)

	const numGoroutines = 8
	const recordsPerGoroutine = 20
	var wg sync.WaitGroup

	// Record more outcomes than memory window can hold
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < recordsPerGoroutine; j++ {
				delay := time.Duration(goroutineID*100+j*50) * time.Millisecond
				success := j%2 == 0 // Alternating success/failure
				adaptive.RecordOutcome(delay, success, 50*time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify strategy is still functional and memory window is respected
	delay := adaptive.Delay(1)
	assert.True(t, delay > 0, "Strategy should be functional after concurrent memory window operations")

	// The exact behavior is hard to predict due to concurrency, but it should not crash
	// and should return reasonable delays
	assert.True(t, delay < 60*time.Second, "Delay should be reasonable even after concurrent operations")
}

// TestAdaptiveStrategy_ConcurrentStateConsistency tests internal state consistency
func TestAdaptiveStrategy_ConcurrentStateConsistency(t *testing.T) {
	fallback := NewLinear(1*time.Second, 30*time.Second)
	adaptive, err := NewAdaptive(fallback, 0.25, 40)
	require.NoError(t, err)

	const numGoroutines = 15
	const operationsPerGoroutine = 100
	var wg sync.WaitGroup

	// Mix of different delay ranges to test bucket consistency
	delayRanges := []time.Duration{
		500 * time.Millisecond,  // Bucket 0: 0-1s
		1500 * time.Millisecond, // Bucket 1: 1-2s
		3 * time.Second,         // Bucket 2: 2-5s
		8 * time.Second,         // Bucket 3: 5-10s
		20 * time.Second,        // Bucket 4: 10-30s
	}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				// Use different delay ranges to test bucket handling
				delay := delayRanges[j%len(delayRanges)]
				success := (goroutineID+j)%3 != 0 // ~67% success rate
				latency := time.Duration(j*5) * time.Millisecond

				adaptive.RecordOutcome(delay, success, latency)

				// Occasionally request delays to test read consistency
				if j%10 == 0 {
					attempt := (j/10)%5 + 1
					resultDelay := adaptive.Delay(attempt)
					assert.True(t, resultDelay > 0, "Delay should always be positive during concurrent operations")
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify final state consistency
	// Multiple calls should return consistent results
	delays := make([]time.Duration, 5)
	for i := 0; i < 5; i++ {
		delays[i] = adaptive.Delay(i + 1)
		assert.True(t, delays[i] > 0, "All delays should be positive")
	}

	// Verify delays are still consistent after concurrent operations
	for i := 0; i < 5; i++ {
		secondCall := adaptive.Delay(i + 1)
		assert.Equal(t, delays[i], secondCall, "Delay should be consistent for same attempt")
	}
}
