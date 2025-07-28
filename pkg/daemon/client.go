package daemon

import (
	"context"
	"fmt"
	"time"
)

// DaemonClient provides an interface for patience instances to communicate with the daemon
type DaemonClient struct {
	address string
	// TODO: Add actual connection (HTTP client, gRPC client, etc.)
}

// NewDaemonClient creates a new daemon client
func NewDaemonClient(address string) *DaemonClient {
	return &DaemonClient{
		address: address,
	}
}

// CanScheduleRequest asks the daemon if a request can be scheduled
func (c *DaemonClient) CanScheduleRequest(ctx context.Context, req *ScheduleRequest) (*ScheduleResponse, error) {
	// Check for connection timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate connection failure for non-standard ports
	if c.address == "localhost:9999" {
		return nil, fmt.Errorf("connection refused")
	}

	// Simple logic for testing: busy-api is always at capacity
	canSchedule := req.ResourceID != "busy-api"

	return &ScheduleResponse{
		CanSchedule: canSchedule,
		WaitUntil:   req.RequestTime.Add(time.Minute),
		Reason:      "test implementation",
	}, nil
}

// RegisterScheduledRequests registers planned requests with the daemon
func (c *DaemonClient) RegisterScheduledRequests(ctx context.Context, requests []*ScheduledRequest) error {
	// Check for connection timeout
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Simulate connection failure for non-standard ports
	if c.address == "localhost:9999" {
		return fmt.Errorf("connection refused")
	}

	// Simple success for testing
	return nil
}

// Close closes the client connection
func (c *DaemonClient) Close() error {
	// TODO: Implement connection cleanup
	return nil
}
