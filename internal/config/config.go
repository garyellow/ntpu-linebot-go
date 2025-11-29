// Package config provides application configuration management.
// It loads settings from environment variables and provides defaults for
// server mode, warmup mode, timeouts, and cache settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/timeouts"
	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// LINE Bot Configuration
	LineChannelToken  string
	LineChannelSecret string

	// GenAI Configuration
	GeminiAPIKey string // Gemini API key for embedding and RAG features

	// Server Configuration
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration

	// Data Configuration
	DataDir  string        // Data directory for SQLite and vector database
	CacheTTL time.Duration // Hard TTL: absolute expiration for cache entries (default: 7 days)

	// Scraper Configuration
	ScraperTimeout    time.Duration
	ScraperMaxRetries int

	// Warmup Configuration
	WarmupModules string // Comma-separated list of modules to warmup (default: "sticker,id,contact,course"). Add "syllabus" to enable syllabus warmup (requires GEMINI_API_KEY)

	// Webhook Configuration
	// See internal/timeouts/timeouts.go for detailed explanation of why 60s is used
	WebhookTimeout time.Duration // Timeout for webhook bot processing

	// Rate Limit Configuration
	UserRateLimitTokens     float64 // Maximum tokens per user (default: 10)
	UserRateLimitRefillRate float64 // Tokens refill rate per second (default: 1/3, i.e., 1 token per 3 seconds)
}

// Load reads configuration from environment variables
// It attempts to load .env file first, then reads from env vars
func Load() (*Config, error) {
	// Try to load .env file (ignore error if file doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		// LINE Bot Configuration
		LineChannelToken:  getEnv("LINE_CHANNEL_ACCESS_TOKEN", ""),
		LineChannelSecret: getEnv("LINE_CHANNEL_SECRET", ""),

		// GenAI Configuration
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),

		// Server Configuration
		Port:            getEnv("PORT", "10000"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),

		// Data Configuration
		DataDir:  getEnv("DATA_DIR", getDefaultDataDir()),
		CacheTTL: getDurationEnv("CACHE_TTL", 168*time.Hour), // Hard TTL: 7 days (資料過期後強制刪除)

		// Scraper Configuration
		ScraperTimeout:    getDurationEnv("SCRAPER_TIMEOUT", timeouts.ScraperRequest), // HTTP request timeout
		ScraperMaxRetries: getIntEnv("SCRAPER_MAX_RETRIES", 5),                        // Retry with exponential backoff

		// Warmup Configuration
		WarmupModules: getEnv("WARMUP_MODULES", "sticker,id,contact,course"),

		// Webhook Configuration
		WebhookTimeout: getDurationEnv("WEBHOOK_TIMEOUT", timeouts.WebhookProcessing),

		// Rate Limit Configuration
		UserRateLimitTokens:     getFloatEnv("USER_RATE_LIMIT_TOKENS", 10.0),
		UserRateLimitRefillRate: getFloatEnv("USER_RATE_LIMIT_REFILL_RATE", 1.0/3.0),
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks if required configuration values are set
func (c *Config) Validate() error {
	if c.LineChannelToken == "" {
		return fmt.Errorf("LINE_CHANNEL_ACCESS_TOKEN is required")
	}
	if c.LineChannelSecret == "" {
		return fmt.Errorf("LINE_CHANNEL_SECRET is required")
	}
	if c.Port == "" {
		return fmt.Errorf("PORT is required")
	}
	if c.WebhookTimeout <= 0 {
		return fmt.Errorf("WEBHOOK_TIMEOUT must be positive")
	}
	if c.UserRateLimitTokens <= 0 {
		return fmt.Errorf("USER_RATE_LIMIT_TOKENS must be positive")
	}
	if c.UserRateLimitRefillRate <= 0 {
		return fmt.Errorf("USER_RATE_LIMIT_REFILL_RATE must be positive")
	}
	if c.DataDir == "" {
		return fmt.Errorf("DATA_DIR is required")
	}
	if c.CacheTTL <= 0 {
		return fmt.Errorf("CACHE_TTL must be positive")
	}
	if c.ScraperTimeout <= 0 {
		return fmt.Errorf("SCRAPER_TIMEOUT must be positive")
	}
	if c.ScraperMaxRetries < 0 {
		return fmt.Errorf("SCRAPER_MAX_RETRIES cannot be negative")
	}
	return nil
}

// getEnv retrieves environment variable with fallback to default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getIntEnv retrieves integer environment variable with fallback to default value
func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getDurationEnv retrieves duration environment variable with fallback to default value
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getFloatEnv retrieves float64 environment variable with fallback to default value
func getFloatEnv(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

// getDefaultDataDir returns platform-specific default data directory
func getDefaultDataDir() string {
	if runtime.GOOS == "windows" {
		return "./data"
	}
	return "/data"
}

// SQLitePath returns the full path to the SQLite database file
func (c *Config) SQLitePath() string {
	return filepath.Join(c.DataDir, "cache.db")
}
