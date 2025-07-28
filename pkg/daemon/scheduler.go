package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
)

// RequestScheduler manages active requests and scheduling decisions
type RequestScheduler struct {
	mutex    sync.RWMutex
	requests map[string][]*ScheduledRequest // keyed by ResourceID
}

// NewRequestScheduler creates a new request scheduler
func NewRequestScheduler() *RequestScheduler {
	return &RequestScheduler{
		requests: make(map[string][]*ScheduledRequest),
	}
}

// AddRequest adds a new scheduled request
func (s *RequestScheduler) AddRequest(req *ScheduledRequest) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check for duplicate IDs
	for _, requests := range s.requests {
		for _, existing := range requests {
			if existing.ID == req.ID {
				return fmt.Errorf("request with ID %s already exists", req.ID)
			}
		}
	}

	// Add request to the appropriate resource
	s.requests[req.ResourceID] = append(s.requests[req.ResourceID], req)
	return nil
}

// GetActiveRequests returns all active requests for a resource
func (s *RequestScheduler) GetActiveRequests(resourceID string) []*ScheduledRequest {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	requests := s.requests[resourceID]
	result := make([]*ScheduledRequest, 0, len(requests))

	now := time.Now()
	for _, req := range requests {
		if req.ExpiresAt.After(now) {
			result = append(result, req)
		}
	}

	return result
}

// CleanupExpiredRequests removes expired requests
func (s *RequestScheduler) CleanupExpiredRequests() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()
	for resourceID, requests := range s.requests {
		activeRequests := make([]*ScheduledRequest, 0, len(requests))
		for _, req := range requests {
			if req.ExpiresAt.After(now) {
				activeRequests = append(activeRequests, req)
			}
		}
		s.requests[resourceID] = activeRequests
	}
}

// CanScheduleWithStrategy checks if a new request can be scheduled using the given strategy
func (s *RequestScheduler) CanScheduleWithStrategy(resourceID string, strategy *backoff.DiophantineStrategy, requestTime time.Time) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Get existing requests as time.Time slice
	requests := s.requests[resourceID]
	existing := make([]time.Time, 0, len(requests))

	now := time.Now()
	for _, req := range requests {
		if req.ExpiresAt.After(now) {
			existing = append(existing, req.ScheduledAt)
		}
	}

	return strategy.CanScheduleRequest(existing, requestTime)
}

// GetNextAvailableSlot finds the next time when a request can be scheduled
func (s *RequestScheduler) GetNextAvailableSlot(resourceID string, strategy *backoff.DiophantineStrategy, preferredTime time.Time) time.Time {
	// Simple implementation: try every minute until we find an available slot
	candidate := preferredTime
	for i := 0; i < 1440; i++ { // Try for up to 24 hours
		if s.CanScheduleWithStrategy(resourceID, strategy, candidate) {
			return candidate
		}
		candidate = candidate.Add(time.Minute)
	}

	// If no slot found in 24 hours, return 24 hours from now
	return preferredTime.Add(24 * time.Hour)
}
