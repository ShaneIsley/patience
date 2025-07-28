package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnixServerSocketCreation(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "t.sock")

	server := NewUnixServer(socketPath)

	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start successfully, got error: %v", err)
	}
	defer server.Stop()

	// Verify socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Fatalf("Expected socket file to exist at %s", socketPath)
	}

	// Verify it's a socket
	fileInfo, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Failed to stat socket file: %v", err)
	}

	if fileInfo.Mode()&os.ModeSocket == 0 {
		t.Fatalf("Expected %s to be a socket file", socketPath)
	}
}

func TestUnixServerBindToSocket(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "b.sock")

	server := NewUnixServer(socketPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Expected server to bind to socket, got error: %v", err)
	}
	defer server.Stop()

	// Try to connect to verify it's listening
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Expected to connect to socket, got error: %v", err)
	}
	conn.Close()
}

func TestUnixServerSocketPermissions(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "p.sock")

	server := NewUnixServer(socketPath)

	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Check socket permissions (should be 0600 for security)
	fileInfo, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("Failed to stat socket file: %v", err)
	}

	expectedPerm := os.FileMode(0600)
	if fileInfo.Mode().Perm() != expectedPerm {
		t.Fatalf("Expected socket permissions %v, got %v", expectedPerm, fileInfo.Mode().Perm())
	}
}

func TestUnixServerSocketCleanupOnShutdown(t *testing.T) {
	socketPath := "/tmp/test_cleanup.sock"
	// Clean up any existing socket
	os.Remove(socketPath)

	server := NewUnixServer(socketPath)

	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}

	// Give server time to fully start
	time.Sleep(10 * time.Millisecond)

	// Verify socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Fatalf("Expected socket file to exist before shutdown")
	}

	// Stop server
	err = server.Stop()
	if err != nil {
		t.Fatalf("Expected clean shutdown, got error: %v", err)
	}

	// Verify socket is cleaned up
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("Expected socket file to be cleaned up after shutdown")
	}
}
func TestUnixServerAcceptConnections(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "a.sock")

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

	// Write a simple message
	_, err = conn.Write([]byte("test message"))
	if err != nil {
		t.Fatalf("Expected to write to connection, got error: %v", err)
	}

	// Server should handle the connection without crashing
	time.Sleep(100 * time.Millisecond)
}

func TestUnixServerMultipleConnections(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "m.sock")

	server := NewUnixServer(socketPath)

	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Create multiple connections
	var connections []net.Conn
	for i := 0; i < 3; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("Expected to create connection %d, got error: %v", i, err)
		}
		connections = append(connections, conn)
	}

	// Clean up connections
	for _, conn := range connections {
		conn.Close()
	}
}

func TestUnixServerConnectionTimeout(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "to.sock")

	server := NewUnixServer(socketPath)
	server.SetConnectionTimeout(100 * time.Millisecond)

	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Connect but don't send anything
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Expected to connect, got error: %v", err)
	}
	defer conn.Close()

	// Wait longer than timeout
	time.Sleep(200 * time.Millisecond)

	// Connection should be closed by server
	_, err = conn.Write([]byte("test"))
	if err == nil {
		t.Fatalf("Expected connection to be closed due to timeout")
	}
}

func TestUnixServerMaxConnections(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "mc.sock")

	server := NewUnixServer(socketPath)
	server.SetMaxConnections(2)

	err := server.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected server to start, got error: %v", err)
	}
	defer server.Stop()

	// Create max connections
	var connections []net.Conn
	for i := 0; i < 2; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("Expected to create connection %d, got error: %v", i, err)
		}
		connections = append(connections, conn)
		// Send some data to ensure connection is established
		conn.Write([]byte("test"))
	}

	// Give server time to process connections
	time.Sleep(50 * time.Millisecond)

	// Third connection should be rejected or timeout
	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err == nil {
		// If connection succeeded, it should be closed immediately by server
		time.Sleep(50 * time.Millisecond)
		_, writeErr := conn.Write([]byte("test"))
		conn.Close()
		if writeErr == nil {
			t.Fatalf("Expected third connection to be rejected or closed")
		}
	}

	// Clean up
	for _, conn := range connections {
		conn.Close()
	}
}
