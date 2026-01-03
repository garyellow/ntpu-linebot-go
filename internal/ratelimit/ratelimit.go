// Package ratelimit provides a generic token bucket rate limiter.
// It can be used for API rate limiting, request throttling, etc.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter implements a token bucket rate limiter.
// It is safe for concurrent use.
//
// The token bucket algorithm:
//   - Tokens are added to the bucket at a constant rate (refillRate per second)
//   - The bucket has a maximum capacity (maxTokens)
//   - Each request consumes one token
//   - If no tokens are available, the request is either rejected or waits
type Limiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// New creates a new rate limiter.
//
// Parameters:
//   - maxTokens: maximum number of tokens in the bucket (burst capacity)
//   - refillRate: number of tokens to add per second
//
// Example:
//
//	// Allow 100 requests per second with burst of 100
//	limiter := ratelimit.New(100, 100)
//
//	// Allow 1000 requests per minute (≈16.67/sec) with burst of 33
//	limiter := ratelimit.NewPerMinute(1000)
func New(maxTokens, refillRate float64) *Limiter {
	return &Limiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// NewPerMinute creates a rate limiter based on requests per minute.
// It automatically converts to per-second rate and sets a reasonable burst size.
//
// Parameters:
//   - requestsPerMinute: maximum requests allowed per minute
//
// The burst size is set to 2 seconds worth of tokens to allow small bursts.
func NewPerMinute(requestsPerMinute float64) *Limiter {
	perSecond := requestsPerMinute / 60
	return &Limiter{
		tokens:     perSecond,     // Start with 1 second of tokens
		maxTokens:  perSecond * 2, // Allow 2 seconds burst
		refillRate: perSecond,     // Refill at per-second rate
		lastRefill: time.Now(),
	}
}

// refill adds tokens based on elapsed time since last refill.
// Must be called with mu held.
func (l *Limiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()

	l.tokens += elapsed * l.refillRate
	if l.tokens > l.maxTokens {
		l.tokens = l.maxTokens
	}
	l.lastRefill = now
}

// Allow checks if a request is allowed based on rate limit.
// Returns true if allowed (token consumed), false otherwise.
// This method is non-blocking. Note: This consumes a token.
// Use Check() + Consume() for atomic multi-layer checks.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= 1.0 {
		l.tokens -= 1.0
		return true
	}

	return false
}

// Check returns true if a request would be allowed (without consuming).
// Use this with Consume() for atomic multi-layer rate limiting.
//
// WARNING: This method is NOT thread-safe for atomic check-then-consume operations
// by itself. The caller MUST hold an external lock that covers both Check()
// and Consume() to prevent race conditions (TOCTOU).
func (l *Limiter) Check() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	return l.tokens >= 1.0
}

// Consume decrements a token (assumes Check() already passed).
// Call this after all rate limit checks pass.
//
// WARNING: This method is NOT thread-safe for atomic check-then-consume operations
// by itself. The caller MUST hold an external lock that covers both Check()
// and Consume() to prevent race conditions (TOCTOU).
func (l *Limiter) Consume() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	if l.tokens >= 1.0 {
		l.tokens -= 1.0
	}
}

// Wait blocks until a token is available or the context is canceled.
// Returns nil if a token was acquired, or ctx.Err() if canceled.
//
// This method is more efficient than polling Allow() as it calculates
// the exact wait time needed.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		l.refill()

		// Check if we have a token
		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}

		// Calculate wait time for next token
		waitTime := time.Duration((1 - l.tokens) / l.refillRate * float64(time.Second))
		l.mu.Unlock()

		// Wait outside the lock
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Retry the loop to acquire token
		}
	}
}

// WaitSimple blocks until a token is available.
// Unlike Wait, this method does not support context cancellation.
// Use Wait when you need timeout/cancellation support.
func (l *Limiter) WaitSimple() {
	for !l.Allow() {
		time.Sleep(100 * time.Millisecond)
	}
}

// Available returns the current number of available tokens.
// This is useful for monitoring and debugging.
func (l *Limiter) Available() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	return l.tokens
}

// IsFull returns true if the bucket is at or near full capacity.
// This is used to detect inactive limiters that can be cleaned up.
func (l *Limiter) IsFull() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	return l.tokens >= l.maxTokens
}

// Reset resets the limiter to full capacity.
// Useful for testing or when rate limit conditions change.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.tokens = l.maxTokens
	l.lastRefill = time.Now()
}
