package errors

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		checkFn  func(error) bool
		expected bool
	}{
		{
			name:     "ErrNotFound is recognized",
			err:      ErrNotFound,
			checkFn:  IsNotFound,
			expected: true,
		},
		{
			name:     "Wrapped ErrNotFound is recognized",
			err:      errors.Join(ErrNotFound, errors.New("additional context")),
			checkFn:  IsNotFound,
			expected: true,
		},
		{
			name:     "Different error is not ErrNotFound",
			err:      ErrRateLimitExceeded,
			checkFn:  IsNotFound,
			expected: false,
		},
		{
			name:     "ErrRateLimitExceeded is recognized",
			err:      ErrRateLimitExceeded,
			checkFn:  IsRateLimitExceeded,
			expected: true,
		},
		{
			name:     "ErrInvalidInput is recognized",
			err:      ErrInvalidInput,
			checkFn:  IsInvalidInput,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.checkFn(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := NewValidationError("email", "invalid format")

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
	baseErr := errors.New("connection timeout")
	err := NewScraperError("https://example.com", 500, baseErr)

	if err.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got '%s'", err.URL)
	}

	if err.StatusCode != 500 {
		t.Errorf("expected status code 500, got %d", err.StatusCode)
	}

	if !errors.Is(err, baseErr) {
		t.Error("expected error to wrap base error")
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("expected non-empty error message")
	}

	// Test without status code
	err2 := NewScraperError("https://example.com", 0, baseErr)
	errMsg2 := err2.Error()
	if errMsg2 == "" {
		t.Error("expected non-empty error message")
	}
}
