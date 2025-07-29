package daemon

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredLogging(t *testing.T) {
	t.Run("NewLogger creates logger with correct level", func(t *testing.T) {
		tests := []struct {
			level    LogLevel
			expected slog.Level
		}{
			{LogLevelDebug, slog.LevelDebug},
			{LogLevelInfo, slog.LevelInfo},
			{LogLevelWarn, slog.LevelWarn},
			{LogLevelError, slog.LevelError},
		}

		for _, tt := range tests {
			t.Run(string(tt.level), func(t *testing.T) {
				logger := NewLogger("test", tt.level)
				assert.NotNil(t, logger)
				assert.Equal(t, "test", logger.component)
			})
		}
	})

	t.Run("Logger outputs structured JSON", func(t *testing.T) {
		var buf bytes.Buffer

		// Create logger with custom handler that writes to buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		logger := &Logger{
			Logger:    slog.New(handler),
			component: "test-component",
		}

		logger.Info("test message", "key", "value", "number", 42)

		// Parse the JSON output
		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "INFO", logEntry["level"])
		assert.Equal(t, "test message", logEntry["msg"])
		assert.Equal(t, "test-component", logEntry["component"])
		assert.Equal(t, "value", logEntry["key"])
		assert.Equal(t, float64(42), logEntry["number"]) // JSON numbers are float64
		assert.Contains(t, logEntry, "time")
	})

	t.Run("WithComponent creates logger with new component", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		originalLogger := &Logger{
			Logger:    slog.New(handler),
			component: "original",
		}

		newLogger := originalLogger.WithComponent("new-component")
		newLogger.Info("test message")

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "new-component", logEntry["component"])
	})

	t.Run("WithRequest adds request context", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "test",
		}

		requestLogger := logger.WithRequest("req-123")
		requestLogger.Info("processing request")

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "req-123", logEntry["request_id"])
		assert.Equal(t, "test", logEntry["component"])
	})

	t.Run("LogDaemonStart includes startup information", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "daemon",
		}

		logger.LogDaemonStart(8080, "1.0.0")

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "daemon starting", logEntry["msg"])
		assert.Equal(t, float64(8080), logEntry["port"])
		assert.Equal(t, "1.0.0", logEntry["version"])
		assert.Contains(t, logEntry, "pid")
	})

	t.Run("LogClientConnection includes client information", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "server",
		}

		logger.LogClientConnection("client-456", "connected")

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "client connection", logEntry["msg"])
		assert.Equal(t, "client-456", logEntry["client_id"])
		assert.Equal(t, "connected", logEntry["action"])
	})

	t.Run("LogScheduleRequest includes scheduling details", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "scheduler",
		}

		logger.LogScheduleRequest("api-resource", true, "within rate limit")

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "schedule request processed", logEntry["msg"])
		assert.Equal(t, "api-resource", logEntry["resource_id"])
		assert.Equal(t, true, logEntry["can_schedule"])
		assert.Equal(t, "within rate limit", logEntry["reason"])
	})

	t.Run("LogError includes error context", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelError,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "daemon",
		}

		testErr := assert.AnError
		logger.LogError("database_connection", testErr, "database", "postgres", "retry_count", 3)

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "ERROR", logEntry["level"])
		assert.Equal(t, "operation failed", logEntry["msg"])
		assert.Equal(t, "database_connection", logEntry["operation"])
		assert.Equal(t, testErr.Error(), logEntry["error"])
		assert.Equal(t, "postgres", logEntry["database"])
		assert.Equal(t, float64(3), logEntry["retry_count"])
	})

	t.Run("Different log levels work correctly", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "test",
		}

		logger.Debug("debug message")
		logger.Info("info message")
		logger.Warn("warn message")
		logger.Error("error message")

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		assert.Len(t, lines, 4)

		// Check each log level
		levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
		messages := []string{"debug message", "info message", "warn message", "error message"}

		for i, line := range lines {
			var logEntry map[string]interface{}
			err := json.Unmarshal([]byte(line), &logEntry)
			require.NoError(t, err)

			assert.Equal(t, levels[i], logEntry["level"])
			assert.Equal(t, messages[i], logEntry["msg"])
			assert.Equal(t, "test", logEntry["component"])
		}
	})
}

func TestLogLevelFiltering(t *testing.T) {
	t.Run("higher log levels filter out lower levels", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelWarn, // Only WARN and ERROR should appear
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "test",
		}

		logger.Debug("debug message") // Should be filtered out
		logger.Info("info message")   // Should be filtered out
		logger.Warn("warn message")   // Should appear
		logger.Error("error message") // Should appear

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")

		// Should only have 2 lines (WARN and ERROR)
		assert.Len(t, lines, 2)

		// Check that only WARN and ERROR appear
		for i, line := range lines {
			var logEntry map[string]interface{}
			err := json.Unmarshal([]byte(line), &logEntry)
			require.NoError(t, err)

			if i == 0 {
				assert.Equal(t, "WARN", logEntry["level"])
				assert.Equal(t, "warn message", logEntry["msg"])
			} else {
				assert.Equal(t, "ERROR", logEntry["level"])
				assert.Equal(t, "error message", logEntry["msg"])
			}
		}
	})
}

func BenchmarkStructuredLogging(b *testing.B) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := &Logger{
		Logger:    slog.New(handler),
		component: "benchmark",
	}

	b.Run("Info_with_context", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info("benchmark message", "iteration", i, "component", "test")
		}
	})

	b.Run("LogDaemonStart", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.LogDaemonStart(8080, "1.0.0")
		}
	})

	b.Run("LogScheduleRequest", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.LogScheduleRequest("resource", i%2 == 0, "test reason")
		}
	})
}
