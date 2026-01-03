package config

import (
	"testing"
	"time"
)

func newTestBotConfig() BotConfig {
	return BotConfig{
		WebhookTimeout:            WebhookProcessing,
		UserRateLimitBurst:        15.0,
		UserRateLimitRefillPerSec: 0.1,
		LLMBurstTokens:            40.0,
		LLMRefillPerHour:          20.0,
		LLMDailyLimit:             100,
		GlobalRateLimitRPS:        100.0,
		MaxMessagesPerReply:       5,
		MaxEventsPerWebhook:       100,
		MinReplyTokenLength:       10,
		MaxMessageLength:          20000,
		MaxPostbackDataSize:       300,
		MaxCoursesPerSearch:       40,
		MaxStudentsPerSearch:      400,
		MaxContactsPerSearch:      100,
		ValidYearStart:            95,
		ValidYearEnd:              112,
	}
}

func TestNewBotConfig(t *testing.T) {
	cfg := newTestBotConfig()

	// Test webhook configuration
	if cfg.WebhookTimeout != WebhookProcessing {
		t.Errorf("expected WebhookTimeout %v, got %v", WebhookProcessing, cfg.WebhookTimeout)
	}

	if cfg.MaxMessagesPerReply != 5 {
		t.Errorf("expected MaxMessagesPerReply 5, got %d", cfg.MaxMessagesPerReply)
	}

	if cfg.MaxEventsPerWebhook != 100 {
		t.Errorf("expected MaxEventsPerWebhook 100, got %d", cfg.MaxEventsPerWebhook)
	}

	// Test rate limiting
	if cfg.GlobalRateLimitRPS != 100.0 {
		t.Errorf("expected GlobalRateLimitRPS 100.0, got %f", cfg.GlobalRateLimitRPS)
	}

	if cfg.LLMBurstTokens != 40.0 {
		t.Errorf("expected LLMBurstTokens 40.0, got %f", cfg.LLMBurstTokens)
	}

	if cfg.LLMRefillPerHour != 20.0 {
		t.Errorf("expected LLMRefillPerHour 20.0, got %f", cfg.LLMRefillPerHour)
	}

	if cfg.LLMDailyLimit != 100 {
		t.Errorf("expected LLMDailyLimit 100, got %d", cfg.LLMDailyLimit)
	}

	if cfg.UserRateLimitBurst != 15.0 {
		t.Errorf("expected UserRateLimitBurst 15.0, got %f", cfg.UserRateLimitBurst)
	}

	if cfg.UserRateLimitRefillPerSec != 0.1 {
		t.Errorf("expected UserRateLimitRefillPerSec 0.1, got %f", cfg.UserRateLimitRefillPerSec)
	}

	// Test module limits
	if cfg.MaxCoursesPerSearch != 40 {
		t.Errorf("expected MaxCoursesPerSearch 40, got %d", cfg.MaxCoursesPerSearch)
	}

	if cfg.MaxStudentsPerSearch != 400 {
		t.Errorf("expected MaxStudentsPerSearch 400, got %d", cfg.MaxStudentsPerSearch)
	}

	// Test year range
	if cfg.ValidYearStart != 95 {
		t.Errorf("expected ValidYearStart 95, got %d", cfg.ValidYearStart)
	}

	if cfg.ValidYearEnd != 112 {
		t.Errorf("expected ValidYearEnd 112, got %d", cfg.ValidYearEnd)
	}
}

func TestBotConfigCustomValues(t *testing.T) {
	cfg := newTestBotConfig()
	cfg.WebhookTimeout = 30 * time.Second
	cfg.UserRateLimitBurst = 10.0
	cfg.UserRateLimitRefillPerSec = 0.5
	cfg.LLMBurstTokens = 100.0

	if cfg.WebhookTimeout != 30*time.Second {
		t.Errorf("expected WebhookTimeout 30s, got %v", cfg.WebhookTimeout)
	}

	if cfg.UserRateLimitBurst != 10.0 {
		t.Errorf("expected UserRateLimitBurst 10.0, got %f", cfg.UserRateLimitBurst)
	}

	if cfg.UserRateLimitRefillPerSec != 0.5 {
		t.Errorf("expected UserRateLimitRefillPerSec 0.5, got %f", cfg.UserRateLimitRefillPerSec)
	}

	if cfg.LLMBurstTokens != 100.0 {
		t.Errorf("expected LLMBurstTokens 100.0, got %f", cfg.LLMBurstTokens)
	}
}

func TestBotConfigTimeouts(t *testing.T) {
	cfg := newTestBotConfig()

	// Ensure timeout is reasonable (between 10s and 60s)
	if cfg.WebhookTimeout < 10*time.Second || cfg.WebhookTimeout > 60*time.Second {
		t.Errorf("WebhookTimeout %v is outside reasonable range (10s-60s)", cfg.WebhookTimeout)
	}
}

func TestBotConfig_Validate(t *testing.T) {
	t.Run("valid default config", func(t *testing.T) {
		cfg := newTestBotConfig()
		if err := cfg.Validate(); err != nil {
			t.Errorf("default config should be valid, got error: %v", err)
		}
	})

	t.Run("invalid webhook timeout", func(t *testing.T) {
		cfg := newTestBotConfig()
		cfg.WebhookTimeout = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected validation error for zero webhook timeout")
		}
	})

	t.Run("invalid max messages per reply", func(t *testing.T) {
		tests := []int{0, 6, 10}
		for _, val := range tests {
			cfg := newTestBotConfig()
			cfg.MaxMessagesPerReply = val
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected validation error for MaxMessagesPerReply=%d", val)
			}
		}
	})

	t.Run("invalid rate limits", func(t *testing.T) {
		tests := []struct {
			name string
			fn   func(*BotConfig)
		}{
			{"negative user burst", func(c *BotConfig) { c.UserRateLimitBurst = -1 }},
			{"zero refill rate", func(c *BotConfig) { c.UserRateLimitRefillPerSec = 0 }},
			{"negative LLM burst", func(c *BotConfig) { c.LLMBurstTokens = -1 }},
			{"zero LLM refill", func(c *BotConfig) { c.LLMRefillPerHour = 0 }},
			{"zero global RPS", func(c *BotConfig) { c.GlobalRateLimitRPS = 0 }},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := newTestBotConfig()
				tt.fn(&cfg)
				if err := cfg.Validate(); err == nil {
					t.Error("expected validation error")
				}
			})
		}
	})

	t.Run("invalid search limits", func(t *testing.T) {
		tests := []struct {
			name string
			fn   func(*BotConfig)
		}{
			{"zero max courses", func(c *BotConfig) { c.MaxCoursesPerSearch = 0 }},
			{"negative max students", func(c *BotConfig) { c.MaxStudentsPerSearch = -1 }},
			{"zero max contacts", func(c *BotConfig) { c.MaxContactsPerSearch = 0 }},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := newTestBotConfig()
				tt.fn(&cfg)
				if err := cfg.Validate(); err == nil {
					t.Error("expected validation error")
				}
			})
		}
	})
}
