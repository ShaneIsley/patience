package patterns

import (
	"strings"
	"testing"
	"time"
)

func TestHTTPPatternMatcher_StatusCodeRouting(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		body         string
		expectedType string
		expected     bool
	}{
		{
			name:         "200 Success Pattern",
			statusCode:   200,
			body:         `{"status": "success", "data": {"id": 123}}`,
			expectedType: "success",
			expected:     true,
		},
		{
			name:         "400 Bad Request Pattern",
			statusCode:   400,
			body:         `{"error": "invalid_request", "message": "Missing required field"}`,
			expectedType: "client_error",
			expected:     true,
		},
		{
			name:         "401 Unauthorized Pattern",
			statusCode:   401,
			body:         `{"error": "unauthorized", "message": "Invalid token"}`,
			expectedType: "auth_error",
			expected:     true,
		},
		{
			name:         "403 Forbidden Pattern",
			statusCode:   403,
			body:         `{"error": "forbidden", "message": "Insufficient permissions"}`,
			expectedType: "auth_error",
			expected:     true,
		},
		{
			name:         "404 Not Found Pattern",
			statusCode:   404,
			body:         `{"error": "not_found", "message": "Resource not found"}`,
			expectedType: "client_error",
			expected:     true,
		},
		{
			name:         "429 Rate Limited Pattern",
			statusCode:   429,
			body:         `{"error": "rate_limited", "message": "Too many requests"}`,
			expectedType: "rate_limit",
			expected:     true,
		},
		{
			name:         "500 Server Error Pattern",
			statusCode:   500,
			body:         `{"error": "internal_error", "message": "Server error"}`,
			expectedType: "server_error",
			expected:     true,
		},
		{
			name:         "502 Bad Gateway Pattern",
			statusCode:   502,
			body:         `<html><body>Bad Gateway</body></html>`,
			expectedType: "server_error",
			expected:     true,
		},
		{
			name:         "503 Service Unavailable Pattern",
			statusCode:   503,
			body:         `{"error": "service_unavailable", "retry_after": 60}`,
			expectedType: "server_error",
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &HTTPResponse{
				StatusCode: tt.statusCode,
				Body:       tt.body,
				Headers:    make(map[string]string),
			}

			matcher, err := NewHTTPPatternMatcher(DefaultHTTPPatternConfig())
			if err != nil {
				t.Errorf("NewHTTPPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.MatchHTTPResponse(response)
			if err != nil {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
				return
			}

			if result.Matched != tt.expected {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() matched = %v, want %v", result.Matched, tt.expected)
			}

			if result.Matched && result.PatternType != tt.expectedType {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() type = %v, want %v", result.PatternType, tt.expectedType)
			}
		})
	}
}

func TestHTTPPatternMatcher_HeaderAwareMatching(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		body        string
		statusCode  int
		expected    bool
		patternName string
	}{
		{
			name: "JSON Content-Type Pattern",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:        `{"error": "validation_failed"}`,
			statusCode:  400,
			expected:    true,
			patternName: "json_error",
		},
		{
			name: "XML Content-Type Pattern",
			headers: map[string]string{
				"Content-Type": "application/xml",
			},
			body:        `<error><code>400</code><message>Bad Request</message></error>`,
			statusCode:  400,
			expected:    true,
			patternName: "xml_error",
		},
		{
			name: "HTML Content-Type Pattern",
			headers: map[string]string{
				"Content-Type": "text/html",
			},
			body:        `<html><body><h1>404 Not Found</h1></body></html>`,
			statusCode:  404,
			expected:    true,
			patternName: "html_error",
		},
		{
			name: "Rate Limit Headers Pattern",
			headers: map[string]string{
				"X-RateLimit-Limit":     "5000",
				"X-RateLimit-Remaining": "0",
				"X-RateLimit-Reset":     "1642248600",
				"Retry-After":           "3600",
			},
			body:        `{"message": "API rate limit exceeded"}`,
			statusCode:  429,
			expected:    true,
			patternName: "rate_limit_detailed",
		},
		{
			name: "Custom API Headers Pattern",
			headers: map[string]string{
				"X-GitHub-Media-Type": "github.v3",
				"X-GitHub-Request-Id": "abc123",
			},
			body:        `{"message": "Not Found"}`,
			statusCode:  404,
			expected:    true,
			patternName: "github_api_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &HTTPResponse{
				StatusCode: tt.statusCode,
				Headers:    tt.headers,
				Body:       tt.body,
			}

			matcher, err := NewHTTPPatternMatcher(DefaultHTTPPatternConfig())
			if err != nil {
				t.Errorf("NewHTTPPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.MatchHTTPResponse(response)
			if err != nil {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
				return
			}

			if result.Matched != tt.expected {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() matched = %v, want %v", result.Matched, tt.expected)
			}

			if result.Matched && result.PatternName != tt.patternName {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() pattern = %v, want %v", result.PatternName, tt.patternName)
			}
		})
	}
}

func TestHTTPPatternMatcher_APISpecificPatterns(t *testing.T) {
	tests := []struct {
		name       string
		apiType    string
		response   *HTTPResponse
		expected   bool
		extraction map[string]interface{}
	}{
		{
			name:    "GitHub API Rate Limit",
			apiType: "github",
			response: &HTTPResponse{
				StatusCode: 403,
				Headers: map[string]string{
					"X-RateLimit-Limit":     "5000",
					"X-RateLimit-Remaining": "0",
					"X-RateLimit-Reset":     "1642248600",
					"X-GitHub-Media-Type":   "github.v3",
				},
				Body: `{
					"message": "API rate limit exceeded for user",
					"documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting"
				}`,
				URL: "https://api.github.com/user/repos",
			},
			expected: true,
			extraction: map[string]interface{}{
				"rate_limit": "5000",
				"remaining":  "0",
				"reset_time": "1642248600",
				"api_type":   "github",
				"error_type": "rate_limit",
			},
		},
		{
			name:    "AWS API Throttling",
			apiType: "aws",
			response: &HTTPResponse{
				StatusCode: 400,
				Headers: map[string]string{
					"Content-Type":     "application/x-amz-json-1.1",
					"X-Amzn-RequestId": "abc-123-def",
					"X-Amzn-ErrorType": "Throttling",
				},
				Body: `{
					"__type": "Throttling",
					"message": "Rate exceeded"
				}`,
				URL: "https://dynamodb.us-east-1.amazonaws.com/",
			},
			expected: true,
			extraction: map[string]interface{}{
				"error_type": "Throttling",
				"request_id": "abc-123-def",
				"api_type":   "aws",
				"service":    "dynamodb",
			},
		},
		{
			name:    "Kubernetes API Forbidden",
			apiType: "kubernetes",
			response: &HTTPResponse{
				StatusCode: 403,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{
					"kind": "Status",
					"apiVersion": "v1",
					"metadata": {},
					"status": "Failure",
					"message": "pods is forbidden: User \"system:serviceaccount:default:default\" cannot list resource \"pods\" in API group \"\" in the namespace \"default\"",
					"reason": "Forbidden",
					"details": {
						"kind": "pods"
					},
					"code": 403
				}`,
				URL: "https://kubernetes.default.svc/api/v1/namespaces/default/pods",
			},
			expected: true,
			extraction: map[string]interface{}{
				"error_type": "Forbidden",
				"resource":   "pods",
				"namespace":  "default",
				"user":       "system:serviceaccount:default:default",
				"api_type":   "kubernetes",
			},
		},
		{
			name:    "Generic REST API Success",
			apiType: "generic",
			response: &HTTPResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{
					"status": "success",
					"data": {
						"id": 123,
						"name": "test"
					}
				}`,
				URL: "https://api.example.com/users/123",
			},
			expected: true,
			extraction: map[string]interface{}{
				"status":   "success",
				"api_type": "generic",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewHTTPPatternMatcher(DefaultHTTPPatternConfig())
			if err != nil {
				t.Errorf("NewHTTPPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.MatchHTTPResponse(tt.response)
			if err != nil {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
				return
			}

			if result.Matched != tt.expected {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() matched = %v, want %v", result.Matched, tt.expected)
			}

			if result.Matched {
				// Verify API type detection
				detectedAPI := matcher.DetectAPIType(tt.response)
				if string(detectedAPI) != tt.apiType {
					t.Errorf("HTTPPatternMatcher.DetectAPIType() = %v, want %v", detectedAPI, tt.apiType)
				}

				// Verify extraction data
				for key, expectedValue := range tt.extraction {
					if actualValue, exists := result.Context[key]; !exists {
						t.Errorf("HTTPPatternMatcher extraction missing key %s", key)
					} else if actualValue != expectedValue {
						t.Errorf("HTTPPatternMatcher extraction %s = %v, want %v", key, actualValue, expectedValue)
					}
				}
			}
		})
	}
}

func TestHTTPPatternMatcher_ResponseParsing(t *testing.T) {
	tests := []struct {
		name            string
		rawResponse     string
		expectedStatus  int
		expectedHeaders map[string]string
		expectedBody    string
		wantErr         bool
	}{
		{
			name: "Complete HTTP Response",
			rawResponse: `HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 45
X-RateLimit-Limit: 5000

{"status": "success", "data": {"id": 123}}`,
			expectedStatus: 200,
			expectedHeaders: map[string]string{
				"Content-Type":      "application/json",
				"Content-Length":    "45",
				"X-RateLimit-Limit": "5000",
			},
			expectedBody: `{"status": "success", "data": {"id": 123}}`,
			wantErr:      false,
		},
		{
			name: "HTTP Response with Error",
			rawResponse: `HTTP/1.1 404 Not Found
Content-Type: application/json
X-GitHub-Media-Type: github.v3

{"message": "Not Found", "documentation_url": "https://docs.github.com"}`,
			expectedStatus: 404,
			expectedHeaders: map[string]string{
				"Content-Type":        "application/json",
				"X-GitHub-Media-Type": "github.v3",
			},
			expectedBody: `{"message": "Not Found", "documentation_url": "https://docs.github.com"}`,
			wantErr:      false,
		},
		{
			name: "HTTP Response without Body",
			rawResponse: `HTTP/1.1 204 No Content
Content-Length: 0

`,
			expectedStatus: 204,
			expectedHeaders: map[string]string{
				"Content-Length": "0",
			},
			expectedBody: "",
			wantErr:      false,
		},
		{
			name:        "Invalid HTTP Response",
			rawResponse: `This is not a valid HTTP response`,
			wantErr:     true,
		},
		{
			name: "HTTP Response with Multi-line Body",
			rawResponse: `HTTP/1.1 500 Internal Server Error
Content-Type: text/plain

Error: Database connection failed
Stack trace:
  at DatabaseConnection.connect()
  at UserService.getUser()`,
			expectedStatus: 500,
			expectedHeaders: map[string]string{
				"Content-Type": "text/plain",
			},
			expectedBody: `Error: Database connection failed
Stack trace:
  at DatabaseConnection.connect()
  at UserService.getUser()`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := ParseHTTPResponse(tt.rawResponse)

			if tt.wantErr {
				if err == nil {
					t.Error("ParseHTTPResponse() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseHTTPResponse() error = %v", err)
				return
			}

			if response.StatusCode != tt.expectedStatus {
				t.Errorf("ParseHTTPResponse() status = %v, want %v", response.StatusCode, tt.expectedStatus)
			}

			for key, expectedValue := range tt.expectedHeaders {
				if actualValue, exists := response.Headers[key]; !exists {
					t.Errorf("ParseHTTPResponse() missing header %s", key)
				} else if actualValue != expectedValue {
					t.Errorf("ParseHTTPResponse() header %s = %v, want %v", key, actualValue, expectedValue)
				}
			}

			if strings.TrimSpace(response.Body) != strings.TrimSpace(tt.expectedBody) {
				t.Errorf("ParseHTTPResponse() body = %v, want %v", response.Body, tt.expectedBody)
			}
		})
	}
}

func TestHTTPPatternMatcher_RateLimitingIntegration(t *testing.T) {
	tests := []struct {
		name               string
		response           *HTTPResponse
		expectedRetryAfter time.Duration
		expectedStrategy   string
		expected           bool
	}{
		{
			name: "GitHub Rate Limit with Retry-After",
			response: &HTTPResponse{
				StatusCode: 403,
				Headers: map[string]string{
					"X-RateLimit-Limit":     "5000",
					"X-RateLimit-Remaining": "0",
					"X-RateLimit-Reset":     "1642248600",
					"Retry-After":           "3600",
				},
				Body: `{"message": "API rate limit exceeded"}`,
			},
			expectedRetryAfter: 3600 * time.Second,
			expectedStrategy:   "fixed_delay",
			expected:           true,
		},
		{
			name: "AWS Throttling with Exponential Backoff",
			response: &HTTPResponse{
				StatusCode: 400,
				Headers: map[string]string{
					"X-Amzn-ErrorType": "Throttling",
				},
				Body: `{"__type": "Throttling", "message": "Rate exceeded"}`,
			},
			expectedRetryAfter: 0, // No specific retry-after
			expectedStrategy:   "exponential",
			expected:           true,
		},
		{
			name: "Custom Rate Limit Headers",
			response: &HTTPResponse{
				StatusCode: 429,
				Headers: map[string]string{
					"X-Rate-Limit-Remaining": "0",
					"X-Rate-Limit-Reset":     "60",
				},
				Body: `{"error": "rate_limited"}`,
			},
			expectedRetryAfter: 60 * time.Second,
			expectedStrategy:   "fixed_delay",
			expected:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewHTTPPatternMatcher(DefaultHTTPPatternConfig())
			if err != nil {
				t.Errorf("NewHTTPPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.MatchHTTPResponse(tt.response)
			if err != nil {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
				return
			}

			if result.Matched != tt.expected {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() matched = %v, want %v", result.Matched, tt.expected)
			}

			if result.Matched {
				// Check retry strategy recommendation
				if strategy, exists := result.Context["retry_strategy"]; !exists {
					t.Error("HTTPPatternMatcher missing retry_strategy in context")
				} else if strategy != tt.expectedStrategy {
					t.Errorf("HTTPPatternMatcher retry_strategy = %v, want %v", strategy, tt.expectedStrategy)
				}

				// Check retry after duration
				if tt.expectedRetryAfter > 0 {
					if retryAfter, exists := result.Context["retry_after"]; !exists {
						t.Error("HTTPPatternMatcher missing retry_after in context")
					} else if retryAfter != tt.expectedRetryAfter.Seconds() {
						t.Errorf("HTTPPatternMatcher retry_after = %v, want %v", retryAfter, tt.expectedRetryAfter.Seconds())
					}
				}
			}
		})
	}
}

func TestHTTPPatternMatcher_Performance(t *testing.T) {
	// Create a complex HTTP response for performance testing
	response := &HTTPResponse{
		StatusCode: 429,
		Headers: map[string]string{
			"Content-Type":          "application/json",
			"X-RateLimit-Limit":     "5000",
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     "1642248600",
			"Retry-After":           "3600",
			"X-GitHub-Media-Type":   "github.v3",
			"X-GitHub-Request-Id":   "abc123def456",
		},
		Body: `{
			"message": "API rate limit exceeded for user",
			"documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting",
			"details": {
				"limit": 5000,
				"remaining": 0,
				"reset": 1642248600
			}
		}`,
		URL: "https://api.github.com/user/repos",
	}

	matcher, err := NewHTTPPatternMatcher(DefaultHTTPPatternConfig())
	if err != nil {
		t.Errorf("NewHTTPPatternMatcher() error = %v", err)
		return
	}

	// Performance test: should complete in <100µs
	start := time.Now()

	for i := 0; i < 100; i++ {
		result, err := matcher.MatchHTTPResponse(response)
		if err != nil {
			t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
			return
		}

		if !result.Matched {
			t.Error("HTTPPatternMatcher.MatchHTTPResponse() expected match")
			return
		}
	}

	duration := time.Since(start)
	avgDuration := duration / 100

	t.Logf("Performance test completed in %v (avg: %v per match)", duration, avgDuration)

	// Target: <100µs per match
	if avgDuration > 100*time.Microsecond {
		t.Errorf("HTTPPatternMatcher performance %v exceeds target of 100µs", avgDuration)
	}
}

func TestHTTPPatternMatcher_BackoffIntegration(t *testing.T) {
	tests := []struct {
		name               string
		response           *HTTPResponse
		expectedBackoff    string
		expectedDelay      time.Duration
		expectedMaxRetries int
	}{
		{
			name: "Rate Limited - Fixed Delay",
			response: &HTTPResponse{
				StatusCode: 429,
				Headers: map[string]string{
					"Retry-After": "60",
				},
				Body: `{"error": "rate_limited"}`,
			},
			expectedBackoff:    "fixed",
			expectedDelay:      60 * time.Second,
			expectedMaxRetries: 3,
		},
		{
			name: "Server Error - Exponential Backoff",
			response: &HTTPResponse{
				StatusCode: 500,
				Body:       `{"error": "internal_server_error"}`,
			},
			expectedBackoff:    "exponential",
			expectedDelay:      1 * time.Second,
			expectedMaxRetries: 5,
		},
		{
			name: "Client Error - No Retry",
			response: &HTTPResponse{
				StatusCode: 400,
				Body:       `{"error": "bad_request"}`,
			},
			expectedBackoff:    "none",
			expectedDelay:      0,
			expectedMaxRetries: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewHTTPPatternMatcher(DefaultHTTPPatternConfig())
			if err != nil {
				t.Errorf("NewHTTPPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.MatchHTTPResponse(tt.response)
			if err != nil {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
				return
			}

			// Get backoff recommendation
			backoffConfig := matcher.GetBackoffRecommendation(result)

			if backoffConfig.Strategy != tt.expectedBackoff {
				t.Errorf("HTTPPatternMatcher backoff strategy = %v, want %v", backoffConfig.Strategy, tt.expectedBackoff)
			}

			if backoffConfig.InitialDelay != tt.expectedDelay {
				t.Errorf("HTTPPatternMatcher initial delay = %v, want %v", backoffConfig.InitialDelay, tt.expectedDelay)
			}

			if backoffConfig.MaxRetries != tt.expectedMaxRetries {
				t.Errorf("HTTPPatternMatcher max retries = %v, want %v", backoffConfig.MaxRetries, tt.expectedMaxRetries)
			}
		})
	}
}

func TestHTTPPatternMatcher_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		response *HTTPResponse
		wantErr  bool
	}{
		{
			name:     "Nil Response",
			response: nil,
			wantErr:  true,
		},
		{
			name:     "Empty Response",
			response: &HTTPResponse{},
			wantErr:  false,
		},
		{
			name: "Very Large Body",
			response: &HTTPResponse{
				StatusCode: 200,
				Body:       strings.Repeat("x", 1024*1024), // 1MB body
			},
			wantErr: false,
		},
		{
			name: "Invalid JSON Body",
			response: &HTTPResponse{
				StatusCode: 400,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{"invalid": json}`,
			},
			wantErr: false, // Should handle gracefully
		},
		{
			name: "Unicode Content",
			response: &HTTPResponse{
				StatusCode: 200,
				Body:       `{"message": "Hello 世界 🌍"}`,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewHTTPPatternMatcher(DefaultHTTPPatternConfig())
			if err != nil {
				t.Errorf("NewHTTPPatternMatcher() error = %v", err)
				return
			}

			_, err = matcher.MatchHTTPResponse(tt.response)

			if tt.wantErr && err == nil {
				t.Error("HTTPPatternMatcher.MatchHTTPResponse() expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() unexpected error = %v", err)
			}
		})
	}
}
