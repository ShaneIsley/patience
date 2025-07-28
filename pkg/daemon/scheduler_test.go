package daemon

import (
	"fmt"
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
)

func TestRequestScheduler_TrackActiveRequests(t *testing.T) {
	scheduler := NewRequestScheduler()

	// Test adding new requests
	req1 := &ScheduledRequest{
		ID:          "req-1",
		ResourceID:  "test-api",
		ScheduledAt: time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	err := scheduler.AddRequest(req1)
	if err != nil {
		t.Errorf("unexpected error adding request: %v", err)
	}

	// Test retrieving requests
	requests := scheduler.GetActiveRequests("test-api")
	if len(requests) != 1 {
		t.Errorf("expected 1 active request, got %d", len(requests))
	}

	if requests[0].ID != "req-1" {
		t.Errorf("expected request ID 'req-1', got '%s'", requests[0].ID)
	}

	// Test adding duplicate request
	err = scheduler.AddRequest(req1)
	if err == nil {
		t.Errorf("expected error when adding duplicate request")
	}

	// Test request expiration
	expiredReq := &ScheduledRequest{
		ID:          "req-expired",
		ResourceID:  "test-api",
		ScheduledAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(-time.Hour), // Already expired
	}

	err = scheduler.AddRequest(expiredReq)
	if err != nil {
		t.Errorf("unexpected error adding expired request: %v", err)
	}

	// Clean up expired requests
	scheduler.CleanupExpiredRequests()

	// Should only have the non-expired request
	requests = scheduler.GetActiveRequests("test-api")
	if len(requests) != 1 {
		t.Errorf("expected 1 active request after cleanup, got %d", len(requests))
	}
	if requests[0].ID != "req-1" {
		t.Errorf("expected remaining request to be 'req-1', got '%s'", requests[0].ID)
	}
}

func TestRequestScheduler_ConcurrentAccess(t *testing.T) {
	scheduler := NewRequestScheduler()

	// Test concurrent access safety
	done := make(chan bool, 10)

	// Start 10 goroutines adding requests concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			req := &ScheduledRequest{
				ID:          fmt.Sprintf("req-%d", id),
				ResourceID:  "concurrent-api",
				ScheduledAt: time.Now(),
				ExpiresAt:   time.Now().Add(time.Hour),
			}
			scheduler.AddRequest(req)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 10 requests
	requests := scheduler.GetActiveRequests("concurrent-api")
	if len(requests) != 10 {
		t.Errorf("expected 10 concurrent requests, got %d", len(requests))
	}
}

func TestRequestScheduler_CanScheduleWithExisting(t *testing.T) {
	scheduler := NewRequestScheduler()

	// Add some existing requests
	baseTime := time.Now().Truncate(time.Minute) // Use current time to avoid expiration issues
	existingRequests := []*ScheduledRequest{
		{
			ID:          "existing-1",
			ResourceID:  "rate-limited-api",
			ScheduledAt: baseTime,
			ExpiresAt:   baseTime.Add(time.Hour),
		},
		{
			ID:          "existing-2",
			ResourceID:  "rate-limited-api",
			ScheduledAt: baseTime.Add(10 * time.Minute),
			ExpiresAt:   baseTime.Add(time.Hour),
		},
		{
			ID:          "existing-3",
			ResourceID:  "rate-limited-api",
			ScheduledAt: baseTime.Add(20 * time.Minute),
			ExpiresAt:   baseTime.Add(time.Hour),
		},
	}

	for _, req := range existingRequests {
		scheduler.AddRequest(req)
	}

	// Create Diophantine strategy with rate limit that makes sense for the test
	strategy := backoff.NewDiophantine(4, 30*time.Minute, []time.Duration{0, 10 * time.Minute})

	// Test scheduling request that would exceed rate limit
	newRequestTime := baseTime.Add(5 * time.Minute)
	canSchedule := scheduler.CanScheduleWithStrategy("rate-limited-api", strategy, newRequestTime)

	if canSchedule {
		t.Errorf("expected scheduling to be denied due to rate limit, but it was allowed")
	}
	// Test scheduling request that should be allowed
	laterRequestTime := baseTime.Add(45 * time.Minute) // Outside the rate limit window
	canSchedule = scheduler.CanScheduleWithStrategy("rate-limited-api", strategy, laterRequestTime)

	if !canSchedule {
		t.Errorf("expected scheduling to be allowed, but it was denied")
	}
}

func TestRequestScheduler_GetNextAvailableSlot(t *testing.T) {
	scheduler := NewRequestScheduler()

	// Add requests that fill up the rate limit
	baseTime := time.Now().Truncate(time.Minute)
	for i := 0; i < 5; i++ {
		req := &ScheduledRequest{
			ID:          fmt.Sprintf("slot-req-%d", i),
			ResourceID:  "slot-api",
			ScheduledAt: baseTime.Add(time.Duration(i) * time.Minute),
			ExpiresAt:   baseTime.Add(time.Hour),
		}
		scheduler.AddRequest(req)
	}

	strategy := backoff.NewDiophantine(3, 10*time.Minute, []time.Duration{0})

	// Find next available slot
	nextSlot := scheduler.GetNextAvailableSlot("slot-api", strategy, baseTime.Add(2*time.Minute))

	// Should be after the rate limit window clears
	expectedEarliest := baseTime.Add(10 * time.Minute) // After first request's window expires
	if nextSlot.Before(expectedEarliest) {
		t.Errorf("next available slot %v is too early, expected after %v", nextSlot, expectedEarliest)
	}
}
