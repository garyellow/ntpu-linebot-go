package webhook

import (
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestRateLimiter_Allow(t *testing.T) {
	// Create a rate limiter with 5 tokens, refilling at 1 token per second
	rl := NewRateLimiter(5.0, 1.0)

	// Should allow first 5 requests immediately
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if rl.Allow() {
		t.Error("6th request should be denied")
	}

	// Wait for 2 seconds to refill 2 tokens
	time.Sleep(2100 * time.Millisecond)

	// Should allow 2 more requests
	if !rl.Allow() {
		t.Error("Request after refill should be allowed")
	}
	if !rl.Allow() {
		t.Error("Second request after refill should be allowed")
	}

	// Next request should be denied
	if rl.Allow() {
		t.Error("Request should be denied after consuming refilled tokens")
	}
}

func TestRateLimiter_GetAvailableTokens(t *testing.T) {
	rl := NewRateLimiter(10.0, 2.0)

	// Initial tokens should be 10
	tokens := rl.GetAvailableTokens()
	if tokens < 9.9 || tokens > 10.1 {
		t.Errorf("Expected ~10 tokens, got %f", tokens)
	}

	// Consume 3 tokens
	for i := 0; i < 3; i++ {
		rl.Allow()
	}

	// Should have 7 tokens left
	tokens = rl.GetAvailableTokens()
	if tokens < 6.9 || tokens > 7.1 {
		t.Errorf("Expected ~7 tokens after consuming 3, got %f", tokens)
	}
}

func TestUserRateLimiter_Allow(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	url := NewUserRateLimiter(1*time.Minute, m)

	// Different users should have independent limits
	user1 := "user1"
	user2 := "user2"

	// User1 consumes 3 tokens
	for i := 0; i < 3; i++ {
		if !url.Allow(user1, 3.0, 1.0) {
			t.Errorf("User1 request %d should be allowed", i+1)
		}
	}

	// User1's 4th request should be denied
	if url.Allow(user1, 3.0, 1.0) {
		t.Error("User1's 4th request should be denied")
	}

	// User2 should still have tokens available
	if !url.Allow(user2, 3.0, 1.0) {
		t.Error("User2's first request should be allowed")
	}
}

func TestUserRateLimiter_RecordsDroppedRequests(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	url := NewUserRateLimiter(1*time.Minute, m)

	// Consume all tokens (3 tokens)
	for i := 0; i < 3; i++ {
		url.Allow("user1", 3.0, 1.0)
	}

	// This request should be dropped and recorded
	if url.Allow("user1", 3.0, 1.0) {
		t.Error("4th request should be denied")
	}

	// Note: We can't easily verify the metric was recorded without
	// exposing internal state or using a mock. The important thing
	// is that the code path doesn't panic.
}

func TestRateLimiter_WaitForToken(t *testing.T) {
	rl := NewRateLimiter(1.0, 10.0) // 1 token, refills at 10/sec

	// Consume the token
	if !rl.Allow() {
		t.Fatal("First request should be allowed")
	}

	// Wait for a token to be available
	start := time.Now()
	rl.WaitForToken()
	elapsed := time.Since(start)

	// Should wait approximately 0.1 seconds (1/10)
	if elapsed < 50*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("Wait time should be ~100ms, got %v", elapsed)
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	rl := NewRateLimiter(1000.0, 100.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow()
	}
}

func BenchmarkUserRateLimiter_Allow(b *testing.B) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	url := NewUserRateLimiter(1*time.Minute, m)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userID := "user" + string(rune(i%100))
		url.Allow(userID, 10.0, 1.0)
	}
}
