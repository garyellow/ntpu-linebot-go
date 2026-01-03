package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// mockMetrics creates a test Metrics instance
func mockMetrics() *metrics.Metrics {
	reg := prometheus.NewRegistry()
	return metrics.New(reg)
}

func TestKeyedLimiter_Basic(t *testing.T) {
	t.Parallel()
	cfg := KeyedConfig{
		Name:          "test",
		Burst:         1,
		RefillRate:    10,
		CleanupPeriod: time.Hour,
	}
	kl := NewKeyedLimiter(cfg)
	defer kl.Stop()

	// First request allows
	if !kl.Allow("user1") {
		t.Error("User1 first request failed")
	}
	// Second request denied (Burst 1)
	if kl.Allow("user1") {
		t.Error("User1 second request allowed (should limit)")
	}
	// Different user allowed
	if !kl.Allow("user2") {
		t.Error("User2 first request failed")
	}
}

func TestKeyedLimiter_Cleanup(t *testing.T) {
	t.Parallel()
	cfg := KeyedConfig{
		Name:          "cleanup_test",
		Burst:         10,
		RefillRate:    100, // Fast refill to fill bucket quickly
		CleanupPeriod: 50 * time.Millisecond,
	}
	kl := NewKeyedLimiter(cfg)
	defer kl.Stop()

	kl.Allow("u1")
	if count := kl.GetActiveCount(); count != 1 {
		t.Errorf("Active count = %d, want 1", count)
	}

	// Wait for refill (bucket full) + cleanup tick
	time.Sleep(200 * time.Millisecond)

	if count := kl.GetActiveCount(); count != 0 {
		t.Errorf("Active count = %d, want 0 after cleanup", count)
	}
}

func TestKeyedLimiter_CleanupWithDaily(t *testing.T) {
	t.Parallel()
	// Test that cleanup DOES NOT remove entry if daily limit still has usage
	cfg := KeyedConfig{
		Name:          "daily_cleanup_test",
		Burst:         10,
		RefillRate:    100, // Fast refill
		CleanupPeriod: 50 * time.Millisecond,
		DailyLimit:    5,
	}
	kl := NewKeyedLimiter(cfg)
	defer kl.Stop()

	kl.Allow("u1") // Consumes daily quota

	// Wait for refill (bucket full) + cleanup tick
	// Daily window is 24h, so usage remains
	time.Sleep(200 * time.Millisecond)

	if count := kl.GetActiveCount(); count != 1 {
		t.Errorf("Active count = %d, want 1 (should not cleanup daily usage)", count)
	}
}

func TestKeyedLimiter_ThreadSafety(t *testing.T) {
	t.Parallel()
	cfg := KeyedConfig{
		Name:          "concurrency_test",
		Burst:         1000,
		RefillRate:    1,
		CleanupPeriod: time.Hour,
	}
	kl := NewKeyedLimiter(cfg)
	defer kl.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("user%d", i%10) // 10 distinct keys
			kl.Allow(key)
			kl.GetAvailable(key)
		}(i)
	}
	wg.Wait()
}

func TestKeyedLimiter_GetAvailable(t *testing.T) {
	t.Parallel()
	cfg := KeyedConfig{
		Name:       "avail",
		Burst:      10,
		RefillRate: 1,
	}
	kl := NewKeyedLimiter(cfg)
	defer kl.Stop()

	// Check non-existent
	if v := kl.GetAvailable("new"); v != 10 {
		t.Errorf("New key available = %f, want 10", v)
	}

	kl.Allow("user1")
	// Should be < 10
	if v := kl.GetAvailable("user1"); v >= 10 {
		t.Errorf("Used key available = %f, want < 10", v)
	}
}

func TestKeyedLimiter_GetDailyRemaining(t *testing.T) {
	t.Parallel()
	cfg := KeyedConfig{
		Name:       "daily",
		Burst:      10,
		RefillRate: 1,
		DailyLimit: 5,
	}
	kl := NewKeyedLimiter(cfg)
	defer kl.Stop()

	// New user
	if r := kl.GetDailyRemaining("u1"); r != 5 {
		t.Errorf("Initial daily = %d, want 5", r)
	}

	kl.Allow("u1")
	if r := kl.GetDailyRemaining("u1"); r != 4 {
		t.Errorf("After usage daily = %d, want 4", r)
	}

	// Disabled daily
	cfg2 := KeyedConfig{Name: "nodaily", Burst: 10}
	kl2 := NewKeyedLimiter(cfg2)
	defer kl2.Stop()
	if r := kl2.GetDailyRemaining("u1"); r != -1 {
		t.Errorf("Disabled daily = %d, want -1", r)
	}
}
