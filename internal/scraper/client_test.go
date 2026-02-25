package scraper

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
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
			t.Parallel()
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

// TestWaitForDomain verifies per-domain rate limiting hostname matching logic.
func TestWaitForDomain(t *testing.T) {
	t.Parallel()

	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw", "https://lms2.ntpu.edu.tw"},
		"sea": {"https://sea.cc.ntpu.edu.tw"},
	}
	client := NewClient(10*time.Second, 1, baseURLs)
	ctx := context.Background()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "Matching LMS primary URL",
			url:     "https://lms.ntpu.edu.tw/course/list",
			wantErr: false,
		},
		{
			name:    "Matching LMS failover URL",
			url:     "https://lms2.ntpu.edu.tw/course/list",
			wantErr: false,
		},
		{
			name:    "Matching SEA URL",
			url:     "https://sea.cc.ntpu.edu.tw/pls/usys/query",
			wantErr: false,
		},
		{
			name:    "Unknown domain - no limiter, proceeds immediately",
			url:     "https://example.com/page",
			wantErr: false,
		},
		{
			name:    "Invalid URL - returns nil (no block)",
			url:     "://invalid",
			wantErr: false,
		},
		{
			name:    "Empty URL - returns nil",
			url:     "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := client.waitForDomain(ctx, tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("waitForDomain(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// TestWaitForDomain_ContextCanceled verifies that waitForDomain respects context cancellation
// when a matching domain limiter is found and tokens are exhausted.
func TestWaitForDomain_ContextCanceled(t *testing.T) {
	t.Parallel()

	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw"},
	}
	// Very low rate: burst 1, 0.1 rps → second request must wait ~10s
	client := NewClient(10*time.Second, 1, map[string][]string{})
	client.baseURLs = baseURLs
	client.domainLimiters = map[string]*ratelimit.Limiter{
		"lms": ratelimit.New(1, 0.1),
	}

	ctx := context.Background()

	// First call consumes the only token — should succeed immediately
	if err := client.waitForDomain(ctx, "https://lms.ntpu.edu.tw/page"); err != nil {
		t.Fatalf("first waitForDomain() should succeed: %v", err)
	}

	// Second call with already-canceled context should fail fast
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	err := client.waitForDomain(canceledCtx, "https://lms.ntpu.edu.tw/page")
	if err == nil {
		t.Fatal("waitForDomain() with canceled context should return error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
