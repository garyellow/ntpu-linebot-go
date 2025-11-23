package config

import (
	"fmt"
	"os"
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

	// Server Configuration
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration

	// SQLite Configuration
	SQLitePath string
	CacheTTL   time.Duration // Cache TTL for all operations (queries and cleanup)

	// Scraper Configuration
	ScraperWorkers    int
	ScraperMinDelay   time.Duration
	ScraperMaxDelay   time.Duration
	ScraperTimeout    time.Duration
	ScraperMaxRetries int

	// Warmup Configuration
	WarmupTimeout time.Duration
	WarmupModules string // Comma-separated list of modules to warmup (default: "id,contact,course,sticker")
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
		CacheTTL:   getDurationEnv("CACHE_TTL", 168*time.Hour), // 7 days

		// Scraper Configuration
		ScraperWorkers:    getIntEnv("SCRAPER_WORKERS", 3),
		ScraperMinDelay:   getDurationEnv("SCRAPER_MIN_DELAY", 5*time.Second),
		ScraperMaxDelay:   getDurationEnv("SCRAPER_MAX_DELAY", 10*time.Second),
		ScraperTimeout:    getDurationEnv("SCRAPER_TIMEOUT", 60*time.Second),
		ScraperMaxRetries: getIntEnv("SCRAPER_MAX_RETRIES", 5),

		// Warmup Configuration
		WarmupTimeout: getDurationEnv("WARMUP_TIMEOUT", 20*time.Minute),
		WarmupModules: getEnv("WARMUP_MODULES", "id,contact,course,sticker"),
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
			return fmt.Errorf("LINE_CHANNEL_ACCESS_TOKEN is required")
		}
		if c.LineChannelSecret == "" {
			return fmt.Errorf("LINE_CHANNEL_SECRET is required")
		}
		if c.Port == "" {
			return fmt.Errorf("PORT is required")
		}
	}
	if c.SQLitePath == "" {
		return fmt.Errorf("SQLITE_PATH is required")
	}
	if c.ScraperWorkers < 1 {
		return fmt.Errorf("SCRAPER_WORKERS must be at least 1")
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

// getDefaultDBPath returns platform-specific default database path
func getDefaultDBPath() string {
	if runtime.GOOS == "windows" {
		return "./data/cache.db"
	}
	return "/data/cache.db"
}
