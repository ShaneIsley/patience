package backoff

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HTTPAware implements an HTTP-aware adaptive backoff strategy that respects
// server-specified retry timing from HTTP responses
type HTTPAware struct {
	fallbackStrategy Strategy
	maxRetryAfter    time.Duration
	lastRetryAfter   time.Duration

	// Compiled regex patterns for performance
	retryAfterPattern     *regexp.Regexp
	rateLimitPattern      *regexp.Regexp
	rateLimitResetPattern *regexp.Regexp
}

// NewHTTPAware creates a new HTTP-aware backoff strategy
// fallback is the strategy to use when no HTTP timing information is available
// maxRetryAfter is the maximum delay to respect from server responses
func NewHTTPAware(fallback Strategy, maxRetryAfter time.Duration) *HTTPAware {
	return &HTTPAware{
		fallbackStrategy:      fallback,
		maxRetryAfter:         maxRetryAfter,
		lastRetryAfter:        0,
		retryAfterPattern:     regexp.MustCompile(`(?i)retry-after:\s*(\d+)`),
		rateLimitPattern:      regexp.MustCompile(`(?i)x-ratelimit-retry-after:\s*(\d+)`),
		rateLimitResetPattern: regexp.MustCompile(`(?i)x-ratelimit-reset:\s*(\d+)`),
	}
}

// Delay returns the delay for the given attempt, using HTTP timing if available
func (h *HTTPAware) Delay(attempt int) time.Duration {
	// If we have HTTP timing information, use it
	if h.lastRetryAfter > 0 {
		return h.lastRetryAfter
	}

	// Otherwise, fall back to the base strategy
	return h.fallbackStrategy.Delay(attempt)
}

// ProcessCommandOutput analyzes command output to extract HTTP retry timing
func (h *HTTPAware) ProcessCommandOutput(stdout, stderr string, exitCode int) {
	// Reset previous timing
	h.lastRetryAfter = 0

	// Memory optimization: Limit processing to first 10KB of output to prevent memory issues
	const maxProcessingSize = 10 * 1024
	if len(stdout) > maxProcessingSize {
		stdout = stdout[:maxProcessingSize]
	}
	if len(stderr) > maxProcessingSize {
		stderr = stderr[:maxProcessingSize]
	}

	// Check both stdout and stderr for HTTP responses
	output := stdout + "\n" + stderr

	// Try to extract retry timing from various sources
	if delay := h.parseRetryAfterHeader(output); delay > 0 {
		h.lastRetryAfter = h.capDelay(delay)
		return
	}

	if delay := h.parseRateLimitHeaders(output); delay > 0 {
		h.lastRetryAfter = h.capDelay(delay)
		return
	}

	if delay := h.parseJSONResponse(output); delay > 0 {
		h.lastRetryAfter = h.capDelay(delay)
		return
	}
}

// SetFallbackStrategy sets the fallback strategy to use when no HTTP timing is available
func (h *HTTPAware) SetFallbackStrategy(strategy Strategy) {
	h.fallbackStrategy = strategy
}

// parseRetryAfterHeader extracts delay from standard Retry-After header
func (h *HTTPAware) parseRetryAfterHeader(output string) time.Duration {
	matches := h.retryAfterPattern.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0
	}

	seconds, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

// parseRateLimitHeaders extracts delay from rate limit headers
func (h *HTTPAware) parseRateLimitHeaders(output string) time.Duration {
	// Try X-RateLimit-Retry-After first
	matches := h.rateLimitPattern.FindStringSubmatch(output)
	if len(matches) >= 2 {
		seconds, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil {
			return time.Duration(seconds) * time.Second
		}
	}

	// Try X-RateLimit-Reset (Unix timestamp)
	matches = h.rateLimitResetPattern.FindStringSubmatch(output)
	if len(matches) >= 2 {
		timestamp, err := strconv.ParseInt(strings.TrimSpace(matches[1]), 10, 64)
		if err == nil {
			resetTime := time.Unix(timestamp, 0)
			delay := time.Until(resetTime)
			if delay > 0 {
				return delay
			}
		}
	}

	return 0
}

// parseJSONResponse extracts retry timing from JSON response bodies
func (h *HTTPAware) parseJSONResponse(output string) time.Duration {
	// Limit search to first 10KB to avoid processing huge outputs
	const maxSearchSize = 10 * 1024
	if len(output) > maxSearchSize {
		output = output[:maxSearchSize]
	}

	// Look for JSON-like content
	if !strings.Contains(output, "{") {
		return 0
	}

	// Try to find balanced JSON objects and parse each one
	// This is more robust than first-{ to last-} which can span unrelated content
	retryFields := []string{"retry_after", "retry_after_seconds", "retryAfter", "retryAfterSeconds", "retry_in"}

	start := 0
	for {
		// Find next potential JSON start
		jsonStart := strings.Index(output[start:], "{")
		if jsonStart == -1 {
			break
		}
		jsonStart += start

		// Find matching closing brace using brace counting
		jsonEnd := findMatchingBrace(output, jsonStart)
		if jsonEnd == -1 {
			start = jsonStart + 1
			continue
		}

		jsonStr := output[jsonStart : jsonEnd+1]

		// Try to parse this JSON object
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			start = jsonStart + 1
			continue
		}

		// Check for retry timing fields in this JSON object
		for _, field := range retryFields {
			if value, exists := data[field]; exists {
				switch v := value.(type) {
				case float64:
					return time.Duration(v) * time.Second
				case int:
					return time.Duration(v) * time.Second
				case string:
					if seconds, err := strconv.Atoi(v); err == nil {
						return time.Duration(seconds) * time.Second
					}
				}
			}
		}

		// This JSON didn't have retry fields, try next one
		start = jsonEnd + 1
	}

	return 0
}

// findMatchingBrace finds the index of the closing brace that matches the opening brace at start
func findMatchingBrace(s string, start int) int {
	if start >= len(s) || s[start] != '{' {
		return -1
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		c := s[i]

		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1 // No matching brace found
}

// capDelay applies the maximum delay cap
func (h *HTTPAware) capDelay(delay time.Duration) time.Duration {
	if h.maxRetryAfter > 0 && delay > h.maxRetryAfter {
		return h.maxRetryAfter
	}
	return delay
}
