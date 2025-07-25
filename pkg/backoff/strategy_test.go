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

func TestDecorrelatedJitter_FirstAttempt(t *testing.T) {
	// Given a decorrelated jitter backoff strategy
	decorrelated := NewDecorrelatedJitter(100*time.Millisecond, 3.0, 0)

	// When Delay() is called for the first attempt
	delay1 := decorrelated.Delay(1)

	// Then delay should be between base delay and base delay * multiplier
	assert.GreaterOrEqual(t, delay1, 100*time.Millisecond, "First delay should be >= base delay")
	assert.LessOrEqual(t, delay1, 300*time.Millisecond, "First delay should be <= base delay * multiplier")
}

func TestDecorrelatedJitter_DelayWithinBounds(t *testing.T) {
	// Given a decorrelated jitter backoff strategy
	decorrelated := NewDecorrelatedJitter(100*time.Millisecond, 3.0, 0)

	// When Delay() is called for multiple attempts
	var previousDelay time.Duration
	for attempt := 1; attempt <= 5; attempt++ {
		delay := decorrelated.Delay(attempt)

		if attempt == 1 {
			// First attempt: should be between base delay and base delay * multiplier
			assert.GreaterOrEqual(t, delay, 100*time.Millisecond, "Delay should be >= base delay for attempt %d", attempt)
			assert.LessOrEqual(t, delay, 300*time.Millisecond, "Delay should be <= base delay * multiplier for attempt %d", attempt)
		} else {
			// Subsequent attempts: should be between base delay and previous delay * multiplier
			maxDelay := time.Duration(float64(previousDelay) * 3.0)
			assert.GreaterOrEqual(t, delay, 100*time.Millisecond, "Delay should be >= base delay for attempt %d", attempt)
			assert.LessOrEqual(t, delay, maxDelay, "Delay should be <= previous delay * multiplier for attempt %d", attempt)
		}
		previousDelay = delay
	}
}

func TestDecorrelatedJitter_IsRandom(t *testing.T) {
	// Given a decorrelated jitter backoff strategy
	decorrelated := NewDecorrelatedJitter(100*time.Millisecond, 3.0, 0)

	// When Delay() is called multiple times for the same attempt sequence
	delays1 := make([]time.Duration, 3)
	delays2 := make([]time.Duration, 3)

	// First sequence
	for i := 0; i < 3; i++ {
		delays1[i] = decorrelated.Delay(i + 1)
	}

	// Reset and second sequence
	decorrelated2 := NewDecorrelatedJitter(100*time.Millisecond, 3.0, 0)
	for i := 0; i < 3; i++ {
		delays2[i] = decorrelated2.Delay(i + 1)
	}

	// Then at least some delays should be different (randomness)
	different := false
	for i := 0; i < 3; i++ {
		if delays1[i] != delays2[i] {
			different = true
			break
		}
	}
	assert.True(t, different, "Decorrelated jitter should produce different sequences")
}

func TestDecorrelatedJitter_WithMaxDelay(t *testing.T) {
	// Given a decorrelated jitter with max delay cap
	decorrelated := NewDecorrelatedJitter(100*time.Millisecond, 3.0, 500*time.Millisecond)

	// When Delay() is called for many attempts
	for attempt := 1; attempt <= 10; attempt++ {
		delay := decorrelated.Delay(attempt)

		// Then delay should never exceed max delay
		assert.LessOrEqual(t, delay, 500*time.Millisecond, "Delay should be capped at max delay for attempt %d", attempt)
		assert.GreaterOrEqual(t, delay, 100*time.Millisecond, "Delay should be >= base delay for attempt %d", attempt)
	}
}

func TestDecorrelatedJitter_EdgeCases(t *testing.T) {
	decorrelated := NewDecorrelatedJitter(100*time.Millisecond, 3.0, 0)

	// Test attempt 0 and negative attempts
	delay0 := decorrelated.Delay(0)
	delayNeg := decorrelated.Delay(-1)

	// Should return delay between base delay and base delay * multiplier for invalid attempts
	assert.GreaterOrEqual(t, delay0, 100*time.Millisecond)
	assert.LessOrEqual(t, delay0, 300*time.Millisecond)
	assert.GreaterOrEqual(t, delayNeg, 100*time.Millisecond)
	assert.LessOrEqual(t, delayNeg, 300*time.Millisecond)
}

func TestFibonacci_DelayFollowsFibonacciSequence(t *testing.T) {
	// Given a fibonacci backoff strategy with 100ms base delay
	fibonacci := NewFibonacci(100*time.Millisecond, 0)

	// When Delay() is called for different attempts
	delay1 := fibonacci.Delay(1) // First retry: 100ms (1st fibonacci)
	delay2 := fibonacci.Delay(2) // Second retry: 100ms (2nd fibonacci)
	delay3 := fibonacci.Delay(3) // Third retry: 200ms (3rd fibonacci)
	delay4 := fibonacci.Delay(4) // Fourth retry: 300ms (4th fibonacci)
	delay5 := fibonacci.Delay(5) // Fifth retry: 500ms (5th fibonacci)
	delay6 := fibonacci.Delay(6) // Sixth retry: 800ms (6th fibonacci)

	// Then delays should follow fibonacci sequence: 1, 1, 2, 3, 5, 8, ...
	assert.Equal(t, 100*time.Millisecond, delay1) // 1 * 100ms
	assert.Equal(t, 100*time.Millisecond, delay2) // 1 * 100ms
	assert.Equal(t, 200*time.Millisecond, delay3) // 2 * 100ms
	assert.Equal(t, 300*time.Millisecond, delay4) // 3 * 100ms
	assert.Equal(t, 500*time.Millisecond, delay5) // 5 * 100ms
	assert.Equal(t, 800*time.Millisecond, delay6) // 8 * 100ms
}

func TestFibonacci_WithMaxDelay(t *testing.T) {
	// Given a fibonacci backoff with max delay cap
	fibonacci := NewFibonacci(100*time.Millisecond, 350*time.Millisecond)

	// When Delay() is called for attempts that would exceed max
	delay1 := fibonacci.Delay(1) // 100ms
	delay2 := fibonacci.Delay(2) // 100ms
	delay3 := fibonacci.Delay(3) // 200ms
	delay4 := fibonacci.Delay(4) // 300ms
	delay5 := fibonacci.Delay(5) // Would be 500ms, capped to 350ms
	delay6 := fibonacci.Delay(6) // Would be 800ms, capped to 350ms

	// Then delays should be capped at max delay
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 100*time.Millisecond, delay2)
	assert.Equal(t, 200*time.Millisecond, delay3)
	assert.Equal(t, 300*time.Millisecond, delay4)
	assert.Equal(t, 350*time.Millisecond, delay5)
	assert.Equal(t, 350*time.Millisecond, delay6)
}

func TestFibonacci_EdgeCases(t *testing.T) {
	fibonacci := NewFibonacci(100*time.Millisecond, 0)

	// Test attempt 0 and negative attempts
	delay0 := fibonacci.Delay(0)
	delayNeg := fibonacci.Delay(-1)

	// Should return base delay for invalid attempts
	assert.Equal(t, 100*time.Millisecond, delay0)
	assert.Equal(t, 100*time.Millisecond, delayNeg)
}

func TestFibonacci_NoMaxDelay(t *testing.T) {
	// Given fibonacci backoff with no max delay (0)
	fibonacci := NewFibonacci(50*time.Millisecond, 0)

	// When calculating delay for high attempt numbers
	delay10 := fibonacci.Delay(10) // 55th fibonacci number * 50ms = 2750ms

	// Then delay should not be capped (55 is the 10th fibonacci number)
	assert.Equal(t, 2750*time.Millisecond, delay10)
}
