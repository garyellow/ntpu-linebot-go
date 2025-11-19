package scraper

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

// RateLimiter implements token bucket with random delay and exponential backoff
type RateLimiter struct {
	tokens         float64
	maxTokens      float64
	refillRate     float64 // tokens per second
	mu             sync.Mutex
	lastRefillTime time.Time
	minDelay       time.Duration
	maxDelay       time.Duration
}

// NewRateLimiter creates a new rate limiter
// workers: number of concurrent workers (determines maxTokens)
// minDelay: minimum random delay between requests
// maxDelay: maximum random delay between requests
func NewRateLimiter(workers int, minDelay, maxDelay time.Duration) *RateLimiter {
	maxTokens := float64(workers)
	refillRate := maxTokens / 10.0 // Refill all tokens in ~10 seconds

	return &RateLimiter{
		tokens:         maxTokens,
		maxTokens:      maxTokens,
		refillRate:     refillRate,
		lastRefillTime: time.Now(),
		minDelay:       minDelay,
		maxDelay:       maxDelay,
	}
}

// Wait blocks until a token is available, then adds random delay
func (rl *RateLimiter) Wait(ctx context.Context) error {
	// Wait for token
	if err := rl.acquire(ctx); err != nil {
		return err
	}

	// Add random delay
	delay := rl.randomDelay()
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// acquire waits for and consumes one token
func (rl *RateLimiter) acquire(ctx context.Context) error {
	for {
		rl.mu.Lock()

		// Refill tokens based on time elapsed
		now := time.Now()
		elapsed := now.Sub(rl.lastRefillTime).Seconds()
		rl.tokens = math.Min(rl.maxTokens, rl.tokens+elapsed*rl.refillRate)
		rl.lastRefillTime = now

		// Try to consume a token
		if rl.tokens >= 1.0 {
			rl.tokens -= 1.0
			rl.mu.Unlock()
			return nil
		}

		// Calculate wait time for next token
		waitTime := time.Duration((1.0-rl.tokens)/rl.refillRate*1000) * time.Millisecond
		rl.mu.Unlock()

		// Wait for token or context cancellation
		select {
		case <-time.After(waitTime):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// randomDelay returns a random delay between minDelay and maxDelay
func (rl *RateLimiter) randomDelay() time.Duration {
	if rl.minDelay >= rl.maxDelay {
		return rl.minDelay
	}

	delta := rl.maxDelay - rl.minDelay
	random := time.Duration(rand.Int63n(int64(delta)))
	return rl.minDelay + random
}

// RetryWithBackoff retries a function with exponential backoff
// maxRetries: maximum number of retry attempts
// initialDelay: initial delay before first retry
// maxDelay: maximum delay between retries
func RetryWithBackoff(ctx context.Context, maxRetries int, initialDelay, maxDelay time.Duration, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Try the function
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		// Don't delay after the last attempt
		if attempt == maxRetries {
			break
		}

		// Calculate exponential backoff delay
		delay := time.Duration(float64(initialDelay) * math.Pow(2, float64(attempt)))
		if delay > maxDelay {
			delay = maxDelay
		}

		// Add jitter (±25%)
		jitter := time.Duration(rand.Int63n(int64(delay) / 2))
		delay = delay - delay/4 + jitter

		// Wait for delay or context cancellation
		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}
