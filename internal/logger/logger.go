package logger

import (
	"context"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

// contextKey is the type for context keys
type contextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey contextKey = "request_id"
	// ModuleKey is the context key for module name
	ModuleKey contextKey = "module"
)

// Logger is the application logger
type Logger struct {
	*logrus.Logger
}

// New creates a new logger instance with JSON formatting
func New(level string) *Logger {
	log := logrus.New()

	// Set log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logLevel = logrus.InfoLevel
	}
	log.SetLevel(logLevel)

	// Use JSON formatter for structured logging
	log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "level",
			logrus.FieldKeyMsg:   "message",
		},
	})

	// Output to stdout
	log.SetOutput(os.Stdout)

	return &Logger{Logger: log}
}

// WithContext creates a new entry with context fields
func (l *Logger) WithContext(ctx context.Context) *logrus.Entry {
	entry := l.WithFields(logrus.Fields{})

	// Add request ID if present
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		entry = entry.WithField("request_id", requestID)
	}

	// Add module name if present
	if module := ctx.Value(ModuleKey); module != nil {
		entry = entry.WithField("module", module)
	}

	return entry
}

// WithModule creates a new entry with module field
func (l *Logger) WithModule(module string) *logrus.Entry {
	return l.WithField("module", module)
}

// WithRequestID creates a new entry with request ID field
func (l *Logger) WithRequestID(requestID string) *logrus.Entry {
	return l.WithField("request_id", requestID)
}

// WithError creates a new entry with error field
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// SetOutput sets the logger output
func (l *Logger) SetOutput(output io.Writer) {
	l.Logger.SetOutput(output)
}

// SetLevel sets the logger level
func (l *Logger) SetLevel(level string) error {
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	l.Logger.SetLevel(logLevel)
	return nil
}
