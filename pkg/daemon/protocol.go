package daemon

import "time"

// ScheduleRequest represents a request to check if a task can be scheduled
type ScheduleRequest struct {
	ResourceID   string          // Identifier for the rate-limited resource
	RateLimit    int             // Maximum requests allowed in the window
	Window       time.Duration   // Time window for rate limiting
	RetryOffsets []time.Duration // Planned retry timing offsets
	RequestTime  time.Time       // When the request wants to be scheduled
}

// ScheduleResponse represents the daemon's response to a schedule request
type ScheduleResponse struct {
	CanSchedule bool      // Whether the request can be scheduled now
	WaitUntil   time.Time // When the request can be scheduled (if not now)
	Reason      string    // Human-readable reason for the decision
}

// ScheduledRequest represents a request that has been scheduled and registered
type ScheduledRequest struct {
	ID          string    // Unique identifier for this request
	ResourceID  string    // Identifier for the rate-limited resource
	ScheduledAt time.Time // When this request is scheduled to execute
	ExpiresAt   time.Time // When this request registration expires
}

// RegisterRequest represents a request to register scheduled requests with the daemon
type RegisterRequest struct {
	Requests []*ScheduledRequest // List of requests to register
}

// RegisterResponse represents the daemon's response to a register request
type RegisterResponse struct {
	Success bool   // Whether the registration was successful
	Message string // Additional information about the registration
}
