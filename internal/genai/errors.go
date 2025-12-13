// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains error classification and handling for retry/fallback logic.
package genai

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrorAction defines the action to take based on error type.
type ErrorAction int

const (
	// ActionRetry indicates the request should be retried with the same provider/model.
	ActionRetry ErrorAction = iota
	// ActionFallback indicates fallback to another provider should be attempted.
	ActionFallback
	// ActionFail indicates the request should fail immediately (permanent error).
	ActionFail
)

// String returns a human-readable string for the error action.
func (a ErrorAction) String() string {
	switch a {
	case ActionRetry:
		return "retry"
	case ActionFallback:
		return "fallback"
	case ActionFail:
		return "fail"
	default:
		return "unknown"
	}
}

// LLMError wraps an error with additional context for retry/fallback decisions.
type LLMError struct {
	Err        error
	StatusCode int
	Provider   Provider
	Retryable  bool
}

// Error implements the error interface.
func (e *LLMError) Error() string {
	if e.StatusCode > 0 {
		return e.Err.Error() + " (status: " + strconv.Itoa(e.StatusCode) + ")"
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *LLMError) Unwrap() error {
	return e.Err
}

// ClassifyError determines the appropriate action based on the error.
// This follows industry best practices for LLM API error handling:
//   - Transient errors (429, 5xx, network) → Retry
//   - Quota exhaustion → Fallback to other provider
//   - Permanent errors (400, 401, 403, 404) → Fail immediately
func ClassifyError(err error) ErrorAction {
	if err == nil {
		return ActionFail
	}

	// Check for context errors first
	if errors.Is(err, context.Canceled) {
		return ActionFail
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// Timeout can be retried, but may need fallback if persistent
		return ActionRetry
	}

	// Check for wrapped LLMError
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return classifyStatusCode(llmErr.StatusCode)
	}

	// Parse error message for status codes and patterns
	errStr := strings.ToLower(err.Error())

	// Check for quota exhaustion first (more severe, immediate fallback)
	if containsAny(errStr, "quota", "daily limit", "monthly limit", "billing", "quota exceeded") {
		return ActionFallback // Quota exhausted, try other provider
	}

	// Then check for rate limiting (transient, can retry)
	if containsAny(errStr, "rate limit", "too many requests", "resource_exhausted") {
		return ActionRetry // Rate limit, can retry after backoff
	}

	// Check for transient errors (retry)
	if containsAny(errStr, "unavailable", "503", "502", "500", "504",
		"service temporarily unavailable", "internal server error",
		"bad gateway", "gateway timeout", "overloaded", "capacity") {
		return ActionRetry
	}

	// Check for rate limiting (retry with backoff)
	if containsAny(errStr, "429", "rate limit", "too many") {
		return ActionRetry
	}

	// Check for timeout/conflict (retry)
	if containsAny(errStr, "408", "409", "timeout", "deadline", "connection") {
		return ActionRetry
	}

	// Check for permanent errors (fail immediately)
	if containsAny(errStr, "400", "invalid", "bad request", "malformed") {
		return ActionFail
	}
	if containsAny(errStr, "401", "unauthorized", "unauthenticated", "invalid api key") {
		return ActionFail
	}
	if containsAny(errStr, "403", "forbidden", "permission denied") {
		return ActionFail
	}
	if containsAny(errStr, "404", "not found") {
		return ActionFail
	}
	if containsAny(errStr, "422", "unprocessable") {
		return ActionFail
	}

	// Default: retry for unknown errors (conservative approach)
	return ActionRetry
}

// classifyStatusCode determines action based on HTTP status code.
func classifyStatusCode(statusCode int) ErrorAction {
	switch {
	// Retry: rate limit, timeout, server errors
	case statusCode == http.StatusTooManyRequests: // 429
		return ActionRetry
	case statusCode == http.StatusRequestTimeout: // 408
		return ActionRetry
	case statusCode == http.StatusConflict: // 409
		return ActionRetry
	case statusCode >= 500 && statusCode < 600: // 5xx
		return ActionRetry

	// Fail: client errors (except those above)
	case statusCode == http.StatusBadRequest: // 400
		return ActionFail
	case statusCode == http.StatusUnauthorized: // 401
		return ActionFail
	case statusCode == http.StatusForbidden: // 403
		return ActionFail
	case statusCode == http.StatusNotFound: // 404
		return ActionFail
	case statusCode == http.StatusUnprocessableEntity: // 422
		return ActionFail
	case statusCode >= 400 && statusCode < 500: // other 4xx
		return ActionFail

	default:
		return ActionRetry // Unknown, try again
	}
}

// ParseRetryAfter parses the Retry-After header value.
// Supports both integer seconds and HTTP-date formats.
// Returns 0 if header is missing or invalid.
func ParseRetryAfter(headers http.Header) time.Duration {
	// Priority 1: retry-after-ms (milliseconds, non-standard but precise)
	if msStr := headers.Get("retry-after-ms"); msStr != "" {
		if ms, err := strconv.Atoi(msStr); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}

	// Priority 2: retry-after (seconds, standard)
	if secStr := headers.Get("retry-after"); secStr != "" {
		// Try as integer seconds
		if sec, err := strconv.Atoi(secStr); err == nil && sec > 0 {
			return time.Duration(sec) * time.Second
		}
		// Try as HTTP-date (RFC 1123)
		if t, err := http.ParseTime(secStr); err == nil {
			return time.Until(t)
		}
	}

	// Priority 3: Groq-specific headers
	if resetStr := headers.Get("x-ratelimit-reset-tokens"); resetStr != "" {
		if d, err := time.ParseDuration(resetStr); err == nil {
			return d
		}
	}

	return 0
}

// ShouldFallback returns true if the error warrants trying another provider.
func ShouldFallback(err error) bool {
	return ClassifyError(err) == ActionFallback
}

// IsRetryable returns true if the error is transient and can be retried.
func IsRetryable(err error) bool {
	return ClassifyError(err) == ActionRetry
}

// IsPermanent returns true if the error is permanent and should not be retried.
func IsPermanent(err error) bool {
	return ClassifyError(err) == ActionFail
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// WrapError wraps an error with provider and status code information.
func WrapError(err error, provider Provider, statusCode int) error {
	if err == nil {
		return nil
	}
	return &LLMError{
		Err:        err,
		StatusCode: statusCode,
		Provider:   provider,
		Retryable:  ClassifyError(err) == ActionRetry,
	}
}
