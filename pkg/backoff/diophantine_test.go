package backoff

import (
	"testing"
	"time"
)

func TestCanScheduleRequest(t *testing.T) {
	// Test case 1: Scheduling a new request that does not exceed the rate limit
	existing := []time.Time{
		time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 10, 10, 0, 0, time.UTC),
	}
	newRequestTime := time.Date(2024, 1, 1, 10, 20, 0, 0, time.UTC)
	retryOffsets := []time.Duration{0, 5 * time.Minute}
	rateLimit := 5
	window := 30 * time.Minute

	strategy := NewDiophantine(rateLimit, window, retryOffsets)

	if !strategy.CanScheduleRequest(existing, newRequestTime) {
		t.Errorf("Expected to be able to schedule request, but it was denied")
	}

	// Test case 2: Scheduling a new request that exceeds the rate limit
	// Existing requests at 10:00, 10:01, 10:02
	// New request at 10:03 with retry at 10:03
	// Rate limit is 2 requests per 5-minute window
	// Window 10:00-10:05 would contain: 10:00, 10:01, 10:02, 10:03 = 4 requests (exceeds limit of 2)
	existing = []time.Time{
		time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 10, 1, 0, 0, time.UTC),
		time.Date(2024, 1, 1, 10, 2, 0, 0, time.UTC),
	}
	newRequestTime = time.Date(2024, 1, 1, 10, 3, 0, 0, time.UTC)
	retryOffsets = []time.Duration{0}
	rateLimit = 2
	window = 5 * time.Minute

	strategy = NewDiophantine(rateLimit, window, retryOffsets)

	if strategy.CanScheduleRequest(existing, newRequestTime) {
		t.Errorf("Expected to not be able to schedule request, but it was allowed")
	}
}
