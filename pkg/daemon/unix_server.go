package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultConnectionTimeout is the default timeout for idle connections
	DefaultConnectionTimeout = 30 * time.Second
	// DefaultMaxConnections is the default maximum number of concurrent connections
	DefaultMaxConnections = 10
	// SocketPermissions defines the file permissions for the Unix socket
	SocketPermissions = 0600
)

// UnixServer represents a Unix domain socket server for daemon communication
type UnixServer struct {
	socketPath        string
	listener          net.Listener
	connectionTimeout time.Duration
	maxConnections    int
	activeConnections int
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
}

// NewUnixServer creates a new Unix socket server
func NewUnixServer(socketPath string) *UnixServer {
	return &UnixServer{
		socketPath:        socketPath,
		connectionTimeout: DefaultConnectionTimeout,
		maxConnections:    DefaultMaxConnections,
	}
}

// SetConnectionTimeout sets the connection timeout
func (s *UnixServer) SetConnectionTimeout(timeout time.Duration) {
	s.connectionTimeout = timeout
}

// SetMaxConnections sets the maximum number of concurrent connections
func (s *UnixServer) SetMaxConnections(max int) {
	s.maxConnections = max
}

// Start starts the Unix socket server
func (s *UnixServer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Remove existing socket file if it exists
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	s.listener = listener

	// Set socket permissions for security
	if err := os.Chmod(s.socketPath, SocketPermissions); err != nil {
		s.listener.Close()
		return err
	}

	// Start accepting connections in a goroutine
	go s.acceptConnections()

	return nil
}

// Stop stops the Unix socket server gracefully
func (s *UnixServer) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}

	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}

	// Wait for all connections to finish
	s.wg.Wait()

	// Clean up socket file
	if removeErr := os.Remove(s.socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
		if err == nil {
			err = removeErr
		}
	}

	return err
}

// acceptConnections handles incoming connections
func (s *UnixServer) acceptConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				continue
			}
		}

		// Check max connections
		s.mu.Lock()
		if s.activeConnections >= s.maxConnections {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.activeConnections++
		s.wg.Add(1)
		s.mu.Unlock()

		// Handle connection in goroutine
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single connection
func (s *UnixServer) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		s.activeConnections--
		s.mu.Unlock()
		s.wg.Done()
	}()

	// Set connection timeout
	if s.connectionTimeout > 0 {
		conn.SetDeadline(time.Now().Add(s.connectionTimeout))
	}

	// Create buffered reader for line-based protocol
	reader := bufio.NewReader(conn)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Read line-delimited JSON messages
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		// Parse and handle the message
		response := s.handleProtocolMessage(strings.TrimSpace(line))

		// Send response with newline
		responseData, err := json.Marshal(response)
		if err != nil {
			continue
		}

		conn.Write(append(responseData, '\n'))
	}
}

// handleProtocolMessage processes a protocol message and returns a response
func (s *UnixServer) handleProtocolMessage(message string) map[string]interface{} {
	var request map[string]interface{}

	// Parse JSON message
	if err := json.Unmarshal([]byte(message), &request); err != nil {
		return map[string]interface{}{
			"type":  "error",
			"error": "invalid JSON",
		}
	}

	// Handle different message types
	msgType, ok := request["type"].(string)
	if !ok {
		return map[string]interface{}{
			"type":  "error",
			"error": "missing or invalid type field",
		}
	}

	switch msgType {
	case "handshake":
		return s.handleHandshake(request)
	case "schedule_request":
		return s.handleScheduleRequest(request)
	case "register_request":
		return s.handleRegisterRequest(request)
	default:
		return map[string]interface{}{
			"type":  "error",
			"error": "unknown message type",
		}
	}
}

// handleHandshake processes handshake messages
func (s *UnixServer) handleHandshake(request map[string]interface{}) map[string]interface{} {
	version, ok := request["version"].(string)
	if !ok {
		return map[string]interface{}{
			"type":  "error",
			"error": "missing version",
		}
	}

	// Only support version 1.0 for now
	if version != "1.0" {
		return map[string]interface{}{
			"type":  "error",
			"error": "unsupported protocol version",
		}
	}

	return map[string]interface{}{
		"type":    "handshake_response",
		"status":  "ok",
		"version": "1.0",
	}
}

// handleScheduleRequest processes schedule request messages
func (s *UnixServer) handleScheduleRequest(request map[string]interface{}) map[string]interface{} {
	// For now, always allow scheduling (simple implementation)
	return map[string]interface{}{
		"type":         "schedule_response",
		"can_schedule": true,
		"wait_until":   time.Now().Format(time.RFC3339),
		"reason":       "test implementation",
	}
}

// handleRegisterRequest processes register request messages
func (s *UnixServer) handleRegisterRequest(request map[string]interface{}) map[string]interface{} {
	// For now, always succeed (simple implementation)
	return map[string]interface{}{
		"type":    "register_response",
		"success": true,
		"message": "requests registered successfully",
	}
}
