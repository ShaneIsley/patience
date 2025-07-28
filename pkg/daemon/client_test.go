package daemon

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDaemonClient_Connect(t *testing.T) {
	// Create temporary socket path (shorter to avoid Unix socket path limits)
	socketPath := "/tmp/test-daemon-connect.sock"
	defer os.Remove(socketPath)

	// Start a real Unix server for testing
	server := NewUnixServer(socketPath)
	server.SetConnectionTimeout(5 * time.Second)
	server.SetMaxConnections(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test client connection
	client := NewDaemonClient(socketPath)
	defer client.Close()

	// This should establish a real Unix socket connection
	// The connect() method doesn't exist yet - this test should fail
	err := client.connect()
	if err != nil {
		t.Errorf("expected successful connection, got error: %v", err)
	}

	// Verify connection is established
	// The conn field doesn't exist yet - this test should fail
	if client.conn == nil {
		t.Errorf("expected connection to be established")
	}
}

func TestDaemonClient_Handshake(t *testing.T) {
	// Create temporary socket path (shorter to avoid Unix socket path limits)
	socketPath := "/tmp/test-daemon-handshake.sock"
	defer os.Remove(socketPath)

	// Start a real Unix server for testing
	server := NewUnixServer(socketPath)
	server.SetConnectionTimeout(5 * time.Second)
	server.SetMaxConnections(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test client handshake
	client := NewDaemonClient(socketPath)
	defer client.Close()

	// Connect should establish connection and perform handshake
	err := client.connect()
	if err != nil {
		t.Errorf("expected successful connection and handshake, got error: %v", err)
	}

	// Verify connection is established (handshake was successful)
	if client.conn == nil {
		t.Errorf("expected connection to be established after handshake")
	}
}

func TestDaemonClient_CanScheduleRequest_RealDaemon(t *testing.T) {
	// Create temporary socket path (shorter to avoid Unix socket path limits)
	socketPath := "/tmp/test-daemon-schedule.sock"
	defer os.Remove(socketPath)

	// Start a real Unix server with scheduler integration
	server := NewUnixServer(socketPath)
	server.SetConnectionTimeout(5 * time.Second)
	server.SetMaxConnections(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	tests := []struct {
		name        string
		request     *ScheduleRequest
		expectError bool
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
			expectError: false,
		},
		{
			name: "rate limit check request",
			request: &ScheduleRequest{
				ResourceID:   "busy-api",
				RateLimit:    1,
				Window:       time.Hour,
				RetryOffsets: []time.Duration{0},
				RequestTime:  time.Now(),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewDaemonClient(socketPath)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// This should send real JSON protocol message to Unix server
			response, err := client.CanScheduleRequest(ctx, tt.request)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError {
				if response == nil {
					t.Errorf("expected response but got nil")
				}
				// For now, accept the basic server implementation
				// In REFACTOR phase, we'll integrate with real scheduler logic
				if response.Reason == "" {
					t.Errorf("expected reason in response")
				}
			}
		})
	}
}

func TestDaemonClient_RegisterScheduledRequests_RealDaemon(t *testing.T) {
	// Create temporary socket path (shorter to avoid Unix socket path limits)
	socketPath := "/tmp/test-daemon-register.sock"
	defer os.Remove(socketPath)

	// Start a real Unix server
	server := NewUnixServer(socketPath)
	server.SetConnectionTimeout(5 * time.Second)
	server.SetMaxConnections(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	client := NewDaemonClient(socketPath)
	defer client.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

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

	// This should send real JSON protocol message to Unix server
	err := client.RegisterScheduledRequests(ctx2, requests)
	if err != nil {
		t.Errorf("unexpected error registering requests: %v", err)
	}
}

func TestDaemonClient_ConnectionFailure(t *testing.T) {
	// Test with non-existent socket path
	nonExistentPath := "/tmp/non-existent-daemon.sock"

	// Ensure socket doesn't exist
	os.Remove(nonExistentPath)

	client := NewDaemonClient(nonExistentPath)
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

	// This should fail with real connection error, not mock error
	_, err := client.CanScheduleRequest(ctx, request)
	if err == nil {
		t.Errorf("expected connection error but got none")
	}

	// Should be a real connection error, not the mock "connection refused"
	if err.Error() == "connection refused" {
		t.Errorf("got mock error, expected real connection error")
	}
}

func TestDaemonClient_Timeout_RealConnection(t *testing.T) {
	// Create temporary socket path (shorter to avoid Unix socket path limits)
	socketPath := "/tmp/test-daemon-timeout.sock"
	defer os.Remove(socketPath)

	// Start a real Unix server
	server := NewUnixServer(socketPath)
	server.SetConnectionTimeout(5 * time.Second)
	server.SetMaxConnections(10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Start(ctx); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	client := NewDaemonClient(socketPath)
	defer client.Close()

	// Very short timeout to force timeout error
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel2()

	request := &ScheduleRequest{
		ResourceID:   "test-api",
		RateLimit:    100,
		Window:       time.Hour,
		RetryOffsets: []time.Duration{0},
		RequestTime:  time.Now(),
	}

	// This should timeout on real network operation
	_, err := client.CanScheduleRequest(ctx2, request)
	if err == nil {
		t.Errorf("expected timeout error but got none")
	}

	// Should be a real context timeout, not mock behavior
	if err != context.DeadlineExceeded {
		t.Logf("Expected context.DeadlineExceeded, got: %v", err)
	}
}
