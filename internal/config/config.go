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

	// LLM Configuration
	GeminiAPIKey string // Gemini API key for NLU and Query Expansion features
	GroqAPIKey   string // Groq API key (alternative LLM provider)

	// LLM Model Configuration (optional, defaults apply if empty)
	GeminiIntentModel           string // Primary Gemini model for intent parsing
	GeminiIntentFallbackModel   string // Fallback Gemini model for intent parsing
	GeminiExpanderModel         string // Primary Gemini model for query expansion
	GeminiExpanderFallbackModel string // Fallback Gemini model for query expansion
	GroqIntentModel             string // Primary Groq model for intent parsing
	GroqIntentFallbackModel     string // Fallback Groq model for intent parsing
	GroqExpanderModel           string // Primary Groq model for query expansion
	GroqExpanderFallbackModel   string // Fallback Groq model for query expansion

	// LLM Provider Configuration
	LLMPrimaryProvider  string // Primary LLM provider: "gemini" or "groq" (default: "gemini")
	LLMFallbackProvider string // Fallback LLM provider: "gemini" or "groq" (default: "groq")

	// Metrics Authentication
	MetricsUsername string // Username for /metrics endpoint Basic Auth (default: "prometheus")
	MetricsPassword string // Password for /metrics endpoint Basic Auth (empty = no auth)

	// Server Configuration
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration

	// Data Configuration
	DataDir  string        // Data directory for SQLite database
	CacheTTL time.Duration // TTL: absolute expiration for cache entries (default: 7 days)

	// Scraper Configuration
	ScraperTimeout    time.Duration
	ScraperMaxRetries int
	ScraperBaseURLs   map[string][]string

	// Bot Configuration (embedded)
	Bot BotConfig
}

// BotConfig holds bot-specific configuration
type BotConfig struct {
	// Timeouts
	WebhookTimeout time.Duration // Timeout for webhook bot processing (see config/timeouts.go)

	// Rate Limits (Token Bucket Algorithm)
	UserRateLimitBurst        float64 // Maximum burst tokens per user (default: 15)
	UserRateLimitRefillPerSec float64 // Tokens refilled per second (default: 0.1 = 1 per 10s)

	// LLM Rate Limits (Multi-Layer: Hourly + Daily)
	LLMBurstTokens   float64 // Maximum burst tokens for LLM (default: 40)
	LLMRefillPerHour float64 // LLM tokens refilled per hour (default: 20)
	LLMDailyLimit    int     // Maximum LLM requests per day (default: 100, 0 = disabled)

	GlobalRateLimitRPS float64 // Global rate limit in requests per second (default: 100)

	// LINE API Constraints
	MaxMessagesPerReply int // Maximum messages per reply (LINE API limit: 5)
	MaxEventsPerWebhook int // Maximum events per webhook (default: 100)
	MinReplyTokenLength int // Minimum reply token length (default: 10)
	MaxMessageLength    int // Maximum message length (LINE API limit: 20000)
	MaxPostbackDataSize int // Maximum postback data size (LINE API limit: 300)

	// Business Limits
	MaxCoursesPerSearch int // Maximum courses per search (default: 40)

	MaxStudentsPerSearch int // Maximum students per search (default: 400)
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

		// LLM Configuration
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
		GroqAPIKey:   getEnv("GROQ_API_KEY", ""),

		// LLM Model Configuration (empty = use defaults from genai package)
		GeminiIntentModel:           getEnv("GEMINI_INTENT_MODEL", ""),
		GeminiIntentFallbackModel:   getEnv("GEMINI_INTENT_FALLBACK_MODEL", ""),
		GeminiExpanderModel:         getEnv("GEMINI_EXPANDER_MODEL", ""),
		GeminiExpanderFallbackModel: getEnv("GEMINI_EXPANDER_FALLBACK_MODEL", ""),
		GroqIntentModel:             getEnv("GROQ_INTENT_MODEL", ""),
		GroqIntentFallbackModel:     getEnv("GROQ_INTENT_FALLBACK_MODEL", ""),
		GroqExpanderModel:           getEnv("GROQ_EXPANDER_MODEL", ""),
		GroqExpanderFallbackModel:   getEnv("GROQ_EXPANDER_FALLBACK_MODEL", ""),

		// LLM Provider Configuration
		LLMPrimaryProvider:  getEnv("LLM_PRIMARY_PROVIDER", "gemini"),
		LLMFallbackProvider: getEnv("LLM_FALLBACK_PROVIDER", "groq"),

		// Metrics Authentication
		MetricsUsername: getEnv("METRICS_USERNAME", "prometheus"),
		MetricsPassword: getEnv("METRICS_PASSWORD", ""),

		// Server Configuration
		Port:            getEnv("PORT", "10000"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),

		// Data Configuration
		DataDir:  getEnv("DATA_DIR", getDefaultDataDir()),
		CacheTTL: getDurationEnv("CACHE_TTL", 168*time.Hour), // TTL: 7 days

		// Scraper Configuration
		ScraperTimeout:    getDurationEnv("SCRAPER_TIMEOUT", ScraperRequest),
		ScraperMaxRetries: getIntEnv("SCRAPER_MAX_RETRIES", 10), // Max 10 retries with exponential backoff (1s initial)
		// IP first for faster scraping (avoids DNS lookup)
		// URLs generated for users are hard-coded to domain in scrapers
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

		// Bot Configuration
		Bot: BotConfig{
			WebhookTimeout:            getDurationEnv("WEBHOOK_TIMEOUT", WebhookProcessing),
			UserRateLimitBurst:        getFloatEnv("USER_RATE_LIMIT_BURST", 15.0),
			UserRateLimitRefillPerSec: getFloatEnv("USER_RATE_LIMIT_REFILL_PER_SEC", 0.1), // 1 per 10s
			LLMBurstTokens:            getFloatEnv("LLM_BURST_TOKENS", 40.0),
			LLMRefillPerHour:          getFloatEnv("LLM_REFILL_PER_HOUR", 20.0),
			LLMDailyLimit:             getIntEnv("LLM_DAILY_LIMIT", 100),
			GlobalRateLimitRPS:        getFloatEnv("GLOBAL_RATE_LIMIT_RPS", 100.0),
			MaxMessagesPerReply:       LINEMaxMessagesPerReply,
			MaxEventsPerWebhook:       100,
			MinReplyTokenLength:       10,
			MaxMessageLength:          LINEMaxTextMessageLength,
			MaxPostbackDataSize:       LINEMaxPostbackDataLength,
			MaxCoursesPerSearch:       40,

			MaxStudentsPerSearch: 400,
			MaxContactsPerSearch: 100,
			ValidYearStart:       95,
			ValidYearEnd:         112,
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

// HasLLMProvider returns true if at least one LLM provider is configured.
func (c *Config) HasLLMProvider() bool {
	return c.GeminiAPIKey != "" || c.GroqAPIKey != ""
}
