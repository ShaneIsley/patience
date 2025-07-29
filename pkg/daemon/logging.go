package daemon

import (
	"log/slog"
	"os"
)

// LogLevel represents the logging level
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Logger wraps slog.Logger with daemon-specific functionality
type Logger struct {
	*slog.Logger
	component string
}

// NewLogger creates a new structured logger for daemon components
func NewLogger(component string, level LogLevel) *Logger {
	var slogLevel slog.Level
	switch level {
	case LogLevelDebug:
		slogLevel = slog.LevelDebug
	case LogLevelInfo:
		slogLevel = slog.LevelInfo
	case LogLevelWarn:
		slogLevel = slog.LevelWarn
	case LogLevelError:
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})

	logger := slog.New(handler)

	return &Logger{
		Logger:    logger,
		component: component,
	}
}

// WithComponent creates a logger with component context
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger:    l.Logger.With("component", component),
		component: component,
	}
}

// WithRequest creates a logger with request context
func (l *Logger) WithRequest(requestID string) *Logger {
	return &Logger{
		Logger:    l.Logger.With("request_id", requestID),
		component: l.component,
	}
}

// Debug logs a debug message with component context
func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, append([]any{"component", l.component}, args...)...)
}

// Info logs an info message with component context
func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, append([]any{"component", l.component}, args...)...)
}

// Warn logs a warning message with component context
func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, append([]any{"component", l.component}, args...)...)
}

// Error logs an error message with component context
func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, append([]any{"component", l.component}, args...)...)
}

// LogDaemonStart logs daemon startup information
func (l *Logger) LogDaemonStart(port int, version string) {
	l.Info("daemon starting",
		"port", port,
		"version", version,
		"pid", os.Getpid())
}

// LogClientConnection logs client connection events
func (l *Logger) LogClientConnection(clientID string, action string) {
	l.Info("client connection",
		"client_id", clientID,
		"action", action)
}

// LogScheduleRequest logs schedule request processing
func (l *Logger) LogScheduleRequest(resourceID string, canSchedule bool, reason string) {
	l.Info("schedule request processed",
		"resource_id", resourceID,
		"can_schedule", canSchedule,
		"reason", reason)
}

// LogError logs error events with context
func (l *Logger) LogError(operation string, err error, context ...any) {
	args := append([]any{"operation", operation, "error", err.Error()}, context...)
	l.Error("operation failed", args...)
}
