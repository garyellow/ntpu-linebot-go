package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// Scraper metrics
	ScraperRequestsTotal   *prometheus.CounterVec
	ScraperDurationSeconds *prometheus.HistogramVec

	// Cache metrics
	CacheHitsTotal   *prometheus.CounterVec
	CacheMissesTotal *prometheus.CounterVec

	// Webhook metrics
	WebhookDurationSeconds *prometheus.HistogramVec
	WebhookRequestsTotal   *prometheus.CounterVec

	// HTTP metrics
	HTTPErrorsTotal *prometheus.CounterVec

	// Data integrity metrics
	CourseDataIntegrity *prometheus.CounterVec

	// Rate limiter metrics
	RateLimiterDropped *prometheus.CounterVec

	// Warmup metrics
	WarmupTasksTotal *prometheus.CounterVec
	WarmupDuration   prometheus.Histogram
}

// New creates a new Metrics instance with all metrics registered
func New(registry *prometheus.Registry) *Metrics {
	m := &Metrics{
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
				Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 60, 120}, // Single request timeout is 120s
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

		// Webhook metrics
		WebhookDurationSeconds: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ntpu_webhook_duration_seconds",
				Help:    "Webhook processing duration in seconds by event type",
				Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5}, // Faster buckets for webhook
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

// RecordWarmupTask records a warmup task completion
func (m *Metrics) RecordWarmupTask(module, status string) {
	m.WarmupTasksTotal.WithLabelValues(module, status).Inc()
}

// RecordWarmupDuration records total warmup duration
func (m *Metrics) RecordWarmupDuration(duration float64) {
	m.WarmupDuration.Observe(duration)
}
