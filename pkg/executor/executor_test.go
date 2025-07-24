package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FakeCommandRunner is a test double for CommandRunner
type FakeCommandRunner struct {
	ExitCode  int
	Error     error
	CallCount int
}

func (f *FakeCommandRunner) Run(command []string) (int, error) {
	f.CallCount++
	return f.ExitCode, f.Error
}

func TestExecutor_SuccessOnFirstTry(t *testing.T) {
	// Given an executor configured for 1 attempt
	executor := NewExecutor(1)

	// And a command that will exit with code 0
	command := []string{"true"} // Unix 'true' command always exits with 0

	// When Run() is called
	result, err := executor.Run(command)

	// Then there should be no error
	require.NoError(t, err)

	// And the result should indicate success
	assert.True(t, result.Success)

	// And the command should have been executed exactly once
	assert.Equal(t, 1, result.AttemptCount)
}

func TestExecutor_FailureWithNoRetries(t *testing.T) {
	// Given an executor configured for 1 attempt
	executor := NewExecutor(1)

	// And a command that will exit with code 1
	command := []string{"false"} // Unix 'false' command always exits with 1

	// When Run() is called
	result, err := executor.Run(command)

	// Then there should be no error (the command ran, it just failed)
	require.NoError(t, err)

	// And the result should indicate failure
	assert.False(t, result.Success)

	// And the command should have been executed exactly once
	assert.Equal(t, 1, result.AttemptCount)

	// And the exit code should be 1
	assert.Equal(t, 1, result.ExitCode)
}

func TestExecutor_WithFakeRunner(t *testing.T) {
	// Given an executor with a fake command runner
	fakeRunner := &FakeCommandRunner{
		ExitCode: 42,
		Error:    nil,
	}
	executor := &Executor{
		MaxAttempts: 1,
		Runner:      fakeRunner,
	}

	// When Run() is called
	result, err := executor.Run([]string{"any", "command"})

	// Then there should be no error
	require.NoError(t, err)

	// And the result should have the fake exit code
	assert.Equal(t, 42, result.ExitCode)
	assert.False(t, result.Success) // 42 != 0, so not success

	// And the fake runner should have been called once
	assert.Equal(t, 1, fakeRunner.CallCount)
}
