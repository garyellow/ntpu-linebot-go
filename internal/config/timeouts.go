// Package config provides centralized timeout constants for the application.
//
// These values are carefully tuned based on:
//   - LINE Messaging API constraints (reply token expiration, webhook timeouts)
//   - NTPU website response times (scraping delays, rate limiting)
//   - SQLite performance characteristics (WAL mode, busy timeout)
//
// # LINE API Constraints
//
// LINE webhook has specific timing requirements:
//   - Reply token: Valid for ~20 minutes, but should reply ASAP for good UX
//   - Webhook response: LINE expects quick acknowledgment (200 OK)
//   - Loading animation: Shows for up to 60 seconds, helps user wait
//
// We use 60s webhook timeout to maximize processing time:
//   - LINE loading animation shows for up to 60s
//   - Scraping time (NTPU websites can be slow)
//   - Allow sufficient time for semantic search operations
package config

import "time"

// Webhook timeouts
const (
	// WebhookProcessing is the timeout for processing a single webhook event.
	// This includes bot message handling, database queries, and potential scraping.
	//
	// Set to 60s because:
	//   - LINE loading animation shows for up to 60s
	//   - Semantic search operations may take significant time
	//   - Scraping + DB operations need ~5-15s in worst case
	//   - Maximizes available processing time within LINE's limits
	WebhookProcessing = 60 * time.Second

	// WebhookHTTPRead is the HTTP server read timeout for webhook requests.
	// Should be short since LINE sends small JSON payloads.
	WebhookHTTPRead = 10 * time.Second

	// WebhookHTTPWrite is the HTTP server write timeout.
	// Should accommodate WebhookProcessing + response serialization.
	WebhookHTTPWrite = 65 * time.Second

	// WebhookHTTPIdle is the HTTP server idle timeout for keep-alive connections.
	WebhookHTTPIdle = 120 * time.Second
)

// Scraper timeouts
const (
	// ScraperRequest is the timeout for a single HTTP request to NTPU websites.
	// NTPU websites can be slow, especially during peak hours.
	ScraperRequest = 60 * time.Second

	// ScraperRetryInitial is the initial delay before retrying a failed request.
	// Uses exponential backoff: 4s -> 8s -> 16s -> 32s -> 64s
	ScraperRetryInitial = 4 * time.Second

	// ScraperRateLimit is the minimum delay between consecutive scraping requests.
	// Prevents overwhelming NTPU servers and getting blocked.
	ScraperRateLimit = 2 * time.Second
)

// Database timeouts
const (
	// DatabaseBusyTimeout is SQLite busy_timeout pragma value.
	// Handles concurrent write contention during warmup operations.
	// Set to 30s to accommodate batch operations.
	DatabaseBusyTimeout = 30 * time.Second

	// DatabaseConnMaxLifetime is the maximum lifetime of database connections.
	// Prevents stale connections and allows connection pool refresh.
	DatabaseConnMaxLifetime = time.Hour
)

// Background job intervals
const (
	// CacheCleanupInterval is how often expired cache entries are deleted.
	CacheCleanupInterval = 12 * time.Hour

	// CacheCleanupInitialDelay is the delay before first cache cleanup.
	// Allows server to stabilize before running cleanup.
	CacheCleanupInitialDelay = 5 * time.Minute

	// StickerRefreshInterval is how often sticker URLs are refreshed.
	StickerRefreshInterval = 24 * time.Hour

	// StickerRefreshInitialDelay is the delay before first sticker refresh.
	// Allows server to stabilize before running refresh.
	StickerRefreshInitialDelay = 1 * time.Hour

	// MetricsUpdateInterval is how often cache size metrics are updated.
	MetricsUpdateInterval = 5 * time.Minute

	// RateLimiterCleanupInterval is how often inactive user rate limiters are cleaned.
	RateLimiterCleanupInterval = 5 * time.Minute
)

// Warmup timeouts
const (
	// WarmupStickerFetch is the timeout for fetching stickers from external sources.
	WarmupStickerFetch = 5 * time.Second
)

// Semantic search timeouts
const (
	// SemanticSearchTimeout is the timeout for semantic search operations.
	// This includes embedding API calls (Gemini) and vector similarity search.
	// Uses a detached context to prevent cancellation from request context
	// (e.g., when LINE server closes connection after receiving 200 OK).
	//
	// Set to 30s because:
	//   - Gemini embedding API typically responds in 1-5s
	//   - Includes retry logic with exponential backoff
	//   - Should complete well within the 60s webhook timeout
	SemanticSearchTimeout = 30 * time.Second
)

// Graceful shutdown
const (
	// GracefulShutdown is the timeout for graceful server shutdown.
	// Allows in-flight requests to complete before forceful termination.
	GracefulShutdown = 30 * time.Second
)
