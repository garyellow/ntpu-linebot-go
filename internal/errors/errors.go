// Package errors provides domain-specific error types and sentinel errors
// for improved error handling across the application.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors for common scenarios.
// Use errors.Is() to check these errors in your code.
var (
	// ErrNotFound indicates a requested resource was not found.
	ErrNotFound = errors.New("resource not found")

	// ErrCacheExpired indicates cached data has exceeded TTL.
	ErrCacheExpired = errors.New("cache expired")

	// ErrRateLimitExceeded indicates rate limit has been exceeded.
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrInvalidInput indicates user provided invalid input.
	ErrInvalidInput = errors.New("invalid input")

	// ErrTimeout indicates an operation timed out.
	ErrTimeout = errors.New("operation timed out")

	// ErrContextCanceled indicates context was canceled.
	ErrContextCanceled = errors.New("context canceled")

	// ErrMissingParameter indicates a required parameter is missing in NLU intent.
	ErrMissingParameter = errors.New("missing required parameter")

	// ErrUnknownIntent indicates an unknown intent was received from NLU.
	ErrUnknownIntent = errors.New("unknown intent")
)

// ValidationError represents input validation failures.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed on %s: %s", e.Field, e.Message)
}

// NewValidationError creates a new validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// ScraperError represents web scraping failures with context.
type ScraperError struct {
	URL        string
	StatusCode int
	Err        error
}

func (e *ScraperError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("scraper error (url=%s, status=%d): %v", e.URL, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("scraper error (url=%s): %v", e.URL, e.Err)
}

func (e *ScraperError) Unwrap() error {
	return e.Err
}

// NewScraperError creates a new scraper error.
func NewScraperError(url string, statusCode int, err error) *ScraperError {
	return &ScraperError{
		URL:        url,
		StatusCode: statusCode,
		Err:        err,
	}
}
