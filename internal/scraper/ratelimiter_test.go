package scraper

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	workers := 5
	minDelay := 100 * time.Millisecond
	maxDelay := 500 * time.Millisecond

	rl := NewRateLimiter(workers, minDelay, maxDelay)

	if rl.maxTokens != float64(workers) {
		t.Errorf("Expected maxTokens %d, got %f", workers, rl.maxTokens)
	}

	if rl.tokens != float64(workers) {
		t.Errorf("Expected initial tokens %d, got %f", workers, rl.tokens)
	}

	if rl.minDelay != minDelay {
		t.Errorf("Expected minDelay %v, got %v", minDelay, rl.minDelay)
	}

	if rl.maxDelay != maxDelay {
		t.Errorf("Expected maxDelay %v, got %v", maxDelay, rl.maxDelay)
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow rate limiter test in short mode")
	}

	rl := NewRateLimiter(2, 10*time.Millisecond, 20*time.Millisecond)
	ctx := context.Background()

	// First two should be immediate (tokens available)
	for i := 0; i < 2; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("Wait %d failed: %v", i+1, err)
		}
	}

	// Third should wait for token refill
	// refillRate = 2/15 = 0.133 tokens/sec, so need ~7.5 seconds for 1 token
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	elapsed := time.Since(start)

	// Should take at least some time for refill
	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected wait time >= 100ms, got %v", elapsed)
	}

	// Should not take more than 10 seconds (refill + max delay)
	if elapsed > 10*time.Second {
		t.Errorf("Wait took too long: %v", elapsed)
	}
}

func TestRateLimiter_WaitContextCanceled(t *testing.T) {
	rl := NewRateLimiter(1, 10*time.Millisecond, 20*time.Millisecond)

	// Exhaust tokens
	_ = rl.Wait(context.Background())

	// Create context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.Wait(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow test in short mode")
	}

	rl := NewRateLimiter(5, 1*time.Millisecond, 2*time.Millisecond)
	ctx := context.Background()

	// Consume all tokens
	for i := 0; i < 5; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("Wait failed: %v", err)
		}
	}

	// Wait for refill (refillRate = 5/15 = 0.333 tokens/sec, so need ~3 sec for 1 token)
	time.Sleep(3 * time.Second)

	// Should be able to get a token now
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait failed after refill: %v", err)
	}
	elapsed := time.Since(start)

	// Should be quick since tokens were refilled (just delay time)
	if elapsed > 1*time.Second {
		t.Errorf("Wait after refill took too long: %v", elapsed)
	}
}

func TestRandomDelay(t *testing.T) {
	minDelay := 100 * time.Millisecond
	maxDelay := 500 * time.Millisecond
	rl := NewRateLimiter(5, minDelay, maxDelay)

	// Test multiple random delays
	for i := 0; i < 100; i++ {
		delay := rl.randomDelay()

		if delay < minDelay {
			t.Errorf("Random delay %v is less than minDelay %v", delay, minDelay)
		}

		if delay > maxDelay {
			t.Errorf("Random delay %v is greater than maxDelay %v", delay, maxDelay)
		}
	}
}

func TestRandomDelay_EqualMinMax(t *testing.T) {
	delay := 100 * time.Millisecond
	rl := NewRateLimiter(5, delay, delay)

	result := rl.randomDelay()
	if result != delay {
		t.Errorf("Expected delay %v when min=max, got %v", delay, result)
	}
}

func TestRetryWithBackoff_Success(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	fn := func() error {
		attempts++
		if attempts == 3 {
			return nil // Success on 3rd attempt
		}
		return &testError{"temporary error"}
	}

	err := RetryWithBackoff(ctx, 5, 10*time.Millisecond, 100*time.Millisecond, fn)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithBackoff_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	expectedError := &testError{"permanent error"}

	fn := func() error {
		attempts++
		return expectedError
	}

	err := RetryWithBackoff(ctx, 3, 10*time.Millisecond, 100*time.Millisecond, fn)
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
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	fn := func() error {
		attempts++
		if attempts == 2 {
			cancel() // Cancel context on 2nd attempt
		}
		return &testError{"error"}
	}

	err := RetryWithBackoff(ctx, 5, 10*time.Millisecond, 100*time.Millisecond, fn)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestRetryWithBackoff_ExponentialBackoff(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	timestamps := []time.Time{}

	fn := func() error {
		attempts++
		timestamps = append(timestamps, time.Now())
		return &testError{"error"}
	}

	_ = RetryWithBackoff(ctx, 3, 50*time.Millisecond, 500*time.Millisecond, fn)

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
		if delay > 600*time.Millisecond {
			t.Errorf("Delay %d too long: %v", i, delay)
		}
	}
}

// testError is a custom error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
