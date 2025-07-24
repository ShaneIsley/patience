package backoff

import (
	"math"
	"time"
)

// Strategy defines the interface for backoff strategies
type Strategy interface {
	// Delay returns the duration to wait before the next attempt
	// attempt is 1-based (1 for first retry, 2 for second retry, etc.)
	Delay(attempt int) time.Duration
}

// Fixed implements a fixed delay strategy
type Fixed struct {
	Duration time.Duration
}

// NewFixed creates a new Fixed backoff strategy
func NewFixed(duration time.Duration) *Fixed {
	return &Fixed{
		Duration: duration,
	}
}

// Delay returns the fixed duration for any attempt
func (f *Fixed) Delay(attempt int) time.Duration {
	return f.Duration
}

// Exponential implements an exponential backoff strategy
type Exponential struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

// NewExponential creates a new Exponential backoff strategy
// baseDelay is the initial delay, multiplier is the factor to increase by each attempt
// maxDelay is the maximum delay (0 means no limit)
func NewExponential(baseDelay time.Duration, multiplier float64, maxDelay time.Duration) *Exponential {
	return &Exponential{
		BaseDelay:  baseDelay,
		Multiplier: multiplier,
		MaxDelay:   maxDelay,
	}
}

// Delay returns the exponentially increasing delay for the given attempt
func (e *Exponential) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return e.BaseDelay
	}

	// Calculate exponential delay: baseDelay * multiplier^(attempt-1)
	delay := float64(e.BaseDelay) * math.Pow(e.Multiplier, float64(attempt-1))

	// Convert back to duration
	result := time.Duration(delay)

	// Apply max delay cap if set
	if e.MaxDelay > 0 && result > e.MaxDelay {
		result = e.MaxDelay
	}

	return result
}
