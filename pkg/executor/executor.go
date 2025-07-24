package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/user/retry/pkg/backoff"
	"github.com/user/retry/pkg/conditions"
)

// CommandOutput holds the output from a command execution
type CommandOutput struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CommandRunner defines the interface for executing commands
type CommandRunner interface {
	Run(command []string) (int, error)
	RunWithContext(ctx context.Context, command []string) (int, error)
	RunWithOutput(command []string) (CommandOutput, error)
	RunWithOutputAndContext(ctx context.Context, command []string) (CommandOutput, error)
}

// SystemCommandRunner implements CommandRunner using os/exec
type SystemCommandRunner struct{}

// Run executes a command using os/exec and returns the exit code
func (r *SystemCommandRunner) Run(command []string) (int, error) {
	return r.RunWithContext(context.Background(), command)
}

// RunWithContext executes a command with context support for timeouts
func (r *SystemCommandRunner) RunWithContext(ctx context.Context, command []string) (int, error) {
	output, err := r.RunWithOutputAndContext(ctx, command)
	return output.ExitCode, err
}

// RunWithOutput executes a command and captures its output
func (r *SystemCommandRunner) RunWithOutput(command []string) (CommandOutput, error) {
	return r.RunWithOutputAndContext(context.Background(), command)
}

// RunWithOutputAndContext executes a command with context and captures output
func (r *SystemCommandRunner) RunWithOutputAndContext(ctx context.Context, command []string) (CommandOutput, error) {
	if len(command) == 0 {
		return CommandOutput{ExitCode: -1}, nil
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)

	// Capture stdout and stderr while also forwarding to terminal
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	err := cmd.Run()

	output := CommandOutput{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if err != nil {
		// Check for context deadline exceeded (timeout)
		if ctx.Err() == context.DeadlineExceeded {
			output.ExitCode = -1
			return output, context.DeadlineExceeded
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			output.ExitCode = exitError.ExitCode()
			return output, nil
		}
		output.ExitCode = -1
		return output, err
	}

	output.ExitCode = 0
	return output, nil
}

// Executor handles command execution with retry logic
type Executor struct {
	MaxAttempts     int
	Runner          CommandRunner
	BackoffStrategy backoff.Strategy
	Timeout         time.Duration
	Conditions      *conditions.Checker
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
	Reason       string
}

// executeAttempt runs a single command attempt and returns the output, error, and timeout status
func (e *Executor) executeAttempt(command []string) (CommandOutput, error, bool) {
	if e.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), e.Timeout)
		defer cancel()

		output, err := e.Runner.RunWithOutputAndContext(ctx, command)
		if err == context.DeadlineExceeded {
			return CommandOutput{ExitCode: -1}, nil, true // Timeout occurred
		}
		return output, err, false
	}

	// No timeout configured, use regular run
	output, err := e.Runner.RunWithOutput(command)
	return output, err, false
}

// Run executes the given command with retry logic and returns the result
func (e *Executor) Run(command []string) (*Result, error) {
	var lastOutput CommandOutput
	var lastError error
	var timedOut bool

	// Retry loop
	for attempt := 1; attempt <= e.MaxAttempts; attempt++ {
		output, err, timeout := e.executeAttempt(command)
		lastOutput = output
		lastError = err
		if timeout {
			timedOut = true
		}

		if err != nil {
			return nil, err
		}

		// Check success conditions (patterns first, then exit code)
		var conditionResult conditions.Result
		if e.Conditions != nil {
			conditionResult = e.Conditions.CheckSuccess(output.ExitCode, output.Stdout, output.Stderr)
		} else {
			// Default behavior: success if exit code is 0
			if output.ExitCode == 0 {
				conditionResult = conditions.Result{
					Success: true,
					Reason:  "exit code 0",
				}
			} else {
				conditionResult = conditions.Result{
					Success: false,
					Reason:  fmt.Sprintf("exit code %d", output.ExitCode),
				}
			}
		}

		// If command succeeded, return immediately
		if conditionResult.Success {
			return &Result{
				AttemptCount: attempt,
				ExitCode:     output.ExitCode,
				Success:      true,
				TimedOut:     false,
				Reason:       conditionResult.Reason,
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
	var finalReason string
	if e.Conditions != nil {
		conditionResult := e.Conditions.CheckSuccess(lastOutput.ExitCode, lastOutput.Stdout, lastOutput.Stderr)
		finalReason = conditionResult.Reason
	} else {
		if lastOutput.ExitCode == 0 {
			finalReason = "exit code 0"
		} else {
			finalReason = fmt.Sprintf("exit code %d", lastOutput.ExitCode)
		}
	}

	return &Result{
		AttemptCount: e.MaxAttempts,
		ExitCode:     lastOutput.ExitCode,
		Success:      false,
		TimedOut:     timedOut,
		Reason:       finalReason,
	}, lastError
}
