package patterns

import (
	"strings"
	"testing"
	"time"
)

func TestMultiLinePatternMatcher_BasicMatching(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		input    string
		expected bool
		wantErr  bool
	}{
		{
			name:     "Simple multi-line regex",
			pattern:  `ERROR.*\n.*failed`,
			input:    "ERROR: Something went wrong\nOperation failed",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Multi-line with DOTALL flag",
			pattern:  `(?s)START.*END`,
			input:    "START\nsome content\nmore content\nEND",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "No match across lines",
			pattern:  `ERROR.*SUCCESS`,
			input:    "ERROR: Something happened\nWARNING: Check this",
			expected: false,
			wantErr:  false,
		},
		{
			name:     "Empty input",
			pattern:  `.*error.*`,
			input:    "",
			expected: false,
			wantErr:  false,
		},
		{
			name:     "Invalid regex pattern",
			pattern:  `[invalid`,
			input:    "some text",
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewMultiLinePatternMatcher(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMultiLinePatternMatcher() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			result, err := matcher.Match(tt.input)
			if err != nil {
				t.Errorf("MultiLinePatternMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("MultiLinePatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMultiLinePatternMatcher_StackTraces(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		input       string
		expected    bool
		expectCause string
	}{
		{
			name:    "Java stack trace",
			pattern: `java_exception`,
			input: `Exception in thread "main" java.lang.NullPointerException: Cannot invoke method
	at com.example.MyClass.doSomething(MyClass.java:42)
	at com.example.Main.main(Main.java:15)`,
			expected:    true,
			expectCause: "java.lang.NullPointerException",
		},
		{
			name:    "Python stack trace",
			pattern: `python_exception`,
			input: `Traceback (most recent call last):
  File "script.py", line 10, in <module>
    result = divide(10, 0)
  File "script.py", line 5, in divide
    return a / b
ZeroDivisionError: division by zero`,
			expected:    true,
			expectCause: "ZeroDivisionError",
		},
		{
			name:    "Go panic",
			pattern: `go_panic`,
			input: `panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x401000]

goroutine 1 [running]:
main.main()
	/path/to/main.go:10 +0x20`,
			expected:    true,
			expectCause: "runtime error: invalid memory address or nil pointer dereference",
		},
		{
			name:    "JavaScript error",
			pattern: `javascript_error`,
			input: `TypeError: Cannot read property 'length' of undefined
    at processArray (/app/script.js:15:20)
    at main (/app/script.js:25:5)
    at Object.<anonymous> (/app/script.js:30:1)`,
			expected:    true,
			expectCause: "TypeError",
		},
		{
			name:     "Not a stack trace",
			pattern:  `java_exception`,
			input:    "This is just a regular log message with no stack trace",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewStackTracePatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewStackTracePatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.MatchWithExtraction(tt.input)
			if err != nil {
				t.Errorf("StackTracePatternMatcher.MatchWithExtraction() error = %v", err)
				return
			}

			if result.Matched != tt.expected {
				t.Errorf("StackTracePatternMatcher.MatchWithExtraction() matched = %v, want %v", result.Matched, tt.expected)
				return
			}

			if tt.expected && tt.expectCause != "" {
				if result.RootCause != tt.expectCause {
					t.Errorf("StackTracePatternMatcher.MatchWithExtraction() root cause = %v, want %v", result.RootCause, tt.expectCause)
				}
			}
		})
	}
}

func TestMultiLinePatternMatcher_StructuredLogs(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		input    string
		expected bool
	}{
		{
			name:    "Multi-line JSON log",
			pattern: `json_error`,
			input: `{
  "timestamp": "2023-01-01T12:00:00Z",
  "level": "ERROR",
  "message": "Database connection failed",
  "error": {
    "type": "ConnectionError",
    "details": "Unable to connect to database server"
  }
}`,
			expected: true,
		},
		{
			name:    "YAML error response",
			pattern: `yaml_error`,
			input: `error:
  code: 500
  message: |
    Internal server error occurred
    Please try again later
  details:
    - Database timeout
    - Connection pool exhausted`,
			expected: true,
		},
		{
			name:    "XML error response",
			pattern: `xml_error`,
			input: `<?xml version="1.0" encoding="UTF-8"?>
<error>
  <code>404</code>
  <message>Resource not found</message>
  <details>
    <path>/api/users/123</path>
    <method>GET</method>
  </details>
</error>`,
			expected: true,
		},
		{
			name:    "Indented log continuation",
			pattern: `log_continuation`,
			input: `2023-01-01 12:00:00 ERROR Failed to process request
    Request ID: abc123
    User ID: user456
    Error: Validation failed
        Field 'email' is required
        Field 'password' must be at least 8 characters`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewStructuredLogPatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewStructuredLogPatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.Match(tt.input)
			if err != nil {
				t.Errorf("StructuredLogPatternMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("StructuredLogPatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMultiLinePatternMatcher_BufferManagement(t *testing.T) {
	// Test with large input to verify memory efficiency
	largeInput := strings.Repeat("This is a line of text that will be repeated many times.\n", 10000)

	matcher, err := NewMultiLinePatternMatcher(`(?s).*repeated.*times.*`)
	if err != nil {
		t.Fatalf("NewMultiLinePatternMatcher() error = %v", err)
	}

	start := time.Now()
	result, err := matcher.Match(largeInput)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("MultiLinePatternMatcher.Match() error = %v", err)
	}

	if !result {
		t.Error("MultiLinePatternMatcher.Match() = false, want true for large input")
	}

	// Should complete within reasonable time (< 100ms for 10k lines)
	if duration > 100*time.Millisecond {
		t.Errorf("MultiLinePatternMatcher.Match() took %v, want < 100ms", duration)
	}

	t.Logf("Large input processing completed in %v", duration)
}

func TestMultiLinePatternMatcher_StreamingSupport(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		chunks   []string
		expected bool
	}{
		{
			name:    "Pattern spans across chunks",
			pattern: `START.*END`,
			chunks: []string{
				"Some initial text\nSTART of important",
				" section with\nmultiple lines of",
				" content that we need\nEND of section",
			},
			expected: true,
		},
		{
			name:    "No match across chunks",
			pattern: `ERROR.*SUCCESS`,
			chunks: []string{
				"INFO: Starting process\n",
				"WARNING: Low memory\n",
				"ERROR: Something failed\n",
			},
			expected: false,
		},
		{
			name:    "Complete pattern in single chunk",
			pattern: `FATAL.*crashed`,
			chunks: []string{
				"DEBUG: Initializing\n",
				"FATAL: Application crashed unexpectedly\n",
				"INFO: Cleanup started\n",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewStreamingMultiLinePatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewStreamingMultiLinePatternMatcher() error = %v", err)
				return
			}

			// Process chunks sequentially
			for _, chunk := range tt.chunks {
				err := matcher.ProcessChunk(chunk)
				if err != nil {
					t.Errorf("StreamingMultiLinePatternMatcher.ProcessChunk() error = %v", err)
					return
				}
			}

			result := matcher.HasMatch()
			if result != tt.expected {
				t.Errorf("StreamingMultiLinePatternMatcher.HasMatch() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMultiLinePatternMatcher_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name     string
		scenario string
		input    string
		expected bool
	}{
		{
			name:     "Docker build failure",
			scenario: "docker_build_error",
			input: `Step 5/10 : RUN npm install
 ---> Running in 1234567890ab
npm ERR! code ENOTFOUND
npm ERR! errno ENOTFOUND
npm ERR! network request to https://registry.npmjs.org/package failed, reason: getaddrinfo ENOTFOUND registry.npmjs.org
npm ERR! network This is a problem related to network connectivity.
npm ERR! network In most cases you are behind a proxy or have bad network settings.

The command '/bin/sh -c npm install' returned a non-zero code: 1`,
			expected: true,
		},
		{
			name:     "Kubernetes pod crash loop",
			scenario: "k8s_crashloop",
			input: `Events:
  Type     Reason     Age                From               Message
  ----     ------     ----               ----               -------
  Normal   Scheduled  2m                 default-scheduler  Successfully assigned default/app-pod to node1
  Normal   Pulling    2m                 kubelet            Pulling image "app:latest"
  Normal   Pulled     2m                 kubelet            Successfully pulled image "app:latest"
  Normal   Created    2m                 kubelet            Created container app
  Normal   Started    2m                 kubelet            Started container app
  Warning  BackOff    1m (x5 over 2m)    kubelet            Back-off restarting failed container`,
			expected: true,
		},
		{
			name:     "Database connection pool exhaustion",
			scenario: "db_pool_exhausted",
			input: `2023-01-01 12:00:00.123 [ERROR] HikariPool-1 - Connection is not available, request timed out after 30000ms.
2023-01-01 12:00:00.124 [ERROR] Failed to obtain JDBC Connection; nested exception is java.sql.SQLTransientConnectionException: HikariPool-1 - Connection is not available, request timed out after 30000ms.
2023-01-01 12:00:00.125 [ERROR] Database operation failed
	at com.zaxxer.hikari.pool.HikariPool.createTimeoutException(HikariPool.java:695)
	at com.zaxxer.hikari.pool.HikariPool.getConnection(HikariPool.java:197)
	at com.zaxxer.hikari.pool.HikariPool.getConnection(HikariPool.java:162)`,
			expected: true,
		},
		{
			name:     "API rate limit with retry info",
			scenario: "api_rate_limit_detailed",
			input: `HTTP/1.1 429 Too Many Requests
Date: Mon, 01 Jan 2023 12:00:00 GMT
Content-Type: application/json
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1672574400
Retry-After: 3600

{
  "error": {
    "code": "rate_limit_exceeded",
    "message": "API rate limit exceeded",
    "details": {
      "limit": 1000,
      "window": "1h",
      "retry_after": 3600
    }
  }
}`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewRealWorldScenarioMatcher(tt.scenario)
			if err != nil {
				t.Errorf("NewRealWorldScenarioMatcher() error = %v", err)
				return
			}

			result, err := matcher.Match(tt.input)
			if err != nil {
				t.Errorf("RealWorldScenarioMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("RealWorldScenarioMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMultiLinePatternMatcher_Performance(t *testing.T) {
	// Generate a large multi-line input with various patterns
	var builder strings.Builder
	for i := 0; i < 1000; i++ {
		builder.WriteString("INFO: Processing item ")
		builder.WriteString(string(rune(i)))
		builder.WriteString("\n")
		if i%100 == 99 {
			builder.WriteString("ERROR: Batch processing failed\n")
			builder.WriteString("  Cause: Network timeout\n")
			builder.WriteString("  Retry: Will retry in 30 seconds\n")
		}
	}
	largeInput := builder.String()

	matcher, err := NewMultiLinePatternMatcher(`(?s)ERROR.*Batch.*failed.*Cause:.*Network.*timeout`)
	if err != nil {
		t.Fatalf("NewMultiLinePatternMatcher() error = %v", err)
	}

	// Performance test - should complete within 50ms for 1000 lines
	start := time.Now()
	result, err := matcher.Match(largeInput)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("MultiLinePatternMatcher.Match() error = %v", err)
	}

	if !result {
		t.Error("MultiLinePatternMatcher.Match() = false, want true")
	}

	if duration > 50*time.Millisecond {
		t.Errorf("MultiLinePatternMatcher.Match() took %v, want < 50ms", duration)
	}

	t.Logf("Performance test completed in %v", duration)
}

func TestMultiLinePatternMatcher_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		input    string
		expected bool
	}{
		{
			name:     "Very long single line",
			pattern:  `error.*failed`,
			input:    "This is a very long line with error in the middle and failed at the end " + strings.Repeat("x", 10000),
			expected: true,
		},
		{
			name:     "Many short lines",
			pattern:  `(?s)start.*end`,
			input:    "start\n" + strings.Repeat("line\n", 1000) + "end",
			expected: true,
		},
		{
			name:     "Unicode content",
			pattern:  `错误.*失败`,
			input:    "系统启动\n错误: 数据库连接失败\n请检查配置",
			expected: true,
		},
		{
			name:     "Mixed line endings",
			pattern:  `ERROR.*\r?\n.*FATAL`,
			input:    "ERROR: Something went wrong\r\nFATAL: System shutdown",
			expected: true,
		},
		{
			name:     "Empty lines in between",
			pattern:  `(?s)START.*END`,
			input:    "START\n\n\n\nEND",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewMultiLinePatternMatcher(tt.pattern)
			if err != nil {
				t.Errorf("NewMultiLinePatternMatcher() error = %v", err)
				return
			}

			result, err := matcher.Match(tt.input)
			if err != nil {
				t.Errorf("MultiLinePatternMatcher.Match() error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("MultiLinePatternMatcher.Match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMultiLinePatternMatcher_Integration(t *testing.T) {
	// Test integration with existing pattern set system
	patternSet := &PatternSet{
		Name:        "multiline_test",
		Version:     "1.0",
		Description: "Multi-line pattern test set",
		Patterns: map[string]PatternDefinition{
			"stack_trace": {
				Pattern:     "multiline:java_exception",
				Description: "Java exception stack trace",
				Priority:    PriorityHigh,
				Category:    CategoryError,
			},
			"json_error": {
				Pattern:     "multiline:json_error",
				Description: "Multi-line JSON error",
				Priority:    PriorityMedium,
				Category:    CategoryError,
			},
		},
	}

	javaStackTrace := `Exception in thread "main" java.lang.RuntimeException: Test error
	at com.example.Test.main(Test.java:10)`

	result, err := patternSet.Match(javaStackTrace)
	if err != nil {
		t.Errorf("PatternSet.Match() error = %v", err)
		return
	}

	if !result.Matched {
		t.Error("PatternSet.Match() expected to match Java stack trace")
		return
	}

	if result.PatternName != "stack_trace" {
		t.Errorf("PatternSet.Match() matched %s, want stack_trace", result.PatternName)
	}
}
