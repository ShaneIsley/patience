package backoff

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPAwareIntegration tests the integration between HTTP-aware strategy and executor
// These tests will initially FAIL to demonstrate the issues that need to be fixed
func TestHTTPAwareIntegration(t *testing.T) {
	t.Run("RetryAfterHeaderParsing", func(t *testing.T) {
		// Create HTTP-aware strategy with exponential fallback
		fallback := NewExponential(100*time.Millisecond, 2.0, 10*time.Second)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		// Simulate curl output with Retry-After header
		httpOutput := `HTTP/1.1 429 Too Many Requests
Retry-After: 2
Content-Type: application/json

{"error": "rate limited"}`

		// Process the HTTP output
		strategy.ProcessCommandOutput(httpOutput, "", 22) // curl -f exits with 22 for HTTP errors

		// First delay should be 2 seconds from Retry-After header
		delay1 := strategy.Delay(1)
		assert.Equal(t, 2*time.Second, delay1, "Should use Retry-After header value")

		// After using HTTP timing, should fall back to exponential strategy
		strategy.ProcessCommandOutput("no http content", "", 1)
		delay2 := strategy.Delay(2)
		assert.Equal(t, 200*time.Millisecond, delay2, "Should fall back to exponential strategy")
	})

	t.Run("JSONRetryFieldParsing", func(t *testing.T) {
		fallback := NewLinear(50*time.Millisecond, 5*time.Second)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		// Simulate API response with JSON retry field
		jsonOutput := `{
  "error": "service temporarily unavailable",
  "retry_after": 3,
  "message": "Please try again later"
}`

		strategy.ProcessCommandOutput(jsonOutput, "", 1)

		// Should extract 3 seconds from JSON retry_after field
		delay := strategy.Delay(1)
		assert.Equal(t, 3*time.Second, delay, "Should parse retry_after from JSON")
	})

	t.Run("MultipleJSONRetryFormats", func(t *testing.T) {
		fallback := NewFixed(100 * time.Millisecond)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		testCases := []struct {
			name     string
			json     string
			expected time.Duration
		}{
			{
				name:     "retry_after",
				json:     `{"status": "error", "retry_after": 5}`,
				expected: 5 * time.Second,
			},
			{
				name:     "retryAfter",
				json:     `{"error": "rate limited", "retryAfter": 10}`,
				expected: 10 * time.Second,
			},
			{
				name:     "retry_after_seconds",
				json:     `{"message": "try again", "retry_after_seconds": 7}`,
				expected: 7 * time.Second,
			},
			{
				name:     "retryAfterSeconds",
				json:     `{"code": 429, "retryAfterSeconds": 15}`,
				expected: 15 * time.Second,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				strategy.ProcessCommandOutput(tc.json, "", 1)
				delay := strategy.Delay(1)
				assert.Equal(t, tc.expected, delay, "Should parse %s field correctly", tc.name)
			})
		}
	})

	t.Run("FallbackStrategyActivation", func(t *testing.T) {
		fallback := NewExponential(200*time.Millisecond, 2.0, 10*time.Second)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		// Test with non-HTTP output (should use fallback)
		nonHttpOutput := "Connection refused"
		strategy.ProcessCommandOutput(nonHttpOutput, "", 1)

		// Should use fallback strategy delays
		delay1 := strategy.Delay(1)
		delay2 := strategy.Delay(2)
		delay3 := strategy.Delay(3)

		assert.Equal(t, 200*time.Millisecond, delay1, "Attempt 1 should use fallback strategy")
		assert.Equal(t, 400*time.Millisecond, delay2, "Attempt 2 should use fallback strategy")
		assert.Equal(t, 800*time.Millisecond, delay3, "Attempt 3 should use fallback strategy")
	})

	t.Run("MaxDelayRespected", func(t *testing.T) {
		fallback := NewFixed(100 * time.Millisecond)
		maxDelay := 5 * time.Second
		strategy := NewHTTPAware(fallback, maxDelay)

		// Test with very large retry-after value
		httpOutput := `HTTP/1.1 503 Service Unavailable
Retry-After: 3600

Service maintenance in progress`

		strategy.ProcessCommandOutput(httpOutput, "", 1)
		delay := strategy.Delay(1)

		assert.Equal(t, maxDelay, delay, "Should cap delay at maxDelay")
	})

	t.Run("RateLimitHeaderParsing", func(t *testing.T) {
		fallback := NewFixed(100 * time.Millisecond)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		// Test X-RateLimit-Retry-After header
		httpOutput := `HTTP/1.1 429 Too Many Requests
X-RateLimit-Retry-After: 4
X-RateLimit-Remaining: 0

Rate limit exceeded`

		strategy.ProcessCommandOutput(httpOutput, "", 1)
		delay := strategy.Delay(1)

		assert.Equal(t, 4*time.Second, delay, "Should parse X-RateLimit-Retry-After header")
	})

	t.Run("HTTPResponseInStderr", func(t *testing.T) {
		fallback := NewFixed(100 * time.Millisecond)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		// Some tools output HTTP headers to stderr
		stderrOutput := `HTTP/1.1 429 Too Many Requests
Retry-After: 6
`
		stdoutOutput := `{"error": "rate limited"}`

		strategy.ProcessCommandOutput(stdoutOutput, stderrOutput, 22)
		delay := strategy.Delay(1)

		assert.Equal(t, 6*time.Second, delay, "Should parse HTTP headers from stderr")
	})

	t.Run("CombinedHTTPAndJSON", func(t *testing.T) {
		fallback := NewFixed(100 * time.Millisecond)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		// HTTP response with both header and JSON (header should take precedence)
		httpOutput := `HTTP/1.1 429 Too Many Requests
Retry-After: 8
Content-Type: application/json

{"error": "rate limited", "retry_after": 12}`

		strategy.ProcessCommandOutput(httpOutput, "", 22)
		delay := strategy.Delay(1)

		assert.Equal(t, 8*time.Second, delay, "HTTP header should take precedence over JSON")
	})
}

// TestHTTPAwareExecutorIntegration tests that the executor properly calls ProcessCommandOutput
// This test will FAIL initially because the executor doesn't call ProcessCommandOutput
func TestHTTPAwareExecutorIntegration(t *testing.T) {
	t.Run("ExecutorCallsProcessCommandOutput", func(t *testing.T) {
		// This test verifies that the executor integration is working
		// It will fail initially because the executor doesn't call ProcessCommandOutput

		fallback := NewFixed(100 * time.Millisecond)
		strategy := NewHTTPAware(fallback, 30*time.Minute)

		// Create a mock command runner that returns HTTP output
		mockRunner := &TestCommandRunner{
			outputs: []TestCommandOutput{
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

		// Create executor with HTTP-aware strategy
		executor := &TestExecutor{
			MaxAttempts:     3,
			Runner:          mockRunner,
			BackoffStrategy: strategy,
		}

		// This should demonstrate that HTTP timing is being used
		// The test will fail because the executor doesn't call ProcessCommandOutput
		result, err := executor.Run([]string{"curl", "-f", "https://api.example.com"})
		require.NoError(t, err)

		// Verify that HTTP timing was used (2 second delay from Retry-After)
		// This assertion will fail because the integration is broken
		assert.True(t, mockRunner.delayBetweenCalls >= 1800*time.Millisecond,
			"Should have waited approximately 2 seconds based on Retry-After header, got %v", mockRunner.delayBetweenCalls)
		assert.True(t, result.Success, "Should eventually succeed")
		assert.Equal(t, 2, result.AttemptCount, "Should succeed on second attempt")
	})
}

// Test types for mocking
type TestCommandOutput struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type TestCommandRunner struct {
	outputs           []TestCommandOutput
	currentCall       int
	delayBetweenCalls time.Duration
	lastCallTime      time.Time
}

func (m *TestCommandRunner) Run(command []string) (int, error) {
	output := m.getNextOutput()
	return output.ExitCode, nil
}

func (m *TestCommandRunner) RunWithContext(ctx context.Context, command []string) (int, error) {
	output := m.getNextOutput()
	return output.ExitCode, nil
}

func (m *TestCommandRunner) RunWithOutput(command []string) (TestCommandOutput, error) {
	return m.getNextOutput(), nil
}

func (m *TestCommandRunner) RunWithOutputAndContext(ctx context.Context, command []string) (TestCommandOutput, error) {
	return m.getNextOutput(), nil
}

func (m *TestCommandRunner) getNextOutput() TestCommandOutput {
	// Track timing between calls
	if !m.lastCallTime.IsZero() {
		m.delayBetweenCalls = time.Since(m.lastCallTime)
	}
	m.lastCallTime = time.Now()

	if m.currentCall >= len(m.outputs) {
		return TestCommandOutput{ExitCode: 1}
	}

	output := m.outputs[m.currentCall]
	m.currentCall++
	return output
}

// TestExecutor for testing (simplified version of real executor)
type TestExecutor struct {
	MaxAttempts     int
	Runner          *TestCommandRunner
	BackoffStrategy Strategy
}

func (e *TestExecutor) Run(command []string) (*TestResult, error) {
	for attempt := 1; attempt <= e.MaxAttempts; attempt++ {
		output := e.Runner.getNextOutput()

		// Check if command succeeded
		if output.ExitCode == 0 {
			return &TestResult{
				Success:      true,
				AttemptCount: attempt,
				ExitCode:     output.ExitCode,
			}, nil
		}

		// If this was the last attempt, return failure
		if attempt == e.MaxAttempts {
			break
		}

		// Calculate delay using backoff strategy
		if e.BackoffStrategy != nil {
			delay := e.BackoffStrategy.Delay(attempt)
			if delay > 0 {
				time.Sleep(delay)
			}
		}
	}

	return &TestResult{
		Success:      false,
		AttemptCount: e.MaxAttempts,
		ExitCode:     1,
	}, nil
}

type TestResult struct {
	Success      bool
	AttemptCount int
	ExitCode     int
}
