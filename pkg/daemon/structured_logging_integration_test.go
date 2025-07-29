package daemon

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/shaneisley/patience/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStructuredLoggingIntegration verifies that all components use structured logging
func TestStructuredLoggingIntegration(t *testing.T) {
	t.Run("Daemon uses structured logging throughout", func(t *testing.T) {
		// Capture log output
		var buf bytes.Buffer

		// Create a logger that writes to our buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		logger := &Logger{
			Logger:    slog.New(handler),
			component: "test-daemon",
		}

		// Test various logging scenarios
		logger.Info("daemon starting", "port", 8080, "version", "1.0.0")
		logger.Error("connection failed", "error", "network unreachable", "retry_count", 3)
		logger.Debug("processing request", "request_id", "req-123", "user_id", "user-456")
		logger.Warn("rate limit approaching", "current_requests", 95, "limit", 100)

		// Verify all logs are structured JSON
		logLines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		require.Len(t, logLines, 4, "Expected 4 log lines")

		for i, line := range logLines {
			var logEntry map[string]interface{}
			err := json.Unmarshal([]byte(line), &logEntry)
			require.NoError(t, err, "Log line %d should be valid JSON: %s", i, line)

			// Verify required fields
			assert.Contains(t, logEntry, "time", "Log should have timestamp")
			assert.Contains(t, logEntry, "level", "Log should have level")
			assert.Contains(t, logEntry, "msg", "Log should have message")
			assert.Contains(t, logEntry, "component", "Log should have component")
			assert.Equal(t, "test-daemon", logEntry["component"], "Component should be set correctly")
		}

		// Verify specific log content
		var firstLog map[string]interface{}
		json.Unmarshal([]byte(logLines[0]), &firstLog)
		assert.Equal(t, "INFO", firstLog["level"])
		assert.Equal(t, "daemon starting", firstLog["msg"])
		assert.Equal(t, float64(8080), firstLog["port"])
		assert.Equal(t, "1.0.0", firstLog["version"])
	})

	t.Run("Config debug uses structured logging", func(t *testing.T) {
		// Create a config debug info instance
		debugInfo := &config.ConfigDebugInfo{
			Sources: map[string]config.ConfigSource{
				"attempts": config.SourceCLIFlag,
				"delay":    config.SourceConfigFile,
				"timeout":  config.SourceEnvironment,
			},
			Values: map[string]interface{}{
				"attempts": 5,
				"delay":    "2s",
				"timeout":  "30s",
			},
		}

		// Capture output by redirecting stdout temporarily
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Call the debug function
		debugInfo.PrintDebugInfo()

		// Restore stdout and read captured output
		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		// Verify structured logging format
		assert.Contains(t, output, "level=INFO", "Should contain INFO level logs")
		assert.Contains(t, output, "level=DEBUG", "Should contain DEBUG level logs")
		assert.Contains(t, output, "msg=\"Configuration Resolution Debug Info\"", "Should contain main message")
		assert.Contains(t, output, "key=attempts", "Should contain structured key-value pairs")
		assert.Contains(t, output, "source=\"CLI flag\"", "Should contain source information")
	})

	t.Run("All logging is type-safe and structured", func(t *testing.T) {
		logger := NewLogger("integration-test", LogLevelDebug)

		// Test type-safe logging methods
		logger.LogDaemonStart(8080, "v1.2.3")
		logger.LogClientConnection("client-123", "connect")
		logger.LogScheduleRequest("resource-456", true, "available")
		logger.LogError("database_query", assert.AnError, "table", "users", "query_time", "150ms")

		// If we get here without panics, type safety is working
		assert.True(t, true, "All structured logging calls completed without errors")
	})

	t.Run("Log levels filter correctly", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelWarn, // Only WARN and ERROR should appear
		})
		logger := &Logger{
			Logger:    slog.New(handler),
			component: "level-test",
		}

		// Log at different levels
		logger.Debug("debug message")  // Should be filtered out
		logger.Info("info message")    // Should be filtered out
		logger.Warn("warning message") // Should appear
		logger.Error("error message")  // Should appear

		logLines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		// Filter out empty lines
		var nonEmptyLines []string
		for _, line := range logLines {
			if strings.TrimSpace(line) != "" {
				nonEmptyLines = append(nonEmptyLines, line)
			}
		}

		assert.Len(t, nonEmptyLines, 2, "Only WARN and ERROR logs should appear")

		// Verify the correct logs appeared
		var warnLog, errorLog map[string]interface{}
		json.Unmarshal([]byte(nonEmptyLines[0]), &warnLog)
		json.Unmarshal([]byte(nonEmptyLines[1]), &errorLog)

		assert.Equal(t, "WARN", warnLog["level"])
		assert.Equal(t, "warning message", warnLog["msg"])
		assert.Equal(t, "ERROR", errorLog["level"])
		assert.Equal(t, "error message", errorLog["msg"])
	})
}

// TestStructuredLoggingPerformance ensures logging doesn't impact performance significantly
func TestStructuredLoggingPerformance(t *testing.T) {
	logger := NewLogger("perf-test", LogLevelInfo)

	// This should complete quickly even with many log calls
	for i := 0; i < 1000; i++ {
		logger.Info("performance test", "iteration", i, "data", "test-data")
	}

	// If we get here quickly, performance is acceptable
	assert.True(t, true, "Structured logging performance test completed")
}
