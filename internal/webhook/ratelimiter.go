package webhook

import (
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
)

// RateLimiter wraps the shared ratelimit.Limiter for LINE API calls.
// LINE API has rate limits: https://developers.line.biz/en/reference/messaging-api/#rate-limits
type RateLimiter struct {
	*ratelimit.Limiter
}

// NewRateLimiter creates a new rate limiter
// maxTokens: maximum number of tokens in the bucket
// refillRate: number of tokens to add per second
func NewRateLimiter(maxTokens, refillRate float64) *RateLimiter {
	return &RateLimiter{
		Limiter: ratelimit.New(maxTokens, refillRate),
	}
}

// WaitForToken waits until a token is available
// Returns immediately if a token is available, otherwise blocks
func (rl *RateLimiter) WaitForToken() {
	rl.WaitSimple()
}

// GetAvailableTokens returns the current number of available tokens
func (rl *RateLimiter) GetAvailableTokens() float64 {
	return rl.Available()
}

// UserRateLimiter tracks rate limits per user
type UserRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiter
	cleanup  time.Duration
	metrics  *metrics.Metrics // Optional metrics recorder for tracking dropped requests
	stopCh   chan struct{}    // Channel to signal cleanup goroutine to stop
}

// NewUserRateLimiter creates a new per-user rate limiter
// metrics parameter is optional and can be nil
// Remember to call Stop() when done to prevent goroutine leaks
func NewUserRateLimiter(cleanup time.Duration, m *metrics.Metrics) *UserRateLimiter {
	url := &UserRateLimiter{
		limiters: make(map[string]*RateLimiter),
		cleanup:  cleanup,
		metrics:  m,
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine
	go url.cleanupLoop()

	return url
}

// Allow checks if a request from a specific user is allowed
// userID: the LINE user ID
// maxTokens: maximum tokens per user (e.g., 6)
// refillRate: refill rate per second (e.g., 1.0/5.0 = 1 request per 5 seconds)
func (url *UserRateLimiter) Allow(userID string, maxTokens, refillRate float64) bool {
	url.mu.RLock()
	limiter, exists := url.limiters[userID]
	url.mu.RUnlock()

	if !exists {
		url.mu.Lock()
		limiter = NewRateLimiter(maxTokens, refillRate)
		url.limiters[userID] = limiter
		url.mu.Unlock()
	}

	allowed := limiter.Allow()
	if !allowed && url.metrics != nil {
		url.metrics.RecordRateLimiterDrop("user")
	}
	return allowed
}

// cleanupLoop periodically removes inactive rate limiters
// Stops when Stop() is called (stopCh is closed)
func (url *UserRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(url.cleanup)
	defer ticker.Stop()

	for {
		select {
		case <-url.stopCh:
			return
		case <-ticker.C:
			url.mu.Lock()
			cleanedCount := 0
			// Remove limiters that are at maximum capacity (inactive users)
			for userID, limiter := range url.limiters {
				if limiter.IsFull() {
					delete(url.limiters, userID)
					cleanedCount++
				}
			}
			activeCount := len(url.limiters)
			url.mu.Unlock()

			// Update metrics if available
			if url.metrics != nil {
				url.metrics.SetRateLimiterActiveUsers(activeCount)
				if cleanedCount > 0 {
					url.metrics.RecordRateLimiterCleanup(cleanedCount)
				}
			}
		}
	}
}

// GetActiveCount returns the current number of active user limiters
func (url *UserRateLimiter) GetActiveCount() int {
	url.mu.RLock()
	defer url.mu.RUnlock()
	return len(url.limiters)
}

// Stop gracefully stops the cleanup goroutine.
// This should be called during server shutdown to prevent goroutine leaks.
// Safe to call multiple times (subsequent calls are no-ops).
func (url *UserRateLimiter) Stop() {
	select {
	case <-url.stopCh:
		// Already closed, do nothing
	default:
		close(url.stopCh)
	}
}
