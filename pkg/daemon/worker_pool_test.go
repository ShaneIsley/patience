package daemon

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/metrics"
)

func TestDaemonWorkerPool_ConcurrentConnections(t *testing.T) {
	// Create daemon with small worker pool for testing
	config := DefaultConfig()
	config.SocketPath = "/tmp/test-worker-pool.sock"
	config.MaxConnections = 5
	config.EnableHTTP = false

	daemon, err := NewDaemon(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	// Start daemon
	if err := daemon.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	// Give daemon time to start
	time.Sleep(100 * time.Millisecond)

	// Test concurrent connections beyond worker pool size
	numConnections := 10
	var wg sync.WaitGroup
	var successCount int64
	var rejectedCount int64

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("unix", config.SocketPath)
			if err != nil {
				atomic.AddInt64(&rejectedCount, 1)
				return
			}
			defer conn.Close()

			// Send test metrics
			testMetrics := metrics.RunMetrics{
				Command:              "test-command",
				CommandHash:          "testhash",
				FinalStatus:          "succeeded",
				TotalDurationSeconds: 1.0,
				TotalAttempts:        1,
				SuccessfulAttempts:   1,
				FailedAttempts:       0,
				Attempts:             []metrics.AttemptMetric{{Duration: time.Second, ExitCode: 0, Success: true}},
				Timestamp:            time.Now().Unix(),
			}

			data, _ := json.Marshal(testMetrics)
			conn.Write(data)

			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	wg.Wait()

	// Verify that connections were handled properly
	// Some should succeed (up to MaxConnections), others may be rejected
	if successCount == 0 {
		t.Error("Expected at least some connections to succeed")
	}

	t.Logf("Successful connections: %d, Rejected: %d", successCount, rejectedCount)
}

func TestDaemonWorkerPool_ResourceExhaustion(t *testing.T) {
	// Create daemon with very small limits for testing
	config := DefaultConfig()
	config.SocketPath = "/tmp/test-resource-exhaustion.sock"
	config.MaxConnections = 2
	config.EnableHTTP = false

	daemon, err := NewDaemon(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if err := daemon.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create connections that hold for a while to test resource exhaustion
	var wg sync.WaitGroup
	var activeConnections int64

	// Create long-running connections to exhaust the pool
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := net.Dial("unix", config.SocketPath)
			if err != nil {
				return
			}
			defer conn.Close()

			atomic.AddInt64(&activeConnections, 1)

			// Hold connection for a while
			time.Sleep(200 * time.Millisecond)

			atomic.AddInt64(&activeConnections, -1)
		}()
	}

	// Give connections time to establish
	time.Sleep(50 * time.Millisecond)

	// Try to create additional connection - should be rejected quickly
	start := time.Now()
	conn, err := net.Dial("unix", config.SocketPath)
	elapsed := time.Since(start)

	if err == nil {
		conn.Close()
	}

	// Connection should either fail quickly or succeed
	// The key is that it shouldn't hang indefinitely
	if elapsed > 5*time.Second {
		t.Errorf("Connection attempt took too long: %v", elapsed)
	}

	wg.Wait()
}

func TestDaemonWorkerPool_GracefulShutdown(t *testing.T) {
	config := DefaultConfig()
	config.SocketPath = "/tmp/test-graceful-shutdown.sock"
	config.MaxConnections = 3
	config.EnableHTTP = false

	daemon, err := NewDaemon(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if err := daemon.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Create some active connections
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := net.Dial("unix", config.SocketPath)
			if err != nil {
				return
			}
			defer conn.Close()

			// Keep connection alive until context is cancelled
			<-ctx.Done()
		}()
	}

	// Give connections time to establish
	time.Sleep(50 * time.Millisecond)

	// Shutdown daemon
	shutdownStart := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel() // Release the connections
	}()

	err = daemon.Stop()
	shutdownDuration := time.Since(shutdownStart)

	if err != nil {
		t.Errorf("Daemon shutdown failed: %v", err)
	}

	// Shutdown should complete within reasonable time
	if shutdownDuration > 5*time.Second {
		t.Errorf("Daemon shutdown took too long: %v", shutdownDuration)
	}

	wg.Wait()
}

func TestDaemonWorkerPool_ConnectionTimeout(t *testing.T) {
	config := DefaultConfig()
	config.SocketPath = "/tmp/test-connection-timeout.sock"
	config.MaxConnections = 5
	config.EnableHTTP = false

	daemon, err := NewDaemon(config)
	if err != nil {
		t.Fatalf("Failed to create daemon: %v", err)
	}

	if err := daemon.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer daemon.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create connection but don't send data (should timeout)
	conn, err := net.Dial("unix", config.SocketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Set a read deadline to avoid hanging the test
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	// Try to read - connection should be closed by daemon due to timeout
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)

	// We expect either EOF (connection closed) or timeout
	if err == nil {
		t.Error("Expected connection to be closed due to timeout")
	}
}
