package backoff

import "time"

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
