// Package ratelimit provides rate limiting mechanisms using token bucket algorithm.
package ratelimit

import (
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// LLMRateLimiter tracks per-user LLM API usage with hourly limits.
// This is separate from the general user rate limiter to control expensive LLM operations
// (NLU intent parsing and query expansion) independently from regular message processing.
//
// Design matches UserRateLimiter for consistency, but applies to LLM-specific operations only.
type LLMRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*Limiter // userID -> Limiter
	cleanup  time.Duration       // cleanup interval
	metrics  *metrics.Metrics    // metrics reporter (optional)
	stopCh   chan struct{}       // cleanup goroutine stop signal
}

// NewLLMRateLimiter creates a new LLM rate limiter with per-hour limits.
//
// Parameters:
//   - maxPerHour: maximum LLM requests per user per hour (e.g., 50)
//   - cleanup: cleanup interval for removing inactive limiters (e.g., 5 minutes)
//   - metrics: optional metrics reporter for tracking active limiters
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
func NewLLMRateLimiter(maxPerHour float64, cleanup time.Duration, metrics *metrics.Metrics) *LLMRateLimiter {
	llm := &LLMRateLimiter{
		limiters: make(map[string]*Limiter),
		cleanup:  cleanup,
		metrics:  metrics,
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine
	go llm.cleanupLoop(maxPerHour)

	return llm
}

// Allow checks if an LLM request from userID is allowed under the rate limit.
// Returns true if allowed (token consumed), false if rate limit exceeded.
//
// This method is thread-safe and non-blocking. It automatically creates a new
// limiter for first-time users.
//
// If the request is denied, it records a rate limit drop metric (if metrics enabled).
func (llm *LLMRateLimiter) Allow(userID string, maxPerHour float64) bool {
	if userID == "" {
		return true // Allow requests without user ID
	}

	llm.mu.RLock()
	limiter, exists := llm.limiters[userID]
	llm.mu.RUnlock()

	if !exists {
		llm.mu.Lock()
		// Double-check after acquiring write lock
		limiter, exists = llm.limiters[userID]
		if !exists {
			// Create new limiter: maxTokens = maxPerHour, refillRate = maxPerHour / 3600
			limiter = New(maxPerHour, maxPerHour/3600.0)
			llm.limiters[userID] = limiter
		}
		llm.mu.Unlock()
	}

	allowed := limiter.Allow()
	if !allowed && llm.metrics != nil {
		llm.metrics.RecordRateLimiterDrop("llm")
	}
	return allowed
}

// GetAvailable returns the number of remaining tokens for a user.
// This can be used to display quota information to users.
//
// Returns maxPerHour if the user has no limiter yet (first-time user).
func (llm *LLMRateLimiter) GetAvailable(userID string, maxPerHour float64) float64 {
	if userID == "" {
		return maxPerHour
	}

	llm.mu.RLock()
	limiter, exists := llm.limiters[userID]
	llm.mu.RUnlock()

	if !exists {
		return maxPerHour // User hasn't used any quota yet
	}

	return limiter.Available()
}

// GetActiveCount returns the current number of active user limiters.
// This is useful for monitoring memory usage and user activity.
func (llm *LLMRateLimiter) GetActiveCount() int {
	llm.mu.RLock()
	defer llm.mu.RUnlock()
	return len(llm.limiters)
}

// cleanupLoop periodically removes inactive rate limiters.
// A limiter is considered inactive if it's at maximum capacity (user hasn't made requests recently).
//
// Stops when Stop() is called (stopCh is closed).
func (llm *LLMRateLimiter) cleanupLoop(maxPerHour float64) {
	ticker := time.NewTicker(llm.cleanup)
	defer ticker.Stop()

	for {
		select {
		case <-llm.stopCh:
			return
		case <-ticker.C:
			llm.mu.Lock()
			cleanedCount := 0
			// Remove limiters that are at maximum capacity (inactive users)
			for userID, limiter := range llm.limiters {
				if limiter.IsFull() {
					delete(llm.limiters, userID)
					cleanedCount++
				}
			}
			activeCount := len(llm.limiters)
			llm.mu.Unlock()

			// Update metrics if available
			if llm.metrics != nil {
				llm.metrics.SetLLMRateLimiterUsers(activeCount)
			}
		}
	}
}

// Stop gracefully stops the cleanup goroutine.
// This should be called during server shutdown to prevent goroutine leaks.
// Safe to call multiple times (subsequent calls are no-ops).
func (llm *LLMRateLimiter) Stop() {
	select {
	case <-llm.stopCh:
		// Already stopped
	default:
		close(llm.stopCh)
	}
}
