// Package config provides centralized configuration management for bot modules.
package config

import "fmt"

// LINE API constraints (https://developers.line.biz/en/docs/messaging-api/)
const (
	// LINEMaxTextMessageLength is the maximum length for LINE text messages (characters, not bytes)
	LINEMaxTextMessageLength = 5000
	// LINEMaxPostbackDataLength is the maximum length for postback data (bytes)
	LINEMaxPostbackDataLength = 300
	// LINEMaxMessagesPerReply is the maximum messages per reply (LINE API limit)
	LINEMaxMessagesPerReply = 5
)

// Validate checks if the bot configuration is valid.
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

	if c.UserRateBurst <= 0 {
		return fmt.Errorf("user rate burst must be positive, got %f", c.UserRateBurst)
	}

	if c.UserRateRefill <= 0 {
		return fmt.Errorf("user rate refill must be positive, got %f", c.UserRateRefill)
	}

	if c.LLMRateBurst <= 0 {
		return fmt.Errorf("LLM rate burst must be positive, got %f", c.LLMRateBurst)
	}

	if c.LLMRateRefill <= 0 {
		return fmt.Errorf("LLM rate refill must be positive, got %f", c.LLMRateRefill)
	}

	// LLMRateDaily can be 0 (disabled)
	if c.LLMRateDaily < 0 {
		return fmt.Errorf("LLM rate daily must be non-negative, got %d", c.LLMRateDaily)
	}

	if c.GlobalRateRPS <= 0 {
		return fmt.Errorf("global rate RPS must be positive, got %f", c.GlobalRateRPS)
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
