package genai

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCalculateBackoff(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		initial time.Duration
		max     time.Duration
		// We test ranges since Full Jitter is random
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{
			name:        "first attempt (no delay)",
			attempt:     0,
			initial:     time.Second,
			max:         10 * time.Second,
			minExpected: 0,
			maxExpected: 0,
		},
		{
			name:        "second attempt",
			attempt:     1,
			initial:     time.Second,
			max:         10 * time.Second,
			minExpected: 0,
			maxExpected: time.Second, // random(0, 1s)
		},
		{
			name:        "third attempt",
			attempt:     2,
			initial:     time.Second,
			max:         10 * time.Second,
			minExpected: 0,
			maxExpected: 2 * time.Second, // random(0, 2s)
		},
		{
			name:        "capped at max",
			attempt:     10,
			initial:     time.Second,
			max:         5 * time.Second,
			minExpected: 0,
			maxExpected: 5 * time.Second, // random(0, cap=5s)
		},
		{
			name:        "negative attempt",
			attempt:     -1,
			initial:     time.Second,
			max:         10 * time.Second,
			minExpected: 0,
			maxExpected: 0,
		},
		{
			name:        "zero initial delay",
			attempt:     1,
			initial:     0,
			max:         10 * time.Second,
			minExpected: 0,
			maxExpected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to verify randomness
			for i := 0; i < 10; i++ {
				result := CalculateBackoff(tt.attempt, tt.initial, tt.max)
				if result < tt.minExpected || result > tt.maxExpected {
					t.Errorf("CalculateBackoff(%d, %v, %v) = %v, want in range [%v, %v]",
						tt.attempt, tt.initial, tt.max, result, tt.minExpected, tt.maxExpected)
				}
			}
		})
	}
}

func TestSleep(t *testing.T) {
	t.Run("normal sleep", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()
		err := Sleep(ctx, 50*time.Millisecond)
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Sleep returned error: %v", err)
		}
		if elapsed < 50*time.Millisecond {
			t.Errorf("Sleep returned too early: %v", elapsed)
		}
	})

	t.Run("zero duration", func(t *testing.T) {
		ctx := context.Background()
		err := Sleep(ctx, 0)
		if err != nil {
			t.Errorf("Sleep(0) returned error: %v", err)
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		ctx := context.Background()
		err := Sleep(ctx, -time.Second)
		if err != nil {
			t.Errorf("Sleep(-1s) returned error: %v", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel immediately
		cancel()

		err := Sleep(ctx, time.Hour)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Sleep with canceled context should return context.Canceled, got: %v", err)
		}
	})

	t.Run("context timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := Sleep(ctx, time.Hour)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Sleep with expired context should return DeadlineExceeded, got: %v", err)
		}
	})
}

func TestWithRetry(t *testing.T) {
	t.Run("success on first attempt", func(t *testing.T) {
		cfg := RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		}

		attempts := 0
		err := WithRetry(context.Background(), cfg, func() error {
			attempts++
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if attempts != 1 {
			t.Errorf("expected 1 attempt, got: %d", attempts)
		}
	})

	t.Run("success after retry", func(t *testing.T) {
		cfg := RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		}

		attempts := 0
		err := WithRetry(context.Background(), cfg, func() error {
			attempts++
			if attempts < 2 {
				return errors.New("service unavailable") // retryable
			}
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if attempts != 2 {
			t.Errorf("expected 2 attempts, got: %d", attempts)
		}
	})

	t.Run("permanent error stops retry", func(t *testing.T) {
		cfg := RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		}

		attempts := 0
		err := WithRetry(context.Background(), cfg, func() error {
			attempts++
			return errors.New("invalid api key") // permanent
		})

		if err == nil {
			t.Error("expected error")
		}
		if attempts != 1 {
			t.Errorf("expected 1 attempt (permanent error), got: %d", attempts)
		}
	})

	t.Run("exhausted retries", func(t *testing.T) {
		cfg := RetryConfig{
			MaxAttempts:  2,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		}

		attempts := 0
		err := WithRetry(context.Background(), cfg, func() error {
			attempts++
			return errors.New("service unavailable") // retryable
		})

		if err == nil {
			t.Error("expected error after exhausted retries")
		}
		if attempts != 2 {
			t.Errorf("expected 2 attempts, got: %d", attempts)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		cfg := RetryConfig{
			MaxAttempts:  5,
			InitialDelay: time.Hour,
			MaxDelay:     time.Hour,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := WithRetry(ctx, cfg, func() error {
			return errors.New("service unavailable")
		})

		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})
}

func TestWithRetryAndMetrics(t *testing.T) {
	t.Run("calls onRetry callback", func(t *testing.T) {
		cfg := RetryConfig{
			MaxAttempts:  3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		}

		var retryAttempts []int
		var retryErrors []error

		attempts := 0
		_ = WithRetryAndMetrics(context.Background(), cfg, func(attempt int, err error) {
			retryAttempts = append(retryAttempts, attempt)
			retryErrors = append(retryErrors, err)
		}, func() error {
			attempts++
			if attempts < 3 {
				return errors.New("service unavailable")
			}
			return nil
		})

		if len(retryAttempts) != 2 {
			t.Errorf("expected 2 retry callbacks, got: %d", len(retryAttempts))
		}
		if len(retryErrors) != 2 {
			t.Errorf("expected 2 retry errors, got: %d", len(retryErrors))
		}
	})

	t.Run("nil callback is safe", func(t *testing.T) {
		cfg := RetryConfig{
			MaxAttempts:  2,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		}

		// Should not panic with nil callback
		_ = WithRetryAndMetrics(context.Background(), cfg, nil, func() error {
			return errors.New("service unavailable")
		})
	})
}

func TestRemainingBudget(t *testing.T) {
	t.Run("no deadline", func(t *testing.T) {
		ctx := context.Background()
		budget := RemainingBudget(ctx)
		if budget != 0 {
			t.Errorf("expected 0 for no deadline, got: %v", budget)
		}
	})

	t.Run("with deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()

		budget := RemainingBudget(ctx)
		if budget < 59*time.Minute || budget > time.Hour {
			t.Errorf("expected ~1h budget, got: %v", budget)
		}
	})

	t.Run("expired deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		time.Sleep(time.Millisecond) // Ensure deadline passes
		defer cancel()

		budget := RemainingBudget(ctx)
		if budget >= 0 {
			t.Errorf("expected negative budget for expired deadline, got: %v", budget)
		}
	})
}

func TestHasSufficientBudget(t *testing.T) {
	t.Run("no deadline has unlimited budget", func(t *testing.T) {
		ctx := context.Background()
		if !HasSufficientBudget(ctx, time.Hour) {
			t.Error("no deadline should have unlimited budget")
		}
	})

	t.Run("sufficient budget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()

		if !HasSufficientBudget(ctx, time.Minute) {
			t.Error("should have sufficient budget")
		}
	})

	t.Run("insufficient budget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		if HasSufficientBudget(ctx, time.Hour) {
			t.Error("should not have sufficient budget")
		}
	})
}
