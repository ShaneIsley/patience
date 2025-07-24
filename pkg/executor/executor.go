package executor

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/user/retry/pkg/backoff"
)

// CommandRunner defines the interface for executing commands
type CommandRunner interface {
	Run(command []string) (int, error)
	RunWithContext(ctx context.Context, command []string) (int, error)
}

// SystemCommandRunner implements CommandRunner using os/exec
type SystemCommandRunner struct{}

// Run executes a command using os/exec and returns the exit code
func (r *SystemCommandRunner) Run(command []string) (int, error) {
	return r.RunWithContext(context.Background(), command)
}

// RunWithContext executes a command with context support for timeouts
func (r *SystemCommandRunner) RunWithContext(ctx context.Context, command []string) (int, error) {
	if len(command) == 0 {
		return -1, nil
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	// Forward stdout and stderr to maintain CLI behavior
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	if err != nil {
		// Check for context deadline exceeded (timeout)
		if ctx.Err() == context.DeadlineExceeded {
			return -1, context.DeadlineExceeded
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode(), nil
		}
		return -1, err
	}

	return 0, nil
}

// Executor handles command execution with retry logic
type Executor struct {
	MaxAttempts     int
	Runner          CommandRunner
	BackoffStrategy backoff.Strategy
	Timeout         time.Duration
}

// NewExecutor creates a new Executor with default SystemCommandRunner and no backoff
func NewExecutor(maxAttempts int) *Executor {
	return &Executor{
		MaxAttempts:     maxAttempts,
		Runner:          &SystemCommandRunner{},
		BackoffStrategy: nil, // No delay by default
		Timeout:         0,   // No timeout by default
	}
}

// NewExecutorWithBackoff creates a new Executor with specified backoff strategy
func NewExecutorWithBackoff(maxAttempts int, strategy backoff.Strategy) *Executor {
	return &Executor{
		MaxAttempts:     maxAttempts,
		Runner:          &SystemCommandRunner{},
		BackoffStrategy: strategy,
		Timeout:         0, // No timeout by default
	}
}

// NewExecutorWithTimeout creates a new Executor with specified timeout
func NewExecutorWithTimeout(maxAttempts int, timeout time.Duration) *Executor {
	return &Executor{
		MaxAttempts:     maxAttempts,
		Runner:          &SystemCommandRunner{},
		BackoffStrategy: nil,
		Timeout:         timeout,
	}
}

// NewExecutorWithBackoffAndTimeout creates a new Executor with backoff and timeout
func NewExecutorWithBackoffAndTimeout(maxAttempts int, strategy backoff.Strategy, timeout time.Duration) *Executor {
	return &Executor{
		MaxAttempts:     maxAttempts,
		Runner:          &SystemCommandRunner{},
		BackoffStrategy: strategy,
		Timeout:         timeout,
	}
}

// Result represents the outcome of a command execution
type Result struct {
	Success      bool
	AttemptCount int
	ExitCode     int
	TimedOut     bool
}

// executeAttempt runs a single command attempt and returns the exit code, error, and timeout status
func (e *Executor) executeAttempt(command []string) (int, error, bool) {
	if e.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), e.Timeout)
		defer cancel()

		exitCode, err := e.Runner.RunWithContext(ctx, command)
		if err == context.DeadlineExceeded {
			return -1, nil, true // Timeout occurred
		}
		return exitCode, err, false
	}

	// No timeout configured, use regular run
	exitCode, err := e.Runner.Run(command)
	return exitCode, err, false
}

// Run executes the given command with retry logic and returns the result
func (e *Executor) Run(command []string) (*Result, error) {
	var lastExitCode int
	var lastError error
	var timedOut bool

	// Retry loop
	for attempt := 1; attempt <= e.MaxAttempts; attempt++ {
		exitCode, err, timeout := e.executeAttempt(command)
		lastExitCode = exitCode
		lastError = err
		if timeout {
			timedOut = true
		}

		if err != nil {
			return nil, err
		}

		// If command succeeded, return immediately
		if exitCode == 0 {
			return &Result{
				AttemptCount: attempt,
				ExitCode:     exitCode,
				Success:      true,
				TimedOut:     false,
			}, nil
		}

		// If this was the last attempt, break out of loop
		if attempt == e.MaxAttempts {
			break
		}

		// Wait before next attempt if backoff strategy is configured
		if e.BackoffStrategy != nil {
			delay := e.BackoffStrategy.Delay(attempt)
			time.Sleep(delay)
		}
	}

	// All attempts failed
	return &Result{
		AttemptCount: e.MaxAttempts,
		ExitCode:     lastExitCode,
		Success:      false,
		TimedOut:     timedOut,
	}, lastError
}
