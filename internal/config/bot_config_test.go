package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultBotConfig(t *testing.T) {
	cfg := DefaultBotConfig()

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
	if cfg.GlobalRateLimitRPS != 80.0 {
		t.Errorf("expected GlobalRateLimitRPS 80.0, got %f", cfg.GlobalRateLimitRPS)
	}

	if cfg.LLMRateLimitPerHour != 10.0 {
		t.Errorf("expected LLMRateLimitPerHour 10.0, got %f", cfg.LLMRateLimitPerHour)
	}

	// Test module limits
	if cfg.MaxCoursesPerSearch != 40 {
		t.Errorf("expected MaxCoursesPerSearch 40, got %d", cfg.MaxCoursesPerSearch)
	}

	if cfg.MaxStudentsPerSearch != 500 {
		t.Errorf("expected MaxStudentsPerSearch 500, got %d", cfg.MaxStudentsPerSearch)
	}

	// Test year range
	if cfg.ValidYearStart != 95 {
		t.Errorf("expected ValidYearStart 95, got %d", cfg.ValidYearStart)
	}

	if cfg.ValidYearEnd != 112 {
		t.Errorf("expected ValidYearEnd 112, got %d", cfg.ValidYearEnd)
	}
}

func TestLoadBotConfig(t *testing.T) {
	// Save original environment
	originalMaxCourses := os.Getenv("BOT_MAX_COURSES")
	originalMaxStudents := os.Getenv("BOT_MAX_STUDENTS")
	originalLLMLimit := os.Getenv("BOT_LLM_RATE_LIMIT")
	originalGlobalRPS := os.Getenv("BOT_GLOBAL_RPS")

	// Clean up after test
	defer func() {
		os.Setenv("BOT_MAX_COURSES", originalMaxCourses)
		os.Setenv("BOT_MAX_STUDENTS", originalMaxStudents)
		os.Setenv("BOT_LLM_RATE_LIMIT", originalLLMLimit)
		os.Setenv("BOT_GLOBAL_RPS", originalGlobalRPS)
	}()

	t.Run("default values when no env vars", func(t *testing.T) {
		os.Unsetenv("BOT_MAX_COURSES")
		os.Unsetenv("BOT_MAX_STUDENTS")
		os.Unsetenv("BOT_LLM_RATE_LIMIT")
		os.Unsetenv("BOT_GLOBAL_RPS")

		cfg, err := LoadBotConfig()
		if err != nil {
			t.Fatalf("LoadBotConfig failed: %v", err)
		}

		if cfg.MaxCoursesPerSearch != 40 {
			t.Errorf("expected default MaxCoursesPerSearch 40, got %d", cfg.MaxCoursesPerSearch)
		}

		if cfg.MaxStudentsPerSearch != 500 {
			t.Errorf("expected default MaxStudentsPerSearch 500, got %d", cfg.MaxStudentsPerSearch)
		}
	})

	t.Run("override with environment variables", func(t *testing.T) {
		os.Setenv("BOT_MAX_COURSES", "50")
		os.Setenv("BOT_MAX_STUDENTS", "1000")
		os.Setenv("BOT_LLM_RATE_LIMIT", "20.5")
		os.Setenv("BOT_GLOBAL_RPS", "100.0")

		cfg, err := LoadBotConfig()
		if err != nil {
			t.Fatalf("LoadBotConfig failed: %v", err)
		}

		if cfg.MaxCoursesPerSearch != 50 {
			t.Errorf("expected MaxCoursesPerSearch 50, got %d", cfg.MaxCoursesPerSearch)
		}

		if cfg.MaxStudentsPerSearch != 1000 {
			t.Errorf("expected MaxStudentsPerSearch 1000, got %d", cfg.MaxStudentsPerSearch)
		}

		if cfg.LLMRateLimitPerHour != 20.5 {
			t.Errorf("expected LLMRateLimitPerHour 20.5, got %f", cfg.LLMRateLimitPerHour)
		}

		if cfg.GlobalRateLimitRPS != 100.0 {
			t.Errorf("expected GlobalRateLimitRPS 100.0, got %f", cfg.GlobalRateLimitRPS)
		}
	})

	t.Run("invalid env var values use defaults", func(t *testing.T) {
		os.Setenv("BOT_MAX_COURSES", "invalid")
		os.Setenv("BOT_MAX_STUDENTS", "-10")
		os.Setenv("BOT_LLM_RATE_LIMIT", "not_a_number")

		cfg, err := LoadBotConfig()
		if err != nil {
			t.Fatalf("LoadBotConfig failed: %v", err)
		}

		// Should fall back to defaults
		if cfg.MaxCoursesPerSearch != 40 {
			t.Errorf("expected default MaxCoursesPerSearch 40, got %d", cfg.MaxCoursesPerSearch)
		}

		if cfg.MaxStudentsPerSearch != 500 {
			t.Errorf("expected default MaxStudentsPerSearch 500, got %d", cfg.MaxStudentsPerSearch)
		}

		if cfg.LLMRateLimitPerHour != 10.0 {
			t.Errorf("expected default LLMRateLimitPerHour 10.0, got %f", cfg.LLMRateLimitPerHour)
		}
	})
}

func TestBotConfigTimeouts(t *testing.T) {
	cfg := DefaultBotConfig()

	// Ensure timeout is reasonable (between 10s and 60s)
	if cfg.WebhookTimeout < 10*time.Second || cfg.WebhookTimeout > 60*time.Second {
		t.Errorf("WebhookTimeout %v is outside reasonable range (10s-60s)", cfg.WebhookTimeout)
	}
}

func TestBotConfig_Validate(t *testing.T) {
	t.Run("valid default config", func(t *testing.T) {
		cfg := DefaultBotConfig()
		if err := cfg.Validate(); err != nil {
			t.Errorf("default config should be valid, got error: %v", err)
		}
	})

	t.Run("invalid webhook timeout", func(t *testing.T) {
		cfg := DefaultBotConfig()
		cfg.WebhookTimeout = 0
		if err := cfg.Validate(); err == nil {
			t.Error("expected validation error for zero webhook timeout")
		}
	})

	t.Run("invalid max messages per reply", func(t *testing.T) {
		tests := []int{0, 6, 10}
		for _, val := range tests {
			cfg := DefaultBotConfig()
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
			{"negative user tokens", func(c *BotConfig) { c.UserRateLimitTokens = -1 }},
			{"zero refill rate", func(c *BotConfig) { c.UserRateLimitRefillRate = 0 }},
			{"negative LLM limit", func(c *BotConfig) { c.LLMRateLimitPerHour = -1 }},
			{"zero global RPS", func(c *BotConfig) { c.GlobalRateLimitRPS = 0 }},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := DefaultBotConfig()
				tt.fn(cfg)
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
				cfg := DefaultBotConfig()
				tt.fn(cfg)
				if err := cfg.Validate(); err == nil {
					t.Error("expected validation error")
				}
			})
		}
	})
}
