package sentry

import (
	"testing"
	"time"
)

func TestInitialize_EmptyDSN(t *testing.T) {
	// Should return nil when DSN is empty (disabled)
	err := Initialize(Config{DSN: ""})
	if err != nil {
		t.Errorf("Expected nil error for empty DSN, got %v", err)
	}

	// IsEnabled should return false
	if IsEnabled() {
		t.Error("Expected IsEnabled() to return false when DSN is empty")
	}
}

func TestInitialize_InvalidSampleRate(t *testing.T) {
	err := Initialize(Config{
		DSN:        "https://test-token@errors.betterstack.com/1",
		SampleRate: 1.2,
	})
	if err == nil {
		t.Error("Expected error for invalid sample rate")
	}
}

func TestInitialize_ValidConfig(t *testing.T) {
	// Cannot use t.Parallel() as Sentry uses global state

	err := Initialize(Config{
		DSN:              "https://test-token@errors.betterstack.com/1",
		Environment:      "test",
		SampleRate:       1.0,
		TracesSampleRate: 0.0,
		HTTPTimeout:      2 * time.Second,
		ServiceName:      "ntpu-linebot-go",
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

func TestFlush(t *testing.T) {
	// Flush should complete quickly when there are no events
	result := Flush(100 * time.Millisecond)
	if !result {
		t.Error("Expected Flush to return true when no events pending")
	}
}
