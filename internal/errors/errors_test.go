package errors

import (
	"errors"
	"net/http"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		err         error
		sentinel    error
		shouldMatch bool
	}{
		{
			name:        "ErrNotFound is recognized",
			err:         ErrNotFound,
			sentinel:    ErrNotFound,
			shouldMatch: true,
		},
		{
			name:        "Wrapped ErrNotFound is recognized",
			err:         errors.Join(ErrNotFound, errors.New("additional context")),
			sentinel:    ErrNotFound,
			shouldMatch: true,
		},
		{
			name:        "Different error is not ErrNotFound",
			err:         ErrRateLimitExceeded,
			sentinel:    ErrNotFound,
			shouldMatch: false,
		},
		{
			name:        "ErrRateLimitExceeded is recognized",
			err:         ErrRateLimitExceeded,
			sentinel:    ErrRateLimitExceeded,
			shouldMatch: true,
		},
		{
			name:        "ErrInvalidInput is recognized",
			err:         ErrInvalidInput,
			sentinel:    ErrInvalidInput,
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := errors.Is(tt.err, tt.sentinel)
			if result != tt.shouldMatch {
				t.Errorf("errors.Is() = %v, want %v", result, tt.shouldMatch)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	t.Parallel()
	err := &ValidationError{
		Field:   "email",
		Message: "invalid format",
	}

	if err.Field != "email" {
		t.Errorf("expected field 'email', got '%s'", err.Field)
	}

	if err.Message != "invalid format" {
		t.Errorf("expected message 'invalid format', got '%s'", err.Message)
	}

	expected := "validation failed on email: invalid format"
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestScraperError(t *testing.T) {
	t.Parallel()
	baseErr := errors.New("connection timeout")
	err := &ScraperError{
		URL:        "https://example.com",
		StatusCode: 500,
		Err:        baseErr,
	}

	if err.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got '%s'", err.URL)
	}

	if err.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status code 500, got %d", err.StatusCode)
	}

	if !errors.Is(err, baseErr) {
		t.Error("expected error to wrap base error")
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}

	err2 := &ScraperError{
		URL:        "https://example.com",
		StatusCode: 0,
		Err:        baseErr,
	}
	errMsg2 := err2.Error()
	if errMsg2 == "" {
		t.Error("expected non-empty error message")
	}
}
