package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestPerKeyLimiter_Allow(t *testing.T) {
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     3,
		RefillRate:    1.0,
		CleanupPeriod: 1 * time.Minute,
	})
	defer limiter.Stop()

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !limiter.Allow("user1") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	if limiter.Allow("user1") {
		t.Error("4th request should be denied")
	}

	// Different user should still be allowed
	if !limiter.Allow("user2") {
		t.Error("Different user should be allowed")
	}
}

func TestPerKeyLimiter_EmptyKey(t *testing.T) {
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     1,
		RefillRate:    0.1,
		CleanupPeriod: 1 * time.Minute,
	})
	defer limiter.Stop()

	// Empty key should always be allowed
	for i := 0; i < 10; i++ {
		if !limiter.Allow("") {
			t.Error("Empty key should always be allowed")
		}
	}
}

func TestPerKeyLimiter_OnDrop(t *testing.T) {
	dropCount := 0
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     1,
		RefillRate:    0.001,
		CleanupPeriod: 1 * time.Minute,
	})
	limiter.OnDrop(func() {
		dropCount++
	})
	defer limiter.Stop()

	// First request allowed
	limiter.Allow("user1")

	// Second request dropped
	limiter.Allow("user1")

	if dropCount != 1 {
		t.Errorf("Expected 1 drop, got %d", dropCount)
	}
}

func TestPerKeyLimiter_GetAvailable(t *testing.T) {
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     10,
		RefillRate:    1.0,
		CleanupPeriod: 1 * time.Minute,
	})
	defer limiter.Stop()

	// New user should have max tokens
	if got := limiter.GetAvailable("newuser"); got != 10 {
		t.Errorf("Expected 10 tokens for new user, got %f", got)
	}

	// After using some tokens
	limiter.Allow("newuser")
	limiter.Allow("newuser")

	if got := limiter.GetAvailable("newuser"); got >= 10 {
		t.Errorf("Expected less than 10 tokens after use, got %f", got)
	}
}

func TestPerKeyLimiter_GetActiveCount(t *testing.T) {
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     10,
		RefillRate:    1.0,
		CleanupPeriod: 1 * time.Minute,
	})
	defer limiter.Stop()

	if limiter.GetActiveCount() != 0 {
		t.Error("Expected 0 active limiters initially")
	}

	limiter.Allow("user1")
	limiter.Allow("user2")
	limiter.Allow("user3")

	if limiter.GetActiveCount() != 3 {
		t.Errorf("Expected 3 active limiters, got %d", limiter.GetActiveCount())
	}
}

func TestPerKeyLimiter_Cleanup(t *testing.T) {
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     10,
		RefillRate:    1000, // Fast refill for testing
		CleanupPeriod: 100 * time.Millisecond,
	})
	defer limiter.Stop()

	// Create some limiters
	limiter.Allow("user1")
	limiter.Allow("user2")

	// Wait for cleanup
	time.Sleep(300 * time.Millisecond)

	// Limiters should be cleaned up (they're at full capacity due to fast refill)
	if limiter.GetActiveCount() != 0 {
		t.Errorf("Expected 0 active limiters after cleanup, got %d", limiter.GetActiveCount())
	}
}

func TestPerKeyLimiter_Stop(t *testing.T) {
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     10,
		RefillRate:    1.0,
		CleanupPeriod: 1 * time.Minute,
	})

	// Should not panic
	limiter.Stop()
	limiter.Stop() // Safe to call multiple times
}

func TestPerKeyLimiter_Concurrent(t *testing.T) {
	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     100,
		RefillRate:    1.0,
		CleanupPeriod: 1 * time.Minute,
	})
	defer limiter.Stop()

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			for j := 0; j < 10; j++ {
				limiter.Allow("user1")
			}
		})
	}
	wg.Wait()
}
