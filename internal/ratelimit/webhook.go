package ratelimit

import (
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// UserRateLimiter tracks rate limits per user using PerKeyLimiter.
type UserRateLimiter struct {
	pkl     *PerKeyLimiter
	metrics *metrics.Metrics
}

// NewUserRateLimiter creates a new per-user rate limiter.
//
// Parameters:
//   - maxTokens: maximum tokens per user (burst capacity, e.g., 6)
//   - refillRate: tokens refilled per second (e.g., 0.2 for 1 token per 5 seconds)
//   - cleanup: cleanup interval for removing inactive limiters
//   - m: optional metrics reporter
//
// Remember to call Stop() when done to prevent goroutine leaks.
func NewUserRateLimiter(maxTokens, refillRate float64, cleanup time.Duration, m *metrics.Metrics) *UserRateLimiter {
	url := &UserRateLimiter{
		metrics: m,
	}

	url.pkl = NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     maxTokens,
		RefillRate:    refillRate,
		CleanupPeriod: cleanup,
	})

	if m != nil {
		url.pkl.OnDrop(func() {
			m.RecordRateLimiterDrop("user")
		})
		url.pkl.OnUpdate(func(count int) {
			m.SetRateLimiterUsers(count)
		})
	}

	return url
}

// Allow checks if a request from a specific user is allowed.
// Returns true if allowed (token consumed), false if rate limit exceeded.
func (url *UserRateLimiter) Allow(userID string) bool {
	return url.pkl.Allow(userID)
}

// GetActiveCount returns the current number of active user limiters.
func (url *UserRateLimiter) GetActiveCount() int {
	return url.pkl.GetActiveCount()
}

// Stop gracefully stops the cleanup goroutine.
// Safe to call multiple times.
func (url *UserRateLimiter) Stop() {
	url.pkl.Stop()
}
