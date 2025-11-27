package config

import (
	"fmt"
	"os"
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

	// Server Configuration
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration

	// SQLite Configuration
	SQLitePath string
	CacheTTL   time.Duration // Hard TTL: absolute expiration for cache entries (default: 7 days)
	SoftTTL    time.Duration // Soft TTL: when to proactively refresh data (default: 5 days)

	// Scraper Configuration
	ScraperTimeout    time.Duration
	ScraperMaxRetries int

	// Warmup Configuration
	WarmupTimeout time.Duration
	WarmupModules string // Comma-separated list of modules to warmup (default: "id,contact,course,sticker")

	// Webhook Configuration
	// See internal/timeouts/timeouts.go for detailed explanation of why 25s is used
	WebhookTimeout time.Duration // Timeout for webhook bot processing

	// Rate Limit Configuration
	UserRateLimitTokens     float64 // Maximum tokens per user (default: 10)
	UserRateLimitRefillRate float64 // Tokens refill rate per second (default: 1/3, i.e., 1 token per 3 seconds)
}

// ValidationMode determines which fields are required during validation
type ValidationMode int

const (
	// ServerMode requires all fields including LINE credentials
	ServerMode ValidationMode = iota
	// WarmupMode only requires scraper and database fields
	WarmupMode
)

// Load reads configuration from environment variables with server mode validation
// It attempts to load .env file first, then reads from env vars
func Load() (*Config, error) {
	return LoadForMode(ServerMode)
}

// LoadForMode loads configuration for a specific execution mode
// ServerMode: Validates LINE credentials (for webhook server)
// WarmupMode: Skips LINE credentials validation (for cache warmup)
func LoadForMode(mode ValidationMode) (*Config, error) {
	// Try to load .env file (ignore error if file doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		// LINE Bot Configuration
		LineChannelToken:  getEnv("LINE_CHANNEL_ACCESS_TOKEN", ""),
		LineChannelSecret: getEnv("LINE_CHANNEL_SECRET", ""),

		// Server Configuration
		Port:            getEnv("PORT", "10000"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),

		// SQLite Configuration
		SQLitePath: getEnv("SQLITE_PATH", getDefaultDBPath()),
		CacheTTL:   getDurationEnv("CACHE_TTL", 168*time.Hour), // Hard TTL: 7 days
		SoftTTL:    getDurationEnv("SOFT_TTL", 120*time.Hour),  // Soft TTL: 5 days (trigger warmup)

		// Scraper Configuration
		ScraperTimeout:    getDurationEnv("SCRAPER_TIMEOUT", timeouts.ScraperRequest), // HTTP request timeout
		ScraperMaxRetries: getIntEnv("SCRAPER_MAX_RETRIES", 5),                        // Retry with exponential backoff

		// Warmup Configuration
		WarmupTimeout: getDurationEnv("WARMUP_TIMEOUT", timeouts.WarmupDefault),
		WarmupModules: getEnv("WARMUP_MODULES", "sticker,id,contact,course"),

		// Webhook Configuration
		WebhookTimeout: getDurationEnv("WEBHOOK_TIMEOUT", timeouts.WebhookProcessing),

		// Rate Limit Configuration
		UserRateLimitTokens:     getFloatEnv("USER_RATE_LIMIT_TOKENS", 10.0),
		UserRateLimitRefillRate: getFloatEnv("USER_RATE_LIMIT_REFILL_RATE", 1.0/3.0),
	}

	// Validate based on mode
	if err := cfg.ValidateForMode(mode); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks if required configuration values are set
// For server mode, all fields are required. For warmup mode, LINE credentials are optional.
func (c *Config) Validate() error {
	return c.ValidateForMode(ServerMode)
}

// ValidateForMode validates config based on the execution mode
// ServerMode requires LINE credentials and server fields
// WarmupMode only requires scraper and database fields
func (c *Config) ValidateForMode(mode ValidationMode) error {
	if mode == ServerMode {
		if c.LineChannelToken == "" {
			return fmt.Errorf("LINE_CHANNEL_ACCESS_TOKEN is required in server mode")
		}
		if c.LineChannelSecret == "" {
			return fmt.Errorf("LINE_CHANNEL_SECRET is required in server mode")
		}
		if c.Port == "" {
			return fmt.Errorf("PORT is required in server mode")
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
	}
	if c.SQLitePath == "" {
		return fmt.Errorf("SQLITE_PATH is required")
	}
	if c.CacheTTL <= 0 {
		return fmt.Errorf("CACHE_TTL must be positive")
	}
	if c.SoftTTL <= 0 {
		return fmt.Errorf("SOFT_TTL must be positive")
	}
	if c.SoftTTL >= c.CacheTTL {
		return fmt.Errorf("SOFT_TTL must be less than CACHE_TTL")
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

// getDefaultDBPath returns platform-specific default database path
func getDefaultDBPath() string {
	if runtime.GOOS == "windows" {
		return "./data/cache.db"
	}
	return "/data/cache.db"
}
