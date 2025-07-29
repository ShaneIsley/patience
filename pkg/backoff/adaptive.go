package backoff

import (
	"fmt"
	"sync"
	"time"
)

// DelayBucket represents a range of delays for learning purposes
type DelayBucket struct {
	MinDelay     time.Duration
	MaxDelay     time.Duration
	SuccessRate  float64
	SampleCount  int
	TotalLatency time.Duration
}

// OutcomeRecord represents a single retry outcome for learning
type OutcomeRecord struct {
	Delay   time.Duration
	Success bool
	Latency time.Duration
}

// Adaptive implements a machine learning-inspired backoff strategy
// that learns from success/failure patterns to optimize retry timing
type Adaptive struct {
	fallbackStrategy Strategy
	learningRate     float64
	memoryWindow     int

	// Learning data structures (protected by mutex)
	mu             sync.RWMutex
	delayBuckets   map[int]*DelayBucket
	recentOutcomes []OutcomeRecord
	totalOutcomes  int
}

// NewAdaptive creates a new adaptive backoff strategy
// fallback is the strategy to use when insufficient learning data is available
// learningRate controls how quickly the strategy adapts (0.01-1.0)
// memoryWindow is the number of recent outcomes to remember (5-10000)
func NewAdaptive(fallback Strategy, learningRate float64, memoryWindow int) (*Adaptive, error) {
	if fallback == nil {
		return nil, fmt.Errorf("fallback strategy cannot be nil")
	}

	if learningRate <= 0 || learningRate > 1.0 {
		return nil, fmt.Errorf("learning rate must be between 0.01 and 1.0, got %f", learningRate)
	}

	if memoryWindow <= 0 || memoryWindow > 10000 {
		return nil, fmt.Errorf("memory window must be between 1 and 10000, got %d", memoryWindow)
	}

	return &Adaptive{
		fallbackStrategy: fallback,
		learningRate:     learningRate,
		memoryWindow:     memoryWindow,
		delayBuckets:     make(map[int]*DelayBucket),
		recentOutcomes:   make([]OutcomeRecord, 0, memoryWindow),
		totalOutcomes:    0,
	}, nil
}

// Delay returns the optimal delay for the given attempt based on learned patterns
func (a *Adaptive) Delay(attempt int) time.Duration {
	// For invalid attempts, use fallback
	if attempt <= 0 {
		return a.fallbackStrategy.Delay(attempt)
	}

	// Use read lock for accessing shared state
	a.mu.RLock()
	totalOutcomes := a.totalOutcomes
	a.mu.RUnlock()

	// If we have insufficient learning data, use fallback
	if totalOutcomes < 3 {
		return a.fallbackStrategy.Delay(attempt)
	}

	// Find the optimal delay based on learned success rates
	optimalDelay := a.calculateOptimalDelay(attempt)
	if optimalDelay > 0 {
		return optimalDelay
	}

	// Fall back to base strategy if no learned pattern available
	return a.fallbackStrategy.Delay(attempt)
}

// RecordOutcome records the outcome of a retry attempt for learning
func (a *Adaptive) RecordOutcome(delay time.Duration, success bool, latency time.Duration) {
	outcome := OutcomeRecord{
		Delay:   delay,
		Success: success,
		Latency: latency,
	}

	// Use write lock for modifying shared state
	a.mu.Lock()
	defer a.mu.Unlock()

	// Add to recent outcomes (FIFO buffer)
	if len(a.recentOutcomes) >= a.memoryWindow {
		// Remove oldest outcome
		a.recentOutcomes = a.recentOutcomes[1:]
	}
	a.recentOutcomes = append(a.recentOutcomes, outcome)
	a.totalOutcomes++

	// Update delay buckets with new learning
	a.updateDelayBucketsLocked()
}

// calculateOptimalDelay finds the delay with the highest success rate
func (a *Adaptive) calculateOptimalDelay(attempt int) time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var bestBucket *DelayBucket
	var bestSuccessRate float64 = -1

	// Find bucket with highest success rate and sufficient samples
	for _, bucket := range a.delayBuckets {
		if bucket.SampleCount >= 2 && bucket.SuccessRate > bestSuccessRate {
			bestSuccessRate = bucket.SuccessRate
			bestBucket = bucket
		}
	}

	if bestBucket == nil {
		return 0 // No suitable learned pattern
	}

	// Return a delay from the best bucket (middle of range)
	bucketRange := bestBucket.MaxDelay - bestBucket.MinDelay
	optimalDelay := bestBucket.MinDelay + bucketRange/2

	// Apply learning rate to blend with fallback
	fallbackDelay := a.fallbackStrategy.Delay(attempt)
	blendedDelay := a.blendDelays(optimalDelay, fallbackDelay)

	return blendedDelay
}

// blendDelays combines learned optimal delay with fallback using learning rate
func (a *Adaptive) blendDelays(optimal, fallback time.Duration) time.Duration {
	// Learning rate determines how much to trust learned vs fallback
	optimalWeight := a.learningRate
	fallbackWeight := 1.0 - a.learningRate

	blended := time.Duration(
		float64(optimal)*optimalWeight + float64(fallback)*fallbackWeight,
	)

	return blended
}

// updateDelayBucketsLocked recalculates success rates for all delay buckets
// Must be called with write lock held
func (a *Adaptive) updateDelayBucketsLocked() {
	// Initialize buckets if not already done
	if len(a.delayBuckets) == 0 {
		a.initializeBucketsLocked()
	} else {
		// Reset existing buckets
		for _, bucket := range a.delayBuckets {
			bucket.SuccessRate = 0.0
			bucket.SampleCount = 0
			bucket.TotalLatency = 0
		}
	}

	// Populate buckets with recent outcomes
	for _, outcome := range a.recentOutcomes {
		bucketIndex := a.findBucketIndexLocked(outcome.Delay)
		if bucketIndex >= 0 {
			bucket := a.delayBuckets[bucketIndex]
			bucket.SampleCount++
			bucket.TotalLatency += outcome.Latency

			// Apply exponential moving average formula: new_rate = (1-α)*old_rate + α*outcome
			// Where α = learning_rate, outcome = 1.0 for success, 0.0 for failure
			var outcomeValue float64
			if outcome.Success {
				outcomeValue = 1.0
			} else {
				outcomeValue = 0.0
			}

			// Apply EMA formula to all samples (starting from initial rate of 0.0)
			bucket.SuccessRate = (1-a.learningRate)*bucket.SuccessRate + a.learningRate*outcomeValue
		}
	}
}

// findBucketIndexLocked returns the bucket index for a given delay
// Must be called with read or write lock held
func (a *Adaptive) findBucketIndexLocked(delay time.Duration) int {
	for index, bucket := range a.delayBuckets {
		if delay >= bucket.MinDelay && delay < bucket.MaxDelay {
			return index
		}
	}

	// Handle delays larger than largest bucket
	if delay >= 300*time.Second {
		return len(a.delayBuckets) - 1 // Use largest bucket
	}

	return -1 // No suitable bucket found
}

// findBucketIndexForTesting is a test helper that exposes bucket index calculation
// This method is only used for testing bucket boundary logic
func (a *Adaptive) findBucketIndexForTesting(delay time.Duration) int {
	return a.findBucketIndexSafe(delay)
}

// getBucketSuccessRateForTesting is a test helper that returns the success rate for a bucket containing the given delay
// This method is only used for testing EMA accuracy
func (a *Adaptive) getBucketSuccessRateForTesting(delay time.Duration) float64 {
	return a.getBucketSuccessRateSafe(delay)
}

// ensureBucketsInitialized safely initializes buckets using double-checked locking
func (a *Adaptive) ensureBucketsInitialized() {
	// Fast path: check if already initialized with read lock
	a.mu.RLock()
	if len(a.delayBuckets) > 0 {
		a.mu.RUnlock()
		return
	}
	a.mu.RUnlock()

	// Slow path: initialize with write lock
	a.mu.Lock()
	defer a.mu.Unlock()

	// Double-check after acquiring write lock
	if len(a.delayBuckets) == 0 {
		a.initializeBucketsLocked()
	}
}

// findBucketIndexSafe safely finds bucket index without lock upgrade pattern
func (a *Adaptive) findBucketIndexSafe(delay time.Duration) int {
	a.ensureBucketsInitialized()

	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.findBucketIndexLocked(delay)
}

// getBucketSuccessRateSafe safely gets bucket success rate without lock upgrade pattern
func (a *Adaptive) getBucketSuccessRateSafe(delay time.Duration) float64 {
	a.ensureBucketsInitialized()

	a.mu.RLock()
	defer a.mu.RUnlock()

	bucketIndex := a.findBucketIndexLocked(delay)
	if bucketIndex < 0 {
		return 0.0
	}

	bucket, exists := a.delayBuckets[bucketIndex]
	if !exists {
		return 0.0
	}

	return bucket.SuccessRate
}

// initializeBucketsLocked initializes the delay buckets
// Must be called with write lock held
func (a *Adaptive) initializeBucketsLocked() {
	a.delayBuckets = make(map[int]*DelayBucket)

	// Define bucket ranges (exponential bucketing)
	bucketRanges := []struct {
		min, max time.Duration
	}{
		{0, 1 * time.Second},
		{1 * time.Second, 2 * time.Second},
		{2 * time.Second, 5 * time.Second},
		{5 * time.Second, 10 * time.Second},
		{10 * time.Second, 30 * time.Second},
		{30 * time.Second, 60 * time.Second},
		{60 * time.Second, 300 * time.Second}, // 5 minutes
	}

	// Initialize buckets
	for i, bucketRange := range bucketRanges {
		a.delayBuckets[i] = &DelayBucket{
			MinDelay:     bucketRange.min,
			MaxDelay:     bucketRange.max,
			SuccessRate:  0.0,
			SampleCount:  0,
			TotalLatency: 0,
		}
	}
}
