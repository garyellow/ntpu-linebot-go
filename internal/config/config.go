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
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// LINE Bot Configuration (Required)
	LineChannelToken  string
	LineChannelSecret string

	// LLM API Keys (Optional - at least one required for NLU features)
	GeminiAPIKey   string
	GroqAPIKey     string
	CerebrasAPIKey string

	// LLM Provider Configuration
	LLMProviders []string // Ordered list of LLM providers for fallback (default: "gemini,groq,cerebras")

	// LLM Model Configuration (optional, defaults apply if empty)
	// Each slice contains models in fallback order: first is primary, rest are fallbacks
	GeminiIntentModels     []string
	GeminiExpanderModels   []string
	GroqIntentModels       []string
	GroqExpanderModels     []string
	CerebrasIntentModels   []string
	CerebrasExpanderModels []string

	// Server Configuration
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration

	// Better Stack Logging (Optional)
	BetterStackToken    string
	BetterStackEndpoint string

	// Sentry Error Tracking (Optional - via Better Stack)
	SentryDSN              string  // Better Stack Sentry DSN (https://TOKEN@HOST/1)
	SentryEnvironment      string  // Environment name (e.g., production, staging)
	SentryRelease          string  // Release version (optional)
	SentrySampleRate       float64 // Error sampling rate (0.0-1.0, default: 1.0)
	SentryTracesSampleRate float64 // Traces sampling rate (0.0-1.0, default: 0.0 = disabled)

	// Data Configuration
	DataDir  string        // Data directory for SQLite database
	CacheTTL time.Duration // TTL: absolute expiration for cache entries (default: 7 days)

	// Scraper Configuration
	ScraperTimeout    time.Duration
	ScraperMaxRetries int
	ScraperBaseURLs   map[string][]string

	// Bot Configuration (embedded - includes Webhook and Rate Limit settings)
	Bot BotConfig

	// Startup Configuration
	WaitForWarmup     bool          // If true, wait for warmup completion before accepting traffic (default: false)
	WarmupGracePeriod time.Duration // Grace period to wait for warmup when WaitForWarmup=true (default: 10m)

	// Metrics Authentication
	MetricsUsername string // Username for /metrics endpoint Basic Auth (default: "prometheus")
	MetricsPassword string // Password for /metrics endpoint Basic Auth (empty = no auth)
}

// BotConfig holds bot-specific configuration (Webhook, Rate Limits, LINE API Constraints)
type BotConfig struct {
	// Webhook Configuration
	WebhookTimeout time.Duration // Timeout for webhook bot processing (default: 60s)

	// Rate Limits - Per-User (Token Bucket Algorithm)
	UserRateBurst  float64 // Burst capacity (default: 15)
	UserRateRefill float64 // Refill rate per second (default: 0.1 = 1 per 10s)

	// Rate Limits - Per-User LLM (Multi-Layer: Hourly + Daily)
	LLMRateBurst  float64 // Burst capacity for LLM (default: 60)
	LLMRateRefill float64 // Refill rate per hour (default: 30)
	LLMRateDaily  int     // Daily limit (default: 180, 0 = disabled)

	// Rate Limits - Global
	GlobalRateRPS float64 // Global rate limit in RPS (default: 100)

	// LINE API Constraints (hard-coded, not configurable)
	MaxMessagesPerReply int // LINE API limit: 5
	MaxEventsPerWebhook int // Default: 100
	MinReplyTokenLength int // Default: 10
	MaxMessageLength    int // LINE API limit: 20000
	MaxPostbackDataSize int // LINE API limit: 300

	// Business Limits (hard-coded, not configurable)
	MaxCoursesPerSearch  int // Default: 40
	MaxStudentsPerSearch int // Default: 400
	MaxContactsPerSearch int // Default: 100
	ValidYearStart       int // Default: 95
	ValidYearEnd         int // Default: 112
}

// Load reads configuration from environment variables
// It attempts to load .env file first, then reads from env vars
func Load() (*Config, error) {
	// Try to load .env file (ignore error if file doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		// LINE Bot Configuration (Required)
		LineChannelToken:  getEnv("LINE_CHANNEL_ACCESS_TOKEN", ""),
		LineChannelSecret: getEnv("LINE_CHANNEL_SECRET", ""),

		// LLM API Keys
		GeminiAPIKey:   getEnv("GEMINI_API_KEY", ""),
		GroqAPIKey:     getEnv("GROQ_API_KEY", ""),
		CerebrasAPIKey: getEnv("CEREBRAS_API_KEY", ""),

		// LLM Provider Configuration
		LLMProviders: getProvidersEnv("LLM_PROVIDERS", []string{"gemini", "groq", "cerebras"}),

		// LLM Model Configuration (empty = use defaults from genai package)
		GeminiIntentModels:     getModelsEnv("GEMINI_INTENT_MODELS"),
		GeminiExpanderModels:   getModelsEnv("GEMINI_EXPANDER_MODELS"),
		GroqIntentModels:       getModelsEnv("GROQ_INTENT_MODELS"),
		GroqExpanderModels:     getModelsEnv("GROQ_EXPANDER_MODELS"),
		CerebrasIntentModels:   getModelsEnv("CEREBRAS_INTENT_MODELS"),
		CerebrasExpanderModels: getModelsEnv("CEREBRAS_EXPANDER_MODELS"),

		// Server Configuration
		Port:            getEnv("PORT", "10000"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),

		// Better Stack Logging (Optional)
		BetterStackToken:    getEnv("BETTERSTACK_SOURCE_TOKEN", ""),
		BetterStackEndpoint: getEnv("BETTERSTACK_ENDPOINT", ""),

		// Sentry Error Tracking (Optional - via Better Stack)
		SentryDSN:              getEnv("SENTRY_DSN", ""),
		SentryEnvironment:      getEnv("SENTRY_ENVIRONMENT", ""),
		SentryRelease:          getEnv("SENTRY_RELEASE", ""),
		SentrySampleRate:       getFloatEnv("SENTRY_SAMPLE_RATE", 1.0),
		SentryTracesSampleRate: getFloatEnv("SENTRY_TRACES_SAMPLE_RATE", 0.0),

		// Data Configuration
		DataDir:  getEnv("DATA_DIR", getDefaultDataDir()),
		CacheTTL: getDurationEnv("CACHE_TTL", 168*time.Hour), // 7 days

		// Scraper Configuration
		ScraperTimeout:    getDurationEnv("SCRAPER_TIMEOUT", ScraperRequest),
		ScraperMaxRetries: getIntEnv("SCRAPER_MAX_RETRIES", 10),
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

		// Bot Configuration (Webhook + Rate Limits + LINE API Constraints)
		Bot: BotConfig{
			// Webhook
			WebhookTimeout: getDurationEnv("WEBHOOK_TIMEOUT", WebhookProcessing),
			// Rate Limits - Per-User
			UserRateBurst:  getFloatEnv("USER_RATE_BURST", 15.0),
			UserRateRefill: getFloatEnv("USER_RATE_REFILL", 0.1),
			// Rate Limits - Per-User LLM
			LLMRateBurst:  getFloatEnv("LLM_RATE_BURST", 60.0),
			LLMRateRefill: getFloatEnv("LLM_RATE_REFILL", 30.0),
			LLMRateDaily:  getIntEnv("LLM_RATE_DAILY", 180),
			// Rate Limits - Global
			GlobalRateRPS: getFloatEnv("GLOBAL_RATE_RPS", 100.0),
			// LINE API Constraints (hard-coded)
			MaxMessagesPerReply: LINEMaxMessagesPerReply,
			MaxEventsPerWebhook: 100,
			MinReplyTokenLength: 10,
			MaxMessageLength:    LINEMaxTextMessageLength,
			MaxPostbackDataSize: LINEMaxPostbackDataLength,
			// Business Limits (hard-coded)
			MaxCoursesPerSearch:  40,
			MaxStudentsPerSearch: 400,
			MaxContactsPerSearch: 100,
			ValidYearStart:       95,
			ValidYearEnd:         112,
		},

		// Startup Configuration
		WaitForWarmup:     getBoolEnv("WAIT_FOR_WARMUP", false),
		WarmupGracePeriod: getDurationEnv("WARMUP_GRACE_PERIOD", 10*time.Minute),

		// Metrics Authentication
		MetricsUsername: getEnv("METRICS_USERNAME", "prometheus"),
		MetricsPassword: getEnv("METRICS_PASSWORD", ""),
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
	if c.SentrySampleRate < 0 || c.SentrySampleRate > 1 {
		errs = append(errs, fmt.Errorf("SENTRY_SAMPLE_RATE must be between 0 and 1, got %v", c.SentrySampleRate))
	}
	if c.SentryTracesSampleRate < 0 || c.SentryTracesSampleRate > 1 {
		errs = append(errs, fmt.Errorf("SENTRY_TRACES_SAMPLE_RATE must be between 0 and 1, got %v", c.SentryTracesSampleRate))
	}
	if c.ScraperMaxRetries < 0 {
		errs = append(errs, fmt.Errorf("SCRAPER_MAX_RETRIES cannot be negative, got %d", c.ScraperMaxRetries))
	}
	if c.WaitForWarmup && c.WarmupGracePeriod <= 0 {
		errs = append(errs, fmt.Errorf("WARMUP_GRACE_PERIOD must be positive when WAIT_FOR_WARMUP is enabled, got %v", c.WarmupGracePeriod))
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

// getBoolEnv retrieves boolean environment variable with fallback to default value.
// Accepts "true", "1", "yes" (case-insensitive) as true values.
// Accepts "false", "0", "no" (case-insensitive) as false values.
// Returns defaultValue for empty or unrecognized values.
func getBoolEnv(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	lower := strings.ToLower(value)
	switch lower {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return defaultValue
	}
}

// getModelsEnv parses comma-separated model list from environment variable.
// Returns nil if the environment variable is not set or empty.
// Leading/trailing whitespace is trimmed from each model name.
func getModelsEnv(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	models := strings.Split(value, ",")
	result := make([]string, 0, len(models))
	for _, m := range models {
		if trimmed := strings.TrimSpace(m); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// getProvidersEnv parses comma-separated provider list from environment variable.
// Returns defaultValue if the environment variable is not set or empty.
// Leading/trailing whitespace is trimmed from each provider name.
func getProvidersEnv(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	providers := strings.Split(value, ",")
	result := make([]string, 0, len(providers))
	for _, p := range providers {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
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
	return c.GeminiAPIKey != "" || c.GroqAPIKey != "" || c.CerebrasAPIKey != ""
}

// HasSentry returns true if Sentry error tracking is configured.
func (c *Config) HasSentry() bool {
	return c.SentryDSN != ""
}
