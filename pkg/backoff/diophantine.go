package backoff

import (
	"sort"
	"time"
)

// DiophantineStrategy implements a proactive scheduling strategy based on Diophantine inequalities.
type DiophantineStrategy struct {
	rateLimit    int
	window       time.Duration
	retryOffsets []time.Duration
}

// NewDiophantine creates a new Diophantine backoff strategy.
func NewDiophantine(rateLimit int, window time.Duration, retryOffsets []time.Duration) *DiophantineStrategy {
	return &DiophantineStrategy{
		rateLimit:    rateLimit,
		window:       window,
		retryOffsets: retryOffsets,
	}
}

// CanScheduleRequest checks if a new request can be scheduled without violating the rate limit.
// It takes a list of existing request times and the time of the new request.
func (d *DiophantineStrategy) CanScheduleRequest(existing []time.Time, newRequestTime time.Time) bool {
	newRetries := make([]time.Time, len(d.retryOffsets))
	for i, offset := range d.retryOffsets {
		newRetries[i] = newRequestTime.Add(offset)
	}

	allRequests := make([]time.Time, 0, len(existing)+len(newRetries))
	allRequests = append(allRequests, existing...)
	allRequests = append(allRequests, newRetries...)
	sort.Slice(allRequests, func(i, j int) bool {
		return allRequests[i].Before(allRequests[j])
	})

	// Check every possible window by using each request time as a potential window start
	for i := 0; i < len(allRequests); i++ {
		windowStart := allRequests[i]
		windowEnd := windowStart.Add(d.window)
		count := 0

		// Count requests in this window [windowStart, windowEnd)
		for j := i; j < len(allRequests) && allRequests[j].Before(windowEnd); j++ {
			count++
		}

		if count > d.rateLimit {
			return false
		}
	}
	return true
}

// Delay implements the Strategy interface for compatibility with existing executor
// For Diophantine strategy, this is a fallback that should rarely be used
// The main scheduling logic is in CanScheduleRequest
func (d *DiophantineStrategy) Delay(attempt int) time.Duration {
	// Simple exponential backoff as fallback when daemon is not available
	if attempt <= 0 {
		return time.Second
	}

	// Use the first retry offset as base delay, with exponential growth
	baseDelay := time.Second
	if len(d.retryOffsets) > 1 {
		baseDelay = d.retryOffsets[1] // Use second offset as it's usually the first retry
	}

	// Exponential backoff: baseDelay * 2^(attempt-1)
	delay := baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > time.Hour {
			return time.Hour // Cap at 1 hour
		}
	}

	return delay
}

// GetRateLimit returns the rate limit
func (d *DiophantineStrategy) GetRateLimit() int {
	return d.rateLimit
}

// GetWindow returns the time window
func (d *DiophantineStrategy) GetWindow() time.Duration {
	return d.window
}

// GetRetryOffsets returns the retry offsets
func (d *DiophantineStrategy) GetRetryOffsets() []time.Duration {
	return d.retryOffsets
}
