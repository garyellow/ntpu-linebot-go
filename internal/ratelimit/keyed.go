// Package ratelimit provides rate limiting mechanisms using token bucket algorithm.
package ratelimit

import (
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// MetricType defines the type of rate limiter for metrics reporting.
type MetricType int

const (
	// MetricTypeUser reports to ntpu_rate_limiter_users
	MetricTypeUser MetricType = iota
	// MetricTypeLLM reports to ntpu_llm_rate_limiter_users
	MetricTypeLLM
)

// KeyedConfig configures a KeyedLimiter instance.
type KeyedConfig struct {
	// Name identifies this limiter for metrics (e.g., "user", "llm")
	Name string

	// MetricType specifies which metric gauge to update
	MetricType MetricType

	// Token bucket settings
	Burst      float64 // Maximum tokens (burst capacity)
	RefillRate float64 // Tokens refilled per second

	// Optional sliding window daily limit (0 = disabled)
	// Uses SlidingWindowCounter for true rolling 24h window
	DailyLimit int

	// Cleanup settings
	CleanupPeriod time.Duration // How often to clean up inactive limiters

	// Optional metrics reporter
	Metrics *metrics.Metrics
}

// KeyedLimiter tracks rate limits per key (e.g., user ID, chat ID).
// It creates a separate rate limiter for each key and automatically
// cleans up inactive limiters.
//
// Supports optional daily limits via SlidingWindowCounter.
type KeyedLimiter struct {
	mu       sync.RWMutex
	entries  map[string]*keyedEntry
	config   KeyedConfig
	onDrop   func()          // Optional callback when request is dropped
	onUpdate func(count int) // Optional callback when active count changes
	stopCh   chan struct{}
}

// keyedEntry holds per-key state: token bucket + optional sliding window counter
// The mutex ensures atomic multi-layer checks (prevents TOCTOU race)
type keyedEntry struct {
	mu      sync.Mutex // Protects atomic multi-layer operations
	limiter *Limiter
	daily   *SlidingWindowCounter // Sliding window for true rolling 24h limit
}

// NewKeyedLimiter creates a new per-key rate limiter.
//
// Example:
//
//	limiter := NewKeyedLimiter(KeyedConfig{
//	    Name:       "user",
//	    Burst:      15,
//	    RefillRate: 0.1, // 1 token per 10 seconds
//	    CleanupPeriod: 5 * time.Minute,
//	})
//	defer limiter.Stop()
//
//	if limiter.Allow("user123") {
//	    // Process request
//	}
func NewKeyedLimiter(cfg KeyedConfig) *KeyedLimiter {
	kl := &KeyedLimiter{
		entries: make(map[string]*keyedEntry),
		config:  cfg,
		stopCh:  make(chan struct{}),
	}

	if kl.config.CleanupPeriod <= 0 {
		kl.config.CleanupPeriod = time.Minute
	}

	// Setup metrics callbacks
	if cfg.Metrics != nil {
		kl.onDrop = func() {
			cfg.Metrics.RecordRateLimiterDrop(cfg.Name)
		}
		kl.onUpdate = func(count int) {
			if cfg.MetricType == MetricTypeLLM {
				cfg.Metrics.SetLLMRateLimiterUsers(count)
			} else {
				cfg.Metrics.SetRateLimiterUsers(count)
			}
		}
	}

	go kl.cleanupLoop()

	return kl
}

// Allow checks if a request for the given key is allowed.
// Returns true if allowed (tokens consumed), false if rate limit exceeded.
//
// When DailyLimit is configured, both hourly and daily limits must pass.
// Uses per-entry mutex for atomic multi-layer check-then-consume (prevents TOCTOU race).
func (kl *KeyedLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	entry := kl.getOrCreateEntry(key)

	// Lock entry for atomic multi-layer operation
	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Phase 1: Check both limits without consuming
	if entry.daily != nil && !entry.daily.Check() {
		if kl.onDrop != nil {
			kl.onDrop()
		}
		return false
	}

	if !entry.limiter.Check() {
		if kl.onDrop != nil {
			kl.onDrop()
		}
		return false
	}

	// Phase 2: Both passed - now consume tokens
	if entry.daily != nil {
		entry.daily.Consume()
	}
	entry.limiter.Consume()

	return true
}

// getOrCreateEntry returns the entry for a key, creating it if needed.
func (kl *KeyedLimiter) getOrCreateEntry(key string) *keyedEntry {
	kl.mu.RLock()
	entry, exists := kl.entries[key]
	kl.mu.RUnlock()

	if exists {
		return entry
	}

	kl.mu.Lock()
	defer kl.mu.Unlock()

	// Double-check after acquiring write lock
	entry, exists = kl.entries[key]
	if exists {
		return entry
	}

	entry = &keyedEntry{
		limiter: New(kl.config.Burst, kl.config.RefillRate),
		daily:   NewSlidingWindowCounter(kl.config.DailyLimit, 24*time.Hour),
	}
	kl.entries[key] = entry
	return entry
}

// GetAvailable returns the number of available tokens for a key.
// Returns Burst if the key has no limiter yet.
func (kl *KeyedLimiter) GetAvailable(key string) float64 {
	if key == "" {
		return kl.config.Burst
	}

	kl.mu.RLock()
	entry, exists := kl.entries[key]
	kl.mu.RUnlock()

	if !exists {
		return kl.config.Burst
	}

	return entry.limiter.Available()
}

// GetDailyRemaining returns the remaining daily quota for a key.
// Returns -1 if daily limit is disabled, or max if key not found.
func (kl *KeyedLimiter) GetDailyRemaining(key string) int {
	if kl.config.DailyLimit <= 0 {
		return -1 // Disabled
	}

	kl.mu.RLock()
	entry, exists := kl.entries[key]
	kl.mu.RUnlock()

	if !exists {
		return kl.config.DailyLimit
	}

	return entry.daily.GetRemaining()
}

// GetActiveCount returns the number of active limiters.
func (kl *KeyedLimiter) GetActiveCount() int {
	kl.mu.RLock()
	defer kl.mu.RUnlock()
	return len(kl.entries)
}

// cleanupLoop periodically removes inactive limiters.
func (kl *KeyedLimiter) cleanupLoop() {
	ticker := time.NewTicker(kl.config.CleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-kl.stopCh:
			return
		case <-ticker.C:
			kl.mu.Lock()
			for key, entry := range kl.entries {
				// Remove if token bucket is full (inactive)
				// modifying logic to also check daily limit if enabled
				shouldDelete := entry.limiter.IsFull()
				if shouldDelete && entry.daily != nil {
					// If daily limit exists, also check if it has reset (no usage in window)
					// GetRemaining returning maxRequests means 0 usage in weighted window
					shouldDelete = entry.daily.GetRemaining() == kl.config.DailyLimit
				}

				if shouldDelete {
					delete(kl.entries, key)
				}
			}
			activeCount := len(kl.entries)
			kl.mu.Unlock()

			if kl.onUpdate != nil {
				kl.onUpdate(activeCount)
			}
		}
	}
}

// Stop gracefully stops the cleanup goroutine.
// Safe to call multiple times.
func (kl *KeyedLimiter) Stop() {
	select {
	case <-kl.stopCh:
		// Already stopped
	default:
		close(kl.stopCh)
	}
}
