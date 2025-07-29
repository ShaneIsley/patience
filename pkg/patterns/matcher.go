package patterns

import (
	"time"
)

// PatternMatcher defines the interface for all pattern matching implementations
type PatternMatcher interface {
	// Match tests if the input matches the pattern
	Match(input string) (bool, error)

	// MatchWithContext tests if the input matches the pattern with context variables
	MatchWithContext(input string, context map[string]interface{}) (bool, error)

	// Validate checks if the pattern is valid
	Validate() error

	// GetMetrics returns matching performance metrics
	GetMetrics() MatchMetrics
}

// MatchMetrics contains performance and usage statistics for pattern matching
type MatchMetrics struct {
	TotalMatches      int           `json:"total_matches"`
	SuccessfulMatches int           `json:"successful_matches"`
	FailedMatches     int           `json:"failed_matches"`
	ErrorCount        int           `json:"error_count"`
	AverageMatchTime  time.Duration `json:"average_match_time"`
	TotalMatchTime    time.Duration `json:"total_match_time"`
	LastMatchTime     time.Time     `json:"last_match_time"`
	PatternComplexity int           `json:"pattern_complexity"`
}

// ComparisonOperator represents the type of comparison to perform
type ComparisonOperator int

const (
	OpEqual ComparisonOperator = iota
	OpNotEqual
	OpGreaterThan
	OpLessThan
	OpGreaterThanOrEqual
	OpLessThanOrEqual
	OpRegexMatch
	OpContains
	OpStartsWith
	OpEndsWith
)

// String returns the string representation of the comparison operator
func (op ComparisonOperator) String() string {
	switch op {
	case OpEqual:
		return "=="
	case OpNotEqual:
		return "!="
	case OpGreaterThan:
		return ">"
	case OpLessThan:
		return "<"
	case OpGreaterThanOrEqual:
		return ">="
	case OpLessThanOrEqual:
		return "<="
	case OpRegexMatch:
		return "=~"
	case OpContains:
		return "contains"
	case OpStartsWith:
		return "startsWith"
	case OpEndsWith:
		return "endsWith"
	default:
		return "unknown"
	}
}

// PatternType represents the type of pattern
type PatternType int

const (
	PatternTypeJSON PatternType = iota
	PatternTypeRegex
	PatternTypeMultiLine
	PatternTypeHTTPStatus
	PatternTypeCustom
)

// String returns the string representation of the pattern type
func (pt PatternType) String() string {
	switch pt {
	case PatternTypeJSON:
		return "json"
	case PatternTypeRegex:
		return "regex"
	case PatternTypeMultiLine:
		return "multiline"
	case PatternTypeHTTPStatus:
		return "http_status"
	case PatternTypeCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// PatternError represents an error that occurred during pattern operations
type PatternError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Pattern string `json:"pattern,omitempty"`
	Input   string `json:"input,omitempty"`
}

// Error implements the error interface
func (pe *PatternError) Error() string {
	if pe.Pattern != "" {
		return pe.Type + ": " + pe.Message + " (pattern: " + pe.Pattern + ")"
	}
	return pe.Type + ": " + pe.Message
}

// NewPatternError creates a new pattern error
func NewPatternError(errorType, message, pattern string) *PatternError {
	return &PatternError{
		Type:    errorType,
		Message: message,
		Pattern: pattern,
	}
}

// ValidationResult represents the result of pattern validation
type ValidationResult struct {
	Valid         bool          `json:"valid"`
	Errors        []string      `json:"errors,omitempty"`
	Warnings      []string      `json:"warnings,omitempty"`
	Complexity    int           `json:"complexity"`
	EstimatedTime time.Duration `json:"estimated_time"`
}

// PatternValidator provides validation capabilities for patterns
type PatternValidator interface {
	// ValidatePattern checks if a pattern is valid and safe to use
	ValidatePattern(pattern string, patternType PatternType) ValidationResult

	// CheckPerformance estimates the performance characteristics of a pattern
	CheckPerformance(pattern string, patternType PatternType) time.Duration

	// CheckSecurity validates that a pattern is safe from ReDoS and other attacks
	CheckSecurity(pattern string, patternType PatternType) []string
}
