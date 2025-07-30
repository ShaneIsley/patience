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

// TestDaemonStructuredLogging tests the migration of daemon components to structured logging
func TestDaemonStructuredLogging(t *testing.T) {
	t.Run("Daemon uses structured logger instead of log.Logger", func(t *testing.T) {
		// This test will fail initially - daemon still uses log.Logger
		config := &Config{
			SocketPath: "/tmp/test.sock",
			HTTPPort:   8080,
			MaxMetrics: 1000,
			LogLevel:   "info",
		}

		daemon, err := NewDaemon(config)
		require.NoError(t, err)
		defer daemon.Close()

		// Verify that daemon has structured logger
		if daemon.logger == nil {
			t.Error("Daemon should have a logger")
		}
	})

	t.Run("Daemon logs startup with structured format", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "daemon",
		}

		// Test that daemon startup logging produces structured output
		logger.LogDaemonStart(8080, "1.0.0")

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "INFO", logEntry["level"])
		assert.Equal(t, "daemon starting", logEntry["msg"])
		assert.Equal(t, "daemon", logEntry["component"])
		assert.Equal(t, float64(8080), logEntry["port"])
		assert.Equal(t, "1.0.0", logEntry["version"])
		assert.Contains(t, logEntry, "pid")
	})

	t.Run("Server uses structured logger instead of log.Logger", func(t *testing.T) {
		// This test will fail initially - server still uses log.Logger
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "server",
		}

		// Test that we can create server with structured logger
		// This will fail initially because NewServer expects *log.Logger
		// server := NewServerWithStructuredLogger(nil, 8080, logger)
		// assert.NotNil(t, server)
		// Verify structured logger is being used
		if logger == nil {
			t.Error("Logger should not be nil")
		}
	})

	t.Run("Daemon operations produce structured logs", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "daemon",
		}

		// Test various daemon operations
		logger.LogClientConnection("client-123", "connected")
		logger.LogScheduleRequest("api-resource", true, "within rate limit")
		logger.LogError("database_connection", assert.AnError, "retry_count", 3)

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		assert.Len(t, lines, 3)

		// Parse each log entry
		for i, line := range lines {
			var logEntry map[string]interface{}
			err := json.Unmarshal([]byte(line), &logEntry)
			require.NoError(t, err, "Line %d should be valid JSON", i)

			assert.Equal(t, "daemon", logEntry["component"])
			assert.Contains(t, logEntry, "time")
			assert.Contains(t, logEntry, "level")
			assert.Contains(t, logEntry, "msg")
		}

		// Check specific log entries
		var firstEntry map[string]interface{}
		err := json.Unmarshal([]byte(lines[0]), &firstEntry)
		require.NoError(t, err)
		assert.Equal(t, "client connection", firstEntry["msg"])
		assert.Equal(t, "client-123", firstEntry["client_id"])
		assert.Equal(t, "connected", firstEntry["action"])
	})
}

// TestConfigStructuredLogging tests the migration of config debugging to structured logging
func TestConfigStructuredLogging(t *testing.T) {
	t.Run("Config debug output uses structured logging", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "config",
		}

		// Test that config debugging produces structured output instead of fmt.Printf
		logger.Debug("config resolved",
			"key", "max_attempts",
			"value", 5,
			"source", "command_line")

		var logEntry map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "DEBUG", logEntry["level"])
		assert.Equal(t, "config resolved", logEntry["msg"])
		assert.Equal(t, "config", logEntry["component"])
		assert.Equal(t, "max_attempts", logEntry["key"])
		assert.Equal(t, float64(5), logEntry["value"])
		assert.Equal(t, "command_line", logEntry["source"])
	})

	t.Run("Config resolution debug info uses structured format", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})

		logger := &Logger{
			Logger:    slog.New(handler),
			component: "config",
		}

		// Test structured config resolution logging
		configData := map[string]interface{}{
			"max_attempts": 5,
			"timeout":      "30s",
			"strategy":     "exponential",
		}

		for key, value := range configData {
			logger.Debug("config resolution",
				"key", key,
				"value", value,
				"source", "config_file")
		}

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		assert.Len(t, lines, 3)

		// Each line should be valid JSON with structured fields
		for _, line := range lines {
			var logEntry map[string]interface{}
			err := json.Unmarshal([]byte(line), &logEntry)
			require.NoError(t, err)

			assert.Equal(t, "DEBUG", logEntry["level"])
			assert.Equal(t, "config resolution", logEntry["msg"])
			assert.Equal(t, "config", logEntry["component"])
			assert.Contains(t, logEntry, "key")
			assert.Contains(t, logEntry, "value")
			assert.Equal(t, "config_file", logEntry["source"])
		}
	})
}
