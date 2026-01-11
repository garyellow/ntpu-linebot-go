package usage

import (
	"context"
	"testing"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
)

func TestHandler_CanHandle(t *testing.T) {
	h := NewHandler(nil, nil, logger.New("debug"), nil)

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Chinese keywords - must have space after or be entire text
		{"用量", "用量", true},
		{"配額", "配額", true},
		{"額度", "額度", true},
		{"扣打", "扣打", true},
		{"用量_with_space", "用量 查詢", true},
		// English keywords
		{"quota", "quota", true},
		{"usage", "usage", true},
		{"limit", "limit", true},
		// With leading/trailing spaces
		{"with_spaces", "  用量  ", true},
		// Case insensitive
		{"uppercase_quota", "QUOTA", true},
		{"mixed_case", "QuOtA", true},
		// Should not match
		{"random_text", "hello world", false},
		{"empty", "", false},
		{"keyword_in_middle", "我的用量", false},      // keyword not at start
		{"keyword_with_suffix", "用量查詢的資訊", false}, // no space after keyword (compound word)
		{"keyword_no_space", "quota123", false},   // no space after keyword
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.CanHandle(tt.input); got != tt.expected {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHandler_HandleMessage(t *testing.T) {
	// Create limiter for testing
	llmLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Burst:      10,
		RefillRate: 0.1,
		DailyLimit: 100,
	})
	defer llmLimiter.Stop()

	userLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Burst:      5,
		RefillRate: 0.1,
	})
	defer userLimiter.Stop()

	h := NewHandler(userLimiter, llmLimiter, logger.New("debug"), nil)

	// Basic test - should return a message
	ctx := context.Background()
	msgs := h.HandleMessage(ctx, "用量")

	if len(msgs) == 0 {
		t.Error("HandleMessage returned no messages")
	}
}

func TestHandler_DispatchIntent(t *testing.T) {
	h := NewHandler(nil, nil, logger.New("debug"), nil)

	tests := []struct {
		name       string
		intent     string
		wantError  bool
		wantMsgCnt int
	}{
		{"query_intent", "query", false, 1},
		{"unknown_intent", "unknown", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs, err := h.DispatchIntent(context.Background(), tt.intent, nil)

			if tt.wantError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantError && len(msgs) != tt.wantMsgCnt {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantMsgCnt)
			}
		})
	}
}

func TestHandler_HandlePostback(t *testing.T) {
	h := NewHandler(nil, nil, logger.New("debug"), nil)

	tests := []struct {
		name       string
		data       string
		wantMsgCnt int
	}{
		{"query_postback", "query", 1},
		{"quota_postback", "配額", 1},
		{"prefixed_query", "usage:query", 1},
		{"unknown_postback", "unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := h.HandlePostback(context.Background(), tt.data)

			if len(msgs) != tt.wantMsgCnt {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantMsgCnt)
			}
		})
	}
}

func TestUsageStats(t *testing.T) {
	// Test with both limiters
	llmLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Burst:      10,
		RefillRate: 0.1,
		DailyLimit: 100,
	})
	defer llmLimiter.Stop()

	// Consume some tokens
	llmLimiter.Allow("test-user")
	llmLimiter.Allow("test-user")

	stats := llmLimiter.GetUsageStats("test-user")

	// BurstMax should be 10
	if stats.BurstMax != 10 {
		t.Errorf("BurstMax = %v, want 10", stats.BurstMax)
	}

	// BurstAvailable should be less than 10 (consumed 2)
	if stats.BurstAvailable >= 10 {
		t.Errorf("BurstAvailable = %v, want less than 10", stats.BurstAvailable)
	}

	// DailyMax should be 100
	if stats.DailyMax != 100 {
		t.Errorf("DailyMax = %v, want 100", stats.DailyMax)
	}

	// DailyRemaining should be less than 100
	if stats.DailyRemaining >= 100 {
		t.Errorf("DailyRemaining = %v, want less than 100", stats.DailyRemaining)
	}
}

func TestUsageStats_NewUser(t *testing.T) {
	llmLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Burst:      10,
		RefillRate: 0.1,
		DailyLimit: 100,
	})
	defer llmLimiter.Stop()

	// New user should have full quota
	stats := llmLimiter.GetUsageStats("new-user")

	if stats.BurstAvailable != 10 {
		t.Errorf("BurstAvailable = %v, want 10", stats.BurstAvailable)
	}

	if stats.DailyRemaining != 100 {
		t.Errorf("DailyRemaining = %v, want 100", stats.DailyRemaining)
	}
}

func TestUsageStats_DisabledDaily(t *testing.T) {
	limiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Burst:      10,
		RefillRate: 0.1,
		DailyLimit: 0, // Disabled
	})
	defer limiter.Stop()

	stats := limiter.GetUsageStats("user")

	// DailyMax should be -1 when disabled
	if stats.DailyMax != -1 {
		t.Errorf("DailyMax = %v, want -1", stats.DailyMax)
	}

	if stats.DailyRemaining != -1 {
		t.Errorf("DailyRemaining = %v, want -1", stats.DailyRemaining)
	}
}
