package genai

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		expected ErrorAction
	}{
		// Nil error
		{
			name:     "nil error",
			err:      nil,
			expected: ActionFail,
		},

		// Context errors
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: ActionFail,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: ActionRetry,
		},

		// Wrapped LLMError
		{
			name: "LLMError 429",
			err: &LLMError{
				Err:        errors.New("rate limited"),
				StatusCode: http.StatusTooManyRequests,
			},
			expected: ActionRetry,
		},
		{
			name: "LLMError 500",
			err: &LLMError{
				Err:        errors.New("server error"),
				StatusCode: http.StatusInternalServerError,
			},
			expected: ActionRetry,
		},
		{
			name: "LLMError 400",
			err: &LLMError{
				Err:        errors.New("bad request"),
				StatusCode: http.StatusBadRequest,
			},
			expected: ActionFail,
		},
		{
			name: "LLMError 401",
			err: &LLMError{
				Err:        errors.New("unauthorized"),
				StatusCode: http.StatusUnauthorized,
			},
			expected: ActionFail,
		},

		// Error message patterns - Quota
		{
			name:     "quota exhausted",
			err:      errors.New("quota exceeded"),
			expected: ActionFallback,
		},
		{
			name:     "resource exhausted",
			err:      errors.New("RESOURCE_EXHAUSTED: quota limit"),
			expected: ActionFallback,
		},
		{
			name:     "daily quota limit",
			err:      errors.New("daily quota limit reached"),
			expected: ActionFallback,
		},
		{
			name:     "monthly quota limit",
			err:      errors.New("monthly quota limit exceeded"),
			expected: ActionFallback,
		},

		// Error message patterns - Rate limit (retry)
		{
			name:     "rate limit",
			err:      errors.New("rate limit exceeded temporarily"),
			expected: ActionRetry,
		},
		{
			name:     "too many requests",
			err:      errors.New("too many requests"),
			expected: ActionRetry,
		},

		// Error message patterns - Transient
		{
			name:     "service unavailable",
			err:      errors.New("service temporarily unavailable"),
			expected: ActionRetry,
		},
		{
			name:     "internal server error",
			err:      errors.New("internal server error"),
			expected: ActionRetry,
		},
		{
			name:     "bad gateway",
			err:      errors.New("bad gateway"),
			expected: ActionRetry,
		},
		{
			name:     "gateway timeout",
			err:      errors.New("gateway timeout"),
			expected: ActionRetry,
		},
		{
			name:     "overloaded",
			err:      errors.New("server overloaded"),
			expected: ActionRetry,
		},

		// Error message patterns - Permanent
		{
			name:     "invalid request",
			err:      errors.New("invalid request format"),
			expected: ActionFail,
		},
		{
			name:     "unauthorized",
			err:      errors.New("unauthorized access"),
			expected: ActionFail,
		},
		{
			name:     "invalid api key",
			err:      errors.New("invalid api key"),
			expected: ActionFail,
		},
		{
			name:     "forbidden",
			err:      errors.New("forbidden"),
			expected: ActionFail,
		},
		{
			name:     "not found",
			err:      errors.New("resource not found"),
			expected: ActionFail,
		},

		// Unknown errors default to retry
		{
			name:     "unknown error",
			err:      errors.New("something unexpected happened"),
			expected: ActionRetry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ClassifyError(tt.err)
			if result != tt.expected {
				t.Errorf("ClassifyError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestClassifyStatusCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code     int
		expected ErrorAction
	}{
		// Retry
		{http.StatusTooManyRequests, ActionRetry},
		{http.StatusRequestTimeout, ActionRetry},
		{http.StatusConflict, ActionRetry},
		{http.StatusInternalServerError, ActionRetry},
		{http.StatusBadGateway, ActionRetry},
		{http.StatusServiceUnavailable, ActionRetry},
		{http.StatusGatewayTimeout, ActionRetry},

		// Fail
		{http.StatusBadRequest, ActionFail},
		{http.StatusUnauthorized, ActionFail},
		{http.StatusForbidden, ActionFail},
		{http.StatusNotFound, ActionFail},
		{http.StatusUnprocessableEntity, ActionFail},

		// Unknown defaults to retry
		{0, ActionRetry},
		{999, ActionRetry},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.code), func(t *testing.T) {
			t.Parallel()
			result := classifyStatusCode(tt.code)
			if result != tt.expected {
				t.Errorf("classifyStatusCode(%d) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		headers  http.Header
		expected time.Duration
	}{
		{
			name:     "empty headers",
			headers:  http.Header{},
			expected: 0,
		},
		{
			name: "retry-after-ms",
			headers: http.Header{
				"Retry-After-Ms": []string{"1500"},
			},
			expected: 1500 * time.Millisecond,
		},
		{
			name: "retry-after seconds",
			headers: http.Header{
				"Retry-After": []string{"5"},
			},
			expected: 5 * time.Second,
		},
		{
			name: "x-ratelimit-reset-tokens",
			headers: http.Header{
				"X-Ratelimit-Reset-Tokens": []string{"2s"},
			},
			expected: 2 * time.Second,
		},
		{
			name: "priority: retry-after-ms over retry-after",
			headers: http.Header{
				"Retry-After-Ms": []string{"500"},
				"Retry-After":    []string{"5"},
			},
			expected: 500 * time.Millisecond,
		},
		{
			name: "invalid value",
			headers: http.Header{
				"Retry-After": []string{"invalid"},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := ParseRetryAfter(tt.headers)
			if result != tt.expected {
				t.Errorf("ParseRetryAfter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLLMError(t *testing.T) {
	t.Parallel()
	t.Run("error with status code", func(t *testing.T) {
		t.Parallel()
		err := &LLMError{
			Err:        errors.New("test error"),
			StatusCode: 429,
			Provider:   ProviderGemini,
		}

		if !errors.Is(err, err.Err) {
			t.Error("Unwrap should return underlying error")
		}

		expected := "test error (status: 429)"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("error without status code", func(t *testing.T) {
		t.Parallel()
		err := &LLMError{
			Err:      errors.New("test error"),
			Provider: ProviderGroq,
		}

		expected := "test error"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})
}

func TestErrorActionString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		action   ErrorAction
		expected string
	}{
		{ActionRetry, "retry"},
		{ActionFallback, "fallback"},
		{ActionFail, "fail"},
		{ErrorAction(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			if got := tt.action.String(); got != tt.expected {
				t.Errorf("ErrorAction.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Parallel()
	t.Run("IsRetryable", func(t *testing.T) {
		t.Parallel()
		if !IsRetryable(errors.New("service unavailable")) {
			t.Error("should be retryable")
		}
		if IsRetryable(errors.New("invalid api key")) {
			t.Error("should not be retryable")
		}
	})

	t.Run("IsPermanent", func(t *testing.T) {
		t.Parallel()
		if !IsPermanent(errors.New("invalid api key")) {
			t.Error("should be permanent")
		}
		if IsPermanent(errors.New("service unavailable")) {
			t.Error("should not be permanent")
		}
	})

	t.Run("ShouldFallback", func(t *testing.T) {
		t.Parallel()
		if !ShouldFallback(errors.New("quota exceeded")) {
			t.Error("should fallback")
		}
		if ShouldFallback(errors.New("service unavailable")) {
			t.Error("should not fallback (should retry instead)")
		}
	})

	t.Run("WrapError", func(t *testing.T) {
		t.Parallel()
		wrapped := WrapError(errors.New("test"), ProviderGemini, http.StatusTooManyRequests)
		var llmErr *LLMError
		if !errors.As(wrapped, &llmErr) {
			t.Error("should be LLMError")
		}
		if llmErr.Provider != ProviderGemini {
			t.Error("wrong provider")
		}
		if llmErr.StatusCode != http.StatusTooManyRequests {
			t.Error("wrong status code")
		}
	})

	t.Run("WrapError nil", func(t *testing.T) {
		t.Parallel()
		if WrapError(nil, ProviderGemini, http.StatusTooManyRequests) != nil {
			t.Error("should return nil for nil error")
		}
	})
}
