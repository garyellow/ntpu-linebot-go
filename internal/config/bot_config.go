// Package config provides centralized configuration management for bot modules.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// BotConfig centralizes all bot module configuration.
// This improves maintainability by keeping all constants in one place.
type BotConfig struct {
	// Webhook configuration
	WebhookTimeout      time.Duration
	MaxMessagesPerReply int
	MaxEventsPerWebhook int
	MinReplyTokenLength int
	MaxMessageLength    int
	MaxPostbackDataSize int

	// Rate limiting configuration
	UserRateLimitTokens     float64
	UserRateLimitRefillRate float64
	LLMRateLimitPerHour     float64
	GlobalRateLimitRPS      float64

	// Course module configuration
	MaxCoursesPerSearch  int
	MaxTitleDisplayChars int

	// ID module configuration
	MaxStudentsPerSearch int
	ValidYearStart       int
	ValidYearEnd         int

	// Contact module configuration
	MaxContactsPerSearch int
}

// DefaultBotConfig returns default configuration values.
// LINE API limits: https://developers.line.biz/en/reference/messaging-api/#rate-limits
func DefaultBotConfig() *BotConfig {
	return &BotConfig{
		// Webhook (from LINE API constraints)
		WebhookTimeout:      WebhookProcessing, // 25 seconds from timeouts.go
		MaxMessagesPerReply: 5,                 // LINE API limit
		MaxEventsPerWebhook: 100,               // LINE API limit
		MinReplyTokenLength: 10,
		MaxMessageLength:    20000, // LINE API limit
		MaxPostbackDataSize: 300,   // LINE API limit

		// Rate limiting (based on LINE API limits)
		UserRateLimitTokens:     6.0,  // 6 tokens per user
		UserRateLimitRefillRate: 0.2,  // 1 token per 5 seconds
		LLMRateLimitPerHour:     10.0, // 10 LLM requests per user per hour
		GlobalRateLimitRPS:      80.0, // 80 RPS (LINE API: 100 RPS, we use 80 for safety)

		// Module limits (optimized for UX)
		MaxCoursesPerSearch:  40,  // 4 carousels @ 10 bubbles each
		MaxTitleDisplayChars: 60,  // Optimal display length
		MaxStudentsPerSearch: 500, // Prevent overwhelming responses
		MaxContactsPerSearch: 100, // Reasonable contact list size

		// ID module year range (NTPU specific)
		ValidYearStart: 95,  // 95 學年度
		ValidYearEnd:   112, // 112 學年度
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
