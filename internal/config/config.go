// Package config provides application configuration management.
// It loads settings from environment variables and provides defaults for
// server mode, warmup mode, timeouts, and cache settings.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// LINE Bot Configuration
	LineChannelToken  string
	LineChannelSecret string

	// GenAI Configuration
	GeminiAPIKey string // Gemini API key for NLU and Query Expansion features

	// Server Configuration
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration

	// Data Configuration
	DataDir  string        // Data directory for SQLite database
	CacheTTL time.Duration // Hard TTL: absolute expiration for cache entries (default: 7 days)

	// Scraper Configuration
	ScraperTimeout    time.Duration
	ScraperMaxRetries int
	ScraperBaseURLs   map[string][]string

	// Warmup Configuration
	WarmupModules string // Comma-separated list of modules to warmup (default: "sticker,id,contact,course"). Add "syllabus" to enable syllabus warmup (requires GEMINI_API_KEY)

	// Bot Configuration (embedded)
	Bot BotConfig
}

// BotConfig holds bot-specific configuration
type BotConfig struct {
	// Timeouts
	WebhookTimeout time.Duration // Timeout for webhook bot processing (see config/timeouts.go)

	// Rate Limits
	UserRateLimitTokens     float64 // Maximum tokens per user (default: 6)
	UserRateLimitRefillRate float64 // Tokens refill rate per second (default: 1/5)
	LLMRateLimitPerHour     float64 // Maximum LLM requests per user per hour (default: 50)
	GlobalRateLimitRPS      float64 // Global rate limit in requests per second (default: 100)

	// LINE API Constraints
	MaxMessagesPerReply int // Maximum messages per reply (LINE API limit: 5)
	MaxEventsPerWebhook int // Maximum events per webhook (default: 100)
	MinReplyTokenLength int // Minimum reply token length (default: 10)
	MaxMessageLength    int // Maximum message length (LINE API limit: 20000)
	MaxPostbackDataSize int // Maximum postback data size (LINE API limit: 300)

	// Business Limits
	MaxCoursesPerSearch  int // Maximum courses per search (default: 40)
	MaxTitleDisplayChars int // Maximum title display characters (default: 60)
	MaxStudentsPerSearch int // Maximum students per search (default: 500)
	MaxContactsPerSearch int // Maximum contacts per search (default: 100)
	ValidYearStart       int // Valid year range start (default: 95)
	ValidYearEnd         int // Valid year range end (default: 112)
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
		CacheTTL: getDurationEnv("CACHE_TTL", 168*time.Hour), // Hard TTL: 7 days

		// Scraper Configuration
		ScraperTimeout:    getDurationEnv("SCRAPER_TIMEOUT", ScraperRequest),
		ScraperMaxRetries: getIntEnv("SCRAPER_MAX_RETRIES", 5),
		ScraperBaseURLs: map[string][]string{
			"lms": {
				"http://120.126.197.52",
				"https://120.126.197.52",
				"https://lms.ntpu.edu.tw",
			},
			"sea": {
				"http://120.126.197.7",
				"https://120.126.197.7",
				"https://sea.cc.ntpu.edu.tw",
			},
		},

		// Warmup Configuration
		WarmupModules: getEnv("WARMUP_MODULES", "sticker,id,contact,course"),

		// Bot Configuration
		Bot: BotConfig{
			WebhookTimeout:          getDurationEnv("WEBHOOK_TIMEOUT", WebhookProcessing),
			UserRateLimitTokens:     getFloatEnv("USER_RATE_LIMIT_TOKENS", 6.0),
			UserRateLimitRefillRate: getFloatEnv("USER_RATE_LIMIT_REFILL_RATE", 1.0/5.0),
			LLMRateLimitPerHour:     getFloatEnv("LLM_RATE_LIMIT_PER_HOUR", 50.0),
			GlobalRateLimitRPS:      100.0,
			MaxMessagesPerReply:     5,
			MaxEventsPerWebhook:     100,
			MinReplyTokenLength:     10,
			MaxMessageLength:        20000,
			MaxPostbackDataSize:     300,
			MaxCoursesPerSearch:     40,
			MaxTitleDisplayChars:    60,
			MaxStudentsPerSearch:    500,
			MaxContactsPerSearch:    100,
			ValidYearStart:          95,
			ValidYearEnd:            112,
		},
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks if required configuration values are set
func (c *Config) Validate() error {
	var errs []error

	if c.LineChannelToken == "" {
		errs = append(errs, errors.New("LINE_CHANNEL_ACCESS_TOKEN is required"))
	}
	if c.LineChannelSecret == "" {
		errs = append(errs, errors.New("LINE_CHANNEL_SECRET is required"))
	}
	if c.Port == "" {
		errs = append(errs, errors.New("PORT is required"))
	}
	if err := c.Bot.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("bot config: %w", err))
	}
	if c.DataDir == "" {
		errs = append(errs, errors.New("DATA_DIR is required"))
	}
	if c.CacheTTL <= 0 {
		errs = append(errs, fmt.Errorf("CACHE_TTL must be positive, got %v", c.CacheTTL))
	}
	if c.ScraperTimeout <= 0 {
		errs = append(errs, fmt.Errorf("SCRAPER_TIMEOUT must be positive, got %v", c.ScraperTimeout))
	}
	if c.ScraperMaxRetries < 0 {
		errs = append(errs, fmt.Errorf("SCRAPER_MAX_RETRIES cannot be negative, got %d", c.ScraperMaxRetries))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
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
