package scraper

import (
	"context"
	"testing"
	"time"
)

func TestRetryWithBackoff_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	attempts := 0

	fn := func() error {
		attempts++
		if attempts == 3 {
			return nil // Success on 3rd attempt
		}
		return &testError{"temporary error"}
	}

	// maxRetries=5, initialDelay=10ms
	err := RetryWithBackoff(ctx, 5, 10*time.Millisecond, fn)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	attempts := 0
	expectedError := &testError{"permanent error"}

	fn := func() error {
		attempts++
		return expectedError
	}

	err := RetryWithBackoff(ctx, 3, 10*time.Millisecond, fn)
	if err == nil {
		t.Fatal("Expected error after max retries")
	}

	// Should try: initial + 3 retries = 4 total
	if attempts != 4 {
		t.Errorf("Expected 4 attempts (initial + 3 retries), got %d", attempts)
	}

	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error '%v', got '%v'", expectedError, err)
	}
}

func TestRetryWithBackoff_ContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	fn := func() error {
		attempts++
		if attempts == 2 {
			cancel() // Cancel context on 2nd attempt
		}
		return &testError{"error"}
	}

	err := RetryWithBackoff(ctx, 5, 10*time.Millisecond, fn)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestRetryWithBackoff_ExponentialBackoff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	attempts := 0
	timestamps := []time.Time{}

	fn := func() error {
		attempts++
		timestamps = append(timestamps, time.Now())
		return &testError{"error"}
	}

	_ = RetryWithBackoff(ctx, 3, 50*time.Millisecond, fn)

	if len(timestamps) < 2 {
		t.Fatal("Need at least 2 attempts to test backoff")
	}

	// Check that delays increase (with jitter, so allow some variance)
	for i := 1; i < len(timestamps)-1; i++ {
		delay := timestamps[i+1].Sub(timestamps[i])

		// Delay should be at least minDelay (50ms) but not too long
		if delay < 40*time.Millisecond {
			t.Errorf("Delay %d too short: %v", i, delay)
		}
	}
}

func TestSleep_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	start := time.Now()
	err := Sleep(ctx, 50*time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// Should wait at least 50ms
	if elapsed < 40*time.Millisecond {
		t.Errorf("Expected delay of ~50ms, got %v", elapsed)
	}
}

func TestSleep_ContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	start := time.Now()
	err := Sleep(ctx, 1*time.Second)
	elapsed := time.Since(start)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}

	// Should return immediately (not wait 1 second)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected immediate return, but waited %v", elapsed)
	}
}

// testError is a custom error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
