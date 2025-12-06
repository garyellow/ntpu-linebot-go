// Package metrics provides Prometheus metrics for monitoring.
//
// Design Philosophy:
// - RED Method for services: Rate, Errors, Duration
// - USE Method for resources: Utilization, Saturation, Errors
// - Custom registry to avoid global state conflicts
// - Consistent naming: ntpu_{component}_{metric}_{unit}
// - Low cardinality labels (avoid high-cardinality values)
// - Histogram buckets aligned with SLO targets
// - Focus on actionable observability over vanity metrics
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the NTPU LineBot.
// Organized by component following the RED/USE methodology.
type Metrics struct {
	registry *prometheus.Registry

	// ============================================
	// Webhook (LINE Bot Core - RED Method)
	// Primary service entry point
	// ============================================
	// Rate: requests per second by event type
	// Errors: tracked via status label (success/error/rate_limited)
	// Duration: processing time from receive to reply
	WebhookTotal    *prometheus.CounterVec
	WebhookDuration *prometheus.HistogramVec

	// ============================================
	// Scraper (External HTTP Calls - RED Method)
	// Calls to NTPU LMS/SEA systems
	// ============================================
	ScraperTotal    *prometheus.CounterVec
	ScraperDuration *prometheus.HistogramVec

	// ============================================
	// Cache (SQLite - USE Method)
	// Local data cache layer
	// ============================================
	CacheOperations *prometheus.CounterVec // hit/miss by module
	CacheSize       *prometheus.GaugeVec   // current entries by module

	// ============================================
	// LLM (Gemini API - RED Method)
	// NLU intent parsing, Query Expansion
	// ============================================
	LLMTotal    *prometheus.CounterVec   // requests by operation and status
	LLMDuration *prometheus.HistogramVec // latency by operation

	// ============================================
	// Smart Search (BM25 - RED Method)
	// Smart course search
	// ============================================
	SearchTotal    *prometheus.CounterVec
	SearchDuration *prometheus.HistogramVec
	SearchResults  *prometheus.HistogramVec // result count distribution

	// Index sizes (Gauges - point-in-time values)
	IndexSize *prometheus.GaugeVec // documents in BM25 index

	// ============================================
	// Rate Limiter (USE Method)
	// Request throttling
	// ============================================
	RateLimiterDropped  *prometheus.CounterVec
	RateLimiterUsers    prometheus.Gauge // active user limiters
	LLMRateLimiterUsers prometheus.Gauge // active LLM rate limiters

	// ============================================
	// Background Jobs (Duration only)
	// Warmup, Cleanup operations
	// ============================================
	JobDuration *prometheus.HistogramVec
}

// New creates a new Metrics instance with all metrics registered.
// The caller should register Go/Process/BuildInfo collectors separately
// to avoid duplicate registration issues.
func New(registry *prometheus.Registry) *Metrics {
	m := &Metrics{
		registry: registry,

		// ============================================
		// Webhook metrics
		// ============================================
		WebhookTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_webhook_total",
				Help: "Total webhook events processed",
			},
			// event_type: message, postback, follow
			// status: success, error, rate_limited
			[]string{"event_type", "status"},
		),

		WebhookDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_webhook_duration_seconds",
				Help: "Webhook processing duration in seconds",
				// Buckets aligned with LINE API expectations:
				// < 2s: excellent (LINE best practice)
				// 2-5s: acceptable
				// > 5s: degraded experience
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
			},
			[]string{"event_type"},
		),

		// ============================================
		// Scraper metrics
		// ============================================
		ScraperTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_scraper_total",
				Help: "Total scraper requests to NTPU systems",
			},
			// module: id, contact, course, syllabus
			// status: success, error, timeout, not_found, smart_fallback
			[]string{"module", "status"},
		),

		ScraperDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_scraper_duration_seconds",
				Help: "Scraper request duration in seconds",
				// Buckets for external HTTP calls:
				// Most should complete in 2-10s
				// Max configured timeout is 120s
				Buckets: []float64{1, 2, 5, 10, 20, 30, 60, 120},
			},
			[]string{"module"},
		),

		// ============================================
		// Cache metrics
		// ============================================
		CacheOperations: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_cache_operations_total",
				Help: "Total cache operations (hits and misses)",
			},
			// module: students, contacts, courses, syllabi, stickers
			// result: hit, miss
			[]string{"module", "result"},
		),

		CacheSize: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ntpu_cache_size",
				Help: "Current number of entries in cache",
			},
			[]string{"module"},
		),

		// ============================================
		// LLM metrics
		// ============================================
		LLMTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_llm_total",
				Help: "Total LLM API requests",
			},
			// operation: nlu (intent parsing)
			// status: success, error, fallback, clarification
			[]string{"operation", "status"},
		),

		LLMDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_llm_duration_seconds",
				Help: "LLM API request duration in seconds",
				// Buckets for Gemini API latency:
				// Fast: < 0.5s (simple queries)
				// Normal: 0.5-2s (typical)
				// Slow: > 2s (complex or retry)
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
			},
			[]string{"operation"},
		),

		// ============================================
		// Smart Search metrics
		// ============================================
		SearchTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_search_total",
				Help: "Total smart search requests",
			},
			// type: bm25, disabled
			// status: success, error, no_results
			[]string{"type", "status"},
		),

		SearchDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_search_duration_seconds",
				Help: "Search operation duration in seconds",
				// Buckets for search latency:
				// BM25: < 50ms (in-memory)
				// Query Expansion: 1-5s (Gemini API)
				Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
			},
			// type: bm25, disabled
			[]string{"type"},
		),

		SearchResults: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ntpu_search_results",
				Help:    "Number of results returned by search",
				Buckets: []float64{0, 1, 5, 10, 20, 40},
			},
			[]string{"type"},
		),

		IndexSize: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ntpu_index_size",
				Help: "Number of documents in search indexes",
			},
			// index: bm25
			[]string{"index"},
		),

		// ============================================
		// Rate Limiter metrics
		// ============================================
		RateLimiterDropped: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_rate_limiter_dropped_total",
				Help: "Total requests dropped by rate limiter",
			},
			// limiter: user, global
			[]string{"limiter"},
		),

		RateLimiterUsers: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "ntpu_rate_limiter_users",
				Help: "Current number of tracked user rate limiters",
			},
		),

		LLMRateLimiterUsers: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "ntpu_llm_rate_limiter_users",
				Help: "Current number of tracked LLM rate limiters",
			},
		),

		// ============================================
		// Background Job metrics
		// ============================================
		JobDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_job_duration_seconds",
				Help: "Background job duration in seconds",
				// Jobs can run for minutes (warmup) to seconds (cleanup)
				Buckets: []float64{1, 10, 30, 60, 120, 300, 600, 1800},
			},
			// job: warmup, cleanup
			// module: id, contact, course, syllabus, total (for warmup)
			[]string{"job", "module"},
		),
	}

	return m
}

// ============================================
// Webhook helpers
// ============================================

// RecordWebhook records a webhook event.
// eventType: message, postback, follow
// status: success, error, rate_limited
func (m *Metrics) RecordWebhook(eventType, status string, duration float64) {
	m.WebhookTotal.WithLabelValues(eventType, status).Inc()
	m.WebhookDuration.WithLabelValues(eventType).Observe(duration)
}

// ============================================
// Scraper helpers
// ============================================

// RecordScraper records a scraper request.
// module: id, contact, course, syllabus
// status: success, error, timeout
func (m *Metrics) RecordScraper(module, status string, duration float64) {
	m.ScraperTotal.WithLabelValues(module, status).Inc()
	m.ScraperDuration.WithLabelValues(module).Observe(duration)
}

// ============================================
// Cache helpers
// ============================================

// RecordCacheHit records a cache hit.
func (m *Metrics) RecordCacheHit(module string) {
	m.CacheOperations.WithLabelValues(module, "hit").Inc()
}

// RecordCacheMiss records a cache miss.
func (m *Metrics) RecordCacheMiss(module string) {
	m.CacheOperations.WithLabelValues(module, "miss").Inc()
}

// SetCacheSize sets the current cache size for a module.
func (m *Metrics) SetCacheSize(module string, size int) {
	m.CacheSize.WithLabelValues(module).Set(float64(size))
}

// ============================================
// LLM helpers
// ============================================

// RecordLLM records an LLM API request.
// operation: nlu (primary user-facing intent parsing)
// status: success, error, fallback, clarification
func (m *Metrics) RecordLLM(operation, status string, duration float64) {
	m.LLMTotal.WithLabelValues(operation, status).Inc()
	m.LLMDuration.WithLabelValues(operation).Observe(duration)
}

// ============================================
// Search helpers
// ============================================

// RecordSearch records a search operation.
// searchType: bm25, disabled
// status: success, error, no_results, skipped
func (m *Metrics) RecordSearch(searchType, status string, duration float64, resultCount int) {
	m.SearchTotal.WithLabelValues(searchType, status).Inc()
	m.SearchDuration.WithLabelValues(searchType).Observe(duration)
	m.SearchResults.WithLabelValues(searchType).Observe(float64(resultCount))
}

// SetIndexSize sets the current index size.
// index: bm25
func (m *Metrics) SetIndexSize(index string, count int) {
	m.IndexSize.WithLabelValues(index).Set(float64(count))
}

// ============================================
// Rate Limiter helpers
// ============================================

// RecordRateLimiterDrop records a dropped request.
// limiter: user, global, llm
func (m *Metrics) RecordRateLimiterDrop(limiter string) {
	m.RateLimiterDropped.WithLabelValues(limiter).Inc()
}

// SetRateLimiterUsers sets the current number of active user limiters.
func (m *Metrics) SetRateLimiterUsers(count int) {
	m.RateLimiterUsers.Set(float64(count))
}

// SetLLMRateLimiterUsers sets the current number of active LLM rate limiters.
func (m *Metrics) SetLLMRateLimiterUsers(count int) {
	m.LLMRateLimiterUsers.Set(float64(count))
}

// ============================================
// Job helpers
// ============================================

// RecordJob records a background job execution.
// job: warmup, cleanup
// module: id, contact, course, syllabus, total
func (m *Metrics) RecordJob(job, module string, duration float64) {
	m.JobDuration.WithLabelValues(job, module).Observe(duration)
}

// ============================================
// Registry access
// ============================================

// Registry returns the custom Prometheus registry.
// Use with promhttp.HandlerFor() for metrics endpoint.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// ============================================
// Aliases (for cleaner API)
// ============================================

// RecordScraperRequest is an alias for RecordScraper.
func (m *Metrics) RecordScraperRequest(module, status string, duration float64) {
	m.RecordScraper(module, status, duration)
}

// RecordLLMRequest is an alias for RecordLLM.
func (m *Metrics) RecordLLMRequest(operation, status string, duration float64) {
	m.RecordLLM(operation, status, duration)
}

// RecordLLMFallback records an LLM fallback event.
func (m *Metrics) RecordLLMFallback(operation string) {
	m.LLMTotal.WithLabelValues(operation, "fallback").Inc()
}
