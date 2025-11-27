// Package timeouts provides centralized timeout constants for the application.
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
// We use 25s webhook timeout to balance:
//   - User experience (not too long to wait)
//   - Scraping time (NTPU websites can be slow)
//   - Safety margin before LINE might retry
package timeouts

import "time"

// Webhook timeouts
const (
	// WebhookProcessing is the timeout for processing a single webhook event.
	// This includes bot message handling, database queries, and potential scraping.
	//
	// Set to 25s because:
	//   - LINE loading animation shows for up to 60s
	//   - User patience is typically 10-30 seconds
	//   - Scraping + DB operations need ~5-15s in worst case
	//   - Leaves margin for retries and error handling
	WebhookProcessing = 25 * time.Second

	// WebhookHTTPRead is the HTTP server read timeout for webhook requests.
	// Should be short since LINE sends small JSON payloads.
	WebhookHTTPRead = 10 * time.Second

	// WebhookHTTPWrite is the HTTP server write timeout.
	// Should accommodate WebhookProcessing + response serialization.
	WebhookHTTPWrite = 30 * time.Second

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

	// StickerRefreshInterval is how often sticker URLs are refreshed.
	StickerRefreshInterval = 24 * time.Hour

	// MetricsUpdateInterval is how often cache size metrics are updated.
	MetricsUpdateInterval = 5 * time.Minute

	// RateLimiterCleanupInterval is how often inactive user rate limiters are cleaned.
	RateLimiterCleanupInterval = 5 * time.Minute
)

// Warmup timeouts
const (
	// WarmupDefault is the default timeout for the entire warmup process.
	WarmupDefault = 30 * time.Minute

	// WarmupStickerFetch is the timeout for fetching stickers from external sources.
	WarmupStickerFetch = 5 * time.Second
)

// Graceful shutdown
const (
	// GracefulShutdown is the timeout for graceful server shutdown.
	// Allows in-flight requests to complete before forceful termination.
	GracefulShutdown = 30 * time.Second
)
