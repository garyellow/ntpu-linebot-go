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
	CacheEntries     *prometheus.GaugeVec

	// Webhook metrics
	WebhookDurationSeconds *prometheus.HistogramVec
	WebhookRequestsTotal   *prometheus.CounterVec

	// System metrics
	ActiveGoroutines prometheus.Gauge
	MemoryBytes      prometheus.Gauge
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
			[]string{"module", "status"}, // status: success, error, timeout
		),

		ScraperDurationSeconds: promauto.With(registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "ntpu_scraper_duration_seconds",
				Help:    "Scraper request duration in seconds by module",
				Buckets: prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
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

		CacheEntries: promauto.With(registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "ntpu_cache_entries",
				Help: "Current number of cache entries by module",
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

		// System metrics
		ActiveGoroutines: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "ntpu_active_goroutines",
				Help: "Current number of active goroutines",
			},
		),

		MemoryBytes: promauto.With(registry).NewGauge(
			prometheus.GaugeOpts{
				Name: "ntpu_memory_bytes",
				Help: "Current memory usage in bytes",
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

// UpdateCacheEntries updates the cache entries gauge
func (m *Metrics) UpdateCacheEntries(module string, count float64) {
	m.CacheEntries.WithLabelValues(module).Set(count)
}

// RecordWebhook records a webhook request
func (m *Metrics) RecordWebhook(eventType, status string, duration float64) {
	m.WebhookRequestsTotal.WithLabelValues(eventType, status).Inc()
	m.WebhookDurationSeconds.WithLabelValues(eventType).Observe(duration)
}

// UpdateSystemMetrics updates system metrics (goroutines, memory)
func (m *Metrics) UpdateSystemMetrics(goroutines int, memoryBytes uint64) {
	m.ActiveGoroutines.Set(float64(goroutines))
	m.MemoryBytes.Set(float64(memoryBytes))
}
