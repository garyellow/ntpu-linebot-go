package sentry

import (
	"testing"
	"time"
)

func TestInitialize_EmptyToken(t *testing.T) {
	t.Parallel()

	// Should return nil when token is empty (disabled)
	err := Initialize(Config{Token: ""})
	if err != nil {
		t.Errorf("Expected nil error for empty token, got %v", err)
	}

	// IsEnabled should return false
	if IsEnabled() {
		t.Error("Expected IsEnabled() to return false when token is empty")
	}
}

func TestInitialize_MissingHost(t *testing.T) {
	t.Parallel()

	// Should return error when token is set but host is empty
	err := Initialize(Config{Token: "test-token", Host: ""})
	if err == nil {
		t.Error("Expected error when host is missing")
	}
}

func TestInitialize_ValidConfig(t *testing.T) {
	// Cannot use t.Parallel() as Sentry uses global state

	err := Initialize(Config{
		Token:       "test-token",
		Host:        "errors.betterstack.com",
		Environment: "test",
		SampleRate:  1.0,
	})
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	if !IsEnabled() {
		t.Error("Expected IsEnabled() to return true after initialization")
	}

	// Clean up - flush any pending events
	Flush(time.Second)
}

func TestInitialize_DefaultSampleRate(t *testing.T) {
	// Cannot use t.Parallel() as Sentry uses global state

	// Zero sample rate should default to 1.0
	err := Initialize(Config{
		Token:      "test-token-2",
		Host:       "errors.betterstack.com",
		SampleRate: 0,
	})
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	Flush(time.Second)
}

func TestFlush(t *testing.T) {
	t.Parallel()

	// Flush should complete quickly when there are no events
	result := Flush(100 * time.Millisecond)
	if !result {
		t.Error("Expected Flush to return true when no events pending")
	}
}
