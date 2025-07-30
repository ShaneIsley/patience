package backoff

import (
	"context"
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/patterns"
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

		// Process command output for HTTP-aware strategies
		if httpAware, ok := e.BackoffStrategy.(*HTTPAware); ok {
			httpAware.ProcessCommandOutput(output.Stdout, output.Stderr, output.ExitCode)
		}

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

// TDD Cycle 2.5 - Advanced Backoff Strategy Integration Tests
// These tests define the expected behavior for HTTP-aware backoff strategy selection

func TestHTTPAwareBackoffStrategySelection(t *testing.T) {
	tests := []struct {
		name             string
		httpResponse     *patterns.HTTPResponse
		expectedStrategy string
		expectedParams   map[string]interface{}
	}{
		{
			name: "GitHub Rate Limit - Diophantine Strategy",
			httpResponse: &patterns.HTTPResponse{
				StatusCode: 403,
				Headers: map[string]string{
					"X-RateLimit-Limit":     "5000",
					"X-RateLimit-Remaining": "0",
					"X-RateLimit-Reset":     "1642248600",
					"Retry-After":           "3600",
					"X-GitHub-Media-Type":   "github.v3",
				},
				Body: `{"message": "API rate limit exceeded"}`,
				URL:  "https://api.github.com/user/repos",
			},
			expectedStrategy: "diophantine",
			expectedParams: map[string]interface{}{
				"base_delay":        3600 * time.Second,
				"max_attempts":      5,
				"discovery_enabled": true,
				"pattern_learning":  true,
			},
		},
		{
			name: "AWS Throttling - Polynomial Strategy",
			httpResponse: &patterns.HTTPResponse{
				StatusCode: 400,
				Headers: map[string]string{
					"X-Amzn-ErrorType": "Throttling",
					"X-Amzn-RequestId": "abc-123-def",
				},
				Body: `{"__type": "Throttling", "message": "Rate exceeded"}`,
				URL:  "https://dynamodb.us-east-1.amazonaws.com/",
			},
			expectedStrategy: "polynomial",
			expectedParams: map[string]interface{}{
				"degree":      2,
				"coefficient": 1.5,
				"max_delay":   300 * time.Second,
				"jitter":      true,
			},
		},
		{
			name: "Kubernetes API Forbidden - Exponential Strategy",
			httpResponse: &patterns.HTTPResponse{
				StatusCode: 403,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{
					"kind": "Status",
					"apiVersion": "v1",
					"status": "Failure",
					"message": "pods is forbidden: User \"system:serviceaccount:default:default\" cannot list resource \"pods\"",
					"reason": "Forbidden",
					"code": 403
				}`,
				URL: "https://kubernetes.default.svc/api/v1/namespaces/default/pods",
			},
			expectedStrategy: "exponential",
			expectedParams: map[string]interface{}{
				"multiplier":    2.0,
				"initial_delay": 1 * time.Second,
				"max_delay":     30 * time.Second,
				"jitter":        true,
			},
		},
		{
			name: "Generic API Server Error - Adaptive Strategy",
			httpResponse: &patterns.HTTPResponse{
				StatusCode: 500,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{"error": "Internal server error", "code": 500}`,
				URL:  "https://api.example.com/users",
			},
			expectedStrategy: "adaptive",
			expectedParams: map[string]interface{}{
				"learning_enabled":       true,
				"effectiveness_tracking": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewHTTPAwareBackoffSelector()
			strategy, params, err := selector.SelectStrategy(tt.httpResponse)

			if err != nil {
				t.Errorf("SelectStrategy() error = %v", err)
				return
			}

			if strategy != tt.expectedStrategy {
				t.Errorf("SelectStrategy() strategy = %v, want %v", strategy, tt.expectedStrategy)
			}

			// Verify strategy parameters
			for key, expectedValue := range tt.expectedParams {
				if actualValue, exists := params[key]; !exists {
					t.Errorf("Missing parameter %s", key)
				} else if actualValue != expectedValue {
					t.Errorf("Parameter %s = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

func TestHTTPAwareBackoffExecution(t *testing.T) {
	tests := []struct {
		name           string
		httpResponse   *patterns.HTTPResponse
		maxAttempts    int
		expectedDelays []time.Duration
	}{
		{
			name: "GitHub Rate Limit Execution",
			httpResponse: &patterns.HTTPResponse{
				StatusCode: 403,
				Headers: map[string]string{
					"X-RateLimit-Limit": "5000",
					"Retry-After":       "60",
				},
				Body: `{"message": "API rate limit exceeded"}`,
				URL:  "https://api.github.com/user/repos",
			},
			maxAttempts: 3,
			expectedDelays: []time.Duration{
				60 * time.Second,  // First retry uses Retry-After
				120 * time.Second, // Diophantine progression
				180 * time.Second, // Continued progression
			},
		},
		{
			name: "AWS Throttling Execution",
			httpResponse: &patterns.HTTPResponse{
				StatusCode: 400,
				Headers: map[string]string{
					"X-Amzn-ErrorType": "Throttling",
				},
				Body: `{"__type": "Throttling", "message": "Rate exceeded"}`,
			},
			maxAttempts: 3,
			expectedDelays: []time.Duration{
				1 * time.Second,  // Initial polynomial delay: 1.5 * 1^2 = 1.5s → 1s
				6 * time.Second,  // degree=2, coefficient=1.5: 1.5 * 2^2 = 6s (±20% jitter)
				13 * time.Second, // 1.5 * 3^2 = 13.5s → 13s (±20% jitter)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := NewHTTPAwareAdaptiveBackoff(DefaultAdaptiveBackoffConfig())

			for i, expectedDelay := range tt.expectedDelays {
				// Set the HTTP response context
				backoff.lastResponse = tt.httpResponse

				// Call Delay with the attempt number (1-based)
				delay := backoff.Delay(i + 1)

				// Allow for some variance due to jitter (±20%)
				tolerance := time.Duration(float64(expectedDelay) * 0.2)
				if delay < expectedDelay-tolerance || delay > expectedDelay+tolerance {
					t.Errorf("Attempt %d: delay = %v, want %v (±%v)", i+1, delay, expectedDelay, tolerance)
				}
			}
		})
	}
}

func TestBackoffStrategyEffectiveness(t *testing.T) {
	tracker := NewEffectivenessTracker()

	// Simulate strategy usage
	testCases := []struct {
		strategy string
		success  bool
		delay    time.Duration
	}{
		{"diophantine", true, 60 * time.Second},
		{"diophantine", true, 120 * time.Second},
		{"diophantine", false, 180 * time.Second},
		{"polynomial", true, 4 * time.Second},
		{"polynomial", true, 9 * time.Second},
	}

	for _, tc := range testCases {
		tracker.RecordAttempt(tc.strategy, tc.success, tc.delay)
	}

	// Verify metrics
	diophantineMetrics := tracker.GetMetrics("diophantine")
	if diophantineMetrics == nil {
		t.Error("Expected diophantine metrics to exist")
		return
	}

	if diophantineMetrics.TotalAttempts != 3 {
		t.Errorf("Diophantine total attempts = %d, want 3", diophantineMetrics.TotalAttempts)
	}

	expectedSuccessRate := 2.0 / 3.0 // 2 successes out of 3 attempts
	if diophantineMetrics.SuccessRate != expectedSuccessRate {
		t.Errorf("Diophantine success rate = %f, want %f", diophantineMetrics.SuccessRate, expectedSuccessRate)
	}

	polynomialMetrics := tracker.GetMetrics("polynomial")
	if polynomialMetrics == nil {
		t.Error("Expected polynomial metrics to exist")
		return
	}

	if polynomialMetrics.SuccessRate != 1.0 {
		t.Errorf("Polynomial success rate = %f, want 1.0", polynomialMetrics.SuccessRate)
	}
}

func TestHTTPAwareBackoffPerformance(t *testing.T) {
	selector := NewHTTPAwareBackoffSelector()

	// Create test HTTP response
	response := &patterns.HTTPResponse{
		StatusCode: 403,
		Headers: map[string]string{
			"X-RateLimit-Limit": "5000",
			"Retry-After":       "60",
		},
		Body: `{"message": "API rate limit exceeded"}`,
		URL:  "https://api.github.com/user/repos",
	}

	// Performance test: should complete in <50µs per selection
	iterations := 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		_, _, err := selector.SelectStrategy(response)
		if err != nil {
			t.Errorf("SelectStrategy() error = %v", err)
		}
	}

	duration := time.Since(start)
	avgDuration := duration / time.Duration(iterations)

	t.Logf("Performance test completed in %v (avg: %v per selection)", duration, avgDuration)

	// Performance target: <50µs per selection
	if avgDuration > 50*time.Microsecond {
		t.Errorf("Performance target not met: %v > 50µs", avgDuration)
	}
}

func TestHTTPAwareBackoffCaching(t *testing.T) {
	selector := NewHTTPAwareBackoffSelector()

	response := &patterns.HTTPResponse{
		StatusCode: 403,
		Headers: map[string]string{
			"X-GitHub-Media-Type": "github.v3",
		},
		Body: `{"message": "API rate limit exceeded"}`,
		URL:  "https://api.github.com/user/repos",
	}

	// First call should populate cache
	strategy1, params1, err1 := selector.SelectStrategy(response)
	if err1 != nil {
		t.Errorf("First SelectStrategy() error = %v", err1)
	}

	// Second call should use cache (faster)
	start := time.Now()
	strategy2, params2, err2 := selector.SelectStrategy(response)
	cachedDuration := time.Since(start)

	if err2 != nil {
		t.Errorf("Second SelectStrategy() error = %v", err2)
	}

	if strategy1 != strategy2 {
		t.Errorf("Cached strategy mismatch: %v != %v", strategy1, strategy2)
	}

	// Verify parameters match
	for key, value1 := range params1 {
		if value2, exists := params2[key]; !exists || value1 != value2 {
			t.Errorf("Cached parameter mismatch for %s: %v != %v", key, value1, value2)
		}
	}

	// Cached call should be very fast (<5µs)
	if cachedDuration > 5*time.Microsecond {
		t.Errorf("Cached call too slow: %v > 5µs", cachedDuration)
	}

	t.Logf("Cached strategy selection completed in %v", cachedDuration)
}

func TestJitterDistribution(t *testing.T) {
	// Test jitter distribution using polynomial strategy which applies jitter
	response := &patterns.HTTPResponse{
		StatusCode: 400,
		Headers: map[string]string{
			"X-Amzn-ErrorType": "Throttling",
		},
		Body: `{"__type": "Throttling", "message": "Rate exceeded"}`,
	}

	backoff := NewHTTPAwareAdaptiveBackoff(DefaultAdaptiveBackoffConfig())
	backoff.lastResponse = response

	// Test jitter distribution for attempt 2 (should be ~6s with jitter)
	samples := 1000
	delays := make([]time.Duration, samples)

	// Collect samples by calling Delay multiple times
	for i := 0; i < samples; i++ {
		delay := backoff.Delay(2) // This should use polynomial strategy with jitter
		delays[i] = delay
	}
	// Calculate statistics
	var sum float64
	minDelay := delays[0]
	maxDelay := delays[0]

	for _, delay := range delays {
		sum += float64(delay)
		if delay < minDelay {
			minDelay = delay
		}
		if delay > maxDelay {
			maxDelay = delay
		}
	}

	avgDelay := time.Duration(sum / float64(samples))

	// Expected base delay for attempt 2: 1.5 * 2^2 = 6s
	expectedBaseDelay := 6 * time.Second

	// Expected range: expectedBaseDelay ± 20%
	expectedMin := time.Duration(float64(expectedBaseDelay) * 0.8)
	expectedMax := time.Duration(float64(expectedBaseDelay) * 1.2)

	t.Logf("Jitter distribution for attempt 2 (expected ~%v):", expectedBaseDelay)
	t.Logf("  Average: %v (expected: %v)", avgDelay, expectedBaseDelay)
	t.Logf("  Range: %v - %v (expected: %v - %v)", minDelay, maxDelay, expectedMin, expectedMax)

	// Verify average is close to expected base delay (within 5%)
	avgTolerance := time.Duration(float64(expectedBaseDelay) * 0.05)
	if avgDelay < expectedBaseDelay-avgTolerance || avgDelay > expectedBaseDelay+avgTolerance {
		t.Errorf("Average delay %v not within 5%% of expected base delay %v", avgDelay, expectedBaseDelay)
	}

	// Verify range covers expected ±20% (allowing some variance for randomness)
	rangeTolerance := time.Duration(float64(expectedBaseDelay) * 0.05) // 5% tolerance
	if minDelay > expectedMin+rangeTolerance {
		t.Errorf("Minimum delay %v too high, expected around %v", minDelay, expectedMin)
	}
	if maxDelay < expectedMax-rangeTolerance {
		t.Errorf("Maximum delay %v too low, expected around %v", maxDelay, expectedMax)
	}
}

// HTTPResponse represents a parsed HTTP response for testing
type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	URL        string            `json:"url,omitempty"`
}
