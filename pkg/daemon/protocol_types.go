package daemon

import "time"

// JSON Protocol message types for type-safe daemon communication
// These replace map[string]interface{} usage throughout daemon package

// HandshakeRequestJSON represents a client handshake request in JSON protocol
type HandshakeRequestJSON struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	Client  string `json:"client"`
}

// HandshakeResponseJSON represents a daemon handshake response in JSON protocol
type HandshakeResponseJSON struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ScheduleRequestJSON represents a request to schedule a command execution in JSON protocol
type ScheduleRequestJSON struct {
	Type        string    `json:"type"`
	ResourceID  string    `json:"resource_id"`
	Command     []string  `json:"command"`
	RequestedAt time.Time `json:"requested_at"`
}

// ScheduleResponseJSON represents a response to a schedule request in JSON protocol
type ScheduleResponseJSON struct {
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	CanSchedule  bool      `json:"can_schedule"`
	Reason       string    `json:"reason,omitempty"`
	Message      string    `json:"message,omitempty"`
	ScheduledAt  time.Time `json:"scheduled_at,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// RequestInfoJSON represents information about a single request for registration in JSON protocol
type RequestInfoJSON struct {
	RequestedAt time.Time `json:"requested_at"`
	Command     []string  `json:"command"`
}

// RegisterRequestJSON represents a request to register multiple requests for rate limiting in JSON protocol
type RegisterRequestJSON struct {
	Type       string            `json:"type"`
	ResourceID string            `json:"resource_id"`
	Requests   []RequestInfoJSON `json:"requests"`
}

// RegisterResponseJSON represents a response to a register request in JSON protocol
type RegisterResponseJSON struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ErrorResponseJSON represents an error response in JSON protocol
type ErrorResponseJSON struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// ProtocolMessageJSON is a union interface for all JSON protocol messages
type ProtocolMessageJSON interface {
	GetType() string
}

// Implement ProtocolMessageJSON interface for all message types
func (m HandshakeRequestJSON) GetType() string  { return m.Type }
func (m HandshakeResponseJSON) GetType() string { return m.Type }
func (m ScheduleRequestJSON) GetType() string   { return m.Type }
func (m ScheduleResponseJSON) GetType() string  { return m.Type }
func (m RegisterRequestJSON) GetType() string   { return m.Type }
func (m RegisterResponseJSON) GetType() string  { return m.Type }
func (m ErrorResponseJSON) GetType() string     { return m.Type }
