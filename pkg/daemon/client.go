package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// DaemonClient provides an interface for patience instances to communicate with the daemon
type DaemonClient struct {
	socketPath        string
	connectionTimeout time.Duration
	conn              net.Conn
	mu                sync.Mutex
}

// NewDaemonClient creates a new daemon client
func NewDaemonClient(socketPath string) *DaemonClient {
	return &DaemonClient{
		socketPath:        socketPath,
		connectionTimeout: 5 * time.Second,
	}
}

// connect establishes a connection to the daemon if not already connected
func (c *DaemonClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil // Already connected
	}

	conn, err := net.DialTimeout("unix", c.socketPath, c.connectionTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon at %s: %w", c.socketPath, err)
	}

	c.conn = conn
	return c.performHandshake()
}

// performHandshake performs the initial protocol handshake using type-safe protocol
func (c *DaemonClient) performHandshake() error {
	handshakeReq := HandshakeRequestJSON{
		Type:    "handshake",
		Version: "1.0",
		Client:  "patience-cli",
	}

	response, err := c.SendHandshakeTypeSafe(handshakeReq)
	if err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	if response.Status != "ok" {
		return fmt.Errorf("handshake rejected by daemon: %s", response.Message)
	}

	return nil
}

// sendRequest sends a JSON request and returns the JSON response
func (c *DaemonClient) sendRequest(request interface{}) (map[string]interface{}, error) {
	// Encode request as JSON
	requestData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	// Send request with newline
	requestLine := string(requestData) + "\n"
	_, err = c.conn.Write([]byte(requestLine))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response line
	scanner := bufio.NewScanner(c.conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		return nil, fmt.Errorf("no response from daemon")
	}

	// Decode JSON response
	var response map[string]interface{}
	if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, nil
}

// CanScheduleRequest asks the daemon if a request can be scheduled
func (c *DaemonClient) CanScheduleRequest(ctx context.Context, req *ScheduleRequest) (*ScheduleResponse, error) {
	// Check for connection timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Ensure connection is established
	if err := c.connect(); err != nil {
		return nil, err
	}

	// Create schedule request message
	scheduleReq := map[string]interface{}{
		"type":          "schedule_request",
		"resource_id":   req.ResourceID,
		"rate_limit":    req.RateLimit,
		"window_ms":     int64(req.Window / time.Millisecond),
		"retry_offsets": convertDurationsToMs(req.RetryOffsets),
		"request_time":  req.RequestTime.Unix(),
	}

	response, err := c.sendRequest(scheduleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send schedule request: %w", err)
	}

	// Parse response
	canSchedule, _ := response["can_schedule"].(bool)
	reason, _ := response["reason"].(string)

	waitUntil := req.RequestTime.Add(time.Minute) // Default fallback
	if waitUntilStr, ok := response["wait_until"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, waitUntilStr); err == nil {
			waitUntil = parsed
		}
	}
	return &ScheduleResponse{
		CanSchedule: canSchedule,
		WaitUntil:   waitUntil,
		Reason:      reason,
	}, nil
}

// convertDurationsToMs converts duration slice to milliseconds
func convertDurationsToMs(durations []time.Duration) []int64 {
	result := make([]int64, len(durations))
	for i, d := range durations {
		result[i] = int64(d / time.Millisecond)
	}
	return result
}

// RegisterScheduledRequests registers planned requests with the daemon
func (c *DaemonClient) RegisterScheduledRequests(ctx context.Context, requests []*ScheduledRequest) error {
	// Check for connection timeout
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Ensure connection is established
	if err := c.connect(); err != nil {
		return err
	}

	// Convert requests to serializable format
	requestsData := make([]map[string]interface{}, len(requests))
	for i, req := range requests {
		requestsData[i] = map[string]interface{}{
			"id":           req.ID,
			"resource_id":  req.ResourceID,
			"scheduled_at": req.ScheduledAt.Unix(),
			"expires_at":   req.ExpiresAt.Unix(),
		}
	}

	// Create register request message
	registerReq := map[string]interface{}{
		"type":     "register_request",
		"requests": requestsData,
	}

	response, err := c.sendRequest(registerReq)
	if err != nil {
		return fmt.Errorf("failed to send register request: %w", err)
	}

	// Check response status (server returns "success" field, not "status")
	if success, ok := response["success"].(bool); !ok || !success {
		if message, ok := response["message"].(string); ok {
			return fmt.Errorf("register request failed: %s", message)
		}
		return fmt.Errorf("register request failed")
	}

	return nil
}

// Close closes the client connection
func (c *DaemonClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// Type-safe protocol methods

// SendHandshakeTypeSafe performs handshake using type-safe protocol
func (c *DaemonClient) SendHandshakeTypeSafe(req HandshakeRequestJSON) (HandshakeResponseJSON, error) {
	// Convert to old format for now to maintain compatibility
	oldRequest := map[string]interface{}{
		"type":    req.Type,
		"version": req.Version,
		"client":  req.Client,
	}

	response, err := c.sendRequest(oldRequest)
	if err != nil {
		return HandshakeResponseJSON{}, err
	}

	// Convert response back to type-safe format
	resp := HandshakeResponseJSON{
		Type: "handshake_response",
	}

	if status, ok := response["status"].(string); ok {
		resp.Status = status
	}
	if message, ok := response["message"].(string); ok {
		resp.Message = message
	}

	return resp, nil
}

// SendScheduleRequestTypeSafe sends schedule request using type-safe protocol
func (c *DaemonClient) SendScheduleRequestTypeSafe(req ScheduleRequestJSON) (ScheduleResponseJSON, error) {
	// Convert to old format for now to maintain compatibility
	oldRequest := map[string]interface{}{
		"type":         req.Type,
		"resource_id":  req.ResourceID,
		"command":      req.Command,
		"requested_at": req.RequestedAt.Format(time.RFC3339),
	}

	response, err := c.sendRequest(oldRequest)
	if err != nil {
		return ScheduleResponseJSON{}, err
	}

	// Convert response back to type-safe format
	resp := ScheduleResponseJSON{
		Type: "schedule_response",
	}

	if status, ok := response["status"].(string); ok {
		resp.Status = status
	}
	if message, ok := response["message"].(string); ok {
		resp.Message = message
	}
	if scheduledAtStr, ok := response["scheduled_at"].(string); ok {
		if scheduledAt, err := time.Parse(time.RFC3339, scheduledAtStr); err == nil {
			resp.ScheduledAt = scheduledAt
		}
	}
	if expiresAtStr, ok := response["expires_at"].(string); ok {
		if expiresAt, err := time.Parse(time.RFC3339, expiresAtStr); err == nil {
			resp.ExpiresAt = expiresAt
		}
	}

	return resp, nil
}

// SendRegisterRequestTypeSafe sends register request using type-safe protocol
func (c *DaemonClient) SendRegisterRequestTypeSafe(req RegisterRequestJSON) (RegisterResponseJSON, error) {
	// Convert to old format for now to maintain compatibility
	requestsData := make([]map[string]interface{}, len(req.Requests))
	for i, r := range req.Requests {
		requestsData[i] = map[string]interface{}{
			"requested_at": r.RequestedAt.Format(time.RFC3339),
			"command":      r.Command,
		}
	}

	oldRequest := map[string]interface{}{
		"type":        req.Type,
		"resource_id": req.ResourceID,
		"requests":    requestsData,
	}

	response, err := c.sendRequest(oldRequest)
	if err != nil {
		return RegisterResponseJSON{}, err
	}

	// Convert response back to type-safe format
	resp := RegisterResponseJSON{
		Type: "register_response",
	}

	if status, ok := response["status"].(string); ok {
		resp.Status = status
	}
	if message, ok := response["message"].(string); ok {
		resp.Message = message
	}

	return resp, nil
}
