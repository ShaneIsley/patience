package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/shaneisley/patience/pkg/metrics"
	"github.com/shaneisley/patience/pkg/storage"
)

func TestDaemon_NewDaemon(t *testing.T) {
	// Given a daemon configuration
	config := &Config{
		SocketPath:    "/tmp/test-daemon.sock",
		HTTPPort:      8081,
		MaxMetrics:    1000,
		MetricsMaxAge: time.Hour,
		LogLevel:      "info",
		PidFile:       "/tmp/test-daemon.pid",
		EnableHTTP:    true,
	}

	// When creating a new daemon
	daemon, err := NewDaemon(config)

	// Then it should succeed
	require.NoError(t, err)
	require.NotNil(t, daemon)
	assert.Equal(t, config, daemon.config)
	assert.NotNil(t, daemon.storage)
	assert.NotNil(t, daemon.logger)
}

func TestDaemon_NewDaemonWithDefaults(t *testing.T) {
	// When creating a daemon with nil config
	daemon, err := NewDaemon(nil)

	// Then it should use default configuration
	require.NoError(t, err)
	require.NotNil(t, daemon)
	assert.Equal(t, "/tmp/retry-daemon.sock", daemon.config.SocketPath)
	assert.Equal(t, 8080, daemon.config.HTTPPort)
}

func TestDaemon_StartStop(t *testing.T) {
	// Given a daemon with test configuration
	tmpDir := t.TempDir()
	config := &Config{
		SocketPath:    filepath.Join(tmpDir, "test-daemon.sock"),
		HTTPPort:      0, // Use random port
		MaxMetrics:    100,
		MetricsMaxAge: time.Hour,
		LogLevel:      "info",
		PidFile:       filepath.Join(tmpDir, "test-daemon.pid"),
		EnableHTTP:    false, // Disable HTTP for simpler test
	}

	daemon, err := NewDaemon(config)
	require.NoError(t, err)

	// When starting the daemon
	err = daemon.Start()
	require.NoError(t, err)

	// Then the socket should exist
	_, err = os.Stat(config.SocketPath)
	assert.NoError(t, err)

	// And PID file should exist
	_, err = os.Stat(config.PidFile)
	assert.NoError(t, err)

	// When stopping the daemon
	err = daemon.Stop()
	require.NoError(t, err)

	// Then socket should be cleaned up
	_, err = os.Stat(config.SocketPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDaemon_HandleConnection(t *testing.T) {
	// Given a running daemon
	tmpDir := t.TempDir()
	socketPath := "/tmp/test-daemon-conn.sock"
	// Clean up socket if it exists
	os.Remove(socketPath)
	defer os.Remove(socketPath)

	config := &Config{
		SocketPath:    socketPath,
		HTTPPort:      0,
		MaxMetrics:    100,
		MetricsMaxAge: time.Hour,
		LogLevel:      "info",
		PidFile:       filepath.Join(tmpDir, "test-daemon.pid"),
		EnableHTTP:    false,
	}

	daemon, err := NewDaemon(config)
	require.NoError(t, err)

	err = daemon.Start()
	require.NoError(t, err)
	defer daemon.Stop()

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	// When sending metrics to the daemon
	testMetric := createTestRunMetrics("echo test", true, 1.5, 2)
	data, err := json.Marshal(testMetric)
	require.NoError(t, err)

	conn, err := net.Dial("unix", config.SocketPath)
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write(data)
	require.NoError(t, err)

	// Close connection to ensure data is processed
	conn.Close()

	// Give daemon time to process with retry
	var recent []storage.StoredMetric
	for i := 0; i < 10; i++ {
		time.Sleep(50 * time.Millisecond)
		recent = daemon.storage.GetRecent(1)
		if len(recent) > 0 {
			break
		}
	}

	// Then the metrics should be stored
	require.Len(t, recent, 1)
	assert.Equal(t, testMetric.Command, recent[0].Metrics.Command)
}

func TestDaemon_IsRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")

	// When no PID file exists
	running, pid, err := IsRunning(pidFile)
	require.NoError(t, err)
	assert.False(t, running)
	assert.Equal(t, 0, pid)

	// When PID file exists with current process
	currentPid := os.Getpid()
	err = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", currentPid)), 0644)
	require.NoError(t, err)

	running, pid, err = IsRunning(pidFile)
	require.NoError(t, err)
	assert.True(t, running)
	assert.Equal(t, currentPid, pid)

	// When PID file exists with non-existent process
	err = os.WriteFile(pidFile, []byte("99999\n"), 0644)
	require.NoError(t, err)

	running, pid, err = IsRunning(pidFile)
	require.NoError(t, err)
	assert.False(t, running)
	assert.Equal(t, 99999, pid)
}

func TestDaemon_GetStats(t *testing.T) {
	// Given a daemon with some metrics
	daemon, err := NewDaemon(nil)
	require.NoError(t, err)

	testMetric := createTestRunMetrics("echo test", true, 1.0, 1)
	daemon.storage.Store(testMetric)

	// When getting daemon stats
	stats := daemon.GetStats()

	// Then it should include storage and daemon info
	require.NotNil(t, stats)
	assert.Contains(t, stats, "total_metrics")
	assert.Contains(t, stats, "daemon_config")
	assert.Equal(t, 1, stats["total_metrics"])
}

func TestDaemon_ConcurrentConnections(t *testing.T) {
	// Given a running daemon
	tmpDir := t.TempDir()
	socketPath := "/tmp/test-daemon-concurrent.sock"
	// Clean up socket if it exists
	os.Remove(socketPath)
	defer os.Remove(socketPath)

	config := &Config{
		SocketPath:    socketPath,
		HTTPPort:      0,
		MaxMetrics:    1000,
		MetricsMaxAge: time.Hour,
		LogLevel:      "info",
		PidFile:       filepath.Join(tmpDir, "test-daemon.pid"),
		EnableHTTP:    false,
	}

	daemon, err := NewDaemon(config)
	require.NoError(t, err)

	err = daemon.Start()
	require.NoError(t, err)
	defer daemon.Stop()

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	// When sending multiple concurrent connections
	numConnections := 10
	done := make(chan bool, numConnections)

	for i := 0; i < numConnections; i++ {
		go func(id int) {
			defer func() { done <- true }()

			testMetric := createTestRunMetrics(fmt.Sprintf("echo test%d", id), true, 1.0, 1)
			data, err := json.Marshal(testMetric)
			if err != nil {
				return
			}

			conn, err := net.Dial("unix", config.SocketPath)
			if err != nil {
				return
			}
			defer conn.Close()

			conn.Write(data)
		}(i)
	}

	// Wait for all connections to complete
	for i := 0; i < numConnections; i++ {
		<-done
	}

	// Give daemon time to process all metrics
	time.Sleep(200 * time.Millisecond)

	// Then all metrics should be stored
	recent := daemon.storage.GetRecent(numConnections)
	assert.Equal(t, numConnections, len(recent))
}

func TestDaemon_GracefulShutdown(t *testing.T) {
	// Given a running daemon
	tmpDir := t.TempDir()
	socketPath := "/tmp/test-daemon-shutdown.sock"
	// Clean up socket if it exists
	os.Remove(socketPath)
	defer os.Remove(socketPath)

	config := &Config{
		SocketPath:    socketPath,
		HTTPPort:      0,
		MaxMetrics:    100,
		MetricsMaxAge: time.Hour,
		LogLevel:      "info",
		PidFile:       filepath.Join(tmpDir, "test-daemon.pid"),
		EnableHTTP:    false,
	}

	daemon, err := NewDaemon(config)
	require.NoError(t, err)

	err = daemon.Start()
	require.NoError(t, err)

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	// Verify socket was created
	_, err = os.Stat(config.SocketPath)
	require.NoError(t, err, "Socket should be created after daemon start")

	// When triggering graceful shutdown
	shutdownDone := make(chan bool)
	go func() {
		daemon.Wait()
		shutdownDone <- true
	}()

	// Trigger shutdown by calling Stop (which also cancels context)
	go func() {
		time.Sleep(50 * time.Millisecond) // Give Wait() time to start
		daemon.Stop()
	}()

	// Then shutdown should complete within reasonable time
	select {
	case <-shutdownDone:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Daemon shutdown timed out")
	}

	// And cleanup should be complete
	// The socket should be removed after shutdown
	_, statErr := os.Stat(config.SocketPath)
	if statErr == nil {
		// If socket still exists, wait a bit and check again
		time.Sleep(100 * time.Millisecond)
		_, statErr = os.Stat(config.SocketPath)
	}
	assert.True(t, os.IsNotExist(statErr), "Socket file should be cleaned up after shutdown")
}

// createTestRunMetrics creates a test RunMetrics instance
func createTestRunMetrics(command string, success bool, durationSeconds float64, attemptCount int) *metrics.RunMetrics {
	attempts := make([]metrics.AttemptMetric, attemptCount)
	for i := 0; i < attemptCount; i++ {
		attempts[i] = metrics.AttemptMetric{
			Duration: time.Duration(durationSeconds/float64(attemptCount)) * time.Second,
			ExitCode: 0,
			Success:  i == attemptCount-1 && success, // Only last attempt succeeds
		}
	}

	finalStatus := "failed"
	if success {
		finalStatus = "succeeded"
	}

	return &metrics.RunMetrics{
		Command:              command,
		CommandHash:          "test-hash",
		FinalStatus:          finalStatus,
		TotalDurationSeconds: durationSeconds,
		TotalAttempts:        attemptCount,
		SuccessfulAttempts: func() int {
			if success {
				return 1
			} else {
				return 0
			}
		}(),
		FailedAttempts: func() int {
			if success {
				return attemptCount - 1
			} else {
				return attemptCount
			}
		}(),
		Attempts:  attempts,
		Timestamp: time.Now().Unix(),
	}
}
