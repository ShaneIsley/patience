package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveStrategy_Interface(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.1, 10)
	require.NoError(t, err)

	// Should implement Strategy interface
	var _ Strategy = adaptive
}

func TestNewAdaptive_ValidParameters(t *testing.T) {
	fallback := NewFixed(1 * time.Second)

	testCases := []struct {
		name         string
		learningRate float64
		memoryWindow int
		expectError  bool
	}{
		{"valid parameters", 0.1, 10, false},
		{"minimum learning rate", 0.01, 5, false},
		{"maximum learning rate", 1.0, 100, false},
		{"zero learning rate", 0.0, 10, true},
		{"negative learning rate", -0.1, 10, true},
		{"learning rate too high", 1.1, 10, true},
		{"zero memory window", 0.1, 0, true},
		{"negative memory window", 0.1, -5, true},
		{"memory window too large", 0.1, 10001, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adaptive, err := NewAdaptive(fallback, tc.learningRate, tc.memoryWindow)
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, adaptive)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adaptive)
			}
		})
	}
}

func TestNewAdaptive_NilFallback(t *testing.T) {
	adaptive, err := NewAdaptive(nil, 0.1, 10)
	assert.Error(t, err)
	assert.Nil(t, adaptive)
	assert.Contains(t, err.Error(), "fallback strategy cannot be nil")
}

func TestAdaptiveStrategy_InitialBehavior_UsesFallback(t *testing.T) {
	fallback := NewFixed(2 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.1, 10)
	require.NoError(t, err)

	// With no learning data, should use fallback strategy
	for attempt := 1; attempt <= 5; attempt++ {
		expected := fallback.Delay(attempt)
		actual := adaptive.Delay(attempt)
		assert.Equal(t, expected, actual, "Attempt %d should use fallback", attempt)
	}
}

func TestAdaptiveStrategy_LearningFromSuccess(t *testing.T) {
	fallback := NewFixed(5 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.5, 10) // High learning rate for fast adaptation
	require.NoError(t, err)

	// Record successful outcomes with shorter delays
	shortDelay := 1 * time.Second
	for i := 0; i < 5; i++ {
		adaptive.RecordOutcome(shortDelay, true, 100*time.Millisecond)
	}

	// After learning, should prefer shorter delays over fallback
	delay := adaptive.Delay(1)
	assert.Less(t, delay, fallback.Delay(1), "Should learn to prefer shorter delays after successes")
}

func TestAdaptiveStrategy_LearningFromFailure(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.5, 10) // High learning rate for fast adaptation
	require.NoError(t, err)

	// Record failed outcomes with short delays
	shortDelay := 500 * time.Millisecond
	for i := 0; i < 5; i++ {
		adaptive.RecordOutcome(shortDelay, false, 0)
	}

	// Record successful outcomes with longer delays
	longDelay := 3 * time.Second
	for i := 0; i < 5; i++ {
		adaptive.RecordOutcome(longDelay, true, 200*time.Millisecond)
	}

	// Should learn to prefer longer delays
	delay := adaptive.Delay(1)
	assert.Greater(t, delay, fallback.Delay(1), "Should learn to prefer longer delays after short delay failures")
}

func TestAdaptiveStrategy_MemoryWindow_FIFO(t *testing.T) {
	fallback := NewFixed(2 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.3, 3) // Small memory window
	require.NoError(t, err)

	// Fill memory window with failures at short delays
	shortDelay := 500 * time.Millisecond
	for i := 0; i < 3; i++ {
		adaptive.RecordOutcome(shortDelay, false, 0)
	}

	// Add one more (should evict oldest)
	adaptive.RecordOutcome(shortDelay, false, 0)

	// Now add successes at short delays (should overwrite the failure pattern)
	for i := 0; i < 4; i++ {
		adaptive.RecordOutcome(shortDelay, true, 100*time.Millisecond)
	}

	// Should now prefer short delays due to recent successes
	delay := adaptive.Delay(1)
	assert.Less(t, delay, fallback.Delay(1), "Should forget old failures and prefer recent successes")
}

func TestAdaptiveStrategy_LearningRate_Impact(t *testing.T) {
	fallback := NewFixed(2 * time.Second)

	// Low learning rate - slow adaptation
	slowAdaptive, err := NewAdaptive(fallback, 0.01, 20)
	require.NoError(t, err)

	// High learning rate - fast adaptation
	fastAdaptive, err := NewAdaptive(fallback, 0.8, 20)
	require.NoError(t, err)

	// Record same successful outcomes for both
	shortDelay := 500 * time.Millisecond
	for i := 0; i < 3; i++ {
		slowAdaptive.RecordOutcome(shortDelay, true, 100*time.Millisecond)
		fastAdaptive.RecordOutcome(shortDelay, true, 100*time.Millisecond)
	}

	slowDelay := slowAdaptive.Delay(1)
	fastDelay := fastAdaptive.Delay(1)

	// Fast learner should adapt more aggressively
	assert.Less(t, fastDelay, slowDelay, "High learning rate should adapt faster")
}

func TestAdaptiveStrategy_DelayBucketing(t *testing.T) {
	fallback := NewFixed(5 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.3, 20)
	require.NoError(t, err)

	// Record successes in different delay buckets
	adaptive.RecordOutcome(500*time.Millisecond, true, 50*time.Millisecond) // Bucket 1: 0-1s
	adaptive.RecordOutcome(1500*time.Millisecond, false, 0)                 // Bucket 2: 1-2s
	adaptive.RecordOutcome(3*time.Second, true, 100*time.Millisecond)       // Bucket 3: 2-5s

	// Should prefer delays from successful buckets
	delay := adaptive.Delay(1)

	// Should avoid the 1-2s bucket (had failure) and prefer 0-1s or 2-5s buckets
	assert.True(t, delay < 1*time.Second || delay >= 2*time.Second,
		"Should avoid delay bucket with failures")
}

func TestAdaptiveStrategy_InsufficientData_UsesFallback(t *testing.T) {
	fallback := NewFixed(3 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 20)
	require.NoError(t, err)

	// Record only one outcome (insufficient for confident learning)
	adaptive.RecordOutcome(1*time.Second, true, 100*time.Millisecond)

	// Should still primarily use fallback due to insufficient data
	delay := adaptive.Delay(1)

	// Should be close to fallback delay (within reasonable learning adjustment)
	fallbackDelay := fallback.Delay(1)
	ratio := float64(delay) / float64(fallbackDelay)
	assert.True(t, ratio > 0.7 && ratio < 1.3,
		"With insufficient data, should stay close to fallback strategy")
}

func TestAdaptiveStrategy_EdgeCases(t *testing.T) {
	fallback := NewFixed(2 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 10)
	require.NoError(t, err)

	t.Run("zero attempt", func(t *testing.T) {
		delay := adaptive.Delay(0)
		expected := fallback.Delay(0)
		assert.Equal(t, expected, delay)
	})

	t.Run("negative attempt", func(t *testing.T) {
		delay := adaptive.Delay(-1)
		expected := fallback.Delay(-1)
		assert.Equal(t, expected, delay)
	})

	t.Run("very high attempt", func(t *testing.T) {
		delay := adaptive.Delay(1000)
		// Should handle gracefully, either use fallback or learned pattern
		assert.True(t, delay > 0, "Should return positive delay for high attempts")
	})
}

func TestAdaptiveStrategy_MixedOutcomes_BalancedLearning(t *testing.T) {
	fallback := NewFixed(2 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.3, 20)
	require.NoError(t, err)

	// Record mixed outcomes: some successes, some failures
	delays := []time.Duration{
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		3 * time.Second,
	}

	successes := []bool{true, false, true, false}

	for i := 0; i < 10; i++ {
		for j, delay := range delays {
			adaptive.RecordOutcome(delay, successes[j], 100*time.Millisecond)
		}
	}

	// Should find a balanced delay that considers both successes and failures
	delay := adaptive.Delay(1)
	assert.True(t, delay > 0, "Should return positive delay")
	assert.True(t, delay < 10*time.Second, "Should not return excessively long delay")
}

func TestAdaptiveStrategy_ZeroLatency_HandledGracefully(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 10)
	require.NoError(t, err)

	// Record outcomes with zero latency (immediate responses)
	adaptive.RecordOutcome(500*time.Millisecond, true, 0)
	adaptive.RecordOutcome(1*time.Second, false, 0)

	// Should handle zero latency gracefully
	delay := adaptive.Delay(1)
	assert.True(t, delay > 0, "Should return positive delay even with zero latency data")
}

func TestAdaptiveStrategy_ConsistentBehavior_SameInputs(t *testing.T) {
	fallback := NewFixed(2 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 10)
	require.NoError(t, err)

	// Record some learning data
	for i := 0; i < 5; i++ {
		adaptive.RecordOutcome(1*time.Second, true, 100*time.Millisecond)
	}

	// Multiple calls with same attempt should return same delay
	delay1 := adaptive.Delay(3)
	delay2 := adaptive.Delay(3)
	delay3 := adaptive.Delay(3)

	assert.Equal(t, delay1, delay2, "Same attempt should return same delay")
	assert.Equal(t, delay2, delay3, "Same attempt should return same delay")
}
