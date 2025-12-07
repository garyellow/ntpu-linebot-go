// Package errors provides error wrapping utilities for consistent error handling.
package errors

import (
	"fmt"
)

// ErrorWrapper provides context-aware error wrapping.
type ErrorWrapper struct {
	operation string
	module    string
}

// NewWrapper creates a new error wrapper with operation and module context.
func NewWrapper(module, operation string) *ErrorWrapper {
	return &ErrorWrapper{
		module:    module,
		operation: operation,
	}
}

// Wrap wraps an error with operation context.
// Returns nil if err is nil.
func (w *ErrorWrapper) Wrap(err error, userMessage string) error {
	if err == nil {
		return nil
	}
	return &WrappedError{
		Operation:   w.operation,
		Module:      w.module,
		Cause:       err,
		UserMessage: userMessage,
	}
}

// Wrapf wraps an error with formatted message.
func (w *ErrorWrapper) Wrapf(err error, userMessageFormat string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return &WrappedError{
		Operation:   w.operation,
		Module:      w.module,
		Cause:       err,
		UserMessage: fmt.Sprintf(userMessageFormat, args...),
	}
}

// WrappedError contains both internal error details and user-facing message.
type WrappedError struct {
	Operation   string // Operation being performed (e.g., "search_student", "get_course")
	Module      string // Module name (e.g., "id", "course", "contact")
	Cause       error  // Underlying error
	UserMessage string // User-friendly message
}

func (e *WrappedError) Error() string {
	return fmt.Sprintf("[%s:%s] %s: %v", e.Module, e.Operation, e.UserMessage, e.Cause)
}

func (e *WrappedError) Unwrap() error {
	return e.Cause
}

// GetUserMessage returns the user-friendly message from a WrappedError.
// Returns the error string if not a WrappedError.
func GetUserMessage(err error) string {
	if err == nil {
		return ""
	}
	if wrapped, ok := err.(*WrappedError); ok {
		return wrapped.UserMessage
	}
	return err.Error()
}
