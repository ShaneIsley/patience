package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"sync"
	"time"

	"github.com/shaneisley/patience/pkg/metrics"
	"github.com/shaneisley/patience/pkg/storage"
)

// WorkerPool manages a fixed-size pool of workers for handling connections
type WorkerPool struct {
	workers  int
	jobQueue chan net.Conn
	workerWg sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	storage  *storage.MetricsStorage
	logger   *Logger
	started  bool
	mu       sync.RWMutex
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workers int, storage *storage.MetricsStorage, logger *Logger) *WorkerPool {
	if workers <= 0 {
		workers = 10 // Default worker count
	}
	if workers > 1000 {
		workers = 1000 // Reasonable upper limit
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		workers:  workers,
		jobQueue: make(chan net.Conn, workers*2), // Buffer for queuing connections
		ctx:      ctx,
		cancel:   cancel,
		storage:  storage,
		logger:   logger,
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.started {
		return
	}

	wp.started = true

	// Start worker goroutines
	for i := 0; i < wp.workers; i++ {
		wp.workerWg.Add(1)
		go wp.worker(i)
	}

	wp.logger.Info("worker pool started", "workers", wp.workers, "queue_size", cap(wp.jobQueue))
}

// Stop stops the worker pool gracefully
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	if !wp.started {
		wp.mu.Unlock()
		return
	}
	wp.mu.Unlock()

	wp.logger.Info("stopping worker pool")

	// Cancel context to signal workers to stop
	wp.cancel()

	// Close job queue to prevent new jobs
	close(wp.jobQueue)

	// Wait for all workers to finish
	wp.workerWg.Wait()

	wp.logger.Info("worker pool stopped")
}

// SubmitConnection submits a connection to be handled by the worker pool
// Returns true if the connection was queued, false if the pool is full
func (wp *WorkerPool) SubmitConnection(conn net.Conn) bool {
	wp.mu.RLock()
	if !wp.started {
		wp.mu.RUnlock()
		conn.Close()
		return false
	}
	wp.mu.RUnlock()

	select {
	case wp.jobQueue <- conn:
		return true
	case <-wp.ctx.Done():
		conn.Close()
		return false
	default:
		// Queue is full, reject connection
		wp.logger.Warn("worker pool queue full, rejecting connection",
			"queue_size", cap(wp.jobQueue), "workers", wp.workers)
		conn.Close()
		return false
	}
}

// worker is the main worker loop
func (wp *WorkerPool) worker(id int) {
	defer wp.workerWg.Done()

	wp.logger.Debug("worker started", "worker_id", id)
	defer wp.logger.Debug("worker stopped", "worker_id", id)

	for {
		select {
		case conn, ok := <-wp.jobQueue:
			if !ok {
				// Channel closed, worker should exit
				return
			}
			wp.handleConnection(conn, id)

		case <-wp.ctx.Done():
			// Context cancelled, worker should exit
			return
		}
	}
}

// handleConnection handles a single connection
func (wp *WorkerPool) handleConnection(conn net.Conn, workerID int) {
	defer conn.Close()

	// Set read timeout to prevent hanging connections
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read metrics data
	data, err := io.ReadAll(conn)
	if err != nil {
		if err != io.EOF && !isTimeoutError(err) {
			wp.logger.Error("error reading from connection",
				"error", err, "worker_id", workerID)
		}
		return
	}

	// Skip empty data (connection closed without sending data)
	if len(data) == 0 {
		return
	}

	// Parse metrics
	var runMetrics metrics.RunMetrics
	if err := json.Unmarshal(data, &runMetrics); err != nil {
		wp.logger.Error("error parsing metrics",
			"error", err, "worker_id", workerID, "data_length", len(data))
		return
	}

	// Store metrics
	if err := wp.storage.Store(&runMetrics); err != nil {
		wp.logger.Error("error storing metrics",
			"error", err, "worker_id", workerID, "command", runMetrics.Command)
		return
	}

	wp.logger.Debug("stored metrics for command",
		"command", runMetrics.Command, "worker_id", workerID)
}

// GetStats returns worker pool statistics
func (wp *WorkerPool) GetStats() map[string]interface{} {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	return map[string]interface{}{
		"workers":        wp.workers,
		"queue_capacity": cap(wp.jobQueue),
		"queue_length":   len(wp.jobQueue),
		"started":        wp.started,
	}
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}
