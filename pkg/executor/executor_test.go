package executor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/retry/pkg/backoff"
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

// FakeCommandRunnerWithSequence allows different exit codes per call
type FakeCommandRunnerWithSequence struct {
	ExitCodes []int
	CallCount int
}

func (f *FakeCommandRunnerWithSequence) Run(command []string) (int, error) {
	if f.CallCount >= len(f.ExitCodes) {
		// Return last exit code if we've exhausted the sequence
		return f.ExitCodes[len(f.ExitCodes)-1], nil
	}
	exitCode := f.ExitCodes[f.CallCount]
	f.CallCount++
	return exitCode, nil
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

func TestExecutor_SucceedsOnSecondAttempt(t *testing.T) {
	// Given an executor configured for 3 attempts
	fakeRunner := &FakeCommandRunnerWithSequence{
		ExitCodes: []int{1, 0}, // Fail first, succeed second
	}
	executor := &Executor{
		MaxAttempts: 3,
		Runner:      fakeRunner,
	}

	// When Run() is called
	result, err := executor.Run([]string{"any", "command"})

	// Then there should be no error
	require.NoError(t, err)

	// And the result should be success
	assert.True(t, result.Success)

	// And the command should have been executed twice
	assert.Equal(t, 2, result.AttemptCount)

	// And the final exit code should be 0
	assert.Equal(t, 0, result.ExitCode)

	// And the fake runner should have been called twice
	assert.Equal(t, 2, fakeRunner.CallCount)
}

func TestExecutor_FailsAfterMaxAttempts(t *testing.T) {
	// Given an executor configured for 3 attempts
	fakeRunner := &FakeCommandRunner{
		ExitCode: 1, // Always fails
	}
	executor := &Executor{
		MaxAttempts: 3,
		Runner:      fakeRunner,
	}

	// When Run() is called
	result, err := executor.Run([]string{"any", "command"})

	// Then there should be no error
	require.NoError(t, err)

	// And the result should be failure
	assert.False(t, result.Success)

	// And the command should have been executed three times
	assert.Equal(t, 3, result.AttemptCount)

	// And the final exit code should be 1
	assert.Equal(t, 1, result.ExitCode)

	// And the fake runner should have been called three times
	assert.Equal(t, 3, fakeRunner.CallCount)
}

func TestExecutor_ZeroAttempts(t *testing.T) {
	// Given an executor configured for 0 attempts
	executor := NewExecutor(0)

	// When Run() is called
	result, err := executor.Run([]string{"true"})

	// Then there should be no error
	require.NoError(t, err)

	// And the result should indicate failure (no attempts made)
	assert.False(t, result.Success)
	assert.Equal(t, 0, result.AttemptCount)
}

func TestExecutor_WaitsForFixedDelay(t *testing.T) {
	// Given an executor configured with a fixed delay of 50ms
	fakeRunner := &FakeCommandRunner{
		ExitCode: 1, // Always fails
	}
	executor := &Executor{
		MaxAttempts:     3,
		Runner:          fakeRunner,
		BackoffStrategy: backoff.NewFixed(50 * time.Millisecond),
	}

	// When Run() is called
	start := time.Now()
	result, err := executor.Run([]string{"any", "command"})
	elapsed := time.Since(start)

	// Then there should be no error
	require.NoError(t, err)

	// And the result should be failure after 3 attempts
	assert.False(t, result.Success)
	assert.Equal(t, 3, result.AttemptCount)

	// And the total execution time should be slightly more than 100ms (2 delays of 50ms each)
	// We allow some tolerance for test execution overhead
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Less(t, elapsed, 200*time.Millisecond) // Should not take too long
}

func TestExecutorWithBackoff_Constructor(t *testing.T) {
	// Given an executor created with the backoff constructor
	strategy := backoff.NewFixed(25 * time.Millisecond)
	executor := NewExecutorWithBackoff(2, strategy)

	// Then the executor should have the correct configuration
	assert.Equal(t, 2, executor.MaxAttempts)
	assert.Equal(t, strategy, executor.BackoffStrategy)
	assert.NotNil(t, executor.Runner)
}

func TestExecutor_NoDelayOnSuccess(t *testing.T) {
	// Given an executor with backoff strategy
	executor := NewExecutorWithBackoff(3, backoff.NewFixed(100*time.Millisecond))

	// When a command succeeds on first try
	start := time.Now()
	result, err := executor.Run([]string{"true"})
	elapsed := time.Since(start)

	// Then there should be no error
	require.NoError(t, err)

	// And the result should be success
	assert.True(t, result.Success)
	assert.Equal(t, 1, result.AttemptCount)

	// And there should be no delay (execution should be fast)
	assert.Less(t, elapsed, 50*time.Millisecond)
}
