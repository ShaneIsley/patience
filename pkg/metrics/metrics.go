package metrics

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

// AttemptMetric represents metrics for a single command attempt
type AttemptMetric struct {
	Duration time.Duration `json:"-"`
	ExitCode int           `json:"exit_code"`
	Success  bool          `json:"success"`
}

// DurationSeconds returns the duration in seconds as a float64
func (a *AttemptMetric) DurationSeconds() float64 {
	return float64(a.Duration) / float64(time.Second)
}

// MarshalJSON implements custom JSON marshaling for AttemptMetric
func (a *AttemptMetric) MarshalJSON() ([]byte, error) {
	type Alias AttemptMetric
	return json.Marshal(&struct {
		DurationSeconds float64 `json:"duration_seconds"`
		*Alias
	}{
		DurationSeconds: a.DurationSeconds(),
		Alias:           (*Alias)(a),
	})
}

// RunMetrics represents metrics for a complete retry run
type RunMetrics struct {
	Command              string          `json:"command"`
	CommandHash          string          `json:"command_hash"`
	FinalStatus          string          `json:"final_status"` // "succeeded" or "failed"
	TotalDurationSeconds float64         `json:"total_duration_seconds"`
	TotalAttempts        int             `json:"total_attempts"`
	SuccessfulAttempts   int             `json:"successful_attempts"`
	FailedAttempts       int             `json:"failed_attempts"`
	Attempts             []AttemptMetric `json:"attempts"`
	Timestamp            int64           `json:"timestamp"` // Unix timestamp
}

// NewRunMetrics creates a new RunMetrics instance
func NewRunMetrics(command []string, success bool, totalDuration time.Duration, attempts []AttemptMetric) *RunMetrics {
	commandStr := strings.Join(command, " ")

	var finalStatus string
	if success {
		finalStatus = "succeeded"
	} else {
		finalStatus = "failed"
	}

	successfulAttempts := 0
	failedAttempts := 0
	for _, attempt := range attempts {
		if attempt.Success {
			successfulAttempts++
		} else {
			failedAttempts++
		}
	}

	return &RunMetrics{
		Command:              commandStr,
		CommandHash:          generateCommandHash(command),
		FinalStatus:          finalStatus,
		TotalDurationSeconds: float64(totalDuration) / float64(time.Second),
		TotalAttempts:        len(attempts),
		SuccessfulAttempts:   successfulAttempts,
		FailedAttempts:       failedAttempts,
		Attempts:             attempts,
		Timestamp:            time.Now().Unix(),
	}
}

// generateCommandHash creates a consistent hash for a command
func generateCommandHash(command []string) string {
	commandStr := strings.Join(command, " ")
	hash := sha256.Sum256([]byte(commandStr))
	// Return first 8 characters of hex representation
	return fmt.Sprintf("%x", hash)[:8]
}

// Client handles communication with the retryd daemon
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient creates a new metrics client
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		timeout:    100 * time.Millisecond, // Short timeout for non-blocking behavior
	}
}

// DefaultSocketPath returns the default Unix socket path for retryd
func DefaultSocketPath() string {
	return "/tmp/retryd.sock"
}

// SendMetrics sends metrics to the daemon synchronously
func (c *Client) SendMetrics(metrics *RunMetrics) error {
	// Serialize metrics to JSON
	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	// Connect to Unix socket with timeout
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer conn.Close()

	// Set write timeout
	conn.SetWriteDeadline(time.Now().Add(c.timeout))

	// Send data
	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to send metrics: %w", err)
	}

	return nil
}

// SendMetricsAsync sends metrics to the daemon asynchronously (fire-and-forget)
func (c *Client) SendMetricsAsync(metrics *RunMetrics) {
	go func() {
		// Ignore errors in async mode - this is fire-and-forget
		_ = c.SendMetrics(metrics)
	}()
}
