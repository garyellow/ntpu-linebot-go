package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()
	l := New(10, 5)
	if l.maxTokens != 10 {
		t.Errorf("maxTokens = %v, want 10", l.maxTokens)
	}
	if l.refillRate != 5 {
		t.Errorf("refillRate = %v, want 5", l.refillRate)
	}
	if l.tokens != 10 {
		t.Errorf("initial tokens = %v, want 10", l.tokens)
	}
}

func TestNewPerMinute(t *testing.T) {
	t.Parallel()
	l := NewPerMinute(60) // 60 per minute = 1 per second
	if l.refillRate != 1 {
		t.Errorf("refillRate = %v, want 1", l.refillRate)
	}
	if l.maxTokens != 2 { // 2 seconds burst
		t.Errorf("maxTokens = %v, want 2", l.maxTokens)
	}
}

func TestAllow(t *testing.T) {
	t.Parallel()
	t.Run("allows when tokens available", func(t *testing.T) {
		t.Parallel()
		l := New(5, 1)
		for i := 0; i < 5; i++ {
			if !l.Allow() {
				t.Errorf("Allow() = false on attempt %d, want true", i+1)
			}
		}
	})

	t.Run("denies when no tokens", func(t *testing.T) {
		t.Parallel()
		l := New(2, 0) // No refill
		l.Allow()
		l.Allow()
		if l.Allow() {
			t.Error("Allow() = true when no tokens, want false")
		}
	})

	t.Run("refills over time", func(t *testing.T) {
		t.Parallel()
		l := New(1, 100) // Fast refill for testing
		l.Allow()        // Consume the token

		// Wait for refill
		time.Sleep(20 * time.Millisecond)

		if !l.Allow() {
			t.Error("Allow() = false after refill time, want true")
		}
	})
}

func TestWait(t *testing.T) {
	t.Parallel()
	t.Run("returns immediately when tokens available", func(t *testing.T) {
		t.Parallel()
		l := New(5, 1)
		ctx := context.Background()

		start := time.Now()
		err := l.Wait(ctx)
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
		if elapsed > 10*time.Millisecond {
			t.Errorf("Wait() took %v, expected immediate return", elapsed)
		}
	})

	t.Run("waits for token", func(t *testing.T) {
		t.Parallel()
		l := New(1, 50) // 50 tokens/sec = 20ms per token
		l.Allow()       // Consume the token

		ctx := context.Background()
		start := time.Now()
		err := l.Wait(ctx)
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
		if elapsed < 15*time.Millisecond {
			t.Errorf("Wait() took %v, expected ~20ms wait", elapsed)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		l := New(0, 0.1) // Very slow refill

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := l.Wait(ctx)
		if err != context.DeadlineExceeded {
			t.Errorf("Wait() error = %v, want context.DeadlineExceeded", err)
		}
	})
}

func TestWaitSimple(t *testing.T) {
	t.Parallel()
	l := New(1, 100) // 100 tokens/sec = 10ms per token
	l.Allow()        // Consume the initial token

	done := make(chan struct{})
	go func() {
		l.WaitSimple()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(200 * time.Millisecond):
		t.Error("WaitSimple() did not return in time")
	}
}

func TestAvailable(t *testing.T) {
	t.Parallel()
	l := New(10, 1)
	l.Allow()
	l.Allow()

	available := l.Available()
	// Allow some tolerance for timing
	if available < 7.9 || available > 8.1 {
		t.Errorf("Available() = %v, want ~8", available)
	}
}

func TestReset(t *testing.T) {
	t.Parallel()
	l := New(10, 1)
	l.Allow()
	l.Allow()
	l.Allow()

	l.Reset()

	if l.tokens != 10 {
		t.Errorf("tokens after Reset() = %v, want 10", l.tokens)
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	l := New(100, 100)

	var wg sync.WaitGroup
	allowed := make(chan struct{}, 200)

	// Spawn 50 goroutines each trying to get 2 tokens
	for range 50 {
		wg.Go(func() {
			if l.Allow() {
				allowed <- struct{}{}
			}
			if l.Allow() {
				allowed <- struct{}{}
			}
		})
	}

	wg.Wait()
	close(allowed)

	count := 0
	for range allowed {
		count++
	}

	// Should have allowed exactly 100 (initial tokens)
	if count != 100 {
		t.Errorf("concurrent Allow() allowed %d requests, want 100", count)
	}
}

func TestIsFull(t *testing.T) {
	t.Parallel()
	t.Run("full when at max capacity", func(t *testing.T) {
		t.Parallel()
		l := New(10, 1)
		if !l.IsFull() {
			t.Error("IsFull() = false for new limiter, want true")
		}
	})

	t.Run("not full after consuming tokens", func(t *testing.T) {
		t.Parallel()
		l := New(10, 0) // No refill
		l.Allow()
		if l.IsFull() {
			t.Error("IsFull() = true after Allow(), want false")
		}
	})

	t.Run("becomes full after refill", func(t *testing.T) {
		t.Parallel()
		l := New(1, 100) // Fast refill
		l.Allow()        // Consume

		// Wait for refill
		time.Sleep(20 * time.Millisecond)

		if !l.IsFull() {
			t.Error("IsFull() = false after refill, want true")
		}
	})
}
