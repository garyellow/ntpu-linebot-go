// Package logger provides structured logging utilities for the application.
// It wraps logrus with JSON formatting and supports context-based logging
// with request IDs and module names.
package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
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
