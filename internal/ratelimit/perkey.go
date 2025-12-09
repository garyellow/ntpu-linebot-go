// Package ratelimit provides rate limiting mechanisms using token bucket algorithm.
package ratelimit

import (
	"sync"
	"time"
)

// PerKeyLimiterConfig configures a PerKeyLimiter instance.
type PerKeyLimiterConfig struct {
	MaxTokens     float64       // Maximum tokens per key (burst capacity)
	RefillRate    float64       // Tokens refilled per second
	CleanupPeriod time.Duration // How often to clean up inactive limiters
}

// PerKeyLimiter tracks rate limits per key (e.g., user ID, chat ID).
// It creates a separate token bucket for each key and automatically
// cleans up inactive buckets.
type PerKeyLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*Limiter
	config   PerKeyLimiterConfig
	onDrop   func()          // Optional callback when request is dropped
	onUpdate func(count int) // Optional callback when active count changes
	stopCh   chan struct{}
}

// NewPerKeyLimiter creates a new per-key rate limiter.
//
// Example:
//
//	limiter := NewPerKeyLimiter(PerKeyLimiterConfig{
//	    MaxTokens:     6,
//	    RefillRate:    0.2, // 1 token per 5 seconds
//	    CleanupPeriod: 5 * time.Minute,
//	})
//	defer limiter.Stop()
//
//	if limiter.Allow("user123") {
//	    // Process request
//	}
func NewPerKeyLimiter(cfg PerKeyLimiterConfig) *PerKeyLimiter {
	pkl := &PerKeyLimiter{
		limiters: make(map[string]*Limiter),
		config:   cfg,
		stopCh:   make(chan struct{}),
	}

	go pkl.cleanupLoop()

	return pkl
}

// OnDrop sets a callback function that is called when a request is dropped due to rate limiting.
func (pkl *PerKeyLimiter) OnDrop(fn func()) {
	pkl.onDrop = fn
}

// OnUpdate sets a callback function that is called when the active limiter count changes.
func (pkl *PerKeyLimiter) OnUpdate(fn func(count int)) {
	pkl.onUpdate = fn
}

// Allow checks if a request for the given key is allowed.
// Returns true if allowed (token consumed), false if rate limit exceeded.
func (pkl *PerKeyLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	pkl.mu.RLock()
	limiter, exists := pkl.limiters[key]
	pkl.mu.RUnlock()

	if !exists {
		pkl.mu.Lock()
		// Double-check after acquiring write lock
		limiter, exists = pkl.limiters[key]
		if !exists {
			limiter = New(pkl.config.MaxTokens, pkl.config.RefillRate)
			pkl.limiters[key] = limiter
		}
		pkl.mu.Unlock()
	}

	allowed := limiter.Allow()
	if !allowed && pkl.onDrop != nil {
		pkl.onDrop()
	}
	return allowed
}

// GetAvailable returns the number of available tokens for a key.
// Returns MaxTokens if the key has no limiter yet.
func (pkl *PerKeyLimiter) GetAvailable(key string) float64 {
	if key == "" {
		return pkl.config.MaxTokens
	}

	pkl.mu.RLock()
	limiter, exists := pkl.limiters[key]
	pkl.mu.RUnlock()

	if !exists {
		return pkl.config.MaxTokens
	}

	return limiter.Available()
}

// GetActiveCount returns the number of active limiters.
func (pkl *PerKeyLimiter) GetActiveCount() int {
	pkl.mu.RLock()
	defer pkl.mu.RUnlock()
	return len(pkl.limiters)
}

// cleanupLoop periodically removes inactive limiters.
func (pkl *PerKeyLimiter) cleanupLoop() {
	ticker := time.NewTicker(pkl.config.CleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-pkl.stopCh:
			return
		case <-ticker.C:
			pkl.mu.Lock()
			for key, limiter := range pkl.limiters {
				if limiter.IsFull() {
					delete(pkl.limiters, key)
				}
			}
			activeCount := len(pkl.limiters)
			pkl.mu.Unlock()

			if pkl.onUpdate != nil {
				pkl.onUpdate(activeCount)
			}
		}
	}
}

// Stop gracefully stops the cleanup goroutine.
// Safe to call multiple times.
func (pkl *PerKeyLimiter) Stop() {
	select {
	case <-pkl.stopCh:
		// Already stopped
	default:
		close(pkl.stopCh)
	}
}
