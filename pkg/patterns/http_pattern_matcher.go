package patterns

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HTTPResponse represents a parsed HTTP response
type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	URL        string            `json:"url,omitempty"`
}

// HTTPMatchResult represents the result of HTTP pattern matching
type HTTPMatchResult struct {
	Matched     bool                   `json:"matched"`
	PatternName string                 `json:"pattern_name,omitempty"`
	PatternType string                 `json:"pattern_type,omitempty"`
	MatchTime   time.Duration          `json:"match_time"`
	Context     map[string]interface{} `json:"context,omitempty"`
	APIType     APIType                `json:"api_type,omitempty"`
}

// APIType represents the type of API detected
type APIType string

const (
	APITypeGitHub     APIType = "github"
	APITypeAWS        APIType = "aws"
	APITypeKubernetes APIType = "kubernetes"
	APITypeGeneric    APIType = "generic"
	APITypeUnknown    APIType = "unknown"
)

// HTTPPatternConfig represents configuration for HTTP pattern matching
type HTTPPatternConfig struct {
	EnableStatusRouting bool                 `json:"enable_status_routing"`
	EnableHeaderRouting bool                 `json:"enable_header_routing"`
	EnableAPIDetection  bool                 `json:"enable_api_detection"`
	StatusPatterns      map[int]string       `json:"status_patterns"`
	HeaderPatterns      map[string]string    `json:"header_patterns"`
	APIPatterns         map[APIType][]string `json:"api_patterns"`
	DefaultPattern      string               `json:"default_pattern"`
}

// BackoffRecommendation represents a recommended backoff strategy
type BackoffRecommendation struct {
	Strategy     string        `json:"strategy"`
	InitialDelay time.Duration `json:"initial_delay"`
	MaxRetries   int           `json:"max_retries"`
	MaxDelay     time.Duration `json:"max_delay,omitempty"`
}

// HTTPPatternMatcher implements HTTP-aware pattern matching
type HTTPPatternMatcher struct {
	config         HTTPPatternConfig
	statusMatchers map[int]PatternMatcher
	apiDetector    APIDetector
	metrics        MatchMetrics
}

// APIDetector interface for detecting API types
type APIDetector interface {
	DetectAPI(response *HTTPResponse) APIType
}

// DefaultAPIDetector implements API detection based on URL and headers
type DefaultAPIDetector struct{}

// DetectAPI detects the API type from HTTP response
func (d *DefaultAPIDetector) DetectAPI(response *HTTPResponse) APIType {
	if response == nil {
		return APITypeUnknown
	}

	// Check URL patterns
	if response.URL != "" {
		if strings.Contains(response.URL, "api.github.com") || strings.Contains(response.URL, "github.com") {
			return APITypeGitHub
		}
		if strings.Contains(response.URL, "amazonaws.com") {
			return APITypeAWS
		}
		if strings.Contains(response.URL, "kubernetes") || strings.Contains(response.URL, "k8s.io") {
			return APITypeKubernetes
		}
	}

	// Check headers
	if response.Headers != nil {
		if _, exists := response.Headers["X-GitHub-Media-Type"]; exists {
			return APITypeGitHub
		}
		if _, exists := response.Headers["X-Amzn-ErrorType"]; exists {
			return APITypeAWS
		}
		if _, exists := response.Headers["X-Amzn-RequestId"]; exists {
			return APITypeAWS
		}
	}

	// Check body content for Kubernetes
	if strings.Contains(response.Body, `"kind":"Status"`) || strings.Contains(response.Body, `"apiVersion"`) {
		return APITypeKubernetes
	}

	// Default to generic
	return APITypeGeneric
}

// DefaultHTTPPatternConfig returns the default HTTP pattern configuration
func DefaultHTTPPatternConfig() HTTPPatternConfig {
	return HTTPPatternConfig{
		EnableStatusRouting: true,
		EnableHeaderRouting: true,
		EnableAPIDetection:  true,
		StatusPatterns: map[int]string{
			200: "$.* != null",
			400: "$.* != null",
			401: "$.* != null",
			403: "$.* != null",
			404: "$.* != null",
			429: "$.* != null",
			500: "$.* != null",
			502: "$.* != null",
			503: "$.* != null",
		},
		HeaderPatterns: map[string]string{
			"Content-Type":        "json_error",
			"X-RateLimit-Limit":   "rate_limit_detailed",
			"X-GitHub-Media-Type": "github_api_error",
			"X-Amzn-ErrorType":    "aws_throttling",
		}, APIPatterns: map[APIType][]string{
			APITypeGitHub:     {"github_rate_limit", "github_api_error"},
			APITypeAWS:        {"aws_throttling", "aws_access_denied"},
			APITypeKubernetes: {"k8s_forbidden", "k8s_not_found", "k8s_conflict"},
			APITypeGeneric:    {"generic_json_error", "generic_success"},
		},
		DefaultPattern: "generic_json_error",
	}
}

// NewHTTPPatternMatcher creates a new HTTP pattern matcher
func NewHTTPPatternMatcher(config HTTPPatternConfig) (*HTTPPatternMatcher, error) {
	// Validate configuration
	if config.EnableStatusRouting && len(config.StatusPatterns) == 0 {
		return nil, NewPatternError("config_error", "status routing enabled but no status patterns provided", "")
	}

	// Create status matchers
	statusMatchers := make(map[int]PatternMatcher)
	if config.EnableStatusRouting {
		for statusCode, pattern := range config.StatusPatterns {
			matcher, err := NewJSONPatternMatcher(pattern)
			if err != nil {
				return nil, NewPatternError("config_error", fmt.Sprintf("invalid status pattern for %d: %v", statusCode, err), pattern)
			}
			statusMatchers[statusCode] = matcher
		}
	}

	// Header patterns are now pattern names, not JSONPath patterns
	// No need to create matchers for them

	// Create API detector
	var apiDetector APIDetector
	if config.EnableAPIDetection {
		apiDetector = &DefaultAPIDetector{}
	}

	return &HTTPPatternMatcher{
		config:         config,
		statusMatchers: statusMatchers,
		apiDetector:    apiDetector,
		metrics:        MatchMetrics{},
	}, nil
}

// MatchHTTPResponse matches an HTTP response against configured patterns
func (h *HTTPPatternMatcher) MatchHTTPResponse(response *HTTPResponse) (*HTTPMatchResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		h.metrics.TotalMatches++
		h.metrics.TotalMatchTime += duration
		h.metrics.AverageMatchTime = h.metrics.TotalMatchTime / time.Duration(h.metrics.TotalMatches)
		h.metrics.LastMatchTime = time.Now()
	}()

	if response == nil {
		h.metrics.ErrorCount++
		return nil, NewPatternError("matcher_error", "response cannot be nil", "")
	}

	result := &HTTPMatchResult{
		Matched:   false,
		MatchTime: time.Since(start),
		Context:   make(map[string]interface{}),
	}

	// Detect API type
	apiType := h.DetectAPIType(response)
	result.APIType = apiType

	// Try header-specific patterns first (highest priority)
	if h.config.EnableHeaderRouting {
		for header, patternName := range h.config.HeaderPatterns {
			if headerValue, exists := response.Headers[header]; exists {
				// Special handling for Content-Type header
				if header == "Content-Type" {
					if strings.Contains(headerValue, "application/json") && (strings.Contains(response.Body, "error") || strings.Contains(response.Body, "message")) {
						result.Matched = true
						result.PatternName = "json_error"
					} else if strings.Contains(headerValue, "application/xml") && (strings.Contains(response.Body, "error") || strings.Contains(response.Body, "message")) {
						result.Matched = true
						result.PatternName = "xml_error"
					} else if strings.Contains(headerValue, "text/html") && (strings.Contains(response.Body, "error") || strings.Contains(response.Body, "Not Found") || response.StatusCode >= 400) {
						result.Matched = true
						result.PatternName = "html_error"
					}
				} else {
					// For other headers, use the configured pattern name directly
					result.Matched = true
					result.PatternName = patternName
				}

				if result.Matched {
					result.PatternType = h.getPatternTypeFromStatus(response.StatusCode)
					result.Context["matched_header"] = header
					result.Context["api_type"] = string(apiType) // Set API type for header matches too
					result.APIType = apiType                     // Ensure APIType is set for extraction
					h.extractContextFromResponse(response, result)
					h.metrics.SuccessfulMatches++
					return result, nil
				}
			}
		}
	}

	// Try API-specific patterns (medium priority)
	if h.config.EnableAPIDetection && apiType != APITypeUnknown {
		if patterns, exists := h.config.APIPatterns[apiType]; exists {
			for _, patternName := range patterns {
				matched, err := h.matchPattern(response, patternName)
				if err == nil && matched {
					result.Matched = true
					result.PatternName = patternName
					result.PatternType = h.getPatternTypeFromStatus(response.StatusCode)
					result.Context["api_type"] = string(apiType)
					result.APIType = apiType // Ensure APIType is set for extraction
					h.extractContextFromResponse(response, result)
					h.metrics.SuccessfulMatches++
					return result, nil
				}
			}
		}
	}
	// Try status-specific patterns (lowest priority)
	if h.config.EnableStatusRouting {
		if matcher, exists := h.statusMatchers[response.StatusCode]; exists {
			matched, err := matcher.Match(response.Body)
			if err == nil && matched {
				result.Matched = true
				result.PatternName = h.config.StatusPatterns[response.StatusCode]
				result.PatternType = h.getPatternTypeFromStatus(response.StatusCode)
				result.Context["status_code"] = response.StatusCode
				h.extractContextFromResponse(response, result)
				h.metrics.SuccessfulMatches++
				return result, nil
			}
		}
	}

	// Try default pattern as fallback
	if h.config.DefaultPattern != "" {
		matched, err := h.matchPattern(response, h.config.DefaultPattern)
		if err == nil && matched {
			result.Matched = true
			result.PatternName = h.config.DefaultPattern
			result.PatternType = "default"
			h.extractContextFromResponse(response, result)
			h.metrics.SuccessfulMatches++
			return result, nil
		}
	}

	// No match found
	h.metrics.FailedMatches++
	return result, nil
}

// DetectAPIType detects the API type from the HTTP response
func (h *HTTPPatternMatcher) DetectAPIType(response *HTTPResponse) APIType {
	if h.apiDetector != nil {
		return h.apiDetector.DetectAPI(response)
	}
	return APITypeUnknown
}

// GetBackoffRecommendation returns a recommended backoff strategy based on the match result
func (h *HTTPPatternMatcher) GetBackoffRecommendation(result *HTTPMatchResult) BackoffRecommendation {
	if result == nil || !result.Matched {
		return BackoffRecommendation{
			Strategy:     "none",
			InitialDelay: 0,
			MaxRetries:   0,
		}
	}

	switch result.PatternType {
	case "rate_limit":
		// Fixed delay for rate limiting
		delay := 60 * time.Second // Default 1 minute
		if retryAfter, exists := result.Context["retry_after"]; exists {
			if retryAfterStr, ok := retryAfter.(string); ok {
				if retryAfterInt, err := strconv.Atoi(retryAfterStr); err == nil {
					delay = time.Duration(retryAfterInt) * time.Second
				}
			}
		}
		return BackoffRecommendation{
			Strategy:     "fixed",
			InitialDelay: delay,
			MaxRetries:   3,
		}

	case "server_error":
		// Exponential backoff for server errors
		return BackoffRecommendation{
			Strategy:     "exponential",
			InitialDelay: 1 * time.Second,
			MaxRetries:   5,
			MaxDelay:     30 * time.Second,
		}

	case "auth_error":
		// No retry for auth errors
		return BackoffRecommendation{
			Strategy:     "none",
			InitialDelay: 0,
			MaxRetries:   0,
		}

	case "client_error":
		// No retry for client errors
		return BackoffRecommendation{
			Strategy:     "none",
			InitialDelay: 0,
			MaxRetries:   0,
		}

	case "success":
		// No retry needed for success
		return BackoffRecommendation{
			Strategy:     "none",
			InitialDelay: 0,
			MaxRetries:   0,
		}

	default:
		// Default exponential backoff
		return BackoffRecommendation{
			Strategy:     "exponential",
			InitialDelay: 1 * time.Second,
			MaxRetries:   3,
			MaxDelay:     10 * time.Second,
		}
	}
}

// GetMetrics returns matching performance metrics
func (h *HTTPPatternMatcher) GetMetrics() MatchMetrics {
	return h.metrics
}

// ParseHTTPResponse parses a raw HTTP response string
func ParseHTTPResponse(rawResponse string) (*HTTPResponse, error) {
	if rawResponse == "" {
		return nil, NewPatternError("parse_error", "empty HTTP response", "")
	}

	lines := strings.Split(rawResponse, "\n")
	if len(lines) == 0 {
		return nil, NewPatternError("parse_error", "invalid HTTP response format", rawResponse)
	}

	// Parse status line
	statusLine := lines[0]
	if !strings.HasPrefix(statusLine, "HTTP/") {
		return nil, NewPatternError("parse_error", "invalid HTTP status line", statusLine)
	}

	statusCode, err := ExtractStatusCode(rawResponse)
	if err != nil {
		return nil, err
	}

	headers, err := ExtractHeaders(rawResponse)
	if err != nil {
		return nil, err
	}

	body, err := ExtractBody(rawResponse)
	if err != nil {
		return nil, err
	}

	return &HTTPResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

// ExtractStatusCode extracts the status code from an HTTP response
func ExtractStatusCode(response string) (int, error) {
	lines := strings.Split(response, "\n")
	if len(lines) == 0 {
		return 0, NewPatternError("parse_error", "no status line found", response)
	}

	statusLine := lines[0]
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		return 0, NewPatternError("parse_error", "invalid status line format", statusLine)
	}

	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, NewPatternError("parse_error", fmt.Sprintf("invalid status code: %v", err), parts[1])
	}

	return statusCode, nil
}

// ExtractHeaders extracts headers from an HTTP response
func ExtractHeaders(response string) (map[string]string, error) {
	headers := make(map[string]string)
	lines := strings.Split(response, "\n")

	// Skip status line
	for i, line := range lines {
		if i == 0 {
			continue // Skip status line
		}

		// Empty line indicates end of headers
		if strings.TrimSpace(line) == "" {
			break
		}

		// Parse header line
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			headers[key] = value
		}
	}

	return headers, nil
}

// ExtractBody extracts the body from an HTTP response
func ExtractBody(response string) (string, error) {
	lines := strings.Split(response, "\n")

	// Find the empty line that separates headers from body
	bodyStartIndex := -1
	for i, line := range lines {
		if i == 0 {
			continue // Skip status line
		}

		if strings.TrimSpace(line) == "" {
			bodyStartIndex = i + 1
			break
		}
	}

	// No body found
	if bodyStartIndex == -1 || bodyStartIndex >= len(lines) {
		return "", nil
	}

	// Join remaining lines as body
	bodyLines := lines[bodyStartIndex:]
	body := strings.Join(bodyLines, "\n")

	return strings.TrimSpace(body), nil
}

// LoadHTTPPatternConfigFromYAML loads HTTP pattern configuration from YAML
func LoadHTTPPatternConfigFromYAML(data []byte) (HTTPPatternConfig, error) {
	// TODO: Implement YAML configuration loading
	return HTTPPatternConfig{}, fmt.Errorf("not implemented")
}

// extractFromJSONBody extracts API-specific information from JSON response body
func (h *HTTPPatternMatcher) extractFromJSONBody(response *HTTPResponse, result *HTTPMatchResult) {
	if response.Body == "" {
		return
	}

	// Try to parse as JSON
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(response.Body), &jsonData); err != nil {
		return // Not valid JSON, skip extraction
	}

	// Extract based on API type
	switch result.APIType {
	case APITypeGitHub:
		h.extractGitHubContext(jsonData, result)
	case APITypeAWS:
		h.extractAWSContext(jsonData, result)
	case APITypeKubernetes:
		h.extractKubernetesContext(jsonData, result)
	case APITypeGeneric:
		h.extractGenericContext(jsonData, result)
	}

	// Extract common fields
	if status, exists := jsonData["status"]; exists {
		if statusStr, ok := status.(string); ok {
			result.Context["status"] = statusStr
		}
	}
}

// extractGitHubContext extracts GitHub-specific context
func (h *HTTPPatternMatcher) extractGitHubContext(jsonData map[string]interface{}, result *HTTPMatchResult) {
	// GitHub API typically returns rate limit errors with specific structure
	if result.PatternType == "rate_limit" || result.PatternType == "auth_error" {
		result.Context["error_type"] = "rate_limit"
	}
}

// extractAWSContext extracts AWS-specific context
func (h *HTTPPatternMatcher) extractAWSContext(jsonData map[string]interface{}, result *HTTPMatchResult) {
	// AWS APIs use __type field for error types
	if errorType, exists := jsonData["__type"]; exists {
		if errorTypeStr, ok := errorType.(string); ok {
			result.Context["error_type"] = errorTypeStr
		}
	}
}

// extractKubernetesContext extracts Kubernetes-specific context
func (h *HTTPPatternMatcher) extractKubernetesContext(jsonData map[string]interface{}, result *HTTPMatchResult) {
	// Extract error type from reason field
	if reason, exists := jsonData["reason"]; exists {
		if reasonStr, ok := reason.(string); ok {
			result.Context["error_type"] = reasonStr
		}
	}

	// Extract resource information from message
	if message, exists := jsonData["message"]; exists {
		if messageStr, ok := message.(string); ok {
			// Parse Kubernetes error message to extract resource, namespace, user
			h.parseKubernetesMessage(messageStr, result)
		}
	}
}

// extractGenericContext extracts generic API context
func (h *HTTPPatternMatcher) extractGenericContext(jsonData map[string]interface{}, result *HTTPMatchResult) {
	// Extract status field for generic APIs
	if status, exists := jsonData["status"]; exists {
		if statusStr, ok := status.(string); ok {
			result.Context["status"] = statusStr
		}
	}
}

// parseKubernetesMessage parses Kubernetes error messages to extract structured information
func (h *HTTPPatternMatcher) parseKubernetesMessage(message string, result *HTTPMatchResult) {
	// Example: pods is forbidden: User "system:serviceaccount:default:default" cannot list resource "pods" in API group "" in the namespace "default"

	// Extract resource
	resourceRegex := regexp.MustCompile(`resource "([^"]+)"`)
	if matches := resourceRegex.FindStringSubmatch(message); len(matches) > 1 {
		result.Context["resource"] = matches[1]
	}

	// Extract namespace
	namespaceRegex := regexp.MustCompile(`namespace "([^"]+)"`)
	if matches := namespaceRegex.FindStringSubmatch(message); len(matches) > 1 {
		result.Context["namespace"] = matches[1]
	}

	// Extract user
	userRegex := regexp.MustCompile(`User "([^"]+)"`)
	if matches := userRegex.FindStringSubmatch(message); len(matches) > 1 {
		result.Context["user"] = matches[1]
	}
}

// setRetryStrategy sets the retry strategy in the context based on response characteristics
func (h *HTTPPatternMatcher) setRetryStrategy(response *HTTPResponse, result *HTTPMatchResult) {
	// Check for rate limiting indicators first (highest priority)
	if response.Headers != nil {
		// Check for rate limit headers
		if _, hasRateLimit := response.Headers["X-RateLimit-Limit"]; hasRateLimit {
			result.Context["retry_strategy"] = "fixed_delay"
			if retryAfter, exists := response.Headers["Retry-After"]; exists {
				if retryAfterInt, err := strconv.Atoi(retryAfter); err == nil {
					result.Context["retry_after"] = float64(retryAfterInt)
				}
			} else if reset, exists := response.Headers["X-Rate-Limit-Reset"]; exists {
				if resetInt, err := strconv.Atoi(reset); err == nil {
					result.Context["retry_after"] = float64(resetInt)
				}
			}
			return
		}

		// Check for AWS throttling
		if errorType, exists := response.Headers["X-Amzn-ErrorType"]; exists && errorType == "Throttling" {
			result.Context["retry_strategy"] = "exponential"
			return
		}

		// Check for generic rate limiting (429 status)
		if response.StatusCode == 429 {
			result.Context["retry_strategy"] = "fixed_delay"
			if retryAfter, exists := response.Headers["Retry-After"]; exists {
				if retryAfterInt, err := strconv.Atoi(retryAfter); err == nil {
					result.Context["retry_after"] = float64(retryAfterInt)
				}
			} else if reset, exists := response.Headers["X-Rate-Limit-Reset"]; exists {
				if resetInt, err := strconv.Atoi(reset); err == nil {
					result.Context["retry_after"] = float64(resetInt)
				}
			}
			return
		}
	}

	// Fallback to pattern type-based strategy
	switch result.PatternType {
	case "rate_limit":
		result.Context["retry_strategy"] = "fixed_delay"

	case "server_error":
		result.Context["retry_strategy"] = "exponential"

	case "auth_error":
		result.Context["retry_strategy"] = "none"

	case "client_error":
		result.Context["retry_strategy"] = "none"

	default:
		result.Context["retry_strategy"] = "none"
	}
}

// Helper methods for HTTPPatternMatcher

// matchPattern matches a response against a named pattern
func (h *HTTPPatternMatcher) matchPattern(response *HTTPResponse, patternName string) (bool, error) {
	// For now, use simple pattern matching that always matches if there's content
	// In a full implementation, this would route to appropriate pattern types based on patternName
	if response.Body != "" {
		return true, nil
	}
	return false, nil
}

// getPatternTypeFromStatus determines pattern type based on HTTP status code
func (h *HTTPPatternMatcher) getPatternTypeFromStatus(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "success"
	case statusCode == 401 || statusCode == 403:
		return "auth_error"
	case statusCode == 429:
		return "rate_limit"
	case statusCode >= 400 && statusCode < 500:
		return "client_error"
	case statusCode >= 500:
		return "server_error"
	default:
		return "unknown"
	}
}

// extractContextFromResponse extracts additional context from the HTTP response
func (h *HTTPPatternMatcher) extractContextFromResponse(response *HTTPResponse, result *HTTPMatchResult) {
	// Extract rate limiting information from headers
	if response.Headers != nil {
		if limit, exists := response.Headers["X-RateLimit-Limit"]; exists {
			result.Context["rate_limit"] = limit
		}
		if remaining, exists := response.Headers["X-RateLimit-Remaining"]; exists {
			result.Context["remaining"] = remaining
		}
		if reset, exists := response.Headers["X-RateLimit-Reset"]; exists {
			result.Context["reset_time"] = reset
		}
		if retryAfter, exists := response.Headers["Retry-After"]; exists {
			result.Context["retry_after"] = retryAfter
		}
		if errorType, exists := response.Headers["X-Amzn-ErrorType"]; exists {
			result.Context["error_type"] = errorType
		}
		if requestId, exists := response.Headers["X-Amzn-RequestId"]; exists {
			result.Context["request_id"] = requestId
		}
	}

	// Extract information from URL
	if response.URL != "" {
		if strings.Contains(response.URL, "amazonaws.com") {
			// Extract AWS service from URL
			parts := strings.Split(response.URL, ".")
			if len(parts) > 0 {
				service := strings.Split(parts[0], "://")
				if len(service) > 1 {
					result.Context["service"] = service[1]
				}
			}
		}
	}

	// Extract API-specific information from JSON body
	h.extractFromJSONBody(response, result)

	// Set retry strategy based on pattern type and response characteristics
	h.setRetryStrategy(response, result)
}
