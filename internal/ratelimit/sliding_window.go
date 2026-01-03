// Package ratelimit provides rate limiting mechanisms.
package ratelimit

import (
	"sync"
	"time"
)

// SlidingWindowCounter implements a memory-efficient sliding window rate limiter.
// It uses the sliding window counter algorithm with two fixed windows and weighted averaging.
//
// Algorithm:
//   - Maintains counts for current and previous time windows
//   - Calculates effective count using weighted average based on window overlap
//   - effectiveCount = currCount + prevCount × (remaining time in current window / window duration)
//
// This provides smooth rate limiting across window boundaries with O(1) space complexity.
//
// Memory: ~32 bytes per counter (2 ints + 1 timestamp + 1 mutex pointer)
//
// Example for 24h window with 100 limit:
//   - User made 80 requests yesterday (ending at 10:00 AM)
//   - Now it's 10:30 AM today (30 min into new window)
//   - overlapRatio = 23.5h / 24h = 0.979
//   - effectiveCount = currCount + 80 × 0.979 ≈ currCount + 78
//   - User can make ~22 more requests before hitting 100
type SlidingWindowCounter struct {
	mu              sync.Mutex
	currCount       int
	prevCount       int
	currWindowStart time.Time
	windowDuration  time.Duration
	maxRequests     int
}

// NewSlidingWindowCounter creates a new sliding window counter.
//
// Parameters:
//   - maxRequests: maximum requests allowed in the window (e.g., 100)
//   - windowDuration: size of the time window (e.g., 24*time.Hour)
//
// Returns nil if maxRequests <= 0 (disabled).
func NewSlidingWindowCounter(maxRequests int, windowDuration time.Duration) *SlidingWindowCounter {
	if maxRequests <= 0 {
		return nil
	}
	return &SlidingWindowCounter{
		currWindowStart: time.Now(),
		windowDuration:  windowDuration,
		maxRequests:     maxRequests,
	}
}

// Allow checks if a request is allowed and consumes a token if so.
// Returns true if allowed, false if rate limit exceeded.
//
// This method is thread-safe.
func (swc *SlidingWindowCounter) Allow() bool {
	if swc == nil {
		return true // Disabled
	}

	swc.mu.Lock()
	defer swc.mu.Unlock()

	swc.maybeRotateWindow()

	effectiveCount := swc.calculateWeightedCount()
	if effectiveCount >= float64(swc.maxRequests) {
		return false
	}

	swc.currCount++
	return true
}

// Check returns true if a request would be allowed (without consuming).
// Use with Consume() for atomic multi-layer rate limiting.
func (swc *SlidingWindowCounter) Check() bool {
	if swc == nil {
		return true
	}

	swc.mu.Lock()
	defer swc.mu.Unlock()

	swc.maybeRotateWindow()

	effectiveCount := swc.calculateWeightedCount()
	return effectiveCount < float64(swc.maxRequests)
}

// Consume increments the counter (assumes Check() already passed).
func (swc *SlidingWindowCounter) Consume() {
	if swc == nil {
		return
	}

	swc.mu.Lock()
	defer swc.mu.Unlock()

	swc.maybeRotateWindow()

	// Only consume if still under limit (safety check)
	effectiveCount := swc.calculateWeightedCount()
	if effectiveCount < float64(swc.maxRequests) {
		swc.currCount++
	}
}

// maybeRotateWindow rotates to a new window if the current one has expired.
// Must be called with mu held.
func (swc *SlidingWindowCounter) maybeRotateWindow() {
	elapsed := time.Since(swc.currWindowStart)

	if elapsed >= swc.windowDuration {
		// How many full windows have passed?
		windowsPassed := int(elapsed / swc.windowDuration)

		if windowsPassed == 1 {
			// Normal case: exactly one window passed
			swc.prevCount = swc.currCount
		} else {
			// More than one window passed: previous window has no relevant data
			swc.prevCount = 0
		}

		swc.currCount = 0
		// Align window start to the beginning of the current window
		swc.currWindowStart = swc.currWindowStart.Add(time.Duration(windowsPassed) * swc.windowDuration)
	}
}

// calculateWeightedCount returns the weighted count for the sliding window.
// Must be called with mu held.
func (swc *SlidingWindowCounter) calculateWeightedCount() float64 {
	elapsed := time.Since(swc.currWindowStart)

	// Calculate overlap ratio: how much of the previous window is still relevant
	overlapRatio := float64(swc.windowDuration-elapsed) / float64(swc.windowDuration)
	if overlapRatio < 0 {
		overlapRatio = 0
	}
	if overlapRatio > 1 {
		overlapRatio = 1
	}

	return float64(swc.currCount) + float64(swc.prevCount)*overlapRatio
}

// GetEffectiveCount returns the current weighted count (for monitoring).
func (swc *SlidingWindowCounter) GetEffectiveCount() float64 {
	if swc == nil {
		return 0
	}

	swc.mu.Lock()
	defer swc.mu.Unlock()

	swc.maybeRotateWindow()
	return swc.calculateWeightedCount()
}

// GetRemaining returns the approximate remaining quota.
func (swc *SlidingWindowCounter) GetRemaining() int {
	if swc == nil {
		return -1 // Unlimited
	}

	swc.mu.Lock()
	defer swc.mu.Unlock()

	swc.maybeRotateWindow()
	effectiveCount := swc.calculateWeightedCount()
	remaining := float64(swc.maxRequests) - effectiveCount
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

// IsFull returns true if the rate limit is currently exceeded.
func (swc *SlidingWindowCounter) IsFull() bool {
	if swc == nil {
		return false
	}

	swc.mu.Lock()
	defer swc.mu.Unlock()

	swc.maybeRotateWindow()
	effectiveCount := swc.calculateWeightedCount()
	return effectiveCount >= float64(swc.maxRequests)
}
