package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/shaneisley/patience/pkg/conditions"
	"github.com/shaneisley/patience/pkg/daemon"
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
	// Use limited buffers for large outputs
	stdoutBuf := &limitedBuffer{limit: DefaultMaxBufferSize}
	stderrBuf := &limitedBuffer{limit: DefaultMaxBufferSize}
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
		// Memory cleanup - removed manual GC call as per best practices
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
	DaemonClient    *daemon.DaemonClient // Optional daemon client for coordination
	ResourceID      string               // Resource identifier for rate limiting
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

// coordinateWithDaemon handles scheduling coordination with the daemon for Diophantine strategy
func (e *Executor) coordinateWithDaemon(strategy *backoff.DiophantineStrategy, command []string) error {
	// If no daemon client is configured, skip coordination (fallback mode)
	if e.DaemonClient == nil {
		return nil
	}

	// Determine resource ID (use configured ResourceID or derive from command)
	resourceID := e.ResourceID
	if resourceID == "" {
		resourceID = e.deriveResourceID(command)
	}

	// Create schedule request
	scheduleReq := &daemon.ScheduleRequest{
		ResourceID:   resourceID,
		RateLimit:    strategy.GetRateLimit(),
		Window:       strategy.GetWindow(),
		RetryOffsets: strategy.GetRetryOffsets(),
		RequestTime:  time.Now(),
	}

	// Ask daemon if we can schedule now
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := e.DaemonClient.CanScheduleRequest(ctx, scheduleReq)
	if err != nil {
		// If daemon communication fails, fall back to local-only mode
		if e.Reporter != nil {
			e.Reporter.ShowWarning("Daemon unavailable, using local scheduling")
		}
		return nil
	}

	// If we can't schedule now, wait until we can
	if !response.CanSchedule {
		waitTime := time.Until(response.WaitUntil)
		if waitTime > 0 {
			if e.Reporter != nil {
				e.Reporter.ShowWaiting(waitTime, "Waiting for rate limit slot...")
			}
			time.Sleep(waitTime)
		}
	}

	// Register our planned requests with the daemon
	plannedRequests := e.createPlannedRequests(resourceID, scheduleReq.RequestTime, strategy.GetRetryOffsets())
	err = e.DaemonClient.RegisterScheduledRequests(ctx, plannedRequests)
	if err != nil {
		// Registration failure is not critical, continue with execution
		if e.Reporter != nil {
			e.Reporter.ShowWarning("Failed to register requests with daemon")
		}
	}

	return nil
}

// deriveResourceID attempts to derive a resource identifier from the command
func (e *Executor) deriveResourceID(command []string) string {
	if len(command) == 0 {
		return "unknown"
	}

	// Simple heuristics for common commands
	switch command[0] {
	case "curl":
		// Extract hostname from curl command
		for i, arg := range command {
			if i > 0 && !strings.HasPrefix(arg, "-") {
				if strings.HasPrefix(arg, "http") {
					// Extract hostname from URL
					if u, err := url.Parse(arg); err == nil {
						return fmt.Sprintf("http-%s", u.Host)
					}
				}
				return fmt.Sprintf("http-%s", arg)
			}
		}
		return "http-api"
	case "psql", "mysql":
		return "database"
	case "aws":
		return "aws-api"
	case "kubectl":
		return "kubernetes-api"
	default:
		return fmt.Sprintf("cmd-%s", command[0])
	}
}

// createPlannedRequests creates scheduled request entries for daemon registration
func (e *Executor) createPlannedRequests(resourceID string, baseTime time.Time, retryOffsets []time.Duration) []*daemon.ScheduledRequest {
	requests := make([]*daemon.ScheduledRequest, len(retryOffsets))

	for i, offset := range retryOffsets {
		requests[i] = &daemon.ScheduledRequest{
			ID:          fmt.Sprintf("%s-%d-%d", resourceID, baseTime.Unix(), i),
			ResourceID:  resourceID,
			ScheduledAt: baseTime.Add(offset),
			ExpiresAt:   baseTime.Add(offset).Add(time.Hour), // Expire after 1 hour
		}
	}

	return requests
}

// Run executes the given command with retry logic and returns the result
// coordinateDaemon handles Diophantine strategy coordination with daemon
func (e *Executor) coordinateDaemon(strategy backoff.Strategy, command []string) error {
	if diophantineStrategy, ok := strategy.(*backoff.DiophantineStrategy); ok {
		return e.coordinateWithDaemon(diophantineStrategy, command)
	}
	return nil
}

// initializeExecution sets up stats, metrics, and variables for a run
func (e *Executor) initializeExecution(command []string) (*ui.RunStats, []metrics.AttemptMetric, time.Time) {
	stats := ui.NewRunStats()
	var attemptMetrics []metrics.AttemptMetric
	runStartTime := time.Now()
	return stats, attemptMetrics, runStartTime
}

// processAttemptResult evaluates success conditions and determines if retry should stop
func (e *Executor) processAttemptResult(output CommandOutput, attempt int) (conditions.Result, bool) {
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

	// Stop retrying if successful or if failure pattern matched
	shouldStop := conditionResult.Success || conditionResult.Reason == "failure pattern matched"
	return conditionResult, shouldStop
}

// recordStrategyOutcome updates adaptive strategies with attempt results
func (e *Executor) recordStrategyOutcome(attempt int, success bool, duration time.Duration) {
	if adaptiveStrategy, ok := e.BackoffStrategy.(interface {
		RecordOutcome(delay time.Duration, success bool, latency time.Duration)
	}); ok {
		// Calculate the delay that was actually used for this attempt
		var actualDelay time.Duration
		if attempt > 1 && e.BackoffStrategy != nil {
			actualDelay = e.BackoffStrategy.Delay(attempt - 1)
		}
		adaptiveStrategy.RecordOutcome(actualDelay, success, duration)
	}

	// Process command output for HTTP-aware strategies
	if httpAware, ok := e.BackoffStrategy.(interface {
		ProcessCommandOutput(stdout, stderr string, exitCode int)
	}); ok {
		// This would need the output, but we'll handle it in the main loop
		_ = httpAware
	}
}

// determineFinalReason calculates the final failure reason
func (e *Executor) determineFinalReason(lastOutput CommandOutput, timedOut bool) string {
	if timedOut {
		if e.MaxAttempts == 1 {
			return "timeout"
		}
		return "max retries reached (timeout)"
	}

	if e.Conditions != nil {
		conditionResult := e.Conditions.CheckSuccess(lastOutput.ExitCode, lastOutput.Stdout, lastOutput.Stderr)
		if e.MaxAttempts == 1 {
			return conditionResult.Reason
		}
		return "max retries reached (" + conditionResult.Reason + ")"
	}

	// Default exit code based reasoning
	exitReason := fmt.Sprintf("exit code %d", lastOutput.ExitCode)
	if lastOutput.ExitCode == 0 {
		exitReason = "exit code 0"
	}

	if e.MaxAttempts == 1 {
		return exitReason
	}
	return "max retries reached (" + exitReason + ")"
}

// buildFinalResult constructs the final Result object
func (e *Executor) buildFinalResult(success bool, attemptCount int, lastOutput CommandOutput, timedOut bool, reason string, stats *ui.RunStats, attemptMetrics []metrics.AttemptMetric, runStartTime time.Time, command []string, lastError error) *Result {
	totalDuration := time.Since(runStartTime)
	runMetrics := metrics.NewRunMetrics(command, success, totalDuration, attemptMetrics)

	return &Result{
		AttemptCount: attemptCount,
		ExitCode:     lastOutput.ExitCode,
		Success:      success,
		TimedOut:     timedOut,
		Reason:       reason,
		Stats:        stats,
		Metrics:      runMetrics,
	}
}

func (e *Executor) Run(command []string) (*Result, error) {
	// Handle Diophantine strategy coordination with daemon
	if err := e.coordinateDaemon(e.BackoffStrategy, command); err != nil {
		return &Result{
			Success:      false,
			AttemptCount: 0,
			ExitCode:     -1,
			TimedOut:     false,
			Reason:       fmt.Sprintf("daemon coordination failed: %v", err),
		}, err
	}

	var lastOutput CommandOutput
	var lastError error
	var timedOut bool

	// Initialize execution tracking
	stats, attemptMetrics, runStartTime := e.initializeExecution(command)

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

		// Check success conditions and determine if we should stop retrying
		conditionResult, shouldStop := e.processAttemptResult(output, attempt)

		// Record attempt result
		stats.RecordAttemptEnd(conditionResult.Success, conditionResult.Reason)

		// Record attempt metrics
		attemptMetrics = append(attemptMetrics, metrics.AttemptMetric{
			Duration: attemptDuration,
			ExitCode: output.ExitCode,
			Success:  conditionResult.Success,
		})

		// Record outcome for adaptive and HTTP-aware strategies
		e.recordStrategyOutcome(attempt, conditionResult.Success, attemptDuration)

		// Process command output for HTTP-aware strategies
		if httpAware, ok := e.BackoffStrategy.(interface {
			ProcessCommandOutput(stdout, stderr string, exitCode int)
		}); ok {
			httpAware.ProcessCommandOutput(output.Stdout, output.Stderr, output.ExitCode)
		}

		// If we should stop retrying (success or failure pattern matched)
		if shouldStop {
			stats.Finalize(conditionResult.Success, conditionResult.Reason)
			return e.buildFinalResult(conditionResult.Success, attempt, output, timedOut, conditionResult.Reason, stats, attemptMetrics, runStartTime, command, lastError), nil
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

	// All attempts failed - determine final reason
	finalReason := e.determineFinalReason(lastOutput, timedOut)
	stats.Finalize(false, finalReason)

	return e.buildFinalResult(false, e.MaxAttempts, lastOutput, timedOut, finalReason, stats, attemptMetrics, runStartTime, command, lastError), lastError
}
