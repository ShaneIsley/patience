package patterns

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// JSONPatternMatcher implements pattern matching for JSON responses
type JSONPatternMatcher struct {
	pattern       string
	jsonPath      string
	operator      ComparisonOperator
	expectedValue interface{}
	compiledRegex *regexp.Regexp
	metrics       MatchMetrics
	mu            sync.RWMutex
}

// NewJSONPatternMatcher creates a new JSON pattern matcher
func NewJSONPatternMatcher(pattern string) (*JSONPatternMatcher, error) {
	matcher := &JSONPatternMatcher{
		pattern: pattern,
		metrics: MatchMetrics{},
	}

	if err := matcher.parsePattern(pattern); err != nil {
		return nil, err
	}

	return matcher, nil
}

// parsePattern parses the pattern string into components
func (j *JSONPatternMatcher) parsePattern(pattern string) error {
	// Simple pattern parsing for JSONPath-like syntax
	// Format: $.path.to.field operator value

	// Find the operator
	operators := map[string]ComparisonOperator{
		"==": OpEqual,
		"!=": OpNotEqual,
		">":  OpGreaterThan,
		"<":  OpLessThan,
		">=": OpGreaterThanOrEqual,
		"<=": OpLessThanOrEqual,
		"=~": OpRegexMatch,
	}

	var foundOp ComparisonOperator
	var parts []string

	for op, opType := range operators {
		if strings.Contains(pattern, " "+op+" ") {
			foundOp = opType
			parts = strings.Split(pattern, " "+op+" ")
			break
		}
	}

	if len(parts) != 2 {
		return NewPatternError("ParseError", "Invalid pattern format", pattern)
	}

	j.jsonPath = strings.TrimSpace(parts[0])
	j.operator = foundOp

	// Validate JSONPath syntax
	if err := j.validateJSONPath(j.jsonPath); err != nil {
		return err
	}

	// Parse expected value
	valueStr := strings.TrimSpace(parts[1])
	if err := j.parseExpectedValue(valueStr); err != nil {
		return err
	}

	// Compile regex if needed
	if j.operator == OpRegexMatch {
		// Add case-insensitive flag for consistent behavior
		regexPattern := "(?i)" + j.expectedValue.(string)
		if regex, err := regexp.Compile(regexPattern); err != nil {
			return NewPatternError("RegexError", "Invalid regex pattern: "+err.Error(), pattern)
		} else {
			j.compiledRegex = regex
		}
	}

	return nil
}

// validateJSONPath validates the JSONPath syntax
func (j *JSONPatternMatcher) validateJSONPath(path string) error {
	// Must start with $.
	if !strings.HasPrefix(path, "$.") {
		return NewPatternError("JSONPathError", "JSONPath must start with '$.'", path)
	}

	// Check for consecutive dots (invalid syntax)
	if strings.Contains(path, "..") {
		return NewPatternError("JSONPathError", "Invalid JSONPath syntax: consecutive dots not supported", path)
	}

	// Check for empty path segments
	pathWithoutRoot := path[2:] // Remove $.
	if pathWithoutRoot == "" {
		return NewPatternError("JSONPathError", "JSONPath cannot be empty after '$.'", path)
	}

	parts := strings.Split(pathWithoutRoot, ".")
	for _, part := range parts {
		if part == "" {
			return NewPatternError("JSONPathError", "JSONPath cannot have empty segments", path)
		}

		// Validate array access syntax
		if strings.Contains(part, "[") {
			if !strings.Contains(part, "]") {
				return NewPatternError("JSONPathError", "Invalid array access syntax: missing closing bracket", path)
			}

			// Check bracket placement
			openIdx := strings.Index(part, "[")
			closeIdx := strings.Index(part, "]")
			if openIdx >= closeIdx {
				return NewPatternError("JSONPathError", "Invalid array access syntax: malformed brackets", path)
			}

			// Validate array index
			indexPart := part[openIdx+1 : closeIdx]
			if indexPart != "*" {
				if _, err := strconv.Atoi(indexPart); err != nil {
					return NewPatternError("JSONPathError", "Invalid array index: must be number or '*'", path)
				}
			}
		}
	}

	return nil
}

// parseExpectedValue parses the expected value from string
func (j *JSONPatternMatcher) parseExpectedValue(valueStr string) error {
	// Remove quotes if present
	if strings.HasPrefix(valueStr, `"`) && strings.HasSuffix(valueStr, `"`) {
		j.expectedValue = strings.Trim(valueStr, `"`)
		return nil
	}

	// Try to parse as number
	if intVal, err := strconv.Atoi(valueStr); err == nil {
		j.expectedValue = intVal
		return nil
	}

	if floatVal, err := strconv.ParseFloat(valueStr, 64); err == nil {
		j.expectedValue = floatVal
		return nil
	}

	// Try to parse as boolean
	if boolVal, err := strconv.ParseBool(valueStr); err == nil {
		j.expectedValue = boolVal
		return nil
	}

	// Check for null
	if valueStr == "null" {
		j.expectedValue = nil
		return nil
	}

	// Default to string
	j.expectedValue = valueStr
	return nil
}

// Match tests if the JSON input matches the pattern
func (j *JSONPatternMatcher) Match(input string) (bool, error) {
	return j.MatchWithContext(input, nil)
}

// MatchWithContext tests if the JSON input matches the pattern with context
func (j *JSONPatternMatcher) MatchWithContext(input string, context map[string]interface{}) (bool, error) {
	start := time.Now()
	defer func() {
		j.updateMetrics(time.Since(start))
	}()

	// Parse JSON
	var jsonData interface{}
	if err := json.Unmarshal([]byte(input), &jsonData); err != nil {
		j.incrementErrorCount()
		return false, NewPatternError("JSONParseError", "Invalid JSON: "+err.Error(), j.pattern)
	}

	// Extract value using JSONPath
	value, found := j.extractValue(jsonData, j.jsonPath)
	if !found {
		return false, nil
	}

	// Resolve expected value from context if needed
	expectedValue := j.expectedValue
	if context != nil {
		expectedValue = j.resolveContextValue(expectedValue, context)
	}

	// Perform comparison
	result := j.compareValues(value, expectedValue)

	if result {
		j.incrementSuccessCount()
	} else {
		j.incrementFailureCount()
	}

	return result, nil
}

// extractValue extracts a value from JSON data using a simple JSONPath
func (j *JSONPatternMatcher) extractValue(data interface{}, path string) (interface{}, bool) {
	if !strings.HasPrefix(path, "$.") {
		return nil, false
	}

	// Remove the $. prefix
	path = path[2:]

	return j.extractValueRecursive(data, strings.Split(path, "."))
}

// extractValueRecursive recursively extracts values, handling array wildcards
func (j *JSONPatternMatcher) extractValueRecursive(current interface{}, parts []string) (interface{}, bool) {
	if len(parts) == 0 {
		return current, true
	}

	part := parts[0]
	remainingParts := parts[1:]

	if part == "" {
		return j.extractValueRecursive(current, remainingParts)
	}

	// Handle array access like items[0] or items[*]
	if strings.Contains(part, "[") && strings.Contains(part, "]") {
		arrayPart := part[:strings.Index(part, "[")]
		indexPart := part[strings.Index(part, "[")+1 : strings.Index(part, "]")]

		// Navigate to array
		if arrayPart != "" {
			var exists bool
			current, exists = j.navigateToField(current, arrayPart)
			if !exists {
				return nil, false
			}
		}

		// Handle array access
		if arr, ok := current.([]interface{}); ok {
			if indexPart == "*" {
				// Wildcard - if there are remaining parts, apply them to each element
				if len(remainingParts) > 0 {
					var results []interface{}
					for _, item := range arr {
						if value, found := j.extractValueRecursive(item, remainingParts); found {
							results = append(results, value)
						}
					}
					if len(results) > 0 {
						return results, true
					}
					return nil, false
				} else {
					// No remaining parts, return the array itself
					return arr, true
				}
			} else if index, err := strconv.Atoi(indexPart); err == nil {
				if index >= 0 && index < len(arr) {
					return j.extractValueRecursive(arr[index], remainingParts)
				} else {
					return nil, false
				}
			} else {
				return nil, false
			}
		} else {
			return nil, false
		}
	} else {
		var exists bool
		current, exists = j.navigateToField(current, part)
		if !exists {
			return nil, false
		}
		return j.extractValueRecursive(current, remainingParts)
	}
}

// navigateToField navigates to a specific field in a JSON object
// Returns the value and a boolean indicating if the field exists
func (j *JSONPatternMatcher) navigateToField(data interface{}, field string) (interface{}, bool) {
	if obj, ok := data.(map[string]interface{}); ok {
		value, exists := obj[field]
		return value, exists
	}
	return nil, false
}

// compareValues compares two values based on the operator
func (j *JSONPatternMatcher) compareValues(actual, expected interface{}) bool {
	switch j.operator {
	case OpEqual:
		return j.compareEqual(actual, expected)
	case OpNotEqual:
		return !j.compareEqual(actual, expected)
	case OpGreaterThan:
		return j.compareNumeric(actual, expected, ">")
	case OpLessThan:
		return j.compareNumeric(actual, expected, "<")
	case OpGreaterThanOrEqual:
		return j.compareNumeric(actual, expected, ">=")
	case OpLessThanOrEqual:
		return j.compareNumeric(actual, expected, "<=")
	case OpRegexMatch:
		return j.compareRegex(actual, expected)
	default:
		return false
	}
}

// compareEqual compares two values for equality
func (j *JSONPatternMatcher) compareEqual(actual, expected interface{}) bool {
	// Handle array wildcard matching (both from [*] and from [*].field)
	if arr, ok := actual.([]interface{}); ok {
		for _, item := range arr {
			if j.compareEqual(item, expected) {
				return true
			}
		}
		return false
	}

	// Handle numeric comparisons (JSON numbers are float64, but expected might be int)
	actualFloat, actualIsNum := j.toFloat64(actual)
	expectedFloat, expectedIsNum := j.toFloat64(expected)
	if actualIsNum && expectedIsNum {
		return actualFloat == expectedFloat
	}

	// Direct comparison for non-numeric types
	return actual == expected
}

// compareNumeric compares numeric values
func (j *JSONPatternMatcher) compareNumeric(actual, expected interface{}, op string) bool {
	actualNum, ok1 := j.toFloat64(actual)
	expectedNum, ok2 := j.toFloat64(expected)

	if !ok1 || !ok2 {
		return false
	}

	switch op {
	case ">":
		return actualNum > expectedNum
	case "<":
		return actualNum < expectedNum
	case ">=":
		return actualNum >= expectedNum
	case "<=":
		return actualNum <= expectedNum
	default:
		return false
	}
}

// compareRegex compares using regex
func (j *JSONPatternMatcher) compareRegex(actual, expected interface{}) bool {
	actualStr, ok := actual.(string)
	if !ok {
		return false
	}

	if j.compiledRegex != nil {
		return j.compiledRegex.MatchString(actualStr)
	}

	expectedStr, ok := expected.(string)
	if !ok {
		return false
	}

	// Case-insensitive regex match
	regex, err := regexp.Compile("(?i)" + expectedStr)
	if err != nil {
		return false
	}

	return regex.MatchString(actualStr)
}

// toFloat64 converts various numeric types to float64
func (j *JSONPatternMatcher) toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	default:
		return 0, false
	}
}

// resolveContextValue resolves context variables in expected values
func (j *JSONPatternMatcher) resolveContextValue(value interface{}, context map[string]interface{}) interface{} {
	if str, ok := value.(string); ok {
		if strings.HasPrefix(str, "$ctx.") {
			key := str[5:] // Remove "$ctx." prefix
			if contextValue, exists := context[key]; exists {
				return contextValue
			}
		}
	}
	return value
}

// Validate checks if the pattern is valid
func (j *JSONPatternMatcher) Validate() error {
	// Pattern was already validated during construction
	return nil
}

// GetMetrics returns matching performance metrics
func (j *JSONPatternMatcher) GetMetrics() MatchMetrics {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.metrics
}

// updateMetrics updates the performance metrics
func (j *JSONPatternMatcher) updateMetrics(duration time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.metrics.TotalMatches++
	j.metrics.TotalMatchTime += duration
	j.metrics.LastMatchTime = time.Now()

	if j.metrics.TotalMatches > 0 {
		j.metrics.AverageMatchTime = j.metrics.TotalMatchTime / time.Duration(j.metrics.TotalMatches)
	}
}

// incrementSuccessCount increments the successful match count
func (j *JSONPatternMatcher) incrementSuccessCount() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.metrics.SuccessfulMatches++
}

// incrementFailureCount increments the failed match count
func (j *JSONPatternMatcher) incrementFailureCount() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.metrics.FailedMatches++
}

// incrementErrorCount increments the error count
func (j *JSONPatternMatcher) incrementErrorCount() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.metrics.ErrorCount++
}
