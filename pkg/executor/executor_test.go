package executor

import (
	"context"
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
	return f.RunWithContext(context.Background(), command)
}

func (f *FakeCommandRunner) RunWithContext(ctx context.Context, command []string) (int, error) {
	f.CallCount++
	return f.ExitCode, f.Error
}

// FakeCommandRunnerWithSequence allows different exit codes per call
type FakeCommandRunnerWithSequence struct {
	ExitCodes []int
	CallCount int
}

func (f *FakeCommandRunnerWithSequence) Run(command []string) (int, error) {
	return f.RunWithContext(context.Background(), command)
}

func (f *FakeCommandRunnerWithSequence) RunWithContext(ctx context.Context, command []string) (int, error) {
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

func TestExecutor_FailsOnTimeout(t *testing.T) {
	// Given an executor configured with a 20ms timeout
	executor := &Executor{
		MaxAttempts:     1,
		Runner:          &SystemCommandRunner{},
		BackoffStrategy: nil,
		Timeout:         20 * time.Millisecond,
	}

	// And a command that sleeps for 100ms (longer than timeout)
	command := []string{"sleep", "0.1"} // Sleep for 100ms

	// When Run() is called
	result, err := executor.Run(command)

	// Then there should be no error (timeout is handled as failure, not error)
	require.NoError(t, err)

	// And the result should be a failure due to timeout
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.AttemptCount)
	assert.True(t, result.TimedOut)

	// And the attempt should have been terminated quickly
	// (not waited for the full 100ms sleep)
}

func TestExecutor_TimeoutWithRetries(t *testing.T) {
	// Given an executor with timeout and multiple attempts
	executor := NewExecutorWithTimeout(3, 30*time.Millisecond)

	// When a command that always times out is run
	start := time.Now()
	result, err := executor.Run([]string{"sleep", "0.1"}) // 100ms sleep, 30ms timeout
	elapsed := time.Since(start)

	// Then there should be no error
	require.NoError(t, err)

	// And all attempts should have timed out
	assert.False(t, result.Success)
	assert.Equal(t, 3, result.AttemptCount)
	assert.True(t, result.TimedOut)

	// And the total time should be around 90ms (3 timeouts of ~30ms each)
	assert.GreaterOrEqual(t, elapsed, 80*time.Millisecond)
	assert.Less(t, elapsed, 150*time.Millisecond)
}

func TestExecutor_NoTimeoutWhenZero(t *testing.T) {
	// Given an executor with zero timeout (disabled)
	executor := NewExecutor(1)
	assert.Equal(t, time.Duration(0), executor.Timeout)

	// When a command is run
	result, err := executor.Run([]string{"true"})

	// Then it should succeed normally
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.False(t, result.TimedOut)
}

func TestExecutorConstructors_WithTimeout(t *testing.T) {
	// Test NewExecutorWithTimeout
	executor1 := NewExecutorWithTimeout(2, 50*time.Millisecond)
	assert.Equal(t, 2, executor1.MaxAttempts)
	assert.Equal(t, 50*time.Millisecond, executor1.Timeout)
	assert.Nil(t, executor1.BackoffStrategy)

	// Test NewExecutorWithBackoffAndTimeout
	strategy := backoff.NewFixed(25 * time.Millisecond)
	executor2 := NewExecutorWithBackoffAndTimeout(3, strategy, 100*time.Millisecond)
	assert.Equal(t, 3, executor2.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, executor2.Timeout)
	assert.Equal(t, strategy, executor2.BackoffStrategy)
}

func TestExecutor_TimeoutWithBackoff(t *testing.T) {
	// Given an executor with both timeout and backoff
	strategy := backoff.NewFixed(20 * time.Millisecond)
	executor := NewExecutorWithBackoffAndTimeout(2, strategy, 30*time.Millisecond)

	// When a command that times out is run
	start := time.Now()
	result, err := executor.Run([]string{"sleep", "0.1"}) // 100ms sleep, 30ms timeout
	elapsed := time.Since(start)

	// Then there should be no error
	require.NoError(t, err)

	// And it should fail with timeout after 2 attempts
	assert.False(t, result.Success)
	assert.Equal(t, 2, result.AttemptCount)
	assert.True(t, result.TimedOut)

	// And the total time should include both timeouts and one backoff delay
	// ~30ms (first timeout) + 20ms (backoff) + ~30ms (second timeout) = ~80ms
	assert.GreaterOrEqual(t, elapsed, 70*time.Millisecond)
	assert.Less(t, elapsed, 120*time.Millisecond)
}

func TestExecutor_WithExponentialBackoff(t *testing.T) {
	// Given an executor with exponential backoff
	strategy := backoff.NewExponential(50*time.Millisecond, 2.0, 0)
	fakeRunner := &FakeCommandRunner{
		ExitCode: 1, // Always fails
	}
	executor := &Executor{
		MaxAttempts:     3,
		Runner:          fakeRunner,
		BackoffStrategy: strategy,
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

	// And the total time should include exponential delays
	// 50ms (first delay) + 100ms (second delay) = ~150ms
	assert.GreaterOrEqual(t, elapsed, 140*time.Millisecond)
	assert.Less(t, elapsed, 200*time.Millisecond)
}
