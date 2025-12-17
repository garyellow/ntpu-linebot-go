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
//   - Allow sufficient time for smart search operations
package config

import "time"

// Webhook timeouts
const (
	// WebhookProcessing is the timeout for processing a single webhook event.
	// This includes bot message handling, database queries, and potential scraping.
	//
	// Set to 60s because:
	//   - LINE loading animation shows for up to 60s
	//   - Smart search operations may take significant time
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
	// CacheCleanupInterval is how often cache cleanup runs (daily).
	// Cleanup runs at fixed time (4:00 AM Taiwan time) after warmup.
	CacheCleanupInterval = 24 * time.Hour

	// CacheCleanupHour is the hour (0-23) when cache cleanup runs daily.
	// Set to 4:00 AM Taiwan time, after warmup completes to avoid deleting fresh data.
	CacheCleanupHour = 4

	// WarmupHour is the hour (0-23) when daily warmup runs.
	// Set to 3:00 AM Taiwan time for fresh cache before business hours.
	WarmupHour = 3

	// MetricsUpdateInterval is how often cache size metrics are updated.
	// Uses Ticker pattern as it's high-frequency monitoring task.
	MetricsUpdateInterval = 5 * time.Minute

	// RateLimiterCleanupInterval is how often inactive user rate limiters are cleaned.
	// Uses Ticker pattern as it's high-frequency cleanup task.
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
