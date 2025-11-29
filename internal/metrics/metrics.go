// Package metrics provides Prometheus metrics for monitoring.
// It tracks scraper performance, cache hit/miss rates, webhook latency,
// and data integrity issues.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// registry is the custom Prometheus registry (avoids global state)
	registry *prometheus.Registry
	// Scraper metrics
	ScraperRequestsTotal   *prometheus.CounterVec
	ScraperDurationSeconds *prometheus.HistogramVec

	// Cache metrics
	CacheHitsTotal   *prometheus.CounterVec
	CacheMissesTotal *prometheus.CounterVec
	CacheSize        *prometheus.GaugeVec // Track cache entry count by module

	// Webhook metrics
	WebhookDurationSeconds *prometheus.HistogramVec
	WebhookRequestsTotal   *prometheus.CounterVec

	// HTTP metrics
	HTTPErrorsTotal *prometheus.CounterVec

	// Data integrity metrics
	CourseDataIntegrity *prometheus.CounterVec

	// Rate limiter metrics
	RateLimiterDropped     *prometheus.CounterVec
	RateLimiterActiveUsers prometheus.Gauge   // Track active user limiters
	RateLimiterCleaned     prometheus.Counter // Track cleanup count

	// Warmup metrics
	WarmupTasksTotal *prometheus.CounterVec
	WarmupDuration   prometheus.Histogram

	// Semantic search metrics
	SemanticSearchTotal    *prometheus.CounterVec   // Total searches by status
	SemanticSearchDuration *prometheus.HistogramVec // Search latency
	SemanticSearchResults  *prometheus.HistogramVec // Number of results returned
	VectorDBSize           prometheus.Gauge         // Current document count in vector store
}

// New creates a new Metrics instance with all metrics registered
// Note: Go runtime, process, and build info collectors should be registered
// by the caller before calling this function to avoid duplicate registration
func New(registry *prometheus.Registry) *Metrics {
	m := &Metrics{
		registry: registry,
		// Scraper metrics
		ScraperRequestsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_scraper_requests_total",
				Help: "Total number of scraper requests by module and status",
			},
			[]string{"module", "status"}, // status: success, error, timeout, not_found
		),

		ScraperDurationSeconds: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ntpu_scraper_duration_seconds",
				Help:    "Scraper request duration in seconds by module",
				Buckets: []float64{1, 2, 5, 10, 15, 30, 60, 90, 120}, // Optimized for 120s timeout
			},
			[]string{"module"}, // module: id, contact, course
		),

		// Cache metrics
		CacheHitsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_cache_hits_total",
				Help: "Total number of cache hits by module",
			},
			[]string{"module"},
		),

		CacheMissesTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_cache_misses_total",
				Help: "Total number of cache misses by module",
			},
			[]string{"module"},
		),

		CacheSize: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ntpu_cache_entries",
				Help: "Current number of entries in cache by module",
			},
			[]string{"module"}, // module: students, contacts, courses, stickers
		),

		// Webhook metrics
		WebhookDurationSeconds: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ntpu_webhook_duration_seconds",
				Help:    "Webhook processing duration in seconds by event type",
				Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 25, 60}, // Extended for webhook timeout (60s)
			},
			[]string{"event_type"}, // event_type: message, postback, follow
		),

		WebhookRequestsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_webhook_requests_total",
				Help: "Total number of webhook requests by event type and status",
			},
			[]string{"event_type", "status"}, // status: success, error
		),

		// HTTP metrics
		HTTPErrorsTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_http_errors_total",
				Help: "Total HTTP errors by type and module",
			},
			[]string{"error_type", "module"}, // error_type: timeout, rate_limit, invalid_signature, etc.
		),

		// Data integrity metrics
		CourseDataIntegrity: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_course_data_integrity_issues_total",
				Help: "Total number of course data integrity issues detected",
			},
			[]string{"issue_type"}, // issue_type: missing_no, empty_title, etc.
		),

		// Rate limiter metrics
		RateLimiterDropped: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_rate_limiter_dropped_total",
				Help: "Total number of requests dropped by rate limiter",
			},
			[]string{"limiter_type"}, // limiter_type: user, global
		),

		RateLimiterActiveUsers: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "ntpu_rate_limiter_active_users",
				Help: "Current number of active user rate limiters",
			},
		),

		RateLimiterCleaned: promauto.With(registry).NewCounter(
			prometheus.CounterOpts{
				Name: "ntpu_rate_limiter_cleaned_total",
				Help: "Total number of user rate limiters cleaned up",
			},
		),

		// Warmup metrics
		WarmupTasksTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_warmup_tasks_total",
				Help: "Total number of warmup tasks by module and status",
			},
			[]string{"module", "status"}, // status: success, error
		),

		WarmupDuration: promauto.With(registry).NewHistogram(
			prometheus.HistogramOpts{
				Name:    "ntpu_warmup_duration_seconds",
				Help:    "Total duration of warmup process",
				Buckets: []float64{10, 30, 60, 120, 300, 600, 900, 1800}, // 10s to 30min
			},
		),

		// Semantic search metrics
		SemanticSearchTotal: promauto.With(registry).NewCounterVec(
			prometheus.CounterOpts{
				Name: "ntpu_semantic_search_total",
				Help: "Total number of semantic searches by status",
			},
			[]string{"status"}, // status: success, error, fallback, disabled
		),

		SemanticSearchDuration: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ntpu_semantic_search_duration_seconds",
				Help:    "Semantic search latency in seconds",
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10}, // 100ms to 10s
			},
			[]string{"type"}, // type: query, embedding
		),

		SemanticSearchResults: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ntpu_semantic_search_results",
				Help:    "Number of results returned by semantic search",
				Buckets: []float64{0, 1, 2, 5, 10, 20}, // 0 to 20 results
			},
			[]string{"source"}, // source: direct, fallback
		),

		VectorDBSize: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "ntpu_vectordb_documents",
				Help: "Current number of documents in vector database",
			},
		),
	}

	return m
}

// RecordScraperRequest records a scraper request with status
func (m *Metrics) RecordScraperRequest(module, status string, duration float64) {
	m.ScraperRequestsTotal.WithLabelValues(module, status).Inc()
	m.ScraperDurationSeconds.WithLabelValues(module).Observe(duration)
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(module string) {
	m.CacheHitsTotal.WithLabelValues(module).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(module string) {
	m.CacheMissesTotal.WithLabelValues(module).Inc()
}

// RecordWebhook records a webhook request
func (m *Metrics) RecordWebhook(eventType, status string, duration float64) {
	m.WebhookRequestsTotal.WithLabelValues(eventType, status).Inc()
	m.WebhookDurationSeconds.WithLabelValues(eventType).Observe(duration)
}

// RecordHTTPError records HTTP error metrics
func (m *Metrics) RecordHTTPError(errorType, module string) {
	m.HTTPErrorsTotal.WithLabelValues(errorType, module).Inc()
}

// RecordCourseIntegrityIssue records a course data integrity issue
func (m *Metrics) RecordCourseIntegrityIssue(issueType string) {
	m.CourseDataIntegrity.WithLabelValues(issueType).Inc()
}

// RecordRateLimiterDrop records a request dropped by rate limiter
func (m *Metrics) RecordRateLimiterDrop(limiterType string) {
	m.RateLimiterDropped.WithLabelValues(limiterType).Inc()
}

// SetRateLimiterActiveUsers sets the current number of active user limiters
func (m *Metrics) SetRateLimiterActiveUsers(count int) {
	m.RateLimiterActiveUsers.Set(float64(count))
}

// RecordRateLimiterCleanup records the number of limiters cleaned up
func (m *Metrics) RecordRateLimiterCleanup(count int) {
	m.RateLimiterCleaned.Add(float64(count))
}

// RecordWarmupTask records a warmup task completion
func (m *Metrics) RecordWarmupTask(module, status string) {
	m.WarmupTasksTotal.WithLabelValues(module, status).Inc()
}

// RecordWarmupDuration records total warmup duration
func (m *Metrics) RecordWarmupDuration(duration float64) {
	m.WarmupDuration.Observe(duration)
}

// SetCacheSize sets the current cache size for a module
func (m *Metrics) SetCacheSize(module string, size int) {
	m.CacheSize.WithLabelValues(module).Set(float64(size))
}

// RecordSemanticSearch records a semantic search request
func (m *Metrics) RecordSemanticSearch(status string, duration float64, resultCount int, source string) {
	m.SemanticSearchTotal.WithLabelValues(status).Inc()
	m.SemanticSearchDuration.WithLabelValues("query").Observe(duration)
	m.SemanticSearchResults.WithLabelValues(source).Observe(float64(resultCount))
}

// RecordEmbeddingLatency records embedding generation latency
func (m *Metrics) RecordEmbeddingLatency(duration float64) {
	m.SemanticSearchDuration.WithLabelValues("embedding").Observe(duration)
}

// SetVectorDBSize sets the current vector database document count
func (m *Metrics) SetVectorDBSize(count int) {
	m.VectorDBSize.Set(float64(count))
}

// Registry returns the custom Prometheus registry
// Use this with promhttp.HandlerFor() instead of the default handler
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
