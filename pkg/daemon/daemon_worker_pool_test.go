package daemon

import (
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/metrics"
)

func TestDaemon_WorkerPoolIntegration(t *testing.T) {
	// Create daemon with small worker pool for testing
	config := DefaultConfig()
	config.SocketPath = "/tmp/test-daemon-worker-pool.sock"
	config.MaxConnections = 3
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

	// Test concurrent connections with worker pool
	numConnections := 10
	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("unix", config.SocketPath)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			defer conn.Close()

			// Create test metrics
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

			data, err := json.Marshal(testMetrics)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}

			// Send data
			_, err = conn.Write(data)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}

			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	wg.Wait()

	// Verify that connections were handled
	if successCount == 0 {
		t.Error("Expected at least some connections to succeed")
	}

	t.Logf("Successful connections: %d, Errors: %d", successCount, errorCount)

	// Check daemon stats include worker pool info
	stats := daemon.GetStats()
	workerPoolStats, ok := stats["worker_pool"].(map[string]interface{})
	if !ok {
		t.Error("Expected worker pool stats in daemon stats")
	} else {
		t.Logf("Worker pool stats: %+v", workerPoolStats)

		if workers, ok := workerPoolStats["workers"].(int); !ok || workers != 3 {
			t.Errorf("Expected 3 workers, got %v", workers)
		}
	}
}

func TestDaemon_WorkerPoolStressTest(t *testing.T) {
	// Create daemon with very small worker pool for stress testing
	config := DefaultConfig()
	config.SocketPath = "/tmp/test-daemon-stress.sock"
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

	// Stress test with many rapid connections
	numConnections := 50
	var wg sync.WaitGroup
	var processedCount int64

	start := time.Now()

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("unix", config.SocketPath)
			if err != nil {
				return // Connection rejected, which is expected under stress
			}
			defer conn.Close()

			// Create minimal test metrics
			testMetrics := metrics.RunMetrics{
				Command:              "stress-test",
				CommandHash:          "stress",
				FinalStatus:          "succeeded",
				TotalDurationSeconds: 0.1,
				TotalAttempts:        1,
				SuccessfulAttempts:   1,
				FailedAttempts:       0,
				Attempts:             []metrics.AttemptMetric{{Duration: 100 * time.Millisecond, ExitCode: 0, Success: true}},
				Timestamp:            time.Now().Unix(),
			}

			data, _ := json.Marshal(testMetrics)
			conn.Write(data)

			atomic.AddInt64(&processedCount, 1)
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// Verify that some connections were processed
	if processedCount == 0 {
		t.Error("Expected at least some connections to be processed")
	}

	t.Logf("Processed %d/%d connections in %v", processedCount, numConnections, duration)

	// Verify daemon is still responsive after stress test
	conn, err := net.Dial("unix", config.SocketPath)
	if err != nil {
		t.Errorf("Daemon not responsive after stress test: %v", err)
	} else {
		conn.Close()
	}
}

func TestDaemon_WorkerPoolGracefulShutdown(t *testing.T) {
	config := DefaultConfig()
	config.SocketPath = "/tmp/test-daemon-graceful.sock"
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
	numConnections := 3

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := net.Dial("unix", config.SocketPath)
			if err != nil {
				return
			}
			defer conn.Close()

			// Send data and then wait a bit to simulate processing
			testMetrics := metrics.RunMetrics{
				Command:              "graceful-test",
				CommandHash:          "graceful",
				FinalStatus:          "succeeded",
				TotalDurationSeconds: 0.5,
				TotalAttempts:        1,
				SuccessfulAttempts:   1,
				FailedAttempts:       0,
				Attempts:             []metrics.AttemptMetric{{Duration: 500 * time.Millisecond, ExitCode: 0, Success: true}},
				Timestamp:            time.Now().Unix(),
			}

			data, _ := json.Marshal(testMetrics)
			conn.Write(data)

			// Simulate some processing time
			time.Sleep(200 * time.Millisecond)
		}()
	}

	// Give connections time to establish
	time.Sleep(50 * time.Millisecond)

	// Shutdown daemon while connections are active
	shutdownStart := time.Now()
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
	t.Logf("Graceful shutdown completed in %v", shutdownDuration)
}
