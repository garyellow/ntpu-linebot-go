package ratelimit

import (
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// WebhookRateLimiter wraps the shared Limiter for LINE API calls.
// LINE API has rate limits: https://developers.line.biz/en/reference/messaging-api/#rate-limits
type WebhookRateLimiter struct {
	*Limiter
}

// NewWebhookRateLimiter creates a new rate limiter
// maxTokens: maximum number of tokens in the bucket
// refillRate: number of tokens to add per second
func NewWebhookRateLimiter(maxTokens, refillRate float64) *WebhookRateLimiter {
	return &WebhookRateLimiter{
		Limiter: New(maxTokens, refillRate),
	}
}

// WaitForToken waits until a token is available
// Returns immediately if a token is available, otherwise blocks
func (rl *WebhookRateLimiter) WaitForToken() {
	rl.WaitSimple()
}

// GetAvailableTokens returns the current number of available tokens
func (rl *WebhookRateLimiter) GetAvailableTokens() float64 {
	return rl.Available()
}

// UserRateLimiter tracks rate limits per user using PerKeyLimiter.
type UserRateLimiter struct {
	pkl        *PerKeyLimiter
	maxTokens  float64
	refillRate float64
	metrics    *metrics.Metrics
}

// NewUserRateLimiter creates a new per-user rate limiter.
// Remember to call Stop() when done to prevent goroutine leaks.
func NewUserRateLimiter(cleanup time.Duration, m *metrics.Metrics) *UserRateLimiter {
	url := &UserRateLimiter{
		metrics: m,
	}

	// We defer setting maxTokens/refillRate until Allow() is called
	// since they're passed dynamically per-call
	url.pkl = nil // Will be initialized on first Allow()

	return url
}

// initPKL initializes the PerKeyLimiter with the given parameters.
// This is called on first Allow() since maxTokens/refillRate are passed dynamically.
func (url *UserRateLimiter) initPKL(maxTokens, refillRate float64, cleanup time.Duration) {
	url.maxTokens = maxTokens
	url.refillRate = refillRate

	url.pkl = NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     maxTokens,
		RefillRate:    refillRate,
		CleanupPeriod: cleanup,
	})

	if url.metrics != nil {
		url.pkl.OnDrop(func() {
			url.metrics.RecordRateLimiterDrop("user")
		})
		url.pkl.OnUpdate(func(count int) {
			url.metrics.SetRateLimiterUsers(count)
		})
	}
}

// Allow checks if a request from a specific user is allowed.
// userID: the LINE user ID
// maxTokens: maximum tokens per user (e.g., 6)
// refillRate: refill rate per second (e.g., 1.0/5.0 = 1 request per 5 seconds)
func (url *UserRateLimiter) Allow(userID string, maxTokens, refillRate float64) bool {
	// Initialize on first call with the provided parameters
	if url.pkl == nil {
		url.initPKL(maxTokens, refillRate, 5*time.Minute)
	}

	return url.pkl.Allow(userID)
}

// GetActiveCount returns the current number of active user limiters.
func (url *UserRateLimiter) GetActiveCount() int {
	if url.pkl == nil {
		return 0
	}
	return url.pkl.GetActiveCount()
}

// Stop gracefully stops the cleanup goroutine.
// Safe to call multiple times.
func (url *UserRateLimiter) Stop() {
	if url.pkl != nil {
		url.pkl.Stop()
	}
}
