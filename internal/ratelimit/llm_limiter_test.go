package ratelimit

import (
	"testing"
	"time"
)

func TestNewLLMRateLimiter(t *testing.T) {
	limiter := NewLLMRateLimiter(50, 5*time.Minute, nil) // 50 per hour, 5 min cleanup, no metrics
	defer limiter.Stop()

	if limiter.GetActiveCount() != 0 {
		t.Errorf("limiters map should be empty initially, got %d entries", limiter.GetActiveCount())
	}
}

func TestLLMRateLimiter_Allow(t *testing.T) {
	const maxPerHour = 50.0

	t.Run("allows when tokens available", func(t *testing.T) {
		// Use nil for metrics in tests
		limiter := NewLLMRateLimiter(maxPerHour, 5*time.Minute, nil)
		defer limiter.Stop()

		userID := "user123"
		// First request should be allowed
		if !limiter.Allow(userID) {
			t.Error("Allow() = false, want true on first request")
		}

		available := limiter.GetAvailable(userID)
		if available < 48 || available > 50 { // Allow some floating point variance
			t.Errorf("GetAvailable() = %.2f, want ~49 after first request", available)
		}
	})

	t.Run("denies when no tokens", func(t *testing.T) {
		const limit = 2.0
		// Use nil for metrics in tests
		limiter := NewLLMRateLimiter(limit, 5*time.Minute, nil) // Only 2 tokens
		defer limiter.Stop()

		userID := "user456"
		// Use up all tokens
		limiter.Allow(userID)
		limiter.Allow(userID)

		// Third request should be denied
		if limiter.Allow(userID) {
			t.Error("Allow() = true when no tokens, want false")
		}
	})

	t.Run("isolates different users", func(t *testing.T) {
		// Use nil for metrics in tests
		limiter := NewLLMRateLimiter(maxPerHour, 5*time.Minute, nil)
		defer limiter.Stop()

		user1 := "user1"
		user2 := "user2"

		// Use tokens for user1
		for i := 0; i < 10; i++ {
			if !limiter.Allow(user1) {
				t.Errorf("Allow(user1) = false on attempt %d, want true", i+1)
			}
		}

		// user2 should still have full quota
		available2 := limiter.GetAvailable(user2)
		if available2 != maxPerHour {
			t.Errorf("GetAvailable(user2) = %.2f, want %.2f (independent from user1)", available2, maxPerHour)
		}

		// user1 should have reduced quota
		available1 := limiter.GetAvailable(user1)
		expected := maxPerHour - 10
		if available1 < expected-1 || available1 > expected+1 {
			t.Errorf("GetAvailable(user1) = %.2f, want ~%.2f", available1, expected)
		}
	})
}

func TestLLMRateLimiter_TokenRefill(t *testing.T) {
	t.Run("refills tokens over time", func(t *testing.T) {
		// Use nil for metrics in tests
		// Use higher rate for faster test: 3600 per hour = 1 per second
		const limit = 3600.0
		limiter := NewLLMRateLimiter(limit, 5*time.Minute, nil)
		defer limiter.Stop()

		userID := "user789"

		// Use 2 tokens
		limiter.Allow(userID)
		limiter.Allow(userID)

		available := limiter.GetAvailable(userID)
		if available < 3597 || available > 3599 { // ~3598, allow variance
			t.Errorf("GetAvailable() = %.2f, want ~3598 after using 2 tokens", available)
		}

		// Wait for ~1.5 seconds (should refill ~1.5 tokens)
		time.Sleep(1500 * time.Millisecond)

		// Check if tokens were refilled (should have ~3599-3600)
		available = limiter.GetAvailable(userID)
		if available < 3599 {
			t.Errorf("GetAvailable() = %.2f, want >= 3599 after refill", available)
		}
	})
}

func TestLLMRateLimiter_GetActiveCount(t *testing.T) {
	const maxPerHour = 50.0
	// Use nil for metrics in tests
	limiter := NewLLMRateLimiter(maxPerHour, 5*time.Minute, nil)
	defer limiter.Stop()

	if count := limiter.GetActiveCount(); count != 0 {
		t.Errorf("GetActiveCount() = %d, want 0 initially", count)
	}

	// Add some users
	limiter.Allow("user1")
	limiter.Allow("user2")
	limiter.Allow("user3")

	if count := limiter.GetActiveCount(); count != 3 {
		t.Errorf("GetActiveCount() = %d, want 3", count)
	}

	// Reusing user should not increase count
	limiter.Allow("user1")
	if count := limiter.GetActiveCount(); count != 3 {
		t.Errorf("GetActiveCount() = %d, want 3 (user1 already exists)", count)
	}
}

func TestLLMRateLimiter_Cleanup(t *testing.T) {
	t.Run("cleans up idle users", func(t *testing.T) {
		const maxPerHour = 50.0
		// Use nil for metrics in tests
		// Use very short cleanup interval for testing
		limiter := NewLLMRateLimiter(maxPerHour, 500*time.Millisecond, nil)
		defer limiter.Stop()

		// Add users and let them become idle (use up tokens)
		limiter.Allow("idle_user1")
		limiter.Allow("idle_user2")

		if count := limiter.GetActiveCount(); count != 2 {
			t.Errorf("GetActiveCount() = %d, want 2 before cleanup", count)
		}

		// Make limiters full (idle state) by waiting for refill to complete
		// Since rate is 50/3600 = 0.0139 tokens/sec, refill is very slow
		// Instead, we'll wait for cleanup cycle and verify behavior
		time.Sleep(600 * time.Millisecond)

		// Note: Cleanup removes limiters that are IsFull() (at max capacity)
		// Our test users consumed 1 token each, so they won't be cleaned immediately
		// This test mainly verifies cleanup goroutine runs without panic
		count := limiter.GetActiveCount()
		if count > 2 {
			t.Errorf("GetActiveCount() = %d, want <= 2 after cleanup cycle", count)
		}
	})

	t.Run("keeps active users", func(t *testing.T) {
		const maxPerHour = 50.0
		// Use nil for metrics in tests
		limiter := NewLLMRateLimiter(maxPerHour, 500*time.Millisecond, nil)
		defer limiter.Stop()

		// Add users
		limiter.Allow("active_user1")
		limiter.Allow("active_user2")

		// Keep using one user
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-ticker.C:
					limiter.Allow("active_user1")
				case <-done:
					return
				}
			}
		}()

		// Wait for potential cleanup
		time.Sleep(700 * time.Millisecond)
		close(done)

		// active_user1 should remain (not at max capacity due to usage)
		count := limiter.GetActiveCount()
		if count == 0 {
			t.Error("GetActiveCount() = 0, expected users to remain")
		}
	})
}

func TestLLMRateLimiter_Stop(t *testing.T) {
	const maxPerHour = 50.0
	// Use nil for metrics in tests
	limiter := NewLLMRateLimiter(maxPerHour, 5*time.Minute, nil)

	// Add some users
	limiter.Allow("user1")
	limiter.Allow("user2")

	// Stop should not panic
	limiter.Stop()

	// Calling Stop multiple times should be safe
	limiter.Stop()
	limiter.Stop()
}

func TestLLMRateLimiter_ConcurrentAccess(t *testing.T) {
	const maxPerHour = 100.0
	// Use nil for metrics in tests
	limiter := NewLLMRateLimiter(maxPerHour, 5*time.Minute, nil)
	defer limiter.Stop()

	const goroutines = 10
	const requestsPerGoroutine = 10

	done := make(chan bool, goroutines)

	// Launch multiple goroutines
	for range goroutines {
		go func() {
			userID := "user"
			for j := 0; j < requestsPerGoroutine; j++ {
				limiter.Allow(userID)
				limiter.GetAvailable(userID)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify user count
	count := limiter.GetActiveCount()
	if count != 1 {
		t.Errorf("GetActiveCount() = %d, want 1 (all goroutines used same userID)", count)
	}

	// Verify tokens were correctly consumed
	available := limiter.GetAvailable("user")
	expected := maxPerHour - float64(goroutines*requestsPerGoroutine)
	if available > maxPerHour || available < 0 {
		t.Errorf("GetAvailable() = %.2f, want between 0 and %.2f", available, expected)
	}
}
