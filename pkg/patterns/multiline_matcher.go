package patterns

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Predefined stack trace patterns
var stackTracePatterns = map[string]struct {
	pattern    string
	language   string
	extractors map[string]string
}{
	"java_exception": {
		pattern:  `(?s)Exception in thread.*?\n(\s+at .*?)`,
		language: "java",
		extractors: map[string]string{
			"exception_type": `Exception in thread ".*?" ([^:]+)`,
			"message":        `Exception in thread ".*?" [^:]+: (.*?)\n`,
			"stack_frames":   `\s+at ([^\n]+)`,
		},
	},
	"python_exception": {
		pattern:  `(?s)Traceback \(most recent call last\):.*?\n([^\n]+Error: .*?)$`,
		language: "python",
		extractors: map[string]string{
			"exception_type": `([^\n]+Error): `,
			"message":        `[^\n]+Error: (.*?)$`,
			"stack_frames":   `File "([^"]+)", line (\d+), in ([^\n]+)`,
		},
	},
	"go_panic": {
		pattern:  `(?s)panic: (.*?)\n\[signal.*?\]\n\ngoroutine.*?\[running\]:`,
		language: "go",
		extractors: map[string]string{
			"panic_message": `panic: (.*?)\n`,
			"signal":        `\[signal ([^\]]+)\]`,
			"goroutine_id":  `goroutine (\d+) \[running\]`,
		},
	},
	"javascript_error": {
		pattern:  `(?s)(\w+Error): ([^\n]+)\n(\s+at .*?\n)+`,
		language: "javascript",
		extractors: map[string]string{
			"error_type":   `(\w+Error): `,
			"message":      `\w+Error: ([^\n]+)`,
			"stack_frames": `\s+at ([^\n]+)`,
		},
	},
	"csharp_exception": {
		pattern:  `(?s)Unhandled exception\. ([^:]+): ([^\n]+)\n(\s+at .*?\n)+`,
		language: "csharp",
		extractors: map[string]string{
			"exception_type": `Unhandled exception\. ([^:]+): `,
			"message":        `Unhandled exception\. [^:]+: ([^\n]+)`,
			"stack_frames":   `\s+at ([^\n]+)`,
		},
	},
	"rust_panic": {
		pattern:  `(?s)thread '.*?' panicked at '([^']+)', ([^\n]+)\nstack backtrace:`,
		language: "rust",
		extractors: map[string]string{
			"panic_message": `thread '.*?' panicked at '([^']+)'`,
			"location":      `panicked at '[^']+', ([^\n]+)`,
			"thread_name":   `thread '([^']+)' panicked`,
		},
	},
}

// Predefined structured log patterns
var structuredLogPatterns = map[string]struct {
	pattern    string
	format     string
	extractors map[string]string
}{
	"json_error": {
		pattern: `(?s)\{[^}]*"level"\s*:\s*"ERROR"[^}]*\}`,
		format:  "json",
		extractors: map[string]string{
			"timestamp":  `"timestamp"\s*:\s*"([^"]+)"`,
			"message":    `"message"\s*:\s*"([^"]+)"`,
			"error_type": `"type"\s*:\s*"([^"]+)"`,
		},
	},
	"yaml_error": {
		pattern: `(?s)error:\s*\n\s+code:\s*\d+\s*\n\s+message:`,
		format:  "yaml",
		extractors: map[string]string{
			"error_code": `code:\s*(\d+)`,
			"message":    `message:\s*([^\n]+)`,
		},
	},
	"xml_error": {
		pattern: `(?s)<error>.*?<code>\d+</code>.*?<message>.*?</message>.*?</error>`,
		format:  "xml",
		extractors: map[string]string{
			"error_code": `<code>(\d+)</code>`,
			"message":    `<message>(.*?)</message>`,
			"path":       `<path>(.*?)</path>`,
		},
	},
	"log_continuation": {
		pattern: `(?s)\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} ERROR [^\n]+\n(\s+[^\n]+\n)+`,
		format:  "text",
		extractors: map[string]string{
			"timestamp":    `(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`,
			"main_message": `ERROR ([^\n]+)`,
			"details":      `\n(\s+[^\n]+)`,
		},
	},
	"multiline_json_array": {
		pattern: `(?s)\[\s*\{[^\]]*"error"[^\]]*\}\s*\]`,
		format:  "json",
		extractors: map[string]string{
			"error_objects": `\{([^}]*"error"[^}]*)\}`,
		},
	},
	"docker_build_error": {
		pattern: `(?s)Step \d+/\d+ : .*?\n.*?npm ERR!.*?\nThe command.*?returned a non-zero code`,
		format:  "text",
		extractors: map[string]string{
			"step":      `Step (\d+/\d+)`,
			"command":   `The command '([^']+)'`,
			"exit_code": `returned a non-zero code: (\d+)`,
		},
	},
	"k8s_crashloop": {
		pattern: `(?s)Events:.*?Type\s+Reason\s+Age.*?Warning\s+BackOff.*?Back-off restarting failed container`,
		format:  "text",
		extractors: map[string]string{
			"pod_name":      `Successfully assigned [^/]+/([^\s]+)`,
			"backoff_count": `\(x(\d+) over`,
		},
	},
	"db_pool_exhausted": {
		pattern: `(?s)HikariPool-\d+ - Connection is not available.*?request timed out.*?Database operation failed`,
		format:  "text",
		extractors: map[string]string{
			"pool_name":  `(HikariPool-\d+)`,
			"timeout_ms": `timed out after (\d+)ms`,
		},
	},
	"api_rate_limit_detailed": {
		pattern: `(?s)HTTP/1\.1 429 Too Many Requests.*?X-RateLimit-Limit: \d+.*?\{.*?"rate_limit_exceeded".*?\}`,
		format:  "text",
		extractors: map[string]string{
			"rate_limit":  `X-RateLimit-Limit: (\d+)`,
			"remaining":   `X-RateLimit-Remaining: (\d+)`,
			"reset_time":  `X-RateLimit-Reset: (\d+)`,
			"retry_after": `Retry-After: (\d+)`,
		},
	},
}

// MultiLinePatternMatcher implements pattern matching for multi-line text
type MultiLinePatternMatcher struct {
	pattern string
	regex   *regexp.Regexp
	metrics MatchMetrics
}

// StackTraceResult represents the result of stack trace pattern matching
type StackTraceResult struct {
	Matched    bool                   `json:"matched"`
	RootCause  string                 `json:"root_cause,omitempty"`
	StackTrace []string               `json:"stack_trace,omitempty"`
	Language   string                 `json:"language,omitempty"`
	MatchTime  time.Duration          `json:"match_time"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

// StackTracePatternMatcher implements pattern matching for stack traces
type StackTracePatternMatcher struct {
	pattern     string
	patternType string
	regex       *regexp.Regexp
	extractors  map[string]*regexp.Regexp
	language    string
	metrics     MatchMetrics
}

// StructuredLogPatternMatcher implements pattern matching for structured logs
type StructuredLogPatternMatcher struct {
	pattern     string
	patternType string
	regex       *regexp.Regexp
	extractors  map[string]*regexp.Regexp
	format      string
	metrics     MatchMetrics
}

// StreamingMultiLinePatternMatcher implements streaming pattern matching
type StreamingMultiLinePatternMatcher struct {
	pattern   string
	regex     *regexp.Regexp
	buffer    strings.Builder
	matched   bool
	maxBuffer int
	metrics   MatchMetrics
}

// RealWorldScenarioMatcher implements pattern matching for real-world scenarios
type RealWorldScenarioMatcher struct {
	scenario string
	regex    *regexp.Regexp
	metrics  MatchMetrics
}

// NewMultiLinePatternMatcher creates a new multi-line pattern matcher
func NewMultiLinePatternMatcher(pattern string) (*MultiLinePatternMatcher, error) {
	if pattern == "" {
		return nil, NewPatternError("invalid_pattern", "pattern cannot be empty", pattern)
	}

	// Compile regex with DOTALL flag if not already present
	regexPattern := pattern
	if !strings.Contains(pattern, "(?s)") && !strings.Contains(pattern, "(?ms)") {
		regexPattern = "(?s)" + pattern
	}

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile regex: %v", err), pattern)
	}

	return &MultiLinePatternMatcher{
		pattern: pattern,
		regex:   regex,
		metrics: MatchMetrics{
			PatternComplexity: len(pattern),
		},
	}, nil
}

// Match tests if the multi-line input matches the pattern
func (m *MultiLinePatternMatcher) Match(input string) (bool, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		m.metrics.TotalMatches++
		m.metrics.TotalMatchTime += duration
		m.metrics.AverageMatchTime = m.metrics.TotalMatchTime / time.Duration(m.metrics.TotalMatches)
		m.metrics.LastMatchTime = time.Now()
	}()

	if m.regex == nil {
		m.metrics.ErrorCount++
		return false, NewPatternError("matcher_error", "regex not compiled", m.pattern)
	}

	matched := m.regex.MatchString(input)
	if matched {
		m.metrics.SuccessfulMatches++
	} else {
		m.metrics.FailedMatches++
	}

	return matched, nil
}

// MatchWithContext tests if the input matches with context variables
func (m *MultiLinePatternMatcher) MatchWithContext(input string, context map[string]interface{}) (bool, error) {
	// For basic multi-line matching, context is not used
	// This could be extended in the future for variable substitution
	return m.Match(input)
}

// Validate checks if the pattern is valid
func (m *MultiLinePatternMatcher) Validate() error {
	if m.pattern == "" {
		return NewPatternError("validation_error", "pattern cannot be empty", m.pattern)
	}
	if m.regex == nil {
		return NewPatternError("validation_error", "regex not compiled", m.pattern)
	}
	return nil
}

// GetMetrics returns matching performance metrics
func (m *MultiLinePatternMatcher) GetMetrics() MatchMetrics {
	return m.metrics
}

// NewStackTracePatternMatcher creates a new stack trace pattern matcher
func NewStackTracePatternMatcher(pattern string) (*StackTracePatternMatcher, error) {
	if pattern == "" {
		return nil, NewPatternError("invalid_pattern", "pattern cannot be empty", pattern)
	}

	// Check if it's a predefined pattern
	if predefined, exists := stackTracePatterns[pattern]; exists {
		regex, err := regexp.Compile(predefined.pattern)
		if err != nil {
			return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile predefined pattern: %v", err), pattern)
		}

		// Compile extractors
		extractors := make(map[string]*regexp.Regexp)
		for name, extractorPattern := range predefined.extractors {
			extractor, err := regexp.Compile(extractorPattern)
			if err != nil {
				return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile extractor %s: %v", name, err), extractorPattern)
			}
			extractors[name] = extractor
		}

		return &StackTracePatternMatcher{
			pattern:     pattern,
			patternType: pattern,
			regex:       regex,
			extractors:  extractors,
			language:    predefined.language,
			metrics: MatchMetrics{
				PatternComplexity: len(predefined.pattern),
			},
		}, nil
	}

	// Handle custom regex pattern
	regexPattern := pattern
	if !strings.Contains(pattern, "(?s)") && !strings.Contains(pattern, "(?ms)") {
		regexPattern = "(?s)" + pattern
	}

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile regex: %v", err), pattern)
	}

	return &StackTracePatternMatcher{
		pattern:     pattern,
		patternType: "custom",
		regex:       regex,
		extractors:  make(map[string]*regexp.Regexp),
		language:    "unknown",
		metrics: MatchMetrics{
			PatternComplexity: len(pattern),
		},
	}, nil
}

// MatchWithExtraction tests if the input matches and extracts stack trace information
func (s *StackTracePatternMatcher) MatchWithExtraction(input string) (*StackTraceResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		s.metrics.TotalMatches++
		s.metrics.TotalMatchTime += duration
		s.metrics.AverageMatchTime = s.metrics.TotalMatchTime / time.Duration(s.metrics.TotalMatches)
		s.metrics.LastMatchTime = time.Now()
	}()

	if s.regex == nil {
		s.metrics.ErrorCount++
		return nil, NewPatternError("matcher_error", "regex not compiled", s.pattern)
	}

	matched := s.regex.MatchString(input)
	result := &StackTraceResult{
		Matched:   matched,
		Language:  s.language,
		MatchTime: time.Since(start),
		Context:   make(map[string]interface{}),
	}

	if !matched {
		s.metrics.FailedMatches++
		return result, nil
	}

	s.metrics.SuccessfulMatches++

	// Extract information using predefined extractors
	if len(s.extractors) > 0 {
		context := make(map[string]interface{})

		// Extract root cause
		if extractor, exists := s.extractors["exception_type"]; exists {
			if matches := extractor.FindStringSubmatch(input); len(matches) > 1 {
				result.RootCause = matches[1]
				context["exception_type"] = matches[1]
			}
		} else if extractor, exists := s.extractors["error_type"]; exists {
			if matches := extractor.FindStringSubmatch(input); len(matches) > 1 {
				result.RootCause = matches[1]
				context["error_type"] = matches[1]
			}
		} else if extractor, exists := s.extractors["panic_message"]; exists {
			if matches := extractor.FindStringSubmatch(input); len(matches) > 1 {
				result.RootCause = matches[1]
				context["panic_message"] = matches[1]
			}
		}

		// Extract message
		if extractor, exists := s.extractors["message"]; exists {
			if matches := extractor.FindStringSubmatch(input); len(matches) > 1 {
				context["message"] = matches[1]
			}
		}

		// Extract stack frames
		if extractor, exists := s.extractors["stack_frames"]; exists {
			allMatches := extractor.FindAllStringSubmatch(input, -1)
			var stackFrames []string
			for _, match := range allMatches {
				if len(match) > 1 {
					stackFrames = append(stackFrames, match[1])
				}
			}
			if len(stackFrames) > 0 {
				result.StackTrace = stackFrames
				context["stack_frames"] = stackFrames
			}
		}

		result.Context = context
	}

	// If no root cause was extracted, try to extract from the main match
	if result.RootCause == "" && s.regex != nil {
		if matches := s.regex.FindStringSubmatch(input); len(matches) > 1 {
			result.RootCause = strings.TrimSpace(matches[1])
		}
	}

	return result, nil
}

// Match tests if the input matches the stack trace pattern
func (s *StackTracePatternMatcher) Match(input string) (bool, error) {
	result, err := s.MatchWithExtraction(input)
	if err != nil {
		return false, err
	}
	return result.Matched, nil
}

// MatchWithContext tests if the input matches with context variables
func (s *StackTracePatternMatcher) MatchWithContext(input string, context map[string]interface{}) (bool, error) {
	// For stack trace matching, context is not used in basic implementation
	return s.Match(input)
}

// Validate checks if the pattern is valid
func (s *StackTracePatternMatcher) Validate() error {
	if s.pattern == "" {
		return NewPatternError("validation_error", "pattern cannot be empty", s.pattern)
	}
	if s.regex == nil {
		return NewPatternError("validation_error", "regex not compiled", s.pattern)
	}
	return nil
}

// GetMetrics returns matching performance metrics
func (s *StackTracePatternMatcher) GetMetrics() MatchMetrics {
	return s.metrics
}

// NewStructuredLogPatternMatcher creates a new structured log pattern matcher
func NewStructuredLogPatternMatcher(pattern string) (*StructuredLogPatternMatcher, error) {
	if pattern == "" {
		return nil, NewPatternError("invalid_pattern", "pattern cannot be empty", pattern)
	}

	// Check if it's a predefined pattern
	if predefined, exists := structuredLogPatterns[pattern]; exists {
		regex, err := regexp.Compile(predefined.pattern)
		if err != nil {
			return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile predefined pattern: %v", err), pattern)
		}

		// Compile extractors
		extractors := make(map[string]*regexp.Regexp)
		for name, extractorPattern := range predefined.extractors {
			extractor, err := regexp.Compile(extractorPattern)
			if err != nil {
				return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile extractor %s: %v", name, err), extractorPattern)
			}
			extractors[name] = extractor
		}

		return &StructuredLogPatternMatcher{
			pattern:     pattern,
			patternType: pattern,
			regex:       regex,
			extractors:  extractors,
			format:      predefined.format,
			metrics: MatchMetrics{
				PatternComplexity: len(predefined.pattern),
			},
		}, nil
	}

	// Handle custom regex pattern
	regexPattern := pattern
	if !strings.Contains(pattern, "(?s)") && !strings.Contains(pattern, "(?ms)") {
		regexPattern = "(?s)" + pattern
	}

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile regex: %v", err), pattern)
	}

	return &StructuredLogPatternMatcher{
		pattern:     pattern,
		patternType: "custom",
		regex:       regex,
		extractors:  make(map[string]*regexp.Regexp),
		format:      "unknown",
		metrics: MatchMetrics{
			PatternComplexity: len(pattern),
		},
	}, nil
}

// Match tests if the input matches the structured log pattern
func (sl *StructuredLogPatternMatcher) Match(input string) (bool, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		sl.metrics.TotalMatches++
		sl.metrics.TotalMatchTime += duration
		sl.metrics.AverageMatchTime = sl.metrics.TotalMatchTime / time.Duration(sl.metrics.TotalMatches)
		sl.metrics.LastMatchTime = time.Now()
	}()

	if sl.regex == nil {
		sl.metrics.ErrorCount++
		return false, NewPatternError("matcher_error", "regex not compiled", sl.pattern)
	}

	matched := sl.regex.MatchString(input)
	if matched {
		sl.metrics.SuccessfulMatches++
	} else {
		sl.metrics.FailedMatches++
	}

	return matched, nil
}

// MatchWithContext tests if the input matches with context variables
func (sl *StructuredLogPatternMatcher) MatchWithContext(input string, context map[string]interface{}) (bool, error) {
	// For structured log matching, context is not used in basic implementation
	return sl.Match(input)
}

// Validate checks if the pattern is valid
func (sl *StructuredLogPatternMatcher) Validate() error {
	if sl.pattern == "" {
		return NewPatternError("validation_error", "pattern cannot be empty", sl.pattern)
	}
	if sl.regex == nil {
		return NewPatternError("validation_error", "regex not compiled", sl.pattern)
	}
	return nil
}

// GetMetrics returns matching performance metrics
func (sl *StructuredLogPatternMatcher) GetMetrics() MatchMetrics {
	return sl.metrics
}

// NewStreamingMultiLinePatternMatcher creates a new streaming multi-line pattern matcher
func NewStreamingMultiLinePatternMatcher(pattern string) (*StreamingMultiLinePatternMatcher, error) {
	if pattern == "" {
		return nil, NewPatternError("invalid_pattern", "pattern cannot be empty", pattern)
	}

	// Compile regex with DOTALL flag if not already present
	regexPattern := pattern
	if !strings.Contains(pattern, "(?s)") && !strings.Contains(pattern, "(?ms)") {
		regexPattern = "(?s)" + pattern
	}

	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile regex: %v", err), pattern)
	}

	return &StreamingMultiLinePatternMatcher{
		pattern:   pattern,
		regex:     regex,
		buffer:    strings.Builder{},
		matched:   false,
		maxBuffer: 1024 * 1024, // 1MB default buffer limit
		metrics: MatchMetrics{
			PatternComplexity: len(pattern),
		},
	}, nil
}

// ProcessChunk processes a chunk of input text
func (sm *StreamingMultiLinePatternMatcher) ProcessChunk(chunk string) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		sm.metrics.TotalMatches++
		sm.metrics.TotalMatchTime += duration
		sm.metrics.AverageMatchTime = sm.metrics.TotalMatchTime / time.Duration(sm.metrics.TotalMatches)
		sm.metrics.LastMatchTime = time.Now()
	}()

	if sm.regex == nil {
		sm.metrics.ErrorCount++
		return NewPatternError("matcher_error", "regex not compiled", sm.pattern)
	}

	// Check if we already found a match
	if sm.matched {
		return nil
	}

	// Check buffer size limit
	if sm.buffer.Len()+len(chunk) > sm.maxBuffer {
		// Trim buffer to make room, keeping the last half
		currentContent := sm.buffer.String()
		sm.buffer.Reset()
		sm.buffer.WriteString(currentContent[len(currentContent)/2:])
	}

	// Add chunk to buffer
	sm.buffer.WriteString(chunk)

	// Test for match
	if sm.regex.MatchString(sm.buffer.String()) {
		sm.matched = true
		sm.metrics.SuccessfulMatches++
	} else {
		sm.metrics.FailedMatches++
	}

	return nil
}

// HasMatch returns true if a match has been found in the processed chunks
func (sm *StreamingMultiLinePatternMatcher) HasMatch() bool {
	return sm.matched
}

// Reset resets the matcher state for reuse
func (sm *StreamingMultiLinePatternMatcher) Reset() {
	sm.buffer.Reset()
	sm.matched = false
}

// GetMetrics returns matching performance metrics
func (sm *StreamingMultiLinePatternMatcher) GetMetrics() MatchMetrics {
	return sm.metrics
}

// NewRealWorldScenarioMatcher creates a new real-world scenario matcher
func NewRealWorldScenarioMatcher(scenario string) (*RealWorldScenarioMatcher, error) {
	if scenario == "" {
		return nil, NewPatternError("invalid_scenario", "scenario cannot be empty", scenario)
	}

	// Check if it's a predefined structured log pattern
	if predefined, exists := structuredLogPatterns[scenario]; exists {
		regex, err := regexp.Compile(predefined.pattern)
		if err != nil {
			return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile scenario pattern: %v", err), scenario)
		}

		return &RealWorldScenarioMatcher{
			scenario: scenario,
			regex:    regex,
			metrics: MatchMetrics{
				PatternComplexity: len(predefined.pattern),
			},
		}, nil
	}

	// Fallback to simple pattern that matches the scenario name
	pattern := fmt.Sprintf("(?s).*%s.*", regexp.QuoteMeta(scenario))
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, NewPatternError("compilation_error", fmt.Sprintf("failed to compile scenario pattern: %v", err), scenario)
	}

	return &RealWorldScenarioMatcher{
		scenario: scenario,
		regex:    regex,
		metrics: MatchMetrics{
			PatternComplexity: len(scenario),
		},
	}, nil
}

// Match tests if the input matches the real-world scenario pattern
func (r *RealWorldScenarioMatcher) Match(input string) (bool, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		r.metrics.TotalMatches++
		r.metrics.TotalMatchTime += duration
		r.metrics.AverageMatchTime = r.metrics.TotalMatchTime / time.Duration(r.metrics.TotalMatches)
		r.metrics.LastMatchTime = time.Now()
	}()

	if r.regex == nil {
		r.metrics.ErrorCount++
		return false, NewPatternError("matcher_error", "regex not compiled", r.scenario)
	}

	matched := r.regex.MatchString(input)
	if matched {
		r.metrics.SuccessfulMatches++
	} else {
		r.metrics.FailedMatches++
	}

	return matched, nil
}

// MatchWithContext tests if the input matches with context variables
func (r *RealWorldScenarioMatcher) MatchWithContext(input string, context map[string]interface{}) (bool, error) {
	return r.Match(input)
}

// Validate checks if the pattern is valid
func (r *RealWorldScenarioMatcher) Validate() error {
	if r.scenario == "" {
		return NewPatternError("validation_error", "scenario cannot be empty", r.scenario)
	}
	if r.regex == nil {
		return NewPatternError("validation_error", "regex not compiled", r.scenario)
	}
	return nil
}

// GetMetrics returns matching performance metrics
func (r *RealWorldScenarioMatcher) GetMetrics() MatchMetrics {
	return r.metrics
}
