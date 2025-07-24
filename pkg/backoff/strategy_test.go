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
