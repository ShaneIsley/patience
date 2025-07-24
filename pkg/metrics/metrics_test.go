package metrics

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_CreateRunMetrics(t *testing.T) {
	// Given command and execution details
	command := []string{"echo", "hello"}
	totalDuration := 2*time.Second + 500*time.Millisecond
	attempts := []AttemptMetric{
		{Duration: 1 * time.Second, ExitCode: 1, Success: false},
		{Duration: 800 * time.Millisecond, ExitCode: 0, Success: true},
	}

	// When creating run metrics
	metrics := NewRunMetrics(command, true, totalDuration, attempts)

	// Then it should have correct values
	assert.Equal(t, "echo hello", metrics.Command)
	assert.NotEmpty(t, metrics.CommandHash)
	assert.Equal(t, "succeeded", metrics.FinalStatus)
	assert.Equal(t, 2.5, metrics.TotalDurationSeconds)
	assert.Equal(t, 2, metrics.TotalAttempts)
	assert.Equal(t, 1, metrics.SuccessfulAttempts)
	assert.Equal(t, 1, metrics.FailedAttempts)
	assert.Len(t, metrics.Attempts, 2)
	assert.True(t, metrics.Timestamp > 0)
}

func TestMetrics_CreateRunMetrics_Failed(t *testing.T) {
	// Given a failed command execution
	command := []string{"false"}
	totalDuration := 1 * time.Second
	attempts := []AttemptMetric{
		{Duration: 500 * time.Millisecond, ExitCode: 1, Success: false},
		{Duration: 500 * time.Millisecond, ExitCode: 1, Success: false},
	}

	// When creating run metrics
	metrics := NewRunMetrics(command, false, totalDuration, attempts)

	// Then it should have correct failure status
	assert.Equal(t, "failed", metrics.FinalStatus)
	assert.Equal(t, 2, metrics.TotalAttempts)
	assert.Equal(t, 0, metrics.SuccessfulAttempts)
	assert.Equal(t, 2, metrics.FailedAttempts)
}

func TestMetrics_CommandHash(t *testing.T) {
	// Given different commands
	cmd1 := []string{"echo", "hello"}
	cmd2 := []string{"echo", "world"}
	cmd3 := []string{"echo", "hello"} // Same as cmd1

	// When generating hashes
	hash1 := generateCommandHash(cmd1)
	hash2 := generateCommandHash(cmd2)
	hash3 := generateCommandHash(cmd3)

	// Then hashes should be consistent and different
	assert.NotEmpty(t, hash1)
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash1, hash2) // Different commands have different hashes
	assert.Equal(t, hash1, hash3)    // Same commands have same hashes
	assert.Len(t, hash1, 8)          // Hash should be 8 characters (first 8 of SHA256)
}

func TestMetrics_JSONSerialization(t *testing.T) {
	// Given run metrics
	command := []string{"test", "command"}
	attempts := []AttemptMetric{
		{Duration: 1 * time.Second, ExitCode: 0, Success: true},
	}
	metrics := NewRunMetrics(command, true, 1*time.Second, attempts)

	// When serializing to JSON
	data, err := json.Marshal(metrics)

	// Then it should serialize successfully
	require.NoError(t, err)
	assert.Contains(t, string(data), "test command")
	assert.Contains(t, string(data), "succeeded")

	// And deserialize back correctly
	var deserialized RunMetrics
	err = json.Unmarshal(data, &deserialized)
	require.NoError(t, err)
	assert.Equal(t, metrics.Command, deserialized.Command)
	assert.Equal(t, metrics.FinalStatus, deserialized.FinalStatus)
}

func TestClient_NewClient(t *testing.T) {
	// When creating a new client
	client := NewClient("/tmp/test-retryd.sock")

	// Then it should have correct socket path
	assert.Equal(t, "/tmp/test-retryd.sock", client.socketPath)
	assert.Equal(t, 100*time.Millisecond, client.timeout)
}

func TestClient_SendMetrics_DaemonNotRunning(t *testing.T) {
	// Given a client with non-existent socket
	client := NewClient("/tmp/non-existent-retryd.sock")

	// And some metrics
	metrics := &RunMetrics{
		Command:     "test",
		FinalStatus: "succeeded",
	}

	// When sending metrics (daemon not running)
	err := client.SendMetrics(metrics)

	// Then it should fail gracefully without blocking
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
}

func TestClient_SendMetricsAsync_DaemonNotRunning(t *testing.T) {
	// Given a client with non-existent socket
	client := NewClient("/tmp/non-existent-retryd.sock")

	// And some metrics
	metrics := &RunMetrics{
		Command:     "test",
		FinalStatus: "succeeded",
	}

	// When sending metrics asynchronously (daemon not running)
	start := time.Now()
	client.SendMetricsAsync(metrics)
	elapsed := time.Since(start)

	// Then it should return immediately (non-blocking)
	assert.Less(t, elapsed, 10*time.Millisecond)
}

func TestClient_SendMetrics_WithMockDaemon(t *testing.T) {
	// Given a mock Unix socket server
	socketPath := "/tmp/test-retryd-" + fmt.Sprintf("%d", time.Now().UnixNano()) + ".sock"

	// Clean up socket file after test
	defer func() {
		os.Remove(socketPath)
	}()

	// Start mock daemon
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	// Channel to receive data
	received := make(chan []byte, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		received <- buf[:n]
	}()

	// And a client
	client := NewClient(socketPath)

	// And some metrics
	metrics := &RunMetrics{
		Command:              "test command",
		CommandHash:          "abcd1234",
		FinalStatus:          "succeeded",
		TotalDurationSeconds: 1.5,
		TotalAttempts:        2,
		Timestamp:            time.Now().Unix(),
	}

	// When sending metrics
	err = client.SendMetrics(metrics)

	// Then it should succeed
	require.NoError(t, err)

	// And daemon should receive the data
	select {
	case data := <-received:
		var receivedMetrics RunMetrics
		err = json.Unmarshal(data, &receivedMetrics)
		require.NoError(t, err)
		assert.Equal(t, "test command", receivedMetrics.Command)
		assert.Equal(t, "succeeded", receivedMetrics.FinalStatus)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for metrics data")
	}
}
func TestClient_SendMetricsAsync_WithMockDaemon(t *testing.T) {
	// Given a mock Unix socket server
	socketPath := "/tmp/test-async-retryd-" + fmt.Sprintf("%d", time.Now().UnixNano()) + ".sock"

	// Clean up socket file after test
	defer func() {
		os.Remove(socketPath)
	}()

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	// Channel to receive data
	received := make(chan []byte, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		received <- buf[:n]
	}()

	// And a client
	client := NewClient(socketPath)

	// And some metrics
	metrics := &RunMetrics{
		Command:     "async test",
		FinalStatus: "failed",
	}

	// When sending metrics asynchronously
	start := time.Now()
	client.SendMetricsAsync(metrics)
	elapsed := time.Since(start)

	// Then it should return immediately
	assert.Less(t, elapsed, 10*time.Millisecond)

	// And daemon should eventually receive the data
	select {
	case data := <-received:
		var receivedMetrics RunMetrics
		err = json.Unmarshal(data, &receivedMetrics)
		require.NoError(t, err)
		assert.Equal(t, "async test", receivedMetrics.Command)
		assert.Equal(t, "failed", receivedMetrics.FinalStatus)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for async metrics data")
	}
}
func TestClient_Timeout(t *testing.T) {
	// Given a client with very short timeout
	client := NewClient("/tmp/non-existent-socket-for-timeout-test.sock")
	client.timeout = 1 * time.Millisecond

	// When sending metrics to non-existent socket
	metrics := &RunMetrics{Command: "test"}
	start := time.Now()
	err := client.SendMetrics(metrics)
	elapsed := time.Since(start)

	// Then it should fail quickly (connection timeout or no such file)
	assert.Error(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond) // Should fail quickly
}
func TestDefaultSocketPath(t *testing.T) {
	// When getting default socket path
	path := DefaultSocketPath()

	// Then it should be the expected path
	assert.Equal(t, "/tmp/retryd.sock", path)
}

func TestAttemptMetric_Creation(t *testing.T) {
	// When creating an attempt metric
	attempt := AttemptMetric{
		Duration: 1500 * time.Millisecond,
		ExitCode: 0,
		Success:  true,
	}

	// Then it should have correct values
	assert.Equal(t, 1.5, attempt.DurationSeconds())
	assert.Equal(t, 0, attempt.ExitCode)
	assert.True(t, attempt.Success)
}
