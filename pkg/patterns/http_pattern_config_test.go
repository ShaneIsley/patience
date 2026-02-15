package patterns

import (
	"testing"
	"time"
)

func TestHTTPPatternConfig_DefaultConfiguration(t *testing.T) {
	config := DefaultHTTPPatternConfig()

	// Verify default settings
	if !config.EnableStatusRouting {
		t.Error("DefaultHTTPPatternConfig() should enable status routing by default")
	}

	if !config.EnableHeaderRouting {
		t.Error("DefaultHTTPPatternConfig() should enable header routing by default")
	}

	if !config.EnableAPIDetection {
		t.Error("DefaultHTTPPatternConfig() should enable API detection by default")
	}

	// Verify default patterns exist
	if len(config.StatusPatterns) == 0 {
		t.Error("DefaultHTTPPatternConfig() should include default status patterns")
	}

	if len(config.HeaderPatterns) == 0 {
		t.Error("DefaultHTTPPatternConfig() should include default header patterns")
	}

	if len(config.APIPatterns) == 0 {
		t.Error("DefaultHTTPPatternConfig() should include default API patterns")
	}
}

func TestHTTPPatternConfig_CustomConfiguration(t *testing.T) {
	config := HTTPPatternConfig{
		EnableStatusRouting: true,
		EnableHeaderRouting: false,
		EnableAPIDetection:  true,
		StatusPatterns: map[int]string{
			200: "$.status != null",
			404: "$.error != null",
			500: "$.error != null",
		},
		HeaderPatterns: map[string]string{
			"Content-Type": "content_type_pattern",
		},
		APIPatterns: map[APIType][]string{
			APITypeGitHub: {"github_pattern_1", "github_pattern_2"},
			APITypeAWS:    {"aws_pattern_1"},
		},
		DefaultPattern: "default_fallback",
	}

	matcher, err := NewHTTPPatternMatcher(config)
	if err != nil {
		t.Errorf("NewHTTPPatternMatcher() with custom config error = %v", err)
		return
	}

	// Test that custom configuration is applied
	response := &HTTPResponse{
		StatusCode: 200,
		Body:       `{"status": "success"}`,
	}

	result, err := matcher.MatchHTTPResponse(response)
	if err != nil {
		t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
		return
	}

	// Should use custom status pattern for 200
	if !result.Matched {
		t.Error("HTTPPatternMatcher should match with custom status pattern")
	}
}

func TestHTTPPatternConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		config  HTTPPatternConfig
		wantErr bool
	}{
		{
			name: "Valid configuration",
			config: HTTPPatternConfig{
				EnableStatusRouting: true,
				StatusPatterns: map[int]string{
					200: "$.* != null",
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid status pattern",
			config: HTTPPatternConfig{
				EnableStatusRouting: true,
				StatusPatterns: map[int]string{
					200: "[invalid_regex",
				},
			},
			wantErr: true,
		},
		{
			name: "Empty configuration with routing enabled",
			config: HTTPPatternConfig{
				EnableStatusRouting: true,
				StatusPatterns:      map[int]string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHTTPPatternMatcher(tt.config)

			if tt.wantErr && err == nil {
				t.Error("NewHTTPPatternMatcher() expected error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("NewHTTPPatternMatcher() unexpected error = %v", err)
			}
		})
	}
}

func TestHTTPPatternConfig_PatternPriority(t *testing.T) {
	config := HTTPPatternConfig{
		EnableStatusRouting: true,
		EnableHeaderRouting: true,
		EnableAPIDetection:  true,
		StatusPatterns: map[int]string{
			429: "$.message != null",
		},
		HeaderPatterns: map[string]string{
			"X-RateLimit-Limit": "specific_rate_limit",
		},
		APIPatterns: map[APIType][]string{
			APITypeGitHub: {"github_rate_limit"},
		},
	}

	matcher, err := NewHTTPPatternMatcher(config)
	if err != nil {
		t.Errorf("NewHTTPPatternMatcher() error = %v", err)
		return
	}

	// Test priority: Header-specific (highest) > API-specific > Status-specific (lowest)
	response := &HTTPResponse{
		StatusCode: 429,
		Headers: map[string]string{
			"X-RateLimit-Limit":   "5000",
			"X-GitHub-Media-Type": "github.v3",
		},
		Body: `{"message": "API rate limit exceeded"}`,
		URL:  "https://api.github.com/user",
	}

	result, err := matcher.MatchHTTPResponse(response)
	if err != nil {
		t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
		return
	}

	if !result.Matched {
		t.Error("HTTPPatternMatcher should match rate limit response")
		return
	}

	// Should use highest priority pattern (header-specific)
	if result.PatternName != "specific_rate_limit" {
		t.Errorf("HTTPPatternMatcher should use highest priority (header) pattern, got %s", result.PatternName)
	}
}

func TestHTTPPatternConfig_LoadFromYAML(t *testing.T) {
	yamlConfig := `
enable_status_routing: true
enable_header_routing: true
enable_api_detection: true
status_patterns:
  200: "success_pattern"
  404: "not_found_pattern"
  500: "server_error_pattern"
header_patterns:
  "Content-Type": "content_type_pattern"
  "X-RateLimit-Limit": "rate_limit_pattern"
api_patterns:
  github:
    - "github_pattern_1"
    - "github_pattern_2"
  aws:
    - "aws_pattern_1"
default_pattern: "fallback_pattern"
`

	config, err := LoadHTTPPatternConfigFromYAML([]byte(yamlConfig))
	if err != nil {
		t.Errorf("LoadHTTPPatternConfigFromYAML() error = %v", err)
		return
	}

	if !config.EnableStatusRouting {
		t.Error("LoadHTTPPatternConfigFromYAML() should enable status routing")
	}

	if !config.EnableHeaderRouting {
		t.Error("LoadHTTPPatternConfigFromYAML() should enable header routing")
	}

	if !config.EnableAPIDetection {
		t.Error("LoadHTTPPatternConfigFromYAML() should enable API detection")
	}

	if len(config.StatusPatterns) != 3 {
		t.Errorf("LoadHTTPPatternConfigFromYAML() status patterns count = %d, want 3", len(config.StatusPatterns))
	}

	if len(config.HeaderPatterns) != 2 {
		t.Errorf("LoadHTTPPatternConfigFromYAML() header patterns count = %d, want 2", len(config.HeaderPatterns))
	}

	if len(config.APIPatterns[APITypeGitHub]) != 2 {
		t.Errorf("LoadHTTPPatternConfigFromYAML() GitHub patterns count = %d, want 2", len(config.APIPatterns[APITypeGitHub]))
	}

	if config.DefaultPattern != "fallback_pattern" {
		t.Errorf("LoadHTTPPatternConfigFromYAML() default pattern = %s, want fallback_pattern", config.DefaultPattern)
	}
}

func TestHTTPPatternConfig_Integration(t *testing.T) {
	// Test integration with existing pattern system
	config := DefaultHTTPPatternConfig()

	matcher, err := NewHTTPPatternMatcher(config)
	if err != nil {
		t.Errorf("NewHTTPPatternMatcher() error = %v", err)
		return
	}

	// Test various HTTP responses
	testCases := []struct {
		name     string
		response *HTTPResponse
		expected bool
	}{
		{
			name: "GitHub API Success",
			response: &HTTPResponse{
				StatusCode: 200,
				Headers: map[string]string{
					"Content-Type":        "application/json",
					"X-GitHub-Media-Type": "github.v3",
				},
				Body: `{"login": "octocat", "id": 1}`,
				URL:  "https://api.github.com/user",
			},
			expected: true,
		},
		{
			name: "AWS API Error",
			response: &HTTPResponse{
				StatusCode: 400,
				Headers: map[string]string{
					"Content-Type":     "application/x-amz-json-1.1",
					"X-Amzn-ErrorType": "ValidationException",
				},
				Body: `{"__type": "ValidationException", "message": "Invalid parameter"}`,
				URL:  "https://dynamodb.us-east-1.amazonaws.com/",
			},
			expected: true,
		},
		{
			name: "Kubernetes API Forbidden",
			response: &HTTPResponse{
				StatusCode: 403,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{
					"kind": "Status",
					"status": "Failure",
					"reason": "Forbidden",
					"code": 403
				}`,
				URL: "https://kubernetes.default.svc/api/v1/pods",
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := matcher.MatchHTTPResponse(tc.response)
			if err != nil {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() error = %v", err)
				return
			}

			if result.Matched != tc.expected {
				t.Errorf("HTTPPatternMatcher.MatchHTTPResponse() matched = %v, want %v", result.Matched, tc.expected)
			}

			if result.Matched {
				// Verify API type detection
				apiType := matcher.DetectAPIType(tc.response)
				if apiType == APITypeUnknown {
					t.Error("HTTPPatternMatcher should detect API type")
				}

				// Verify performance
				if result.MatchTime > 100*time.Microsecond {
					t.Errorf("HTTPPatternMatcher performance %v exceeds 100µs target", result.MatchTime)
				}
			}
		})
	}
}
