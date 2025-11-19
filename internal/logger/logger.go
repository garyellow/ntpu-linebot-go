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

// Global logger instance
var defaultLogger *Logger

// Init initializes the global logger
func Init(level string) {
	defaultLogger = New(level)
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if defaultLogger == nil {
		defaultLogger = New("info")
	}
	return defaultLogger
}

// Convenience functions for global logger
func Info(args ...interface{}) {
	GetLogger().Info(args...)
}

func Infof(format string, args ...interface{}) {
	GetLogger().Infof(format, args...)
}

func Warn(args ...interface{}) {
	GetLogger().Warn(args...)
}

func Warnf(format string, args ...interface{}) {
	GetLogger().Warnf(format, args...)
}

func Error(args ...interface{}) {
	GetLogger().Error(args...)
}

func Errorf(format string, args ...interface{}) {
	GetLogger().Errorf(format, args...)
}

func Debug(args ...interface{}) {
	GetLogger().Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	GetLogger().Debugf(format, args...)
}

func Fatal(args ...interface{}) {
	GetLogger().Fatal(args...)
}

func Fatalf(format string, args ...interface{}) {
	GetLogger().Fatalf(format, args...)
}

func WithModule(module string) *logrus.Entry {
	return GetLogger().WithModule(module)
}

func WithRequestID(requestID string) *logrus.Entry {
	return GetLogger().WithRequestID(requestID)
}

func WithError(err error) *logrus.Entry {
	return GetLogger().WithError(err)
}

func WithContext(ctx context.Context) *logrus.Entry {
	return GetLogger().WithContext(ctx)
}
