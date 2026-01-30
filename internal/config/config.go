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
	// ========================================================================
	// Core Configuration (Required)
	// ========================================================================

	// LINE Bot Configuration
	LineChannelToken  string
	LineChannelSecret string

	// Server Configuration
	Port            string
	LogLevel        string
	ShutdownTimeout time.Duration
	ServerName      string
	InstanceID      string

	// Data Configuration
	DataDir  string        // Data directory for SQLite database
	CacheTTL time.Duration // TTL: absolute expiration for cache entries (default: 7 days)

	// ========================================================================
	// Bot Business Logic Configuration
	// ========================================================================

	// Bot Configuration (embedded - includes Webhook and Rate Limit settings)
	Bot BotConfig

	// Scraper Configuration
	ScraperTimeout    time.Duration
	ScraperMaxRetries int
	ScraperBaseURLs   map[string][]string

	// Maintenance Scheduling
	// NTPU_WARMUP_WAIT: if true, reject /webhook until warmup is ready (default: false)
	// NTPU_WARMUP_GRACE_PERIOD: readiness grace period for initial warmup (default: 10m)
	// NTPU_MAINTENANCE_REFRESH_INTERVAL: refresh interval (default: 24h)
	// NTPU_MAINTENANCE_CLEANUP_INTERVAL: cleanup interval (default: 24h)
	WaitForWarmup              bool          // If true, reject /webhook until warmup is ready
	WarmupGracePeriod          time.Duration // Readiness grace period for initial warmup
	MaintenanceRefreshInterval time.Duration // Interval for refresh tasks
	MaintenanceCleanupInterval time.Duration // Interval for cleanup tasks

	// ========================================================================
	// Optional Features
	// ========================================================================

	// 1. LLM Features (NLU, Query Expansion)
	// Flag: NTPU_LLM_ENABLED
	LLMEnabled   bool
	LLMProviders []string // Ordered list of LLM providers for fallback
	// Gemini
	GeminiAPIKey         string
	GeminiIntentModels   []string
	GeminiExpanderModels []string
	// Groq
	GroqAPIKey         string
	GroqIntentModels   []string
	GroqExpanderModels []string
	// Cerebras
	CerebrasAPIKey         string
	CerebrasIntentModels   []string
	CerebrasExpanderModels []string
	// OpenAI-Compatible
	OpenAIAPIKey         string
	OpenAIEndpoint       string
	OpenAIIntentModels   []string
	OpenAIExpanderModels []string

	// 2. R2 Snapshot Sync (Distributed Warmup)
	// Flag: NTPU_R2_ENABLED
	// Enables distributed snapshot synchronization for multi-node deployments.
	// Polling interval is configured via NTPU_R2_SNAPSHOT_POLL_INTERVAL (default: 15m)
	R2Enabled              bool
	R2AccountID            string        // Cloudflare Account ID
	R2AccessKeyID          string        // R2 Access Key ID
	R2SecretKey            string        // R2 Secret Access Key
	R2BucketName           string        // R2 Bucket name
	R2SnapshotKey          string        // Object key for snapshot (default: snapshots/cache.db.zst)
	R2LockKey              string        // Object key for distributed lock (default: locks/leader.json)
	R2LockTTL              time.Duration // TTL for distributed lock (default: 1h)
	R2SnapshotPollInterval time.Duration // Interval for polling R2 snapshots (default: 15m)
	R2DeltaPrefix          string        // Prefix for delta logs (default: deltas)
	R2ScheduleKey          string        // Object key for maintenance schedule state (default: schedules/maintenance.json)

	// 3. Sentry Error Tracking
	// Flag: NTPU_SENTRY_ENABLED
	SentryEnabled          bool
	SentryDSN              string  // Sentry DSN (https://TOKEN@HOST/1)
	SentryEnvironment      string  // Environment name (e.g., production, staging)
	SentryRelease          string  // Release version (optional)
	SentrySampleRate       float64 // Error sampling rate (0.0-1.0, default: 1.0)
	SentryTracesSampleRate float64 // Traces sampling rate (0.0-1.0, default: 0.0 = disabled)

	// 4. Better Stack Logging
	// Flag: NTPU_BETTERSTACK_ENABLED
	BetterStackEnabled  bool
	BetterStackToken    string
	BetterStackEndpoint string

	// 5. Metrics Authentication
	// Flag: NTPU_METRICS_AUTH_ENABLED
	MetricsAuthEnabled bool
	MetricsUsername    string // Username for /metrics endpoint Basic Auth (default: "prometheus")
	MetricsPassword    string // Password for /metrics Basic Auth
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
	MaxMessageLength    int // LINE API limit: 5000
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
		LineChannelToken:  getEnv(EnvLineChannelAccessToken, ""),
		LineChannelSecret: getEnv(EnvLineChannelSecret, ""),

		// Server Configuration
		Port:            getEnv(EnvPort, "10000"),
		LogLevel:        getEnv(EnvLogLevel, "info"),
		ShutdownTimeout: getDurationEnv(EnvShutdownTimeout, 30*time.Second),
		ServerName:      getEnv(EnvServerName, ""),
		InstanceID:      getEnv(EnvInstanceID, ""),

		// Data Configuration
		DataDir:  getEnv(EnvDataDir, getDefaultDataDir()),
		CacheTTL: getDurationEnv(EnvCacheTTL, 168*time.Hour), // 7 days

		// Bot Configuration (Webhook + Rate Limits + LINE API Constraints)
		Bot: BotConfig{
			// Webhook
			WebhookTimeout: getDurationEnv(EnvWebhookTimeout, WebhookProcessing),
			// Rate Limits - Per-User
			UserRateBurst:  getFloatEnv(EnvUserRateBurst, 15.0),
			UserRateRefill: getFloatEnv(EnvUserRateRefill, 0.1),
			// Rate Limits - Per-User LLM
			LLMRateBurst:  getFloatEnv(EnvLLMRateBurst, 60.0),
			LLMRateRefill: getFloatEnv(EnvLLMRateRefill, 30.0),
			LLMRateDaily:  getIntEnv(EnvLLMRateDaily, 180),
			// Rate Limits - Global
			GlobalRateRPS: getFloatEnv(EnvGlobalRateRPS, 100.0),
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

		// Scraper Configuration
		ScraperTimeout:    getDurationEnv(EnvScraperTimeout, ScraperRequest),
		ScraperMaxRetries: getIntEnv(EnvScraperMaxRetries, 10),
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

		// Maintenance Scheduling
		WaitForWarmup:              getBoolEnv(EnvWarmupWait, false),
		WarmupGracePeriod:          getDurationEnv(EnvWarmupGracePeriod, 10*time.Minute),
		MaintenanceRefreshInterval: getDurationEnv(EnvMaintenanceRefreshInterval, MaintenanceRefreshIntervalDefault),
		MaintenanceCleanupInterval: getDurationEnv(EnvMaintenanceCleanupInterval, MaintenanceCleanupIntervalDefault),

		// 1. LLM Features
		LLMEnabled:             getBoolEnv(EnvLLMEnabled, false),
		GeminiAPIKey:           getEnv(EnvGeminiAPIKey, ""),
		GroqAPIKey:             getEnv(EnvGroqAPIKey, ""),
		CerebrasAPIKey:         getEnv(EnvCerebrasAPIKey, ""),
		LLMProviders:           getProvidersEnv(EnvLLMProviders, []string{"gemini", "groq", "cerebras", "openai"}),
		GeminiIntentModels:     getModelsEnv(EnvGeminiIntentModels),
		GeminiExpanderModels:   getModelsEnv(EnvGeminiExpanderModels),
		GroqIntentModels:       getModelsEnv(EnvGroqIntentModels),
		GroqExpanderModels:     getModelsEnv(EnvGroqExpanderModels),
		CerebrasIntentModels:   getModelsEnv(EnvCerebrasIntentModels),
		CerebrasExpanderModels: getModelsEnv(EnvCerebrasExpanderModels),
		OpenAIAPIKey:           getEnv(EnvOpenAIAPIKey, ""),
		OpenAIEndpoint:         getEnv(EnvOpenAIEndpoint, ""),
		OpenAIIntentModels:     getModelsEnv(EnvOpenAIIntentModels),
		OpenAIExpanderModels:   getModelsEnv(EnvOpenAIExpanderModels),

		// 2. R2 Snapshot Storage
		R2Enabled:              getBoolEnv(EnvR2Enabled, false),
		R2AccountID:            getEnv(EnvR2AccountID, ""),
		R2AccessKeyID:          getEnv(EnvR2AccessKeyID, ""),
		R2SecretKey:            getEnv(EnvR2SecretAccessKey, ""),
		R2BucketName:           getEnv(EnvR2BucketName, ""),
		R2SnapshotKey:          getEnv(EnvR2SnapshotKey, "snapshots/cache.db.zst"),
		R2LockKey:              getEnv(EnvR2LockKey, "locks/leader.json"),
		R2LockTTL:              getDurationEnv(EnvR2LockTTL, time.Hour),
		R2SnapshotPollInterval: getDurationEnv(EnvR2SnapshotPollInterval, R2SnapshotPollIntervalDefault),
		R2DeltaPrefix:          getEnv(EnvR2DeltaPrefix, "deltas"),
		R2ScheduleKey:          getEnv(EnvR2ScheduleKey, "schedules/maintenance.json"),

		// 3. Sentry Error Tracking
		SentryEnabled:          getBoolEnv(EnvSentryEnabled, false),
		SentryDSN:              getEnv(EnvSentryDSN, ""),
		SentryEnvironment:      getEnv(EnvSentryEnvironment, ""),
		SentryRelease:          getEnv(EnvSentryRelease, ""),
		SentrySampleRate:       getFloatEnv(EnvSentrySampleRate, 1.0),
		SentryTracesSampleRate: getFloatEnv(EnvSentryTracesSampleRate, 0.0),

		// 4. Better Stack Logging
		BetterStackEnabled:  getBoolEnv(EnvBetterStackEnabled, false),
		BetterStackToken:    getEnv(EnvBetterStackToken, ""),
		BetterStackEndpoint: getEnv(EnvBetterStackEndpoint, ""),

		// 5. Metrics Authentication
		MetricsAuthEnabled: getBoolEnv(EnvMetricsAuthEnabled, false),
		MetricsUsername:    getEnv(EnvMetricsUsername, "prometheus"),
		MetricsPassword:    getEnv(EnvMetricsPassword, ""),
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
		errs = append(errs, errors.New("NTPU_LINE_CHANNEL_ACCESS_TOKEN is required"))
	}
	if c.LineChannelSecret == "" {
		errs = append(errs, errors.New("NTPU_LINE_CHANNEL_SECRET is required"))
	}
	if c.Port == "" {
		errs = append(errs, errors.New("NTPU_PORT is required"))
	}
	if err := c.Bot.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("bot config: %w", err))
	}
	if c.DataDir == "" {
		errs = append(errs, errors.New("NTPU_DATA_DIR is required"))
	}
	if c.CacheTTL <= 0 {
		errs = append(errs, fmt.Errorf("NTPU_CACHE_TTL must be positive, got %v", c.CacheTTL))
	}
	if c.ScraperTimeout <= 0 {
		errs = append(errs, fmt.Errorf("NTPU_SCRAPER_TIMEOUT must be positive, got %v", c.ScraperTimeout))
	}
	if c.MaintenanceRefreshInterval <= 0 {
		errs = append(errs, fmt.Errorf("NTPU_MAINTENANCE_REFRESH_INTERVAL must be positive, got %v", c.MaintenanceRefreshInterval))
	}
	if c.MaintenanceCleanupInterval <= 0 {
		errs = append(errs, fmt.Errorf("NTPU_MAINTENANCE_CLEANUP_INTERVAL must be positive, got %v", c.MaintenanceCleanupInterval))
	}

	// 1. LLM Validation (only if enabled)
	if c.IsLLMEnabled() {
		if c.GeminiAPIKey == "" && c.GroqAPIKey == "" && c.CerebrasAPIKey == "" && c.OpenAIAPIKey == "" {
			errs = append(errs, errors.New("NTPU_LLM_ENABLED=true requires at least one API key (NTPU_GEMINI_API_KEY, NTPU_GROQ_API_KEY, NTPU_CEREBRAS_API_KEY, NTPU_OPENAI_API_KEY)"))
		}
		validProviders := map[string]struct{}{"gemini": {}, "groq": {}, "cerebras": {}, "openai": {}}
		var hasSupported bool
		for _, p := range c.LLMProviders {
			if _, ok := validProviders[p]; ok {
				hasSupported = true
				continue
			}
			if p != "" {
				errs = append(errs, fmt.Errorf("unsupported NTPU_LLM_PROVIDERS entry: %q", p))
			}
		}
		if !hasSupported {
			errs = append(errs, errors.New("NTPU_LLM_PROVIDERS must include at least one of: gemini, groq, cerebras, openai"))
		}
		// OpenAI-compatible endpoint requires both API key and endpoint
		if c.OpenAIAPIKey != "" && c.OpenAIEndpoint == "" {
			errs = append(errs, errors.New("NTPU_OPENAI_ENDPOINT is required when NTPU_OPENAI_API_KEY is set"))
		}
		if c.OpenAIEndpoint != "" && c.OpenAIAPIKey == "" {
			errs = append(errs, errors.New("NTPU_OPENAI_API_KEY is required when NTPU_OPENAI_ENDPOINT is set"))
		}
		if c.OpenAIEndpoint != "" && !strings.HasPrefix(c.OpenAIEndpoint, "http://") && !strings.HasPrefix(c.OpenAIEndpoint, "https://") {
			errs = append(errs, fmt.Errorf("NTPU_OPENAI_ENDPOINT must start with http:// or https://, got %q", c.OpenAIEndpoint))
		}
		if c.OpenAIAPIKey != "" && c.OpenAIEndpoint != "" {
			var hasOpenAIProvider bool
			for _, p := range c.LLMProviders {
				if p == "openai" {
					hasOpenAIProvider = true
					break
				}
			}
			if hasOpenAIProvider && len(c.OpenAIIntentModels) == 0 && len(c.OpenAIExpanderModels) == 0 {
				errs = append(errs, errors.New("NTPU_OPENAI_INTENT_MODELS or NTPU_OPENAI_EXPANDER_MODELS is required when OpenAI provider is enabled"))
			}
		}
	}

	// 2. R2 Validation (only if enabled)
	if c.IsR2Enabled() {
		if c.R2AccountID == "" {
			errs = append(errs, errors.New("NTPU_R2_ACCOUNT_ID is required when NTPU_R2_ENABLED=true"))
		}
		if c.R2AccessKeyID == "" {
			errs = append(errs, errors.New("NTPU_R2_ACCESS_KEY_ID is required when NTPU_R2_ENABLED=true"))
		}
		if c.R2SecretKey == "" {
			errs = append(errs, errors.New("NTPU_R2_SECRET_ACCESS_KEY is required when NTPU_R2_ENABLED=true"))
		}
		if c.R2BucketName == "" {
			errs = append(errs, errors.New("NTPU_R2_BUCKET_NAME is required when NTPU_R2_ENABLED=true"))
		}
		if c.R2SnapshotKey == "" {
			errs = append(errs, errors.New("NTPU_R2_SNAPSHOT_KEY must not be empty when NTPU_R2_ENABLED=true"))
		}
		if c.R2LockKey == "" {
			errs = append(errs, errors.New("NTPU_R2_LOCK_KEY must not be empty when NTPU_R2_ENABLED=true"))
		}
		if c.R2LockTTL <= 0 {
			errs = append(errs, fmt.Errorf("NTPU_R2_LOCK_TTL must be positive, got %v", c.R2LockTTL))
		}
		if c.R2SnapshotPollInterval <= 0 {
			errs = append(errs, fmt.Errorf("NTPU_R2_SNAPSHOT_POLL_INTERVAL must be positive, got %v", c.R2SnapshotPollInterval))
		}
		if c.R2DeltaPrefix == "" {
			errs = append(errs, errors.New("NTPU_R2_DELTA_PREFIX must not be empty when NTPU_R2_ENABLED=true"))
		}
		if c.R2ScheduleKey == "" {
			errs = append(errs, errors.New("NTPU_R2_SCHEDULE_KEY must not be empty when NTPU_R2_ENABLED=true"))
		}
	}

	// 3. Sentry Validation (only if enabled)
	if c.IsSentryEnabled() {
		if c.SentryDSN == "" {
			errs = append(errs, errors.New("NTPU_SENTRY_DSN is required when NTPU_SENTRY_ENABLED=true"))
		}
		if c.SentrySampleRate < 0 || c.SentrySampleRate > 1 {
			errs = append(errs, fmt.Errorf("NTPU_SENTRY_SAMPLE_RATE must be between 0 and 1, got %v", c.SentrySampleRate))
		}
		if c.SentryTracesSampleRate < 0 || c.SentryTracesSampleRate > 1 {
			errs = append(errs, fmt.Errorf("NTPU_SENTRY_TRACES_SAMPLE_RATE must be between 0 and 1, got %v", c.SentryTracesSampleRate))
		}
	}

	// 4. Better Stack Validation (only if enabled)
	if c.IsBetterStackEnabled() {
		if c.BetterStackToken == "" {
			errs = append(errs, errors.New("NTPU_BETTERSTACK_TOKEN is required when NTPU_BETTERSTACK_ENABLED=true"))
		}
	}

	// 5. Metrics Validation (only if enabled)
	if c.IsMetricsAuthEnabled() {
		if c.MetricsPassword == "" {
			errs = append(errs, errors.New("NTPU_METRICS_PASSWORD is required when NTPU_METRICS_AUTH_ENABLED=true"))
		}
		if strings.TrimSpace(c.MetricsUsername) == "" {
			errs = append(errs, errors.New("NTPU_METRICS_USERNAME is required when NTPU_METRICS_AUTH_ENABLED=true"))
		}
	}

	// Scraper internal validation
	if c.ScraperMaxRetries < 0 {
		errs = append(errs, fmt.Errorf("NTPU_SCRAPER_MAX_RETRIES cannot be negative, got %d", c.ScraperMaxRetries))
	}
	if c.WarmupGracePeriod <= 0 {
		errs = append(errs, fmt.Errorf("NTPU_WARMUP_GRACE_PERIOD must be positive, got %v", c.WarmupGracePeriod))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Feature Enablement Checks (Unified Pattern)
// ----------------------------------------------------------------------------

// IsLLMEnabled returns true if LLM features are enabled.
func (c *Config) IsLLMEnabled() bool {
	return c.LLMEnabled
}

// IsR2Enabled returns true if R2 snapshot storage is enabled.
func (c *Config) IsR2Enabled() bool {
	return c.R2Enabled
}

// IsSentryEnabled returns true if Sentry error tracking is enabled.
func (c *Config) IsSentryEnabled() bool {
	return c.SentryEnabled
}

// IsBetterStackEnabled returns true if Better Stack logging is enabled.
func (c *Config) IsBetterStackEnabled() bool {
	return c.BetterStackEnabled
}

// IsMetricsAuthEnabled returns true if Basic Auth is enabled for metrics endpoint.
func (c *Config) IsMetricsAuthEnabled() bool {
	return c.MetricsAuthEnabled
}

// ----------------------------------------------------------------------------
// Helper Methods
// ----------------------------------------------------------------------------

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
			result = append(result, strings.ToLower(trimmed))
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

// R2Endpoint returns the R2 S3-compatible endpoint URL.
func (c *Config) R2Endpoint() string {
	if c.R2AccountID == "" {
		return ""
	}
	return "https://" + c.R2AccountID + ".r2.cloudflarestorage.com"
}
