// Package logger provides structured logging utilities for the application.
// It wraps log/slog with JSON formatting and supports context-based logging
// with request IDs and module names.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
)

// Logger is the application logger
type Logger struct {
	*slog.Logger
}

// New creates a new logger instance with JSON formatting
func New(level string) *Logger {
	return NewWithWriter(level, os.Stdout)
}

// NewWithWriter creates a new logger instance with JSON formatting writing to the provided writer
func NewWithWriter(level string, w io.Writer) *Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Key = "timestamp"
				// slog uses RFC3339Nano by default, which is fine
			}
			if a.Key == slog.LevelKey {
				a.Key = "level"
				level := a.Value.String()
				if level == "WARN" {
					level = "warning"
				} else {
					level = strings.ToLower(level)
				}
				a.Value = slog.StringValue(level)
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	}
	// Wrap the base handler with ContextHandler to automatically extract
	// tracing values (userID, chatID, requestID) from context
	baseHandler := slog.NewJSONHandler(w, opts)
	contextHandler := NewContextHandler(baseHandler)
	return &Logger{Logger: slog.New(contextHandler)}
}

// WithModule creates a new entry with module field
func (l *Logger) WithModule(module string) *Logger {
	return &Logger{Logger: l.With("module", module)}
}

// WithRequestID creates a new entry with request ID field
func (l *Logger) WithRequestID(requestID string) *Logger {
	return &Logger{Logger: l.With("request_id", requestID)}
}

// WithError creates a new entry with error field
func (l *Logger) WithError(err error) *Logger {
	return &Logger{Logger: l.With("error", err)}
}

// WithField creates a new entry with a single field
func (l *Logger) WithField(key string, value any) *Logger {
	return &Logger{Logger: l.With(key, value)}
}

// WithFields creates a new entry with multiple fields
func (l *Logger) WithFields(fields map[string]any) *Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{Logger: l.With(args...)}
}

// Compatibility methods for logrus-style formatting

// Infof logs a formatted message at info level.
func (l *Logger) Infof(format string, args ...any) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warnf logs a formatted message at warn level.
func (l *Logger) Warnf(format string, args ...any) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs a formatted message at error level.
func (l *Logger) Errorf(format string, args ...any) {
	l.Error(fmt.Sprintf(format, args...))
}

// Debugf logs a formatted message at debug level.
func (l *Logger) Debugf(format string, args ...any) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Context-aware methods for better tracing and cancellation support

// InfoContext logs a message at info level with tracing data from context.
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.withContextFields(ctx).Info(msg, args...)
}

// WarnContext logs a message at warn level with tracing data from context.
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.withContextFields(ctx).Warn(msg, args...)
}

// ErrorContext logs a message at error level with tracing data from context.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.withContextFields(ctx).Error(msg, args...)
}

// DebugContext logs a message at debug level with tracing data from context.
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.withContextFields(ctx).Debug(msg, args...)
}

// withContextFields extracts tracing fields from context and returns a logger with those fields.
func (l *Logger) withContextFields(ctx context.Context) *Logger {
	logger := l

	// Add userID if present
	if userID := ctxutil.GetUserID(ctx); userID != "" {
		logger = logger.WithField("user_id", userID)
	}

	// Add chatID if present
	if chatID := ctxutil.GetChatID(ctx); chatID != "" {
		logger = logger.WithField("chat_id", chatID)
	}

	// Add requestID if present
	if requestID, ok := ctxutil.GetRequestID(ctx); ok && requestID != "" {
		logger = logger.WithRequestID(requestID)
	}

	return logger
}
