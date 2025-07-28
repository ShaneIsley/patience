package executor

import (
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/shaneisley/patience/pkg/daemon"
)

func TestExecutor_DiophantineWithDaemon(t *testing.T) {
	// Create executor with Diophantine strategy
	strategy := backoff.NewDiophantine(5, time.Hour, []time.Duration{0, 10 * time.Minute})
	executor := NewExecutorWithBackoff(3, strategy)

	// Add daemon client to executor (using Unix socket path)
	daemonClient := daemon.NewDaemonClient("/tmp/patience-daemon.sock")
	executor.DaemonClient = daemonClient

	// Test successful execution (should work even without daemon running due to fallback)
	result, err := executor.Run([]string{"echo", "test"})
	if err != nil {
		t.Errorf("Expected successful execution, got error: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected successful result, got failure: %s", result.Reason)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}

func TestExecutor_DiophantineFallbackWithoutDaemon(t *testing.T) {
	// Create executor with Diophantine strategy but no daemon
	strategy := backoff.NewDiophantine(5, time.Hour, []time.Duration{0, 10 * time.Minute})
	executor := NewExecutorWithBackoff(3, strategy)
	// No daemon client set - should fall back to local-only mode

	// Should fall back to local-only mode
	result, err := executor.Run([]string{"echo", "fallback"})
	if err != nil {
		t.Errorf("Expected fallback execution to succeed, got error: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected successful result, got failure: %s", result.Reason)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}
