package backoff

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFixed_Delay(t *testing.T) {
	// Given a fixed backoff strategy with 100ms delay
	fixed := NewFixed(100 * time.Millisecond)

	// When Delay() is called for different attempts
	delay1 := fixed.Delay(1)
	delay2 := fixed.Delay(2)
	delay3 := fixed.Delay(3)

	// Then all delays should be the same
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 100*time.Millisecond, delay2)
	assert.Equal(t, 100*time.Millisecond, delay3)
}

func TestExponential_DelayIncreasesCorrectly(t *testing.T) {
	// Given an exponential backoff strategy with 100ms base delay
	exponential := NewExponential(100*time.Millisecond, 2.0, 0)

	// When Delay() is called for different attempts
	delay1 := exponential.Delay(1) // First retry
	delay2 := exponential.Delay(2) // Second retry
	delay3 := exponential.Delay(3) // Third retry
	delay4 := exponential.Delay(4) // Fourth retry

	// Then delays should increase exponentially: 100ms, 200ms, 400ms, 800ms
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 200*time.Millisecond, delay2)
	assert.Equal(t, 400*time.Millisecond, delay3)
	assert.Equal(t, 800*time.Millisecond, delay4)
}

func TestExponential_WithMaxDelay(t *testing.T) {
	// Given an exponential backoff with max delay cap
	exponential := NewExponential(100*time.Millisecond, 2.0, 300*time.Millisecond)

	// When Delay() is called for attempts that would exceed max
	delay1 := exponential.Delay(1) // 100ms
	delay2 := exponential.Delay(2) // 200ms
	delay3 := exponential.Delay(3) // Would be 400ms, capped to 300ms
	delay4 := exponential.Delay(4) // Would be 800ms, capped to 300ms

	// Then delays should be capped at max delay
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 200*time.Millisecond, delay2)
	assert.Equal(t, 300*time.Millisecond, delay3)
	assert.Equal(t, 300*time.Millisecond, delay4)
}

func TestExponential_WithCustomMultiplier(t *testing.T) {
	// Given an exponential backoff with 1.5x multiplier
	exponential := NewExponential(100*time.Millisecond, 1.5, 0)

	// When Delay() is called for different attempts
	delay1 := exponential.Delay(1) // 100ms
	delay2 := exponential.Delay(2) // 150ms
	delay3 := exponential.Delay(3) // 225ms

	// Then delays should increase by 1.5x each time
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 150*time.Millisecond, delay2)
	assert.Equal(t, 225*time.Millisecond, delay3)
}

func TestExponential_EdgeCases(t *testing.T) {
	exponential := NewExponential(100*time.Millisecond, 2.0, 0)

	// Test attempt 0 and negative attempts
	delay0 := exponential.Delay(0)
	delayNeg := exponential.Delay(-1)

	// Should return base delay for invalid attempts
	assert.Equal(t, 100*time.Millisecond, delay0)
	assert.Equal(t, 100*time.Millisecond, delayNeg)
}

func TestExponential_NoMaxDelay(t *testing.T) {
	// Given exponential backoff with no max delay (0)
	exponential := NewExponential(100*time.Millisecond, 2.0, 0)

	// When calculating delay for high attempt numbers
	delay10 := exponential.Delay(10) // 100ms * 2^9 = 51200ms

	// Then delay should not be capped
	expected := 100 * time.Millisecond * time.Duration(math.Pow(2, 9))
	assert.Equal(t, expected, delay10)
}

func TestJitter_DelayIsRandom(t *testing.T) {
	// Given a jitter backoff strategy with 1000ms base delay
	jitter := NewJitter(1000*time.Millisecond, 2.0, 0)

	// When Delay() is called multiple times for the same attempt
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = jitter.Delay(1)
	}

	// Then delays should vary (at least some should be different)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "Jitter delays should vary, but all were the same: %v", delays[0])
}

func TestJitter_DelayWithinBounds(t *testing.T) {
	// Given a jitter backoff strategy with 1000ms base delay
	jitter := NewJitter(1000*time.Millisecond, 2.0, 0)

	// When Delay() is called for different attempts
	for attempt := 1; attempt <= 5; attempt++ {
		delay := jitter.Delay(attempt)

		// Calculate expected exponential delay without jitter
		expectedBase := float64(1000*time.Millisecond) * math.Pow(2.0, float64(attempt-1))
		maxDelay := time.Duration(expectedBase)

		// Then delay should be between 0 and the exponential base delay
		assert.GreaterOrEqual(t, delay, time.Duration(0), "Delay should be >= 0 for attempt %d", attempt)
		assert.LessOrEqual(t, delay, maxDelay, "Delay should be <= %v for attempt %d", maxDelay, attempt)
	}
}

func TestJitter_WithMaxDelay(t *testing.T) {
	// Given a jitter backoff with max delay cap
	jitter := NewJitter(100*time.Millisecond, 2.0, 300*time.Millisecond)

	// When Delay() is called for attempts that would exceed max
	for i := 0; i < 20; i++ {
		delay := jitter.Delay(10) // High attempt number

		// Then delay should never exceed max delay
		assert.LessOrEqual(t, delay, 300*time.Millisecond, "Delay should be capped at max delay")
		assert.GreaterOrEqual(t, delay, time.Duration(0), "Delay should be >= 0")
	}
}

func TestJitter_EdgeCases(t *testing.T) {
	jitter := NewJitter(100*time.Millisecond, 2.0, 0)

	// Test attempt 0 and negative attempts
	delay0 := jitter.Delay(0)
	delayNeg := jitter.Delay(-1)

	// Should return random delay between 0 and base delay for invalid attempts
	assert.GreaterOrEqual(t, delay0, time.Duration(0))
	assert.LessOrEqual(t, delay0, 100*time.Millisecond)
	assert.GreaterOrEqual(t, delayNeg, time.Duration(0))
	assert.LessOrEqual(t, delayNeg, 100*time.Millisecond)
}

func TestLinear_DelayIncreasesLinearly(t *testing.T) {
	// Given a linear backoff strategy with 100ms increment
	linear := NewLinear(100*time.Millisecond, 0)

	// When Delay() is called for different attempts
	delay1 := linear.Delay(1) // First retry: 100ms
	delay2 := linear.Delay(2) // Second retry: 200ms
	delay3 := linear.Delay(3) // Third retry: 300ms
	delay4 := linear.Delay(4) // Fourth retry: 400ms

	// Then delays should increase linearly
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 200*time.Millisecond, delay2)
	assert.Equal(t, 300*time.Millisecond, delay3)
	assert.Equal(t, 400*time.Millisecond, delay4)
}

func TestLinear_WithMaxDelay(t *testing.T) {
	// Given a linear backoff with max delay cap
	linear := NewLinear(100*time.Millisecond, 250*time.Millisecond)

	// When Delay() is called for attempts that would exceed max
	delay1 := linear.Delay(1) // 100ms
	delay2 := linear.Delay(2) // 200ms
	delay3 := linear.Delay(3) // Would be 300ms, capped to 250ms
	delay4 := linear.Delay(4) // Would be 400ms, capped to 250ms

	// Then delays should be capped at max delay
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 200*time.Millisecond, delay2)
	assert.Equal(t, 250*time.Millisecond, delay3)
	assert.Equal(t, 250*time.Millisecond, delay4)
}

func TestLinear_EdgeCases(t *testing.T) {
	linear := NewLinear(100*time.Millisecond, 0)

	// Test attempt 0 and negative attempts
	delay0 := linear.Delay(0)
	delayNeg := linear.Delay(-1)

	// Should return increment for invalid attempts
	assert.Equal(t, 100*time.Millisecond, delay0)
	assert.Equal(t, 100*time.Millisecond, delayNeg)
}

func TestLinear_NoMaxDelay(t *testing.T) {
	// Given linear backoff with no max delay (0)
	linear := NewLinear(50*time.Millisecond, 0)

	// When calculating delay for high attempt numbers
	delay10 := linear.Delay(10) // 50ms * 10 = 500ms

	// Then delay should not be capped
	assert.Equal(t, 500*time.Millisecond, delay10)
}
