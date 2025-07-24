package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/user/retry/pkg/daemon"
)

var (
	configFile  = flag.String("config", "", "Configuration file path")
	socketPath  = flag.String("socket", "/tmp/retry-daemon.sock", "Unix socket path")
	httpPort    = flag.Int("port", 8080, "HTTP server port")
	maxMetrics  = flag.Int("max-metrics", 10000, "Maximum number of metrics to store")
	maxAge      = flag.Duration("max-age", 24*time.Hour, "Maximum age of metrics")
	pidFile     = flag.String("pid-file", "/tmp/retry-daemon.pid", "PID file path")
	logLevel    = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	enableHTTP  = flag.Bool("enable-http", true, "Enable HTTP API server")
	enableProf  = flag.Bool("enable-profiling", false, "Enable profiling endpoints")
	daemonize   = flag.Bool("daemon", false, "Run as daemon (background process)")
	showVersion = flag.Bool("version", false, "Show version information")
	showStatus  = flag.Bool("status", false, "Show daemon status")
	stopDaemon  = flag.Bool("stop", false, "Stop running daemon")
)

const version = "1.0.0"

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("retryd version %s\n", version)
		os.Exit(0)
	}

	if *showStatus {
		showDaemonStatus()
		os.Exit(0)
	}

	if *stopDaemon {
		stopRunningDaemon()
		os.Exit(0)
	}

	// Load configuration
	config, err := loadConfiguration()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Check if daemon is already running
	if running, pid, _ := daemon.IsRunning(config.PidFile); running {
		fmt.Fprintf(os.Stderr, "Daemon is already running with PID %d\n", pid)
		os.Exit(1)
	}

	// Create daemon
	d, err := daemon.NewDaemon(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating daemon: %v\n", err)
		os.Exit(1)
	}

	// Daemonize if requested
	if *daemonize {
		if err := daemonizeProcess(); err != nil {
			fmt.Fprintf(os.Stderr, "Error daemonizing: %v\n", err)
			os.Exit(1)
		}
	}

	// Start daemon
	if err := d.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
		os.Exit(1)
	}

	// Wait for daemon to finish
	d.Wait()
}

// loadConfiguration loads daemon configuration from file or flags
func loadConfiguration() (*daemon.Config, error) {
	config := daemon.DefaultConfig()

	// Load from config file if specified
	if *configFile != "" {
		if err := loadConfigFromFile(*configFile, config); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Override with command line flags (always apply if config file wasn't loaded)
	if *configFile == "" || *socketPath != "/tmp/retry-daemon.sock" {
		config.SocketPath = *socketPath
	}
	if *configFile == "" || *httpPort != 8080 {
		config.HTTPPort = *httpPort
	}
	if *configFile == "" || *maxMetrics != 10000 {
		config.MaxMetrics = *maxMetrics
	}
	if *configFile == "" || *maxAge != 24*time.Hour {
		config.MetricsMaxAge = *maxAge
	}
	if *configFile == "" || *pidFile != "/tmp/retry-daemon.pid" {
		config.PidFile = *pidFile
	}
	if *configFile == "" || *logLevel != "info" {
		config.LogLevel = *logLevel
	}
	if *configFile == "" || *enableHTTP != true {
		config.EnableHTTP = *enableHTTP
	}
	if *configFile == "" || *enableProf != false {
		config.EnableProfiling = *enableProf
	}

	return config, nil
}

// loadConfigFromFile loads configuration from a JSON file
func loadConfigFromFile(filename string, config *daemon.Config) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, config)
}

// showDaemonStatus shows the current daemon status
func showDaemonStatus() {
	config := daemon.DefaultConfig()
	if *pidFile != "" {
		config.PidFile = *pidFile
	}

	running, pid, err := daemon.IsRunning(config.PidFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking daemon status: %v\n", err)
		return
	}

	if running {
		fmt.Printf("Daemon is running with PID %d\n", pid)

		// Try to get stats from HTTP API
		// This would require implementing a client, for now just show basic info
		fmt.Printf("Socket: %s\n", config.SocketPath)
		fmt.Printf("HTTP Port: %d\n", config.HTTPPort)
	} else {
		fmt.Println("Daemon is not running")
		if pid != 0 {
			fmt.Printf("Stale PID file found with PID %d\n", pid)
		}
	}
}

// stopRunningDaemon stops a running daemon
func stopRunningDaemon() {
	config := daemon.DefaultConfig()
	if *pidFile != "" {
		config.PidFile = *pidFile
	}

	running, pid, err := daemon.IsRunning(config.PidFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking daemon status: %v\n", err)
		return
	}

	if !running {
		fmt.Println("Daemon is not running")
		return
	}

	// Send SIGTERM to the process
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding process %d: %v\n", pid, err)
		return
	}

	if err := process.Signal(os.Interrupt); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping daemon: %v\n", err)
		return
	}

	fmt.Printf("Sent stop signal to daemon (PID %d)\n", pid)

	// Wait a bit and check if it stopped
	time.Sleep(2 * time.Second)
	if running, _, _ := daemon.IsRunning(config.PidFile); !running {
		fmt.Println("Daemon stopped successfully")
	} else {
		fmt.Println("Daemon may still be running, check status")
	}
}

// daemonizeProcess forks the process to run in background
func daemonizeProcess() error {
	// This is a simplified daemonization
	// In a production system, you'd want to use a proper daemonization library
	// or system service manager like systemd

	// For now, just print a message
	fmt.Println("Daemonization requested - in production this would fork to background")
	fmt.Println("Consider using systemd or similar service manager instead")

	return nil
}

// createExampleConfig creates an example configuration file
func createExampleConfig(filename string) error {
	config := daemon.DefaultConfig()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// init sets up flag usage information
func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "retryd - Retry metrics collection daemon\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s                          # Start daemon with defaults\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -daemon                  # Start as background daemon\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -status                  # Show daemon status\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -stop                    # Stop running daemon\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -config /etc/retryd.json # Use custom config file\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nThe daemon listens on a Unix socket for metrics from retry CLI instances\n")
		fmt.Fprintf(os.Stderr, "and provides an HTTP API for accessing aggregated statistics.\n")
	}
}
