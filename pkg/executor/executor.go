package executor

import (
	"os/exec"
)

// CommandRunner defines the interface for executing commands
type CommandRunner interface {
	Run(command []string) (int, error)
}

// SystemCommandRunner implements CommandRunner using os/exec
type SystemCommandRunner struct{}

// Run executes a command using os/exec and returns the exit code
func (r *SystemCommandRunner) Run(command []string) (int, error) {
	if len(command) == 0 {
		return -1, nil
	}

	cmd := exec.Command(command[0], command[1:]...)
	err := cmd.Run()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode(), nil
		}
		return -1, err
	}

	return 0, nil
}

// Executor handles command execution with retry logic
type Executor struct {
	MaxAttempts int
	Runner      CommandRunner
}

// NewExecutor creates a new Executor with default SystemCommandRunner
func NewExecutor(maxAttempts int) *Executor {
	return &Executor{
		MaxAttempts: maxAttempts,
		Runner:      &SystemCommandRunner{},
	}
}

// Result represents the outcome of a command execution
type Result struct {
	Success      bool
	AttemptCount int
	ExitCode     int
}

// executeAttempt runs a single command attempt and returns the exit code and error
func (e *Executor) executeAttempt(command []string) (int, error) {
	return e.Runner.Run(command)
}

// Run executes the given command with retry logic and returns the result
func (e *Executor) Run(command []string) (*Result, error) {
	var lastExitCode int
	var lastError error

	// Retry loop
	for attempt := 1; attempt <= e.MaxAttempts; attempt++ {
		exitCode, err := e.executeAttempt(command)
		lastExitCode = exitCode
		lastError = err

		if err != nil {
			return nil, err
		}

		// If command succeeded, return immediately
		if exitCode == 0 {
			return &Result{
				AttemptCount: attempt,
				ExitCode:     exitCode,
				Success:      true,
			}, nil
		}

		// If this was the last attempt, break out of loop
		if attempt == e.MaxAttempts {
			break
		}

		// Continue to next attempt (no delay in this cycle)
	}

	// All attempts failed
	return &Result{
		AttemptCount: e.MaxAttempts,
		ExitCode:     lastExitCode,
		Success:      false,
	}, lastError
}
