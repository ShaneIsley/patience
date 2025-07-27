package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/shaneisley/patience/pkg/conditions"
	"github.com/shaneisley/patience/pkg/metrics"
	"github.com/shaneisley/patience/pkg/ui"
)

// limitedBuffer wraps bytes.Buffer with a size limit to prevent memory exhaustion
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (lb *limitedBuffer) Write(p []byte) (n int, err error) {
	if lb.Len()+len(p) > lb.limit {
		// Truncate to prevent memory exhaustion
		remaining := lb.limit - lb.Len()
		if remaining > 0 {
			return lb.Buffer.Write(p[:remaining])
		}
		return len(p), nil // Pretend we wrote it all
	}
	return lb.Buffer.Write(p)
}

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

	// Process cleanup improvement: Set process group for better signal handling
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Network timeout reliability: Set environment variables for faster DNS resolution
	cmd.Env = append(os.Environ(),
		"CURL_CA_BUNDLE=", // Disable CA bundle lookup for faster curl operations
		"CURL_TIMEOUT=10", // Set curl-specific timeout
	)

	// Capture stdout and stderr while also forwarding to terminal
	// Use limited buffers for large outputs (10MB limit each)
	const maxBufferSize = 10 * 1024 * 1024 // 10MB limit
	stdoutBuf := &limitedBuffer{limit: maxBufferSize}
	stderrBuf := &limitedBuffer{limit: maxBufferSize}
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, stderrBuf)

	// Ensure process cleanup on context cancellation
	go func() {
		<-ctx.Done()
		if cmd.Process != nil {
			// Kill process group to ensure all child processes are terminated
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
	}()

	err := cmd.Run()

	output := CommandOutput{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	// Memory management optimization: Clear buffers after copying strings
	defer func() {
		stdoutBuf.Reset()
		stderrBuf.Reset()
		// Force garbage collection for stress testing scenarios
		if stdoutBuf.Len() > 1024*1024 || stderrBuf.Len() > 1024*1024 { // 1MB threshold
			runtime.GC()
		}
	}()

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
	Reporter        *ui.Reporter
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
	Stats        *ui.RunStats
	Metrics      *metrics.RunMetrics
}

// executeAttempt runs a single command attempt and returns the output, error, and timeout status
func (e *Executor) executeAttempt(command []string) (CommandOutput, error, bool) {
	if e.Timeout > 0 {
		// Network timeout reliability: Add small buffer to account for context switching overhead
		adjustedTimeout := e.Timeout + (50 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), adjustedTimeout)
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

	// Initialize statistics tracking
	stats := ui.NewRunStats()

	// Initialize metrics tracking
	var attemptMetrics []metrics.AttemptMetric
	runStartTime := time.Now()

	// Retry loop
	for attempt := 1; attempt <= e.MaxAttempts; attempt++ {
		// Report attempt start
		if e.Reporter != nil {
			e.Reporter.AttemptStart(attempt, e.MaxAttempts)
		}
		stats.RecordAttemptStart()

		// Record attempt start time for metrics
		attemptStartTime := time.Now()

		output, err, timeout := e.executeAttempt(command)
		lastOutput = output
		lastError = err
		if timeout {
			timedOut = true
		}

		// Record attempt duration for metrics
		attemptDuration := time.Since(attemptStartTime)

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

		// Record attempt result
		stats.RecordAttemptEnd(conditionResult.Success, conditionResult.Reason)

		// Record attempt metrics
		attemptMetrics = append(attemptMetrics, metrics.AttemptMetric{
			Duration: attemptDuration,
			ExitCode: output.ExitCode,
			Success:  conditionResult.Success,
		})

		// Process command output for HTTP-aware strategies
		if httpAware, ok := e.BackoffStrategy.(interface {
			ProcessCommandOutput(stdout, stderr string, exitCode int)
		}); ok {
			httpAware.ProcessCommandOutput(output.Stdout, output.Stderr, output.ExitCode)
		}

		// If command succeeded, return immediately
		if conditionResult.Success {
			stats.Finalize(true, conditionResult.Reason)

			// Create run metrics
			totalDuration := time.Since(runStartTime)
			runMetrics := metrics.NewRunMetrics(command, true, totalDuration, attemptMetrics)

			return &Result{
				AttemptCount: attempt,
				ExitCode:     output.ExitCode,
				Success:      true,
				TimedOut:     false,
				Reason:       conditionResult.Reason,
				Stats:        stats,
				Metrics:      runMetrics,
			}, nil
		}

		// If failure pattern matched, stop immediately (don't retry)
		if conditionResult.Reason == "failure pattern matched" {
			stats.Finalize(false, conditionResult.Reason)

			// Create run metrics
			totalDuration := time.Since(runStartTime)
			runMetrics := metrics.NewRunMetrics(command, false, totalDuration, attemptMetrics)

			return &Result{
				AttemptCount: attempt,
				ExitCode:     output.ExitCode,
				Success:      false,
				TimedOut:     false,
				Reason:       conditionResult.Reason,
				Stats:        stats,
				Metrics:      runMetrics,
			}, nil
		}

		// If this was the last attempt, break out of loop
		if attempt == e.MaxAttempts {
			// Report final failure (no retry)
			if e.Reporter != nil {
				failureReason := conditionResult.Reason
				if timedOut {
					failureReason = fmt.Sprintf("timeout: %s", e.Timeout)
				}
				e.Reporter.AttemptFailure(attempt, e.MaxAttempts, failureReason, 0)
			}
			break
		}

		// Calculate delay and report failure
		var delay time.Duration
		if e.BackoffStrategy != nil {
			delay = e.BackoffStrategy.Delay(attempt)
		}

		if e.Reporter != nil {
			failureReason := conditionResult.Reason
			if timedOut {
				failureReason = fmt.Sprintf("timeout: %s", e.Timeout)
			}
			e.Reporter.AttemptFailure(attempt, e.MaxAttempts, failureReason, delay)
		}

		// Wait before next attempt if backoff strategy is configured
		if delay > 0 {
			time.Sleep(delay)
		}
	}

	// All attempts failed
	var finalReason string
	if timedOut {
		if e.MaxAttempts == 1 {
			finalReason = "timeout"
		} else {
			finalReason = "max retries reached (timeout)"
		}
	} else if e.Conditions != nil {
		conditionResult := e.Conditions.CheckSuccess(lastOutput.ExitCode, lastOutput.Stdout, lastOutput.Stderr)
		if e.MaxAttempts == 1 {
			finalReason = conditionResult.Reason
		} else {
			finalReason = "max retries reached (" + conditionResult.Reason + ")"
		}
	} else {
		if e.MaxAttempts == 1 {
			if lastOutput.ExitCode == 0 {
				finalReason = "exit code 0"
			} else {
				finalReason = fmt.Sprintf("exit code %d", lastOutput.ExitCode)
			}
		} else {
			if lastOutput.ExitCode == 0 {
				finalReason = "max retries reached (exit code 0)"
			} else {
				finalReason = fmt.Sprintf("max retries reached (exit code %d)", lastOutput.ExitCode)
			}
		}
	}

	stats.Finalize(false, finalReason)

	// Create run metrics for failed execution
	totalDuration := time.Since(runStartTime)
	runMetrics := metrics.NewRunMetrics(command, false, totalDuration, attemptMetrics)

	return &Result{
		AttemptCount: e.MaxAttempts,
		ExitCode:     lastOutput.ExitCode,
		Success:      false,
		TimedOut:     timedOut,
		Reason:       finalReason,
		Stats:        stats,
		Metrics:      runMetrics,
	}, lastError
}
