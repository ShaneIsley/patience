package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdaptiveStrategy_EMAAccuracy tests the mathematical accuracy of exponential moving average
func TestAdaptiveStrategy_EMAAccuracy(t *testing.T) {
	fallback := NewFixed(2 * time.Second)

	testCases := []struct {
		name         string
		learningRate float64
		outcomes     []bool // true = success, false = failure
		expectedRate float64
		tolerance    float64
	}{
		{
			name:         "all successes with 0.1 learning rate",
			learningRate: 0.1,
			outcomes:     []bool{true, true, true, true, true},
			expectedRate: 0.41, // EMA with 5 successes: 0.1 -> 0.19 -> 0.271 -> 0.344 -> 0.41
			tolerance:    0.01,
		},
		{
			name:         "all failures with 0.1 learning rate",
			learningRate: 0.1,
			outcomes:     []bool{false, false, false, false, false},
			expectedRate: 0.0, // Should remain 0.0
			tolerance:    0.01,
		},
		{
			name:         "alternating pattern with 0.5 learning rate",
			learningRate: 0.5,
			outcomes:     []bool{true, false, true, false, true, false},
			expectedRate: 0.328, // EMA with alternating: 0.5 -> 0.25 -> 0.625 -> 0.3125 -> 0.65625 -> 0.328125
			tolerance:    0.01,
		},
		{
			name:         "manual EMA calculation verification",
			learningRate: 0.3,
			outcomes:     []bool{true, false, true}, // Specific sequence for manual verification
			expectedRate: 0.447,                     // Manually calculated: 0.3 -> 0.21 -> 0.447
			tolerance:    0.01,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adaptive, err := NewAdaptive(fallback, tc.learningRate, 20)
			require.NoError(t, err)

			// Record outcomes in the same bucket to test EMA on single bucket
			delay := 1500 * time.Millisecond // Bucket 1: 1-2s
			for _, success := range tc.outcomes {
				adaptive.RecordOutcome(delay, success, 100*time.Millisecond)
			}

			// Get the success rate from the bucket
			successRate := adaptive.getBucketSuccessRateForTesting(delay)

			assert.InDelta(t, tc.expectedRate, successRate, tc.tolerance,
				"EMA success rate should match expected value within tolerance")
		})
	}
}

// TestAdaptiveStrategy_EMAFormula tests the exact EMA formula implementation
func TestAdaptiveStrategy_EMAFormula(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	learningRate := 0.2
	adaptive, err := NewAdaptive(fallback, learningRate, 10)
	require.NoError(t, err)

	delay := 500 * time.Millisecond // Bucket 0: 0-1s

	// Manual EMA calculation: new_rate = (1-α)*old_rate + α*outcome
	// Where α = learning_rate, outcome = 1.0 for success, 0.0 for failure

	expectedRates := []float64{}
	currentRate := 0.0 // Initial rate

	outcomes := []bool{true, false, true, true, false}

	for _, success := range outcomes {
		outcome := 0.0
		if success {
			outcome = 1.0
		}

		// Apply EMA formula
		currentRate = (1-learningRate)*currentRate + learningRate*outcome
		expectedRates = append(expectedRates, currentRate)

		// Record outcome in adaptive strategy
		adaptive.RecordOutcome(delay, success, 50*time.Millisecond)

		// Verify the calculated rate matches our manual calculation
		actualRate := adaptive.getBucketSuccessRateForTesting(delay)
		assert.InDelta(t, currentRate, actualRate, 0.001,
			"EMA calculation should match manual formula at step %d", len(expectedRates))
	}
}

// TestAdaptiveStrategy_LearningConvergence tests that EMA responds to patterns over time
func TestAdaptiveStrategy_LearningConvergence(t *testing.T) {
	fallback := NewFixed(1 * time.Second)

	t.Run("consistent successes should increase rate", func(t *testing.T) {
		adaptive, err := NewAdaptive(fallback, 0.3, 50)
		require.NoError(t, err)

		delay := 1500 * time.Millisecond // Bucket 1: 1-2s

		// Record consistent successes
		for i := 0; i < 20; i++ {
			adaptive.RecordOutcome(delay, true, 100*time.Millisecond)
		}

		finalRate := adaptive.getBucketSuccessRateForTesting(delay)
		assert.Greater(t, finalRate, 0.8, "Consistent successes should result in high success rate")
	})

	t.Run("consistent failures should decrease rate", func(t *testing.T) {
		adaptive, err := NewAdaptive(fallback, 0.3, 50)
		require.NoError(t, err)

		delay := 1500 * time.Millisecond // Bucket 1: 1-2s

		// Start with some successes
		for i := 0; i < 5; i++ {
			adaptive.RecordOutcome(delay, true, 100*time.Millisecond)
		}

		// Then consistent failures
		for i := 0; i < 20; i++ {
			adaptive.RecordOutcome(delay, false, 0)
		}

		finalRate := adaptive.getBucketSuccessRateForTesting(delay)
		assert.Less(t, finalRate, 0.2, "Consistent failures should result in low success rate")
	})

	t.Run("alternating pattern should stabilize", func(t *testing.T) {
		adaptive, err := NewAdaptive(fallback, 0.2, 50)
		require.NoError(t, err)

		delay := 1500 * time.Millisecond // Bucket 1: 1-2s

		// Alternating pattern (50% success rate)
		for i := 0; i < 40; i++ {
			success := i%2 == 0
			adaptive.RecordOutcome(delay, success, 100*time.Millisecond)
		}

		finalRate := adaptive.getBucketSuccessRateForTesting(delay)
		// EMA with alternating pattern converges to ~0.5, not the learning rate
		// The learning rate controls convergence speed, not the final value
		assert.InDelta(t, 0.5, finalRate, 0.15, "Alternating pattern should stabilize around 50% success rate")
	})
}

// TestAdaptiveStrategy_LearningRateImpact tests how different learning rates affect convergence speed
func TestAdaptiveStrategy_LearningRateImpact(t *testing.T) {
	fallback := NewFixed(1 * time.Second)

	t.Run("higher learning rates converge faster", func(t *testing.T) {
		lowLR, err := NewAdaptive(fallback, 0.1, 50)
		require.NoError(t, err)

		highLR, err := NewAdaptive(fallback, 0.7, 50)
		require.NoError(t, err)

		delay := 2500 * time.Millisecond // Bucket 2: 2-5s

		// Record same pattern for both: start with failures, then successes
		pattern := []bool{false, false, false, true, true, true, true, true}

		for _, success := range pattern {
			lowLR.RecordOutcome(delay, success, 100*time.Millisecond)
			highLR.RecordOutcome(delay, success, 100*time.Millisecond)
		}

		lowRate := lowLR.getBucketSuccessRateForTesting(delay)
		highRate := highLR.getBucketSuccessRateForTesting(delay)

		// Higher learning rate should be more responsive to recent successes
		assert.Greater(t, highRate, lowRate,
			"Higher learning rate should be more responsive to recent data")

		// Both should show learning from the pattern
		assert.Greater(t, lowRate, 0.3, "Low learning rate should show some adaptation")
		assert.Greater(t, highRate, 0.5, "High learning rate should show stronger adaptation")
	})

	t.Run("all learning rates show progress", func(t *testing.T) {
		learningRates := []float64{0.1, 0.3, 0.7}

		for _, lr := range learningRates {
			adaptive, err := NewAdaptive(fallback, lr, 50)
			require.NoError(t, err)

			delay := 3500 * time.Millisecond // Different bucket for isolation

			// Record consistent successes
			for i := 0; i < 15; i++ {
				adaptive.RecordOutcome(delay, true, 100*time.Millisecond)
			}

			finalRate := adaptive.getBucketSuccessRateForTesting(delay)
			assert.Greater(t, finalRate, 0.3,
				"Learning rate %.1f should show learning progress", lr)
		}
	})
}

// TestAdaptiveStrategy_EMAStability tests EMA stability with consistent inputs
func TestAdaptiveStrategy_EMAStability(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 30)
	require.NoError(t, err)

	delay := 7 * time.Second // Bucket 3: 5-10s

	// Record consistent successes
	for i := 0; i < 20; i++ {
		adaptive.RecordOutcome(delay, true, 100*time.Millisecond)
	}

	rate1 := adaptive.getBucketSuccessRateForTesting(delay)

	// Record more consistent successes
	for i := 0; i < 10; i++ {
		adaptive.RecordOutcome(delay, true, 100*time.Millisecond)
	}

	rate2 := adaptive.getBucketSuccessRateForTesting(delay)

	// Rate should be stable (close to 1.0) and not change much
	assert.Greater(t, rate1, 0.95, "Should be close to 1.0 after consistent successes")
	assert.Greater(t, rate2, 0.95, "Should remain close to 1.0")
	assert.InDelta(t, rate1, rate2, 0.05, "Should be stable with consistent inputs")
}

// TestAdaptiveStrategy_EMARecovery tests recovery from initial poor performance
func TestAdaptiveStrategy_EMARecovery(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.4, 30) // Higher learning rate for faster recovery
	require.NoError(t, err)

	delay := 20 * time.Second // Bucket 4: 10-30s

	// Start with failures
	for i := 0; i < 10; i++ {
		adaptive.RecordOutcome(delay, false, 0)
	}

	poorRate := adaptive.getBucketSuccessRateForTesting(delay)
	assert.Less(t, poorRate, 0.2, "Should have poor success rate after failures")

	// Switch to successes
	for i := 0; i < 15; i++ {
		adaptive.RecordOutcome(delay, true, 100*time.Millisecond)
	}

	recoveredRate := adaptive.getBucketSuccessRateForTesting(delay)
	assert.Greater(t, recoveredRate, 0.7, "Should recover with consistent successes")
	assert.Greater(t, recoveredRate, poorRate+0.5, "Should show significant improvement")
}

// TestAdaptiveStrategy_MultipleBucketEMA tests EMA behavior across different buckets
func TestAdaptiveStrategy_MultipleBucketEMA(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.3, 100) // Larger memory window
	require.NoError(t, err)

	// Test that different buckets can learn independently
	// Focus on one bucket at a time to avoid interference

	t.Run("bucket isolation", func(t *testing.T) {
		// Test bucket 0 with high success rate
		delay1 := 500 * time.Millisecond
		for i := 0; i < 20; i++ {
			success := i < 18 // 90% success rate
			adaptive.RecordOutcome(delay1, success, 100*time.Millisecond)
		}

		rate1 := adaptive.getBucketSuccessRateForTesting(delay1)
		assert.Greater(t, rate1, 0.4, "High success bucket should show learning")

		// Test bucket 1 with low success rate
		delay2 := 1500 * time.Millisecond
		for i := 0; i < 20; i++ {
			success := i < 6 // 30% success rate
			adaptive.RecordOutcome(delay2, success, 100*time.Millisecond)
		}

		rate2 := adaptive.getBucketSuccessRateForTesting(delay2)
		assert.Less(t, rate2, 0.5, "Low success bucket should show lower learning")

		// Verify buckets learned different patterns
		assert.Greater(t, rate1, rate2, "Different buckets should learn different patterns")
	})

	t.Run("bucket independence", func(t *testing.T) {
		// Create fresh adaptive strategy
		adaptive2, err := NewAdaptive(fallback, 0.4, 50)
		require.NoError(t, err)

		// Record to different buckets simultaneously
		delays := []time.Duration{
			800 * time.Millisecond,  // Bucket 0
			2500 * time.Millisecond, // Bucket 2
			15 * time.Second,        // Bucket 4
		}

		for i := 0; i < 15; i++ {
			for j, delay := range delays {
				// Different success patterns per bucket
				var success bool
				switch j {
				case 0: // High success
					success = i < 12
				case 1: // Medium success
					success = i < 8
				case 2: // Low success
					success = i < 4
				}
				adaptive2.RecordOutcome(delay, success, 100*time.Millisecond)
			}
		}

		// Verify each bucket learned independently
		rates := make([]float64, len(delays))
		for i, delay := range delays {
			rates[i] = adaptive2.getBucketSuccessRateForTesting(delay)
		}

		// High success bucket should have highest rate
		assert.Greater(t, rates[0], rates[1], "High success bucket should outperform medium")
		assert.Greater(t, rates[1], rates[2], "Medium success bucket should outperform low")
	})
}
