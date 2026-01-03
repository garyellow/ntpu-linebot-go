package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestNewSlidingWindowCounter(t *testing.T) {
	t.Parallel()
	// Test disabled
	if NewSlidingWindowCounter(0, time.Hour) != nil {
		t.Error("expected nil for maxRequests <= 0")
	}
	// Test normal
	swc := NewSlidingWindowCounter(10, time.Hour)
	if swc == nil {
		t.Error("expected non-nil counter")
	}
}

func TestSlidingWindowCounter_Allow(t *testing.T) {
	t.Parallel()
	swc := NewSlidingWindowCounter(5, time.Second)

	for i := 0; i < 5; i++ {
		if !swc.Allow() {
			t.Errorf("Allow() failed at request %d", i+1)
		}
	}
	if swc.Allow() {
		t.Error("Allow() passed when limit exceeded")
	}
}

func TestSlidingWindowCounter_WindowRotation(t *testing.T) {
	t.Parallel()
	// Use small window for testing
	window := 50 * time.Millisecond
	swc := NewSlidingWindowCounter(10, window)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		swc.Allow()
	}
	if swc.Allow() {
		t.Error("should be limited")
	}

	// Wait > 1 window to rotate
	time.Sleep(window + 20*time.Millisecond)

	// After rotation, we should be able to allow at least one
	// (Prev window full, but weight decreases as time passes)
	if !swc.Allow() {
		t.Error("should allow after window rotation")
	}
}

func TestSlidingWindowCounter_WeightedCount(t *testing.T) {
	t.Parallel()
	// Logic verification:
	// Window 100ms, Limit 10.
	// T=0: consume 10.
	// Sleep 150ms -> 1.5 windows passed.
	// Current window start shifted by 100ms.
	// Elapsed in current window = 50ms.
	// Overlap = (100 - 50) / 100 = 0.5.
	// Effective = curr(0) + prev(10) * 0.5 = 5.
	// Available = 10 - 5 = 5.

	window := 100 * time.Millisecond
	swc := NewSlidingWindowCounter(10, window)

	for i := 0; i < 10; i++ {
		swc.Allow()
	}

	// Sleep 1.5 windows
	time.Sleep(150 * time.Millisecond)

	remaining := swc.GetRemaining()
	// Allow small tolerance for timing variations
	if remaining < 4 || remaining > 6 {
		t.Errorf("expected ~5 remaining, got %d", remaining)
	}

	effective := swc.GetEffectiveCount()
	if effective < 4.0 || effective > 6.0 {
		t.Errorf("expected ~5.0 effective count, got %f", effective)
	}
}

func TestSlidingWindowCounter_CheckConsume(t *testing.T) {
	t.Parallel()
	swc := NewSlidingWindowCounter(1, time.Minute)

	if !swc.Check() {
		t.Error("Check() should return true for empty counter")
	}

	// Atomic-like usage pattern
	swc.Consume()

	if swc.Check() {
		t.Error("Check() should return false after limit reached")
	}
}

func TestSlidingWindowCounter_Concurrency(t *testing.T) {
	t.Parallel()
	limit := 100
	swc := NewSlidingWindowCounter(limit, time.Hour)

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Spawn 200 goroutines trying to consume
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if swc.Allow() {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount != limit {
		t.Errorf("Allowed %d requests concurrently, want %d", successCount, limit)
	}
}

func TestSlidingWindowCounter_MultiWindowGap(t *testing.T) {
	t.Parallel()
	// Test behavior when multiple windows have passed (gap > window)
	window := 20 * time.Millisecond
	swc := NewSlidingWindowCounter(10, window)

	swc.Allow() // Usage in W1

	// Sleep 3 windows
	time.Sleep(65 * time.Millisecond)

	// Both curr and prev should be 0
	if swc.GetEffectiveCount() != 0 {
		t.Errorf("Expected 0 effective count after long gap, got %f", swc.GetEffectiveCount())
	}
}
