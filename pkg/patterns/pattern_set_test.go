package patterns

import (
	"testing"
	"time"
)

func TestPatternSet_LoadFromYAML(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
		wantErr  bool
		expected int // number of patterns expected
	}{
		{
			name: "Valid GitHub pattern set",
			yamlData: `
name: github
description: GitHub API patterns
version: "1.0"
patterns:
  rate_limit_exceeded:
    description: "GitHub API rate limit exceeded"
    pattern: "$.message =~ \"rate limit\""
    priority: high
    category: error
  api_success:
    description: "GitHub API successful response"
    pattern: "$.status == \"success\""
    priority: medium
    category: success
`,
			wantErr:  false,
			expected: 2,
		},
		{
			name: "Invalid YAML syntax",
			yamlData: `
name: invalid
patterns:
  - invalid: yaml: syntax
`,
			wantErr:  true,
			expected: 0,
		},
		{
			name: "Missing required fields",
			yamlData: `
name: incomplete
patterns:
  missing_pattern:
    description: "Missing pattern field"
    priority: high
`,
			wantErr:  true,
			expected: 0,
		},
		{
			name: "Invalid pattern syntax",
			yamlData: `
name: invalid_pattern
patterns:
  bad_pattern:
    pattern: "$..invalid..syntax"
    description: "Invalid JSONPath"
`,
			wantErr:  true,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patternSet, err := LoadPatternSetFromYAML([]byte(tt.yamlData))

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadPatternSetFromYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(patternSet.Patterns) != tt.expected {
					t.Errorf("LoadPatternSetFromYAML() loaded %d patterns, want %d", len(patternSet.Patterns), tt.expected)
				}

				if patternSet.Name == "" {
					t.Error("LoadPatternSetFromYAML() pattern set name is empty")
				}

				if patternSet.Version == "" {
					t.Error("LoadPatternSetFromYAML() pattern set version is empty")
				}
			}
		})
	}
}

func TestPatternSet_PredefinedSets(t *testing.T) {
	tests := []struct {
		name        string
		setName     string
		wantErr     bool
		minPatterns int
	}{
		{
			name:        "GitHub pattern set",
			setName:     "github",
			wantErr:     false,
			minPatterns: 5, // Should have at least 5 common GitHub patterns
		},
		{
			name:        "AWS pattern set",
			setName:     "aws",
			wantErr:     false,
			minPatterns: 8, // Should have at least 8 common AWS patterns
		},
		{
			name:        "Kubernetes pattern set",
			setName:     "kubernetes",
			wantErr:     false,
			minPatterns: 6, // Should have at least 6 common K8s patterns
		},
		{
			name:        "Docker pattern set",
			setName:     "docker",
			wantErr:     false,
			minPatterns: 4, // Should have at least 4 common Docker patterns
		},
		{
			name:        "Non-existent pattern set",
			setName:     "nonexistent",
			wantErr:     true,
			minPatterns: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patternSet, err := LoadPredefinedPatternSet(tt.setName)

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadPredefinedPatternSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(patternSet.Patterns) < tt.minPatterns {
					t.Errorf("LoadPredefinedPatternSet() loaded %d patterns, want at least %d", len(patternSet.Patterns), tt.minPatterns)
				}

				// Validate that all patterns are valid
				for name, pattern := range patternSet.Patterns {
					if pattern.Pattern == "" {
						t.Errorf("Pattern %s has empty pattern string", name)
					}

					if pattern.Description == "" {
						t.Errorf("Pattern %s has empty description", name)
					}

					// Try to create a matcher to validate syntax
					_, err := NewJSONPatternMatcher(pattern.Pattern)
					if err != nil {
						t.Errorf("Pattern %s has invalid syntax: %v", name, err)
					}
				}
			}
		})
	}
}

func TestPatternSet_MatchWithSet(t *testing.T) {
	// Create a test pattern set
	patternSet := &PatternSet{
		Name:        "test",
		Version:     "1.0",
		Description: "Test pattern set",
		Patterns: map[string]PatternDefinition{
			"api_error": {
				Pattern:     "$.error.code >= 400",
				Description: "API error response",
				Priority:    PriorityHigh,
				Category:    CategoryError,
			},
			"rate_limit": {
				Pattern:     "$.message =~ \"rate limit\"",
				Description: "Rate limit exceeded",
				Priority:    PriorityCritical,
				Category:    CategoryError,
			},
			"success": {
				Pattern:     "$.status == \"ok\"",
				Description: "Successful response",
				Priority:    PriorityMedium,
				Category:    CategorySuccess,
			},
		},
	}

	tests := []struct {
		name          string
		jsonInput     string
		expectedMatch string // expected pattern name that should match
		shouldMatch   bool
	}{
		{
			name:          "API error match",
			jsonInput:     `{"error": {"code": 404, "message": "Not found"}}`,
			expectedMatch: "api_error",
			shouldMatch:   true,
		},
		{
			name:          "Rate limit match",
			jsonInput:     `{"message": "Rate limit exceeded", "retry_after": 60}`,
			expectedMatch: "rate_limit",
			shouldMatch:   true,
		},
		{
			name:          "Success match",
			jsonInput:     `{"status": "ok", "data": {"id": 123}}`,
			expectedMatch: "success",
			shouldMatch:   true,
		},
		{
			name:          "No match",
			jsonInput:     `{"status": "pending", "data": null}`,
			expectedMatch: "",
			shouldMatch:   false,
		},
		{
			name:          "Multiple potential matches - should return highest priority",
			jsonInput:     `{"error": {"code": 429}, "message": "Rate limit exceeded"}`,
			expectedMatch: "rate_limit", // Both could match, but rate_limit is more specific
			shouldMatch:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := patternSet.Match(tt.jsonInput)
			if err != nil {
				t.Errorf("PatternSet.Match() error = %v", err)
				return
			}

			if tt.shouldMatch {
				if result == nil {
					t.Error("PatternSet.Match() returned nil, expected a match")
					return
				}

				if result.PatternName != tt.expectedMatch {
					t.Errorf("PatternSet.Match() matched pattern %s, want %s", result.PatternName, tt.expectedMatch)
				}

				if result.Matched != true {
					t.Error("PatternSet.Match() result.Matched = false, want true")
				}
			} else {
				if result != nil && result.Matched {
					t.Errorf("PatternSet.Match() unexpectedly matched pattern %s", result.PatternName)
				}
			}
		})
	}
}

func TestPatternSet_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name        string
		patternSet  string
		jsonInput   string
		expectMatch bool
		expectName  string
	}{
		{
			name:       "GitHub API rate limit",
			patternSet: "github",
			jsonInput: `{
				"message": "API rate limit exceeded for user",
				"documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting"
			}`,
			expectMatch: true,
			expectName:  "rate_limit_exceeded",
		},
		{
			name:       "AWS throttling error",
			patternSet: "aws",
			jsonInput: `{
				"__type": "ThrottlingException",
				"message": "Rate exceeded"
			}`,
			expectMatch: true,
			expectName:  "throttling_error",
		},
		{
			name:       "Kubernetes pod pending",
			patternSet: "kubernetes",
			jsonInput: `{
				"kind": "Pod",
				"status": {
					"phase": "Pending",
					"conditions": [
						{
							"type": "PodScheduled",
							"status": "False",
							"reason": "Unschedulable"
						}
					]
				}
			}`,
			expectMatch: true,
			expectName:  "pod_pending",
		},
		{
			name:       "Docker registry authentication error",
			patternSet: "docker",
			jsonInput: `{
				"errors": [
					{
						"code": "UNAUTHORIZED",
						"message": "authentication required"
					}
				]
			}`,
			expectMatch: true,
			expectName:  "auth_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patternSet, err := LoadPredefinedPatternSet(tt.patternSet)
			if err != nil {
				t.Fatalf("LoadPredefinedPatternSet() error = %v", err)
			}

			result, err := patternSet.Match(tt.jsonInput)
			if err != nil {
				t.Errorf("PatternSet.Match() error = %v", err)
				return
			}

			if tt.expectMatch {
				if result == nil || !result.Matched {
					t.Error("PatternSet.Match() expected match but got none")
					return
				}

				if result.PatternName != tt.expectName {
					t.Errorf("PatternSet.Match() matched %s, want %s", result.PatternName, tt.expectName)
				}
			} else {
				if result != nil && result.Matched {
					t.Errorf("PatternSet.Match() unexpectedly matched %s", result.PatternName)
				}
			}
		})
	}
}

func TestPatternSet_Performance(t *testing.T) {
	// Load a pattern set with many patterns
	patternSet, err := LoadPredefinedPatternSet("github")
	if err != nil {
		t.Fatalf("LoadPredefinedPatternSet() error = %v", err)
	}

	jsonInput := `{
		"message": "API rate limit exceeded",
		"documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting",
		"status": 403
	}`

	// Performance test - should complete within 5ms even with many patterns
	start := time.Now()
	result, err := patternSet.Match(jsonInput)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("PatternSet.Match() error = %v", err)
	}

	if result == nil || !result.Matched {
		t.Error("PatternSet.Match() expected to match GitHub rate limit pattern")
	}

	if duration > 5*time.Millisecond {
		t.Errorf("PatternSet.Match() took %v, want < 5ms", duration)
	}

	t.Logf("Pattern set matching completed in %v", duration)
}

func TestPatternSet_ListAvailableSets(t *testing.T) {
	sets := ListAvailablePatternSets()

	expectedSets := []string{"github", "aws", "kubernetes", "docker"}

	if len(sets) < len(expectedSets) {
		t.Errorf("ListAvailablePatternSets() returned %d sets, want at least %d", len(sets), len(expectedSets))
	}

	for _, expected := range expectedSets {
		found := false
		for _, actual := range sets {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListAvailablePatternSets() missing expected set: %s", expected)
		}
	}
}

func TestPatternSet_Validation(t *testing.T) {
	tests := []struct {
		name        string
		patternSet  *PatternSet
		wantErr     bool
		expectedErr string
	}{
		{
			name: "Valid pattern set",
			patternSet: &PatternSet{
				Name:        "test",
				Version:     "1.0",
				Description: "Test set",
				Patterns: map[string]PatternDefinition{
					"test_pattern": {
						Pattern:     "$.status == \"ok\"",
						Description: "Test pattern",
						Priority:    PriorityMedium,
						Category:    CategorySuccess,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Missing name",
			patternSet: &PatternSet{
				Version:     "1.0",
				Description: "Test set",
				Patterns:    map[string]PatternDefinition{},
			},
			wantErr:     true,
			expectedErr: "pattern set name is required",
		},
		{
			name: "Invalid pattern syntax",
			patternSet: &PatternSet{
				Name:        "test",
				Version:     "1.0",
				Description: "Test set",
				Patterns: map[string]PatternDefinition{
					"bad_pattern": {
						Pattern:     "$..invalid..syntax",
						Description: "Bad pattern",
						Priority:    PriorityMedium,
						Category:    CategoryError,
					},
				},
			},
			wantErr:     true,
			expectedErr: "invalid pattern syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.patternSet.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("PatternSet.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.expectedErr != "" {
				if err == nil || !contains(err.Error(), tt.expectedErr) {
					t.Errorf("PatternSet.Validate() error = %v, want error containing %s", err, tt.expectedErr)
				}
			}
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && s[:len(substr)] == substr) ||
		(len(s) > len(substr) && s[len(s)-len(substr):] == substr) ||
		containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
