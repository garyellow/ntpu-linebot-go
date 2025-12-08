// Package config provides centralized configuration management for bot modules.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// BotConfig centralizes bot module configuration.
type BotConfig struct {
	WebhookTimeout          time.Duration
	MaxMessagesPerReply     int
	MaxEventsPerWebhook     int
	MinReplyTokenLength     int
	MaxMessageLength        int
	MaxPostbackDataSize     int
	UserRateLimitTokens     float64
	UserRateLimitRefillRate float64
	LLMRateLimitPerHour     float64
	GlobalRateLimitRPS      float64
	MaxCoursesPerSearch     int
	MaxTitleDisplayChars    int
	MaxStudentsPerSearch    int
	ValidYearStart          int
	ValidYearEnd            int
	MaxContactsPerSearch    int
}

// DefaultBotConfig returns default configuration values.
func DefaultBotConfig() *BotConfig {
	return &BotConfig{
		WebhookTimeout:          WebhookProcessing,
		MaxMessagesPerReply:     5,
		MaxEventsPerWebhook:     100,
		MinReplyTokenLength:     10,
		MaxMessageLength:        20000,
		MaxPostbackDataSize:     300,
		UserRateLimitTokens:     6.0,
		UserRateLimitRefillRate: 0.2,
		LLMRateLimitPerHour:     10.0,
		GlobalRateLimitRPS:      80.0,
		MaxCoursesPerSearch:     40,
		MaxTitleDisplayChars:    60,
		MaxStudentsPerSearch:    500,
		MaxContactsPerSearch:    100,
		ValidYearStart:          95,
		ValidYearEnd:            112,
	}
}

// LoadBotConfig loads configuration from environment variables.
// Falls back to defaults if environment variables are not set.
// Validates configuration before returning.
func LoadBotConfig() (*BotConfig, error) {
	cfg := DefaultBotConfig()

	// Allow environment variable overrides
	if v := os.Getenv("BOT_MAX_COURSES"); v != "" {
		if val, err := strconv.Atoi(v); err == nil && val > 0 {
			cfg.MaxCoursesPerSearch = val
		}
	}

	if v := os.Getenv("BOT_MAX_STUDENTS"); v != "" {
		if val, err := strconv.Atoi(v); err == nil && val > 0 {
			cfg.MaxStudentsPerSearch = val
		}
	}

	if v := os.Getenv("BOT_LLM_RATE_LIMIT"); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil && val > 0 {
			cfg.LLMRateLimitPerHour = val
		}
	}

	if v := os.Getenv("BOT_GLOBAL_RPS"); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil && val > 0 {
			cfg.GlobalRateLimitRPS = val
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
// Returns error describing validation failures.
func (c *BotConfig) Validate() error {
	if c.WebhookTimeout <= 0 {
		return fmt.Errorf("webhook timeout must be positive, got %v", c.WebhookTimeout)
	}

	if c.MaxMessagesPerReply < 1 || c.MaxMessagesPerReply > 5 {
		return fmt.Errorf("max messages per reply must be 1-5 (LINE API limit), got %d", c.MaxMessagesPerReply)
	}

	if c.MaxEventsPerWebhook < 1 {
		return fmt.Errorf("max events per webhook must be positive, got %d", c.MaxEventsPerWebhook)
	}

	if c.UserRateLimitTokens <= 0 {
		return fmt.Errorf("user rate limit tokens must be positive, got %f", c.UserRateLimitTokens)
	}

	if c.UserRateLimitRefillRate <= 0 {
		return fmt.Errorf("user rate limit refill rate must be positive, got %f", c.UserRateLimitRefillRate)
	}

	if c.LLMRateLimitPerHour <= 0 {
		return fmt.Errorf("LLM rate limit must be positive, got %f", c.LLMRateLimitPerHour)
	}

	if c.GlobalRateLimitRPS <= 0 {
		return fmt.Errorf("global rate limit RPS must be positive, got %f", c.GlobalRateLimitRPS)
	}

	if c.MaxCoursesPerSearch < 1 {
		return fmt.Errorf("max courses per search must be positive, got %d", c.MaxCoursesPerSearch)
	}

	if c.MaxStudentsPerSearch < 1 {
		return fmt.Errorf("max students per search must be positive, got %d", c.MaxStudentsPerSearch)
	}

	if c.MaxContactsPerSearch < 1 {
		return fmt.Errorf("max contacts per search must be positive, got %d", c.MaxContactsPerSearch)
	}

	return nil
}
