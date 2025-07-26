package executor

import (
	"context"
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecutorHTTPAwareIntegration tests that the executor properly integrates with HTTP-aware strategies
func TestExecutorHTTPAwareIntegration(t *testing.T) {
	t.Run("ExecutorCallsProcessCommandOutput", func(t *testing.T) {
		// Create HTTP-aware strategy with fixed fallback
		fallback := backoff.NewFixed(100 * time.Millisecond)
		strategy := backoff.NewHTTPAware(fallback, 30*time.Minute)

		// Create mock command runner that simulates HTTP responses
		mockRunner := &MockHTTPCommandRunner{
			responses: []MockHTTPResponse{
				{
					ExitCode: 22, // curl -f exit code for HTTP errors
					Stdout:   "",
					Stderr: `HTTP/1.1 429 Too Many Requests
Retry-After: 2

Rate limited`,
				},
				{
					ExitCode: 0,
					Stdout:   "Success",
					Stderr:   "",
				},
			},
		}

		// Create executor with HTTP-aware strategy and mock runner
		executor := &Executor{
			MaxAttempts:     3,
			Runner:          mockRunner,
			BackoffStrategy: strategy,
		}

		// Execute command
		start := time.Now()
		result, err := executor.Run([]string{"curl", "-f", "https://api.example.com"})
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.True(t, result.Success, "Should eventually succeed")
		assert.Equal(t, 2, result.AttemptCount, "Should succeed on second attempt")

		// Verify that HTTP timing was used (2 second delay from Retry-After)
		// Allow some variance for timing precision
		assert.True(t, elapsed >= 1800*time.Millisecond,
			"Should have waited approximately 2 seconds based on Retry-After header, got %v", elapsed)
		assert.True(t, elapsed <= 2500*time.Millisecond,
			"Should not have waited much longer than 2 seconds, got %v", elapsed)
	})

	t.Run("FallbackWhenNoHTTPInfo", func(t *testing.T) {
		// Create HTTP-aware strategy with exponential fallback
		fallback := backoff.NewExponential(200*time.Millisecond, 2.0, 10*time.Second)
		strategy := backoff.NewHTTPAware(fallback, 30*time.Minute)

		// Create mock command runner with non-HTTP responses
		mockRunner := &MockHTTPCommandRunner{
			responses: []MockHTTPResponse{
				{
					ExitCode: 1,
					Stdout:   "Connection refused",
					Stderr:   "",
				},
				{
					ExitCode: 1,
					Stdout:   "Connection refused",
					Stderr:   "",
				},
				{
					ExitCode: 0,
					Stdout:   "Success",
					Stderr:   "",
				},
			},
		}

		// Create executor
		executor := &Executor{
			MaxAttempts:     3,
			Runner:          mockRunner,
			BackoffStrategy: strategy,
		}

		// Execute command
		start := time.Now()
		result, err := executor.Run([]string{"curl", "https://api.example.com"})
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.True(t, result.Success, "Should eventually succeed")
		assert.Equal(t, 3, result.AttemptCount, "Should succeed on third attempt")

		// Verify that fallback exponential timing was used
		// First delay: 200ms, second delay: 400ms = 600ms total minimum
		assert.True(t, elapsed >= 550*time.Millisecond,
			"Should have used exponential fallback delays, got %v", elapsed)
	})

	t.Run("JSONRetryFieldParsing", func(t *testing.T) {
		// Create HTTP-aware strategy
		fallback := backoff.NewFixed(50 * time.Millisecond)
		strategy := backoff.NewHTTPAware(fallback, 30*time.Minute)

		// Create mock command runner with JSON retry response
		mockRunner := &MockHTTPCommandRunner{
			responses: []MockHTTPResponse{
				{
					ExitCode: 1,
					Stdout: `{
  "error": "service temporarily unavailable",
  "retry_after": 1,
  "message": "Please try again later"
}`,
					Stderr: "",
				},
				{
					ExitCode: 0,
					Stdout:   "Success",
					Stderr:   "",
				},
			},
		}

		// Create executor
		executor := &Executor{
			MaxAttempts:     3,
			Runner:          mockRunner,
			BackoffStrategy: strategy,
		}

		// Execute command
		start := time.Now()
		result, err := executor.Run([]string{"api-call"})
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.True(t, result.Success, "Should eventually succeed")
		assert.Equal(t, 2, result.AttemptCount, "Should succeed on second attempt")

		// Verify that JSON retry timing was used (1 second from retry_after)
		assert.True(t, elapsed >= 900*time.Millisecond,
			"Should have waited approximately 1 second based on JSON retry_after, got %v", elapsed)
		assert.True(t, elapsed <= 1300*time.Millisecond,
			"Should not have waited much longer than 1 second, got %v", elapsed)
	})
}

// MockHTTPCommandRunner simulates command execution with HTTP responses
type MockHTTPCommandRunner struct {
	responses   []MockHTTPResponse
	currentCall int
}

type MockHTTPResponse struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func (m *MockHTTPCommandRunner) Run(command []string) (int, error) {
	output, err := m.RunWithOutput(command)
	return output.ExitCode, err
}

func (m *MockHTTPCommandRunner) RunWithContext(ctx context.Context, command []string) (int, error) {
	output, err := m.RunWithOutputAndContext(ctx, command)
	return output.ExitCode, err
}

func (m *MockHTTPCommandRunner) RunWithOutput(command []string) (CommandOutput, error) {
	return m.RunWithOutputAndContext(context.Background(), command)
}

func (m *MockHTTPCommandRunner) RunWithOutputAndContext(ctx context.Context, command []string) (CommandOutput, error) {
	if m.currentCall >= len(m.responses) {
		return CommandOutput{ExitCode: 1, Stdout: "No more responses", Stderr: ""}, nil
	}

	response := m.responses[m.currentCall]
	m.currentCall++

	return CommandOutput{
		ExitCode: response.ExitCode,
		Stdout:   response.Stdout,
		Stderr:   response.Stderr,
	}, nil
}
