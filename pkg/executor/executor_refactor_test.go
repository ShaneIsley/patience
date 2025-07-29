package executor

import (
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/stretchr/testify/assert"
)

// ExecutorBuilder provides a fluent interface for creating Executors
type ExecutorBuilder struct {
	maxAttempts     int
	backoffStrategy backoff.Strategy
	timeout         time.Duration
	runner          CommandRunner
}

// NewExecutorBuilder creates a new ExecutorBuilder with sensible defaults
func NewExecutorBuilder() *ExecutorBuilder {
	return &ExecutorBuilder{
		maxAttempts: 1,
		runner:      &SystemCommandRunner{},
	}
}

// WithMaxAttempts sets the maximum number of retry attempts
func (b *ExecutorBuilder) WithMaxAttempts(attempts int) *ExecutorBuilder {
	b.maxAttempts = attempts
	return b
}

// WithBackoff sets the backoff strategy for delays between retries
func (b *ExecutorBuilder) WithBackoff(strategy backoff.Strategy) *ExecutorBuilder {
	b.backoffStrategy = strategy
	return b
}

// WithTimeout sets the timeout for individual command executions
func (b *ExecutorBuilder) WithTimeout(timeout time.Duration) *ExecutorBuilder {
	b.timeout = timeout
	return b
}

// WithRunner sets the command runner implementation
func (b *ExecutorBuilder) WithRunner(runner CommandRunner) *ExecutorBuilder {
	b.runner = runner
	return b
}

// Build creates the final Executor instance
func (b *ExecutorBuilder) Build() *Executor {
	return &Executor{
		MaxAttempts:     b.maxAttempts,
		BackoffStrategy: b.backoffStrategy,
		Timeout:         b.timeout,
		Runner:          b.runner,
	}
}

func TestConstants(t *testing.T) {
	t.Run("constants have expected values", func(t *testing.T) {
		assert.Equal(t, 10*1024*1024, DefaultMaxBufferSize)
		assert.Equal(t, 1024*1024, MemoryThreshold)
		assert.Equal(t, 1000, MaxAttemptsLimit)
		assert.Equal(t, 0600, SocketPermissions)
		assert.Equal(t, 1440, DaemonMaxMinutes)
	})
}

func TestExecutorBuilder(t *testing.T) {
	t.Run("builder creates executor with defaults", func(t *testing.T) {
		executor := NewExecutorBuilder().Build()

		assert.NotNil(t, executor)
		assert.Equal(t, 1, executor.MaxAttempts)
		assert.NotNil(t, executor.Runner)
		assert.Nil(t, executor.BackoffStrategy)
		assert.Equal(t, time.Duration(0), executor.Timeout)
	})

	t.Run("builder sets max attempts", func(t *testing.T) {
		executor := NewExecutorBuilder().
			WithMaxAttempts(5).
			Build()

		assert.Equal(t, 5, executor.MaxAttempts)
	})

	t.Run("builder sets backoff strategy", func(t *testing.T) {
		strategy := backoff.NewFixed(time.Second)
		executor := NewExecutorBuilder().
			WithBackoff(strategy).
			Build()

		assert.Equal(t, strategy, executor.BackoffStrategy)
	})

	t.Run("builder sets timeout", func(t *testing.T) {
		timeout := 30 * time.Second
		executor := NewExecutorBuilder().
			WithTimeout(timeout).
			Build()

		assert.Equal(t, timeout, executor.Timeout)
	})

	t.Run("builder chains methods", func(t *testing.T) {
		strategy := backoff.NewExponential(time.Second, 2.0, time.Minute)
		timeout := 45 * time.Second

		executor := NewExecutorBuilder().
			WithMaxAttempts(10).
			WithBackoff(strategy).
			WithTimeout(timeout).
			Build()

		assert.Equal(t, 10, executor.MaxAttempts)
		assert.Equal(t, strategy, executor.BackoffStrategy)
		assert.Equal(t, timeout, executor.Timeout)
	})

	t.Run("builder replaces old constructors", func(t *testing.T) {
		strategy := backoff.NewFixed(2 * time.Second)
		timeout := 30 * time.Second

		oldStyle := NewExecutorWithBackoffAndTimeout(5, strategy, timeout)
		newStyle := NewExecutorBuilder().
			WithMaxAttempts(5).
			WithBackoff(strategy).
			WithTimeout(timeout).
			Build()

		assert.Equal(t, oldStyle.MaxAttempts, newStyle.MaxAttempts)
		assert.Equal(t, oldStyle.BackoffStrategy, newStyle.BackoffStrategy)
		assert.Equal(t, oldStyle.Timeout, newStyle.Timeout)
	})
}

func TestBuilderReplacesOldConstructors(t *testing.T) {
	t.Run("replaces NewExecutor", func(t *testing.T) {
		old := NewExecutor(3)
		new := NewExecutorBuilder().WithMaxAttempts(3).Build()

		assert.Equal(t, old.MaxAttempts, new.MaxAttempts)
		assert.Equal(t, old.BackoffStrategy, new.BackoffStrategy)
		assert.Equal(t, old.Timeout, new.Timeout)
	})

	t.Run("replaces NewExecutorWithBackoff", func(t *testing.T) {
		strategy := backoff.NewLinear(time.Second, 10*time.Second)
		old := NewExecutorWithBackoff(5, strategy)
		new := NewExecutorBuilder().WithMaxAttempts(5).WithBackoff(strategy).Build()

		assert.Equal(t, old.MaxAttempts, new.MaxAttempts)
		assert.Equal(t, old.BackoffStrategy, new.BackoffStrategy)
		assert.Equal(t, old.Timeout, new.Timeout)
	})

	t.Run("replaces NewExecutorWithTimeout", func(t *testing.T) {
		timeout := 15 * time.Second
		old := NewExecutorWithTimeout(2, timeout)
		new := NewExecutorBuilder().WithMaxAttempts(2).WithTimeout(timeout).Build()

		assert.Equal(t, old.MaxAttempts, new.MaxAttempts)
		assert.Equal(t, old.BackoffStrategy, new.BackoffStrategy)
		assert.Equal(t, old.Timeout, new.Timeout)
	})
}
