package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"
)

func TestUnixServerProtocolHandshake(t *testing.T) {
	socketPath := "/tmp/test_protocol_handshake.sock"
	os.Remove(socketPath)

	server := NewUnixServer(socketPath)
	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Connect to server
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Expected to connect to server, got error: %v", err)
	}
	defer conn.Close()

	// Send handshake message
	handshake := map[string]interface{}{
		"type":    "handshake",
		"version": "1.0",
		"client":  "test-client",
	}

	data, err := json.Marshal(handshake)
	if err != nil {
		t.Fatalf("Failed to marshal handshake: %v", err)
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		t.Fatalf("Failed to send handshake: %v", err)
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("Failed to read handshake response: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal(buffer[:n], &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal handshake response: %v", err)
	}

	if response["status"] != "ok" {
		t.Fatalf("Expected handshake status 'ok', got %v", response["status"])
	}
}

func TestUnixServerScheduleRequest(t *testing.T) {
	socketPath := "/tmp/test_schedule_request.sock"
	os.Remove(socketPath)

	server := NewUnixServer(socketPath)
	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Connect to server
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Expected to connect to server, got error: %v", err)
	}
	defer conn.Close()

	// Send schedule request
	scheduleReq := map[string]interface{}{
		"type":         "schedule_request",
		"resource_id":  "test-api",
		"rate_limit":   100,
		"window":       "1h",
		"request_time": time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(scheduleReq)
	if err != nil {
		t.Fatalf("Failed to marshal schedule request: %v", err)
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		t.Fatalf("Failed to send schedule request: %v", err)
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("Failed to read schedule response: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal(buffer[:n], &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal schedule response: %v", err)
	}

	if response["type"] != "schedule_response" {
		t.Fatalf("Expected response type 'schedule_response', got %v", response["type"])
	}

	if _, ok := response["can_schedule"]; !ok {
		t.Fatalf("Expected 'can_schedule' field in response")
	}
}

func TestUnixServerRegisterRequest(t *testing.T) {
	socketPath := "/tmp/test_register_request.sock"
	os.Remove(socketPath)

	server := NewUnixServer(socketPath)
	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Connect to server
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Expected to connect to server, got error: %v", err)
	}
	defer conn.Close()

	// Send register request
	registerReq := map[string]interface{}{
		"type": "register_request",
		"requests": []map[string]interface{}{
			{
				"id":           "req-1",
				"resource_id":  "test-api",
				"scheduled_at": time.Now().Add(time.Minute).Format(time.RFC3339),
				"expires_at":   time.Now().Add(time.Hour).Format(time.RFC3339),
			},
		},
	}

	data, err := json.Marshal(registerReq)
	if err != nil {
		t.Fatalf("Failed to marshal register request: %v", err)
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("Failed to read register response: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal(buffer[:n], &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal register response: %v", err)
	}

	if response["type"] != "register_response" {
		t.Fatalf("Expected response type 'register_response', got %v", response["type"])
	}

	if response["success"] != true {
		t.Fatalf("Expected successful registration")
	}
}

func TestUnixServerInvalidRequest(t *testing.T) {
	socketPath := "/tmp/test_invalid_request.sock"
	os.Remove(socketPath)

	server := NewUnixServer(socketPath)
	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Connect to server
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Expected to connect to server, got error: %v", err)
	}
	defer conn.Close()

	// Send invalid JSON
	_, err = conn.Write([]byte("invalid json\n"))
	if err != nil {
		t.Fatalf("Failed to send invalid request: %v", err)
	}

	// Read error response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("Failed to read error response: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal(buffer[:n], &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if response["type"] != "error" {
		t.Fatalf("Expected response type 'error', got %v", response["type"])
	}
}

func TestUnixServerProtocolVersioning(t *testing.T) {
	socketPath := "/tmp/test_protocol_version.sock"
	os.Remove(socketPath)

	server := NewUnixServer(socketPath)
	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Connect to server
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Expected to connect to server, got error: %v", err)
	}
	defer conn.Close()

	// Send handshake with unsupported version
	handshake := map[string]interface{}{
		"type":    "handshake",
		"version": "2.0", // Unsupported version
		"client":  "test-client",
	}

	data, err := json.Marshal(handshake)
	if err != nil {
		t.Fatalf("Failed to marshal handshake: %v", err)
	}

	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		t.Fatalf("Failed to send handshake: %v", err)
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		t.Fatalf("Failed to read handshake response: %v", err)
	}

	var response map[string]interface{}
	err = json.Unmarshal(buffer[:n], &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal handshake response: %v", err)
	}

	if response["type"] != "error" {
		t.Fatalf("Expected error response for unsupported version, got %v", response["type"])
	}
}
