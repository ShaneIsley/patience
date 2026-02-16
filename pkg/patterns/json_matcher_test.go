package patterns

import (
	"strconv"
	"testing"
	"time"
)

func TestJSONPatternMatcher_BasicMatching(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		pattern  string
		expected bool
		wantErr  bool
	}{
		{
			name:     "Simple string equality",
			json:     `{"status": "success"}`,
			pattern:  `$.status == "success"`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Simple string inequality",
			json:     `{"status": "error"}`,
			pattern:  `$.status == "success"`,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "Numeric equality",
			json:     `{"code": 200}`,
			pattern:  `$.code == 200`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Boolean matching",
			json:     `{"success": true}`,
			pattern:  `$.success == true`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Null value matching",
			json:     `{"error": null}`,
			pattern:  `$.error == null`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Invalid JSON",
			json:     `{"invalid": json}`,
			pattern:  `$.status == "success"`,
			expected: false,
			wantErr:  true,
		},
		{
			name:     "Invalid JSONPath",
			json:     `{"status": "success"}`,
			pattern:  `$..invalid..path == "success"`,
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewJSONPatternMatcher(tt.pattern)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("NewJSONPatternMatcher() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			result, err := matcher.Match(tt.json)
			if (err != nil) != tt.wantErr {
				t.Errorf("JSONPatternMatcher.Match() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result != tt.expected {
				t.Errorf("JSONPatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestJSONPatternMatcher_NestedPatterns(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		pattern  string
		expected bool
	}{
		{
			name:     "Nested object access",
			json:     `{"response": {"data": {"status": "ok"}}}`,
			pattern:  `$.response.data.status == "ok"`,
			expected: true,
		},
		{
			name:     "Deep nesting",
			json:     `{"a": {"b": {"c": {"d": "value"}}}}`,
			pattern:  `$.a.b.c.d == "value"`,
			expected: true,
		},
		{
			name:     "Missing nested field",
			json:     `{"response": {"data": {}}}`,
			pattern:  `$.response.data.status == "ok"`,
			expected: false,
		},
		{
			name:     "Nested array access",
			json:     `{"items": [{"name": "item1"}, {"name": "item2"}]}`,
			pattern:  `$.items[0].name == "item1"`,
			expected: true,
		},
		{
			name:     "Array index out of bounds",
			json:     `{"items": [{"name": "item1"}]}`,
			pattern:  `$.items[5].name == "item1"`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewJSONPatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewJSONPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.Match(tt.json)
			if err != nil {
				t.Errorf("JSONPatternMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("JSONPatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestJSONPatternMatcher_ArrayPatterns(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		pattern  string
		expected bool
	}{
		{
			name:     "Array contains value",
			json:     `{"tags": ["production", "web", "api"]}`,
			pattern:  `$.tags[*] == "production"`,
			expected: true,
		},
		{
			name:     "Array does not contain value",
			json:     `{"tags": ["staging", "web", "api"]}`,
			pattern:  `$.tags[*] == "production"`,
			expected: false,
		},
		{
			name:     "Array of objects - any match",
			json:     `{"users": [{"status": "active"}, {"status": "inactive"}]}`,
			pattern:  `$.users[*].status == "active"`,
			expected: true,
		},
		{
			name:     "Array of objects - no match",
			json:     `{"users": [{"status": "inactive"}, {"status": "suspended"}]}`,
			pattern:  `$.users[*].status == "active"`,
			expected: false,
		},
		{
			name:     "Empty array",
			json:     `{"items": []}`,
			pattern:  `$.items[*] == "anything"`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewJSONPatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewJSONPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.Match(tt.json)
			if err != nil {
				t.Errorf("JSONPatternMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("JSONPatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestJSONPatternMatcher_ComparisonOperators(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		pattern  string
		expected bool
	}{
		{
			name:     "Greater than",
			json:     `{"count": 10}`,
			pattern:  `$.count > 5`,
			expected: true,
		},
		{
			name:     "Less than",
			json:     `{"count": 3}`,
			pattern:  `$.count < 5`,
			expected: true,
		},
		{
			name:     "Greater than or equal",
			json:     `{"count": 5}`,
			pattern:  `$.count >= 5`,
			expected: true,
		},
		{
			name:     "Less than or equal",
			json:     `{"count": 5}`,
			pattern:  `$.count <= 5`,
			expected: true,
		},
		{
			name:     "Not equal",
			json:     `{"status": "error"}`,
			pattern:  `$.status != "success"`,
			expected: true,
		},
		{
			name:     "Regex match",
			json:     `{"message": "Rate limit exceeded"}`,
			pattern:  `$.message =~ "rate limit"`,
			expected: true,
		},
		{
			name:     "Regex no match",
			json:     `{"message": "Success"}`,
			pattern:  `$.message =~ "rate limit"`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewJSONPatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewJSONPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.Match(tt.json)
			if err != nil {
				t.Errorf("JSONPatternMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("JSONPatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestJSONPatternMatcher_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		pattern  string
		expected bool
		scenario string
	}{
		{
			name: "GitHub API Success",
			json: `{
				"status": "success",
				"data": {
					"login": "octocat",
					"id": 1,
					"type": "User"
				}
			}`,
			pattern:  `$.status == "success"`,
			expected: true,
			scenario: "GitHub API successful response",
		},
		{
			name: "AWS API Error",
			json: `{
				"Error": {
					"Code": "ThrottlingException",
					"Message": "Rate exceeded"
				},
				"ResponseMetadata": {
					"HTTPStatusCode": 429
				}
			}`,
			pattern:  `$.Error.Code == "ThrottlingException"`,
			expected: true,
			scenario: "AWS API throttling error",
		},
		{
			name: "Kubernetes Pod Status",
			json: `{
				"status": {
					"phase": "Running",
					"conditions": [
						{
							"type": "Ready",
							"status": "True"
						}
					]
				}
			}`,
			pattern:  `$.status.phase == "Running"`,
			expected: true,
			scenario: "Kubernetes pod running status",
		},
		{
			name: "Docker Registry Error",
			json: `{
				"errors": [
					{
						"code": "TOOMANYREQUESTS",
						"message": "You have reached your pull rate limit"
					}
				]
			}`,
			pattern:  `$.errors[*].code == "TOOMANYREQUESTS"`,
			expected: true,
			scenario: "Docker Hub rate limit error",
		},
		{
			name: "Generic REST API Success",
			json: `{
				"success": true,
				"data": {
					"items": [
						{"id": 1, "name": "item1"},
						{"id": 2, "name": "item2"}
					]
				},
				"pagination": {
					"total": 2,
					"page": 1
				}
			}`,
			pattern:  `$.success == true`,
			expected: true,
			scenario: "Generic REST API success response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewJSONPatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewJSONPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.Match(tt.json)
			if err != nil {
				t.Errorf("JSONPatternMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("JSONPatternMatcher.Match() = %v, want %v for scenario: %s", result, tt.expected, tt.scenario)
			}
		})
	}
}

func TestJSONPatternMatcher_Performance(t *testing.T) {
	// Large JSON for performance testing
	largeJSON := `{
		"data": {
			"items": [`

	// Generate 1000 items
	for i := 0; i < 1000; i++ {
		if i > 0 {
			largeJSON += ","
		}
		largeJSON += `{"id": ` + strconv.Itoa(i) + `, "status": "active"}`
	}
	largeJSON += `]
		},
		"meta": {
			"total": 1000,
			"status": "success"
		}
	}`

	matcher, err := NewJSONPatternMatcher(`$.meta.status == "success"`)
	if err != nil {
		t.Fatalf("NewJSONPatternMatcher() error = %v", err)
	}

	// Performance test - should complete within 1ms
	start := time.Now()
	result, err := matcher.Match(largeJSON)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("JSONPatternMatcher.Match() error = %v", err)
	}

	if !result {
		t.Errorf("JSONPatternMatcher.Match() = %v, want true", result)
	}

	if duration > 10*time.Millisecond {
		t.Errorf("JSONPatternMatcher.Match() took %v, want < 10ms", duration)
	}

	t.Logf("Performance test completed in %v", duration)
}

func TestJSONPatternMatcher_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		pattern  string
		expected bool
		wantErr  bool
	}{
		{
			name:     "Empty JSON object",
			json:     `{}`,
			pattern:  `$.status == "success"`,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "Empty string",
			json:     ``,
			pattern:  `$.status == "success"`,
			expected: false,
			wantErr:  true,
		},
		{
			name:     "JSON with special characters",
			json:     `{"message": "Hello \"world\" with \n newlines"}`,
			pattern:  `$.message =~ "Hello"`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Very deep nesting",
			json:     `{"a":{"b":{"c":{"d":{"e":{"f":{"g":"deep"}}}}}}}`,
			pattern:  `$.a.b.c.d.e.f.g == "deep"`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Unicode content",
			json:     `{"message": "Hello 世界 🌍"}`,
			pattern:  `$.message =~ "世界"`,
			expected: true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewJSONPatternMatcher(tt.pattern)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("NewJSONPatternMatcher() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			result, err := matcher.Match(tt.json)
			if (err != nil) != tt.wantErr {
				t.Errorf("JSONPatternMatcher.Match() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result != tt.expected {
				t.Errorf("JSONPatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestJSONPatternMatcher_WithContext(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		pattern  string
		context  map[string]interface{}
		expected bool
	}{
		{
			name:     "Context variable substitution",
			json:     `{"status": "success"}`,
			pattern:  `$.status == $ctx.expected_status`,
			context:  map[string]interface{}{"expected_status": "success"},
			expected: true,
		},
		{
			name:     "Context with numeric value",
			json:     `{"count": 42}`,
			pattern:  `$.count == $ctx.threshold`,
			context:  map[string]interface{}{"threshold": 42},
			expected: true,
		},
		{
			name:     "Missing context variable",
			json:     `{"status": "success"}`,
			pattern:  `$.status == $ctx.missing_var`,
			context:  map[string]interface{}{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewJSONPatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewJSONPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.MatchWithContext(tt.json, tt.context)
			if err != nil {
				t.Errorf("JSONPatternMatcher.MatchWithContext() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("JSONPatternMatcher.MatchWithContext() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestJSONPatternMatcher_Metrics(t *testing.T) {
	matcher, err := NewJSONPatternMatcher(`$.status == "success"`)
	if err != nil {
		t.Fatalf("NewJSONPatternMatcher() error = %v", err)
	}

	// Perform several matches
	testCases := []string{
		`{"status": "success"}`,
		`{"status": "error"}`,
		`{"status": "success"}`,
	}

	for _, json := range testCases {
		_, _ = matcher.Match(json)
	}

	metrics := matcher.GetMetrics()

	if metrics.TotalMatches != 3 {
		t.Errorf("Expected 3 total matches, got %d", metrics.TotalMatches)
	}

	if metrics.SuccessfulMatches != 2 {
		t.Errorf("Expected 2 successful matches, got %d", metrics.SuccessfulMatches)
	}

	if metrics.AverageMatchTime <= 0 {
		t.Errorf("Expected positive average match time, got %v", metrics.AverageMatchTime)
	}
}
