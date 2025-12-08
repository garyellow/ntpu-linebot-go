// Package config provides centralized configuration management for bot modules.
package config

import "fmt"

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
