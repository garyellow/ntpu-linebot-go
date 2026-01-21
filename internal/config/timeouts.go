// Package config provides centralized timeout and interval constants.
// Values are tuned for LINE Messaging API constraints, NTPU website response times,
// and SQLite performance characteristics.
package config

import "time"

// Webhook timeouts
const (
	// WebhookProcessing is the timeout for processing a single webhook event.
	// Set to 60s to match LINE loading animation duration.
	WebhookProcessing = 60 * time.Second

	// WebhookHTTPRead is the HTTP server read timeout for webhook requests.
	WebhookHTTPRead = 10 * time.Second

	// WebhookHTTPWrite is the HTTP server write timeout.
	// Must exceed WebhookProcessing to allow full event processing.
	WebhookHTTPWrite = 65 * time.Second

	// WebhookHTTPIdle is the HTTP server idle timeout for keep-alive connections.
	WebhookHTTPIdle = 120 * time.Second
)

// Sentry timeouts
const (
	// SentryHTTPTimeout is the timeout for sending events to Sentry.
	SentryHTTPTimeout = 5 * time.Second

	// SentryFlushTimeout is the timeout for flushing buffered Sentry events on shutdown.
	SentryFlushTimeout = 5 * time.Second
)

// Scraper timeouts
const (
	// ScraperRequest is the timeout for a single HTTP request to NTPU websites.
	ScraperRequest = 60 * time.Second

	// ScraperRetryInitial is the initial delay for exponential backoff (4s -> 8s -> 16s -> 32s -> 64s).
	ScraperRetryInitial = 4 * time.Second

	// ScraperRateLimit is the minimum delay between consecutive requests.
	ScraperRateLimit = 2 * time.Second
)

// Database timeouts
const (
	// DatabaseBusyTimeout is SQLite busy_timeout pragma value for concurrent write contention.
	DatabaseBusyTimeout = 30 * time.Second

	// DatabaseConnMaxLifetime is the maximum lifetime of database connections.
	DatabaseConnMaxLifetime = time.Hour

	// HotSwapCloseGracePeriod is the delay before closing old SQLite connections
	// after a hot-swap, giving in-flight queries time to finish.
	HotSwapCloseGracePeriod = 5 * time.Second
)

// R2 timeouts
const (
	// R2RequestTimeout is the timeout for a single R2 request.
	R2RequestTimeout = 60 * time.Second
)

// Background job intervals
const (
	// DataRefreshIntervalDefault is the default interval for data refresh tasks.
	DataRefreshIntervalDefault = 24 * time.Hour

	// DataCleanupIntervalDefault is the default interval for data cleanup tasks.
	DataCleanupIntervalDefault = 24 * time.Hour

	// R2SnapshotPollIntervalDefault is the default interval for polling R2 snapshots.
	R2SnapshotPollIntervalDefault = 15 * time.Minute

	// MetricsUpdateInterval is how often cache size metrics are updated.
	MetricsUpdateInterval = 5 * time.Minute

	// RateLimiterCleanupInterval is how often inactive user rate limiters are cleaned.
	RateLimiterCleanupInterval = 5 * time.Minute
)

// Warmup timeouts
const (
	// WarmupStickerFetch is the timeout for fetching stickers from external sources.
	WarmupStickerFetch = 5 * time.Second

	// WarmupProactive is the timeout for proactive cache warmup operations.
	// Warmup involves concurrent scraping of multiple data sources:
	//   - Students: ~252 departments × 12 years (~40 departments/10min observed)
	//   - Courses: 4 semesters (4 scraping operations, each with U/M/N/P codes)
	//   - Contacts: Single organization scrape
	//   - Syllabi: Hash-based incremental updates (~2000 courses, 243 processed/10min)
	//
	// Set to 2 hours to accommodate:
	//   - Network latency to NTPU servers
	//   - Rate limiting delays (2s per request)
	//   - Concurrent scraping with exponential backoff on failures
	//   - Full student database scraping (252 departments × ~15s/dept ≈ 63 min)
	//   - Syllabus scraping (2000 courses with hash-based incremental updates)
	WarmupProactive = 2 * time.Hour
)

// Smart search timeouts
const (
	// SmartSearchTimeout is the timeout for smart search operations.
	// This includes BM25 search and Query Expansion (Gemini API call).
	// Uses a detached context to prevent cancellation from request context
	// (e.g., when LINE server closes connection after receiving 200 OK).
	//
	// Set to 30s because:
	//   - Query Expansion API typically responds in 1-5s
	//   - BM25 search is in-memory and very fast (<10ms)
	//   - Should complete well within the 60s webhook timeout
	SmartSearchTimeout = 30 * time.Second

	// ReadinessCheckTimeout is the timeout for readiness probe checks.
	// Set to 3s to allow SQLite ping operations to complete while maintaining
	// fast probe responses for Kubernetes orchestration.
	ReadinessCheckTimeout = 3 * time.Second

	// ReadinessWarmupTimeout is the default grace period for initial warmup.
	// This is the default value for the NTPU_WARMUP_GRACE_PERIOD environment variable.
	// After this duration, readiness will return OK even if warmup is still running.
	// Set to 10 minutes based on typical warmup duration (contact + course: ~2-5 min).
	ReadinessWarmupTimeout = 10 * time.Minute
)

// Graceful shutdown
const (
	// GracefulShutdown is the timeout for graceful server shutdown.
	// Allows in-flight requests to complete before forceful termination.
	//
	// Set to 70s with the following considerations:
	//   - Webhook requests: up to 60s processing time (WebhookProcessing)
	//   - Background jobs: should exit quickly after context cancellation
	//   - Safety margin: 10s buffer for cleanup and resource closure
	//
	// Shutdown sequence:
	//   1. Stop accepting new HTTP requests (immediate)
	//   2. Wait for in-flight requests (webhook: max 60s, but most complete within 10s)
	//   3. Cancel background job contexts (immediate)
	//   4. Wait for background jobs to exit (typically < 1s)
	//   5. Close resources (DB, API clients - typically < 1s)
	//
	// In practice, most shutdowns complete within 10-15s.
	// The 70s timeout ensures worst-case webhook requests can complete.
	GracefulShutdown = 70 * time.Second
)
