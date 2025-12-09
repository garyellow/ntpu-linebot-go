// Package ratelimit provides rate limiting mechanisms using token bucket algorithm.
package ratelimit

import (
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// LLMRateLimiter tracks per-user LLM API usage with hourly limits.
// This is separate from the general user rate limiter to control expensive LLM operations
// (NLU intent parsing and query expansion) independently from regular message processing.
type LLMRateLimiter struct {
	pkl        *PerKeyLimiter
	maxPerHour float64
	metrics    *metrics.Metrics
}

// NewLLMRateLimiter creates a new LLM rate limiter with per-hour limits.
//
// Parameters:
//   - maxPerHour: maximum LLM requests per user per hour (e.g., 50)
//   - cleanup: cleanup interval for removing inactive limiters (e.g., 5 minutes)
//   - m: optional metrics reporter for tracking active limiters
//
// The limiter uses a token bucket with:
//   - maxTokens = maxPerHour (burst capacity)
//   - refillRate = maxPerHour / 3600 (tokens per second)
//
// Example:
//
//	limiter := NewLLMRateLimiter(50, 5*time.Minute, metrics)
//	defer limiter.Stop()
//
//	if limiter.Allow("user123") {
//	    // Make LLM API call
//	}
func NewLLMRateLimiter(maxPerHour float64, cleanup time.Duration, m *metrics.Metrics) *LLMRateLimiter {
	llm := &LLMRateLimiter{
		maxPerHour: maxPerHour,
		metrics:    m,
	}

	llm.pkl = NewPerKeyLimiter(PerKeyLimiterConfig{
		MaxTokens:     maxPerHour,
		RefillRate:    maxPerHour / 3600.0,
		CleanupPeriod: cleanup,
	})

	if m != nil {
		llm.pkl.OnDrop(func() {
			m.RecordRateLimiterDrop("llm")
		})
		llm.pkl.OnUpdate(func(count int) {
			m.SetLLMRateLimiterUsers(count)
		})
	}

	return llm
}

// Allow checks if an LLM request from userID is allowed under the rate limit.
// Returns true if allowed (token consumed), false if rate limit exceeded.
func (llm *LLMRateLimiter) Allow(userID string) bool {
	return llm.pkl.Allow(userID)
}

// GetAvailable returns the number of remaining tokens for a user.
// Returns maxPerHour if the user has no limiter yet (first-time user).
func (llm *LLMRateLimiter) GetAvailable(userID string) float64 {
	if userID == "" {
		return llm.maxPerHour
	}
	return llm.pkl.GetAvailable(userID)
}

// GetActiveCount returns the current number of active user limiters.
func (llm *LLMRateLimiter) GetActiveCount() int {
	return llm.pkl.GetActiveCount()
}

// Stop gracefully stops the cleanup goroutine.
// Safe to call multiple times.
func (llm *LLMRateLimiter) Stop() {
	llm.pkl.Stop()
}
