package webhook

import (
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// RateLimiter implements a token bucket rate limiter for LINE API calls
// LINE API has rate limits: https://developers.line.biz/en/reference/messaging-api/#rate-limits
type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
// maxTokens: maximum number of tokens in the bucket
// refillRate: number of tokens to add per second
func NewRateLimiter(maxTokens, refillRate float64) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed based on rate limit
// Returns true if the request is allowed, false otherwise
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()

	// Refill tokens
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	// Check if we have tokens available
	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}

	return false
}

// WaitForToken waits until a token is available
// Returns immediately if a token is available, otherwise blocks
func (rl *RateLimiter) WaitForToken() {
	for !rl.Allow() {
		time.Sleep(100 * time.Millisecond)
	}
}

// GetAvailableTokens returns the current number of available tokens
func (rl *RateLimiter) GetAvailableTokens() float64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()

	tokens := rl.tokens + (elapsed * rl.refillRate)
	if tokens > rl.maxTokens {
		tokens = rl.maxTokens
	}

	return tokens
}

// UserRateLimiter tracks rate limits per user
type UserRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*RateLimiter
	cleanup  time.Duration
	metrics  *metrics.Metrics // Optional metrics recorder for tracking dropped requests
}

// NewUserRateLimiter creates a new per-user rate limiter
// metrics parameter is optional and can be nil
func NewUserRateLimiter(cleanup time.Duration, m *metrics.Metrics) *UserRateLimiter {
	url := &UserRateLimiter{
		limiters: make(map[string]*RateLimiter),
		cleanup:  cleanup,
		metrics:  m,
	}

	// Start cleanup goroutine
	go url.cleanupLoop()

	return url
}

// Allow checks if a request from a specific user is allowed
// userID: the LINE user ID
// maxTokens: maximum tokens per user (e.g., 10)
// refillRate: refill rate per second (e.g., 1.0/3.0 = 1 request per 3 seconds)
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
func (url *UserRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(url.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		url.mu.Lock()
		// Remove limiters that are at maximum capacity (inactive users)
		for userID, limiter := range url.limiters {
			if limiter.GetAvailableTokens() >= limiter.maxTokens {
				delete(url.limiters, userID)
			}
		}
		url.mu.Unlock()
	}
}
