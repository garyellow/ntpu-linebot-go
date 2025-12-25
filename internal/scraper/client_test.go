package scraper

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "permanent error",
			err:      &permanentError{err: errors.New("client error")},
			expected: false,
		},
		{
			name:     "wrapped permanent error",
			err:      fmt.Errorf("wrapped: %w", &permanentError{err: errors.New("client error")}),
			expected: false,
		},
		{
			name:     "timeout error",
			err:      &netTimeError{timeout: true},
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp 127.0.0.1:8080: connection refused"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("read: connection reset by peer"),
			expected: true,
		},
		{
			name:     "server error",
			err:      errors.New("internal server error"),
			expected: true,
		},
		{
			name:     "rate limited",
			err:      errors.New("rate limited"),
			expected: true,
		},
		{
			name:     "unknown generic error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNetworkError(tt.err); got != tt.expected {
				t.Errorf("IsNetworkError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// netTimeError mocks a net.Error with Timeout() support
type netTimeError struct {
	timeout   bool
	temporary bool
}

func (e *netTimeError) Error() string   { return "net error" }
func (e *netTimeError) Timeout() bool   { return e.timeout }
func (e *netTimeError) Temporary() bool { return e.temporary }
