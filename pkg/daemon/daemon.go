package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/shaneisley/patience/pkg/metrics"
	"github.com/shaneisley/patience/pkg/storage"
)

// Daemon represents the retry metrics daemon
type Daemon struct {
	config   *Config
	storage  *storage.MetricsStorage
	listener net.Listener
	server   *Server
	logger   *log.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// Config holds daemon configuration
type Config struct {
	SocketPath      string        `json:"socket_path"`
	HTTPPort        int           `json:"http_port"`
	MaxMetrics      int           `json:"max_metrics"`
	MetricsMaxAge   time.Duration `json:"metrics_max_age"`
	LogLevel        string        `json:"log_level"`
	PidFile         string        `json:"pid_file"`
	EnableHTTP      bool          `json:"enable_http"`
	EnableProfiling bool          `json:"enable_profiling"`
}

// DefaultConfig returns a default daemon configuration
func DefaultConfig() *Config {
	return &Config{
		SocketPath:      "/tmp/retry-daemon.sock",
		HTTPPort:        8080,
		MaxMetrics:      10000,
		MetricsMaxAge:   24 * time.Hour,
		LogLevel:        "info",
		PidFile:         "/tmp/retry-daemon.pid",
		EnableHTTP:      true,
		EnableProfiling: false,
	}
}

// NewDaemon creates a new daemon instance
func NewDaemon(config *Config) (*Daemon, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger
	logger := log.New(os.Stdout, "[retryd] ", log.LstdFlags)

	// Create metrics storage
	metricsStorage := storage.NewMetricsStorage(config.MaxMetrics, config.MetricsMaxAge)

	daemon := &Daemon{
		config:  config,
		storage: metricsStorage,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}

	return daemon, nil
}

// Start starts the daemon
func (d *Daemon) Start() error {
	d.logger.Printf("Starting retry daemon on socket %s", d.config.SocketPath)

	// Write PID file
	if err := d.writePidFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Clean up old socket file
	if err := d.cleanupSocket(); err != nil {
		return fmt.Errorf("failed to cleanup socket: %w", err)
	}

	// Create Unix domain socket listener
	listener, err := net.Listen("unix", d.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	d.listener = listener

	// Set socket permissions
	if err := os.Chmod(d.config.SocketPath, 0666); err != nil {
		d.logger.Printf("Warning: failed to set socket permissions: %v", err)
	}

	// Start HTTP server if enabled
	if d.config.EnableHTTP {
		d.server = NewServer(d.storage, d.config.HTTPPort, d.logger)
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			if err := d.server.Start(d.ctx); err != nil {
				d.logger.Printf("HTTP server error: %v", err)
			}
		}()
	}

	// Start socket server
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.handleConnections()
	}()

	// Setup signal handling
	d.setupSignalHandling()

	d.logger.Printf("Daemon started successfully")
	return nil
}

// Stop stops the daemon gracefully
func (d *Daemon) Stop() error {
	d.logger.Printf("Stopping daemon...")

	// Cancel context to signal shutdown
	d.cancel()

	// Close listener
	if d.listener != nil {
		d.listener.Close()
	}

	// Stop HTTP server
	if d.server != nil {
		d.server.Stop()
	}

	// Wait for all goroutines to finish
	d.wg.Wait()

	// Cleanup
	d.cleanupSocket()
	d.removePidFile()

	d.logger.Printf("Daemon stopped")
	return nil
}

// Wait waits for the daemon to finish
func (d *Daemon) Wait() {
	d.wg.Wait()
}

// handleConnections handles incoming socket connections
func (d *Daemon) handleConnections() {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
			// Set a timeout for Accept to allow checking context
			if tcpListener, ok := d.listener.(*net.UnixListener); ok {
				tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
			}

			conn, err := d.listener.Accept()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue // Timeout, check context and try again
				}
				if d.ctx.Err() != nil {
					return // Context cancelled
				}
				d.logger.Printf("Error accepting connection: %v", err)
				continue
			}

			// Handle connection in goroutine
			d.wg.Add(1)
			go func() {
				defer d.wg.Done()
				d.handleConnection(conn)
			}()
		}
	}
}

// handleConnection handles a single client connection
func (d *Daemon) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set read timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read metrics data
	data, err := io.ReadAll(conn)
	if err != nil {
		d.logger.Printf("Error reading from connection: %v", err)
		return
	}

	// Parse metrics
	var runMetrics metrics.RunMetrics
	if err := json.Unmarshal(data, &runMetrics); err != nil {
		d.logger.Printf("Error parsing metrics: %v", err)
		return
	}

	// Store metrics
	if err := d.storage.Store(&runMetrics); err != nil {
		d.logger.Printf("Error storing metrics: %v", err)
		return
	}

	d.logger.Printf("Stored metrics for command: %s", runMetrics.Command)
}

// setupSignalHandling sets up graceful shutdown on signals
func (d *Daemon) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		select {
		case sig := <-sigChan:
			d.logger.Printf("Received signal %v, shutting down...", sig)
			d.cancel()
		case <-d.ctx.Done():
			return
		}
	}()
}

// writePidFile writes the process ID to a file
func (d *Daemon) writePidFile() error {
	if d.config.PidFile == "" {
		return nil
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(d.config.PidFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write PID
	pid := os.Getpid()
	return os.WriteFile(d.config.PidFile, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

// removePidFile removes the PID file
func (d *Daemon) removePidFile() {
	if d.config.PidFile != "" {
		os.Remove(d.config.PidFile)
	}
}

// cleanupSocket removes the socket file if it exists
func (d *Daemon) cleanupSocket() error {
	if _, err := os.Stat(d.config.SocketPath); err == nil {
		return os.Remove(d.config.SocketPath)
	}
	return nil
}

// GetStats returns daemon statistics
func (d *Daemon) GetStats() map[string]interface{} {
	stats := d.storage.GetStats()
	stats["daemon_config"] = d.config
	stats["uptime"] = time.Since(time.Now()) // This would be tracked properly in a real implementation
	return stats
}

// IsRunning checks if the daemon is running by checking the PID file
func IsRunning(pidFile string) (bool, int, error) {
	if pidFile == "" {
		return false, 0, nil
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return false, 0, fmt.Errorf("invalid PID file format: %w", err)
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid, nil
	}

	// Try to send signal 0 to check if process is alive
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false, pid, nil
	}

	return true, pid, nil
}
