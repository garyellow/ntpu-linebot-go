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

// Package-level metrics for use by other packages (e.g., genai).
// These are initialized by InitGlobal() after creating the Metrics instance.
var (
	// LLMTotal is the global LLM request counter.
	LLMTotal *prometheus.CounterVec

	// LLMDuration is the global LLM duration histogram.
	LLMDuration *prometheus.HistogramVec

	// LLMFallbackTotal is the global LLM fallback counter.
	LLMFallbackTotal *prometheus.CounterVec

	// LLMFallbackLatency is the global LLM fallback latency histogram.
	LLMFallbackLatency *prometheus.HistogramVec
)

// InitGlobal initializes the package-level metric variables.
// Must be called after New() to enable metrics in other packages.
func InitGlobal(m *Metrics) {
	LLMTotal = m.LLMTotal
	LLMDuration = m.LLMDuration
	LLMFallbackTotal = m.LLMFallbackTotal
	LLMFallbackLatency = m.LLMFallbackLatency
}

// Metrics holds all Prometheus metrics for the NTPU LineBot.
// Organized by component following the RED/USE methodology.
type Metrics struct {
	registry *prometheus.Registry

	// ============================================
	// Webhook (LINE Bot Core - RED Method)
	// Primary service entry point
	// ============================================
	// Batch: incoming webhook requests (HTTP)
	WebhookBatchTotal *prometheus.CounterVec
	// Rate: requests per second by event type
	// Errors: tracked via status label (success/error/reply_error)
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
	// LLM (Gemini/Groq/Cerebras API - RED Method)
	// NLU intent parsing, Query Expansion
	// ============================================
	LLMTotal           *prometheus.CounterVec   // requests by provider, operation, and status
	LLMDuration        *prometheus.HistogramVec // latency by provider and operation
	LLMFallbackTotal   *prometheus.CounterVec   // fallback events by provider pair and operation
	LLMFallbackLatency *prometheus.HistogramVec // additional latency from fallback by provider pair and operation

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
		WebhookBatchTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_webhook_batch_total",
				Help: "Total webhook batches received",
			},
			// status: accepted, invalid_signature, parse_error
			[]string{"status"},
		),

		WebhookTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_webhook_total",
				Help: "Total webhook events processed",
			},
			// event_type: message, postback, follow, join
			// status: success, error, reply_error
			[]string{"event_type", "status"},
		),

		WebhookDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_webhook_duration_seconds",
				Help: "Webhook processing duration in seconds",
				// Buckets aligned with LINE API expectations:
				// < 2s: excellent
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
			// status: success, error, timeout, not_found
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
			// module: students, contacts, courses, syllabi, program, stickers
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
			// provider: gemini, groq, cerebras
			// operation: nlu (intent parsing), expander (query expansion)
			// status: success, error, rate_limit, quota_exhausted
			[]string{"provider", "operation", "status"},
		),

		LLMDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_llm_duration_seconds",
				Help: "LLM API request duration in seconds",
				// Buckets for LLM API latency:
				// Fast: < 0.5s (simple queries)
				// Normal: 0.5-2s (typical)
				// Slow: > 2s (complex or retry)
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
			},
			// provider: gemini, groq, cerebras
			// operation: nlu, expander
			[]string{"provider", "operation"},
		),

		LLMFallbackTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_llm_fallback_total",
				Help: "Total LLM provider fallback events",
			},
			// from_provider: gemini, groq, cerebras (primary that failed)
			// to_provider: gemini, groq, cerebras (fallback used)
			// operation: nlu, expander
			[]string{"from_provider", "to_provider", "operation"},
		),

		LLMFallbackLatency: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "ntpu_llm_fallback_latency_seconds",
				Help: "Additional latency introduced by provider fallback",
				// Buckets for fallback overhead:
				// Fast: < 0.5s (immediate fallback)
				// Normal: 0.5-2s (with retry)
				// Slow: > 2s (multiple retries)
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
			},
			// from_provider: gemini, groq, cerebras (primary that failed)
			// to_provider: gemini, groq, cerebras (fallback used)
			// operation: nlu, expander
			[]string{"from_provider", "to_provider", "operation"},
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
			// status: success, error, no_results, skipped
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
			// job: warmup, cache_cleanup, sticker_refresh
			// module: id, contact, course, syllabus, total, all
			[]string{"job", "module"},
		),
	}

	return m
}

// ============================================
// Webhook helpers
// ============================================

// RecordWebhookBatch records a webhook batch (HTTP request).
// status: accepted, invalid_signature, parse_error
func (m *Metrics) RecordWebhookBatch(status string) {
	m.WebhookBatchTotal.WithLabelValues(status).Inc()
}

// RecordWebhook records a webhook event.
// eventType: message, postback, follow, join
// status: success, error, reply_error
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
// provider: gemini, groq, cerebras
// operation: nlu (intent parsing), expander (query expansion)
// status: success, error, rate_limit, quota_exhausted
func (m *Metrics) RecordLLM(provider, operation, status string, duration float64) {
	m.LLMTotal.WithLabelValues(provider, operation, status).Inc()
	m.LLMDuration.WithLabelValues(provider, operation).Observe(duration)
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
func (m *Metrics) RecordLLMRequest(provider, operation, status string, duration float64) {
	m.RecordLLM(provider, operation, status, duration)
}

// RecordLLMFallback records an LLM provider fallback event.
// fromProvider: the primary provider that failed
// toProvider: the fallback provider used
// operation: nlu, expander
func (m *Metrics) RecordLLMFallback(fromProvider, toProvider, operation string) {
	m.LLMFallbackTotal.WithLabelValues(fromProvider, toProvider, operation).Inc()
}
