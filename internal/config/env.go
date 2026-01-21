// Package config defines environment variable keys for configuration.
package config

//nolint:gosec,revive // Environment variable keys are not credentials and do not need per-const comments.
const (
	// Core (Required)
	EnvLineChannelAccessToken = "NTPU_LINE_CHANNEL_ACCESS_TOKEN"
	EnvLineChannelSecret      = "NTPU_LINE_CHANNEL_SECRET"

	// Server
	EnvPort            = "NTPU_PORT"
	EnvLogLevel        = "NTPU_LOG_LEVEL"
	EnvShutdownTimeout = "NTPU_SHUTDOWN_TIMEOUT"
	EnvServerName      = "NTPU_SERVER_NAME"
	EnvInstanceID      = "NTPU_INSTANCE_ID"

	// Data
	EnvDataDir  = "NTPU_DATA_DIR"
	EnvCacheTTL = "NTPU_CACHE_TTL"

	// Scraper
	EnvScraperTimeout    = "NTPU_SCRAPER_TIMEOUT"
	EnvScraperMaxRetries = "NTPU_SCRAPER_MAX_RETRIES"

	// Webhook
	EnvWebhookTimeout = "NTPU_WEBHOOK_TIMEOUT"

	// Rate Limits
	EnvGlobalRateRPS  = "NTPU_GLOBAL_RATE_RPS"
	EnvUserRateBurst  = "NTPU_USER_RATE_BURST"
	EnvUserRateRefill = "NTPU_USER_RATE_REFILL"
	EnvLLMRateBurst   = "NTPU_LLM_RATE_BURST"
	EnvLLMRateRefill  = "NTPU_LLM_RATE_REFILL"
	EnvLLMRateDaily   = "NTPU_LLM_RATE_DAILY"

	// Background Tasks
	EnvWarmupWait             = "NTPU_WARMUP_WAIT"
	EnvWarmupGracePeriod      = "NTPU_WARMUP_GRACE_PERIOD"
	EnvDataRefreshInterval    = "NTPU_DATA_REFRESH_INTERVAL"
	EnvDataCleanupInterval    = "NTPU_DATA_CLEANUP_INTERVAL"
	EnvR2SnapshotPollInterval = "NTPU_R2_SNAPSHOT_POLL_INTERVAL"

	// LLM Feature
	EnvLLMEnabled             = "NTPU_LLM_ENABLED"
	EnvLLMProviders           = "NTPU_LLM_PROVIDERS"
	EnvGeminiAPIKey           = "NTPU_GEMINI_API_KEY"
	EnvGroqAPIKey             = "NTPU_GROQ_API_KEY"
	EnvCerebrasAPIKey         = "NTPU_CEREBRAS_API_KEY"
	EnvGeminiIntentModels     = "NTPU_GEMINI_INTENT_MODELS"
	EnvGeminiExpanderModels   = "NTPU_GEMINI_EXPANDER_MODELS"
	EnvGroqIntentModels       = "NTPU_GROQ_INTENT_MODELS"
	EnvGroqExpanderModels     = "NTPU_GROQ_EXPANDER_MODELS"
	EnvCerebrasIntentModels   = "NTPU_CEREBRAS_INTENT_MODELS"
	EnvCerebrasExpanderModels = "NTPU_CEREBRAS_EXPANDER_MODELS"

	// R2 Snapshot Feature
	EnvR2Enabled         = "NTPU_R2_ENABLED"
	EnvR2AccountID       = "NTPU_R2_ACCOUNT_ID"
	EnvR2AccessKeyID     = "NTPU_R2_ACCESS_KEY_ID"
	EnvR2SecretAccessKey = "NTPU_R2_SECRET_ACCESS_KEY"
	EnvR2BucketName      = "NTPU_R2_BUCKET_NAME"
	EnvR2SnapshotKey = "NTPU_R2_SNAPSHOT_KEY"
	EnvR2LockKey     = "NTPU_R2_LOCK_KEY"
	EnvR2LockTTL     = "NTPU_R2_LOCK_TTL"
	EnvR2DeltaPrefix = "NTPU_R2_DELTA_PREFIX"
	EnvR2ScheduleKey = "NTPU_R2_SCHEDULE_KEY"

	// Sentry Feature
	EnvSentryEnabled          = "NTPU_SENTRY_ENABLED"
	EnvSentryDSN              = "NTPU_SENTRY_DSN"
	EnvSentryEnvironment      = "NTPU_SENTRY_ENVIRONMENT"
	EnvSentryRelease          = "NTPU_SENTRY_RELEASE"
	EnvSentrySampleRate       = "NTPU_SENTRY_SAMPLE_RATE"
	EnvSentryTracesSampleRate = "NTPU_SENTRY_TRACES_SAMPLE_RATE"

	// Better Stack Feature
	EnvBetterStackEnabled  = "NTPU_BETTERSTACK_ENABLED"
	EnvBetterStackToken    = "NTPU_BETTERSTACK_TOKEN"
	EnvBetterStackEndpoint = "NTPU_BETTERSTACK_ENDPOINT"

	// Metrics Auth Feature
	EnvMetricsAuthEnabled = "NTPU_METRICS_AUTH_ENABLED"
	EnvMetricsUsername    = "NTPU_METRICS_USERNAME"
	EnvMetricsPassword    = "NTPU_METRICS_PASSWORD"
)
