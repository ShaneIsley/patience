package backoff

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AdaptiveStrategy extends Strategy with the ability to process command output
// and adapt retry timing based on external feedback
type AdaptiveStrategy interface {
	Strategy
	// ProcessCommandOutput analyzes command output to extract retry timing information
	ProcessCommandOutput(stdout, stderr string, exitCode int)
	// SetFallbackStrategy sets the strategy to use when no adaptive timing is available
	SetFallbackStrategy(Strategy)
}

// TestHTTPAwareStrategy_Interface verifies HTTPAware implements required interfaces
func TestHTTPAwareStrategy_Interface(t *testing.T) {
	fallback := NewFixed(time.Second)
	strategy := NewHTTPAware(fallback, 5*time.Minute)

	// Should implement Strategy
	var _ Strategy = strategy

	// Should implement AdaptiveStrategy (new interface)
	var _ AdaptiveStrategy = strategy
}

// TestHTTPAwareStrategy_RetryAfterHeader tests parsing of Retry-After header
func TestHTTPAwareStrategy_RetryAfterHeader(t *testing.T) {
	tests := []struct {
		name          string
		commandOutput string
		expectedDelay time.Duration
		description   string
	}{
		{
			name:          "retry_after_seconds",
			commandOutput: "HTTP/1.1 429 Too Many Requests\r\nRetry-After: 120\r\n\r\n",
			expectedDelay: 120 * time.Second,
			description:   "Should parse Retry-After header with seconds",
		},
		{
			name:          "retry_after_case_insensitive",
			commandOutput: "HTTP/1.1 429 Too Many Requests\r\nretry-after: 60\r\n\r\n",
			expectedDelay: 60 * time.Second,
			description:   "Should parse retry-after header case insensitively",
		},
		{
			name:          "retry_after_with_spaces",
			commandOutput: "HTTP/1.1 429 Too Many Requests\r\nRetry-After:   30   \r\n\r\n",
			expectedDelay: 30 * time.Second,
			description:   "Should handle spaces around Retry-After value",
		},
		{
			name:          "no_retry_after_header",
			commandOutput: "HTTP/1.1 500 Internal Server Error\r\n\r\n",
			expectedDelay: 0,
			description:   "Should return 0 when no Retry-After header present",
		},
		{
			name:          "invalid_retry_after_value",
			commandOutput: "HTTP/1.1 429 Too Many Requests\r\nRetry-After: invalid\r\n\r\n",
			expectedDelay: 0,
			description:   "Should return 0 for invalid Retry-After values",
		},
	}

	fallback := NewFixed(time.Second)
	strategy := NewHTTPAware(fallback, 5*time.Minute)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Process the command output
			strategy.ProcessCommandOutput(tt.commandOutput, "", 429)

			// Get the next delay
			delay := strategy.Delay(1)

			if tt.expectedDelay == 0 {
				// Should fall back to base strategy
				assert.Equal(t, fallback.Delay(1), delay, tt.description)
			} else {
				assert.Equal(t, tt.expectedDelay, delay, tt.description)
			}
		})
	}
}

// TestHTTPAwareStrategy_MaxRetryAfterCap tests the maximum delay cap
func TestHTTPAwareStrategy_MaxRetryAfterCap(t *testing.T) {
	fallback := NewFixed(time.Second)
	maxDelay := 2 * time.Minute
	strategy := NewHTTPAware(fallback, maxDelay)

	// Server requests 10 minute delay
	commandOutput := "HTTP/1.1 429 Too Many Requests\r\nRetry-After: 600\r\n\r\n"
	strategy.ProcessCommandOutput(commandOutput, "", 429)

	delay := strategy.Delay(1)

	// Should be capped at maxDelay
	assert.Equal(t, maxDelay, delay, "Should cap delay at maximum configured value")
}

// TestHTTPAwareStrategy_FallbackBehavior tests fallback to base strategy
func TestHTTPAwareStrategy_FallbackBehavior(t *testing.T) {
	fallback := NewExponential(time.Second, 2.0, 10*time.Second)
	strategy := NewHTTPAware(fallback, 5*time.Minute)

	// No HTTP response processed yet
	delay1 := strategy.Delay(1)
	delay2 := strategy.Delay(2)
	delay3 := strategy.Delay(3)

	// Should match fallback strategy exactly
	assert.Equal(t, fallback.Delay(1), delay1)
	assert.Equal(t, fallback.Delay(2), delay2)
	assert.Equal(t, fallback.Delay(3), delay3)
}

// TestHTTPAwareStrategy_RateLimitHeaders tests various rate limit headers
func TestHTTPAwareStrategy_RateLimitHeaders(t *testing.T) {
	tests := []struct {
		name          string
		commandOutput string
		expectedDelay time.Duration
		description   string
	}{
		{
			name:          "x_ratelimit_retry_after",
			commandOutput: "HTTP/1.1 429 Too Many Requests\r\nX-RateLimit-Retry-After: 45\r\n\r\n",
			expectedDelay: 45 * time.Second,
			description:   "Should parse X-RateLimit-Retry-After header",
		},
		{
			name:          "x_ratelimit_reset_timestamp",
			commandOutput: fmt.Sprintf("HTTP/1.1 429 Too Many Requests\r\nX-RateLimit-Reset: %d\r\n\r\n", time.Now().Add(60*time.Second).Unix()), // Future timestamp
			expectedDelay: 60 * time.Second,                                                                                                      // Should be approximately 60 seconds
			description:   "Should handle X-RateLimit-Reset timestamp",
		},
	}

	fallback := NewFixed(time.Second)
	strategy := NewHTTPAware(fallback, 5*time.Minute)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy.ProcessCommandOutput(tt.commandOutput, "", 429)
			delay := strategy.Delay(1)

			if tt.expectedDelay > 0 {
				if tt.name == "x_ratelimit_reset_timestamp" {
					// For timestamp-based tests, allow some tolerance (±5 seconds)
					assert.InDelta(t, float64(tt.expectedDelay), float64(delay), float64(5*time.Second), tt.description)
				} else {
					assert.Equal(t, tt.expectedDelay, delay, tt.description)
				}
			} else {
				// For timestamp-based tests, just verify it's not the fallback
				assert.NotEqual(t, fallback.Delay(1), delay, tt.description)
			}
		})
	}
}

// TestHTTPAwareStrategy_JSONResponseParsing tests JSON response parsing
func TestHTTPAwareStrategy_JSONResponseParsing(t *testing.T) {
	tests := []struct {
		name          string
		commandOutput string
		expectedDelay time.Duration
		description   string
	}{
		{
			name:          "json_retry_after",
			commandOutput: `{"error": "rate_limited", "retry_after": 90}`,
			expectedDelay: 90 * time.Second,
			description:   "Should parse retry_after from JSON response",
		},
		{
			name:          "json_retry_after_seconds",
			commandOutput: `{"message": "Too many requests", "retry_after_seconds": 120}`,
			expectedDelay: 120 * time.Second,
			description:   "Should parse retry_after_seconds from JSON response",
		},
		{
			name:          "json_no_retry_info",
			commandOutput: `{"error": "server_error", "message": "Internal error"}`,
			expectedDelay: 0,
			description:   "Should fall back when no retry info in JSON",
		},
	}

	fallback := NewFixed(time.Second)
	strategy := NewHTTPAware(fallback, 5*time.Minute)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy.ProcessCommandOutput(tt.commandOutput, "", 429)
			delay := strategy.Delay(1)

			if tt.expectedDelay == 0 {
				assert.Equal(t, fallback.Delay(1), delay, tt.description)
			} else {
				assert.Equal(t, tt.expectedDelay, delay, tt.description)
			}
		})
	}
}

// TestHTTPAwareStrategy_MultipleAttempts tests behavior across multiple attempts
func TestHTTPAwareStrategy_MultipleAttempts(t *testing.T) {
	fallback := NewExponential(time.Second, 2.0, 10*time.Second)
	strategy := NewHTTPAware(fallback, 5*time.Minute)

	// First attempt - no HTTP info, should use fallback
	delay1 := strategy.Delay(1)
	assert.Equal(t, fallback.Delay(1), delay1)

	// Process HTTP response with retry info
	commandOutput := "HTTP/1.1 429 Too Many Requests\r\nRetry-After: 30\r\n\r\n"
	strategy.ProcessCommandOutput(commandOutput, "", 429)

	// Second attempt - should use HTTP timing
	delay2 := strategy.Delay(2)
	assert.Equal(t, 30*time.Second, delay2)

	// Third attempt - HTTP info should persist until new response
	delay3 := strategy.Delay(3)
	assert.Equal(t, 30*time.Second, delay3)

	// Process new response without retry info
	strategy.ProcessCommandOutput("HTTP/1.1 500 Internal Server Error\r\n\r\n", "", 500)

	// Fourth attempt - should fall back to base strategy
	delay4 := strategy.Delay(4)
	assert.Equal(t, fallback.Delay(4), delay4)
}

// TestNewHTTPAware tests the constructor
func TestNewHTTPAware(t *testing.T) {
	fallback := NewFixed(time.Second)
	maxDelay := 5 * time.Minute

	strategy := NewHTTPAware(fallback, maxDelay)

	require.NotNil(t, strategy)
	assert.Equal(t, maxDelay, strategy.maxRetryAfter)
	assert.Equal(t, fallback, strategy.fallbackStrategy)
}

// TestHTTPAwareStrategy_EdgeCases tests edge cases and error conditions
func TestHTTPAwareStrategy_EdgeCases(t *testing.T) {
	fallback := NewFixed(time.Second)
	strategy := NewHTTPAware(fallback, 5*time.Minute)

	tests := []struct {
		name          string
		commandOutput string
		stderr        string
		exitCode      int
		description   string
	}{
		{
			name:          "empty_output",
			commandOutput: "",
			stderr:        "",
			exitCode:      1,
			description:   "Should handle empty command output gracefully",
		},
		{
			name:          "malformed_http_response",
			commandOutput: "Not an HTTP response at all",
			stderr:        "",
			exitCode:      1,
			description:   "Should handle non-HTTP output gracefully",
		},
		{
			name:          "http_response_in_stderr",
			commandOutput: "",
			stderr:        "HTTP/1.1 429 Too Many Requests\r\nRetry-After: 60\r\n\r\n",
			exitCode:      1,
			description:   "Should parse HTTP response from stderr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			require.NotPanics(t, func() {
				strategy.ProcessCommandOutput(tt.commandOutput, tt.stderr, tt.exitCode)
				delay := strategy.Delay(1)
				// Should either return HTTP timing or fallback, never panic
				assert.True(t, delay >= 0, tt.description)
			})
		})
	}
}
