package daemon

import (
	"context"
	"testing"
	"time"
)

func TestDaemonClient_CanScheduleRequest(t *testing.T) {
	tests := []struct {
		name           string
		request        *ScheduleRequest
		expectedResult bool
		expectError    bool
	}{
		{
			name: "successful scheduling request",
			request: &ScheduleRequest{
				ResourceID:   "test-api",
				RateLimit:    100,
				Window:       time.Hour,
				RetryOffsets: []time.Duration{0, 10 * time.Minute, 30 * time.Minute},
				RequestTime:  time.Now(),
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "rate limit exceeded",
			request: &ScheduleRequest{
				ResourceID:   "busy-api",
				RateLimit:    1,
				Window:       time.Hour,
				RetryOffsets: []time.Duration{0},
				RequestTime:  time.Now(),
			},
			expectedResult: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewDaemonClient("localhost:8080")
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			response, err := client.CanScheduleRequest(ctx, tt.request)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && response.CanSchedule != tt.expectedResult {
				t.Errorf("expected CanSchedule=%v, got %v", tt.expectedResult, response.CanSchedule)
			}
		})
	}
}

func TestDaemonClient_RegisterScheduledRequests(t *testing.T) {
	client := NewDaemonClient("localhost:8080")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	requests := []*ScheduledRequest{
		{
			ID:          "req-1",
			ResourceID:  "test-api",
			ScheduledAt: time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		},
		{
			ID:          "req-2",
			ResourceID:  "test-api",
			ScheduledAt: time.Now().Add(10 * time.Minute),
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}

	err := client.RegisterScheduledRequests(ctx, requests)
	if err != nil {
		t.Errorf("unexpected error registering requests: %v", err)
	}
}

func TestDaemonClient_ConnectionFailure(t *testing.T) {
	// Test with non-existent daemon
	client := NewDaemonClient("localhost:9999")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	request := &ScheduleRequest{
		ResourceID:   "test-api",
		RateLimit:    100,
		Window:       time.Hour,
		RetryOffsets: []time.Duration{0},
		RequestTime:  time.Now(),
	}

	_, err := client.CanScheduleRequest(ctx, request)
	if err == nil {
		t.Errorf("expected connection error but got none")
	}
}

func TestDaemonClient_Timeout(t *testing.T) {
	client := NewDaemonClient("localhost:8080")
	defer client.Close()

	// Very short timeout to force timeout error
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	request := &ScheduleRequest{
		ResourceID:   "test-api",
		RateLimit:    100,
		Window:       time.Hour,
		RetryOffsets: []time.Duration{0},
		RequestTime:  time.Now(),
	}

	_, err := client.CanScheduleRequest(ctx, request)
	if err == nil {
		t.Errorf("expected timeout error but got none")
	}
}
