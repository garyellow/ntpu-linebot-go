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
	"time"

	slogbetterstack "github.com/samber/slog-betterstack"
)

// Logger is the application logger
type Logger struct {
	*slog.Logger
	shutdown func(context.Context) error
}

// Options configures logger outputs and Better Stack integration.
type Options struct {
	BetterStackToken    string
	BetterStackEndpoint string
	Version             string
}

// New creates a new logger instance with JSON formatting
func New(level string) *Logger {
	return NewWithOptions(level, os.Stdout, Options{})
}

// NewWithWriter creates a new logger instance with JSON formatting writing to the provided writer
func NewWithWriter(level string, w io.Writer) *Logger {
	return NewWithOptions(level, w, Options{})
}

// NewWithOptions creates a new logger instance with configurable sinks.
// When BetterStackToken is provided, logs are also sent to Better Stack.
func NewWithOptions(level string, w io.Writer, opts Options) *Logger {
	logLevel := parseLevel(level)
	replaceAttr := replaceAttrFunc()

	jsonHandler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       logLevel,
		AddSource:   true,
		ReplaceAttr: replaceAttr,
	})

	handlers := []slog.Handler{jsonHandler}
	var asyncShutdown func(context.Context) error
	if opts.BetterStackToken != "" {
		bsOption := slogbetterstack.Option{
			Level:       logLevel,
			Token:       opts.BetterStackToken,
			Endpoint:    opts.BetterStackEndpoint,
			Timeout:     5 * time.Second,
			ReplaceAttr: replaceAttr,
		}
		asyncHandler := NewAsyncHandler(bsOption.NewBetterstackHandler(), AsyncOptions{})
		asyncShutdown = asyncHandler.Shutdown
		handlers = append(handlers, asyncHandler)
	}

	var handler slog.Handler
	if len(handlers) == 1 {
		handler = handlers[0]
	} else {
		handler = NewMultiHandler(handlers...)
	}

	contextHandler := NewContextHandler(handler)
	baseLogger := slog.New(contextHandler)
	if opts.Version != "" {
		baseLogger = baseLogger.With("version", opts.Version)
	}
	return &Logger{Logger: baseLogger, shutdown: asyncShutdown}
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func replaceAttrFunc() func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
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
	}
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
	l.Logger.InfoContext(ctx, msg, args...)
}

// WarnContext logs a message at warn level with tracing data from context.
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Logger.WarnContext(ctx, msg, args...)
}

// ErrorContext logs a message at error level with tracing data from context.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Logger.ErrorContext(ctx, msg, args...)
}

// DebugContext logs a message at debug level with tracing data from context.
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Logger.DebugContext(ctx, msg, args...)
}

// Shutdown flushes any async logging pipelines (best-effort).
func (l *Logger) Shutdown(ctx context.Context) error {
	if l == nil || l.shutdown == nil {
		return nil
	}
	return l.shutdown(ctx)
}
