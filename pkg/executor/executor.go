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

// Run executes the given command and returns the result
func (e *Executor) Run(command []string) (*Result, error) {
	exitCode, err := e.Runner.Run(command)
	if err != nil {
		return nil, err
	}

	result := &Result{
		AttemptCount: 1,
		ExitCode:     exitCode,
		Success:      exitCode == 0,
	}

	return result, nil
}
