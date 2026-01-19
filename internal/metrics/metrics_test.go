package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestNew verifies that all metrics are properly initialized
func TestNew(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	if m == nil {
		t.Fatal("New() returned nil")
	}

	// Verify all metric fields are initialized
	tests := []struct {
		name  string
		check func() bool
	}{
		// Webhook metrics
		{"WebhookBatchTotal", func() bool { return m.WebhookBatchTotal != nil }},
		{"WebhookTotal", func() bool { return m.WebhookTotal != nil }},
		{"WebhookDuration", func() bool { return m.WebhookDuration != nil }},

		// Scraper metrics
		{"ScraperTotal", func() bool { return m.ScraperTotal != nil }},
		{"ScraperDuration", func() bool { return m.ScraperDuration != nil }},

		// Cache metrics
		{"CacheOperations", func() bool { return m.CacheOperations != nil }},
		{"CacheSize", func() bool { return m.CacheSize != nil }},

		// LLM metrics
		{"LLMTotal", func() bool { return m.LLMTotal != nil }},
		{"LLMDuration", func() bool { return m.LLMDuration != nil }},

		// Search metrics
		{"SearchTotal", func() bool { return m.SearchTotal != nil }},
		{"SearchDuration", func() bool { return m.SearchDuration != nil }},
		{"SearchResults", func() bool { return m.SearchResults != nil }},
		{"IndexSize", func() bool { return m.IndexSize != nil }},

		// Rate limiter metrics
		{"RateLimiterDropped", func() bool { return m.RateLimiterDropped != nil }},
		{"RateLimiterUsers", func() bool { return m.RateLimiterUsers != nil }},
		{"LLMRateLimiterUsers", func() bool { return m.LLMRateLimiterUsers != nil }},

		// Job metrics
		{"JobDuration", func() bool { return m.JobDuration != nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !tt.check() {
				t.Errorf("%s is nil", tt.name)
			}
		})
	}
}

// TestRegistry verifies registry is accessible
func TestRegistry(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	if m.Registry() != registry {
		t.Error("Registry() should return the same registry")
	}
}

// ============================================
// Webhook metrics tests
// ============================================

func TestRecordWebhook(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	testCases := []struct {
		eventType string
		status    string
		duration  float64
	}{
		{"message", "success", 0.5},
		{"postback", "error", 1.0},
		{"follow", "rate_limited", 0.1},
		{"join", "success", 0.2},
	}

	for _, tc := range testCases {
		m.RecordWebhook(tc.eventType, tc.status, tc.duration)
	}
}

func TestRecordWebhookBatch(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	statuses := []string{"accepted", "invalid_signature", "parse_error"}
	for _, status := range statuses {
		m.RecordWebhookBatch(status)
	}
}

// ============================================
// Scraper metrics tests
// ============================================

func TestRecordScraper(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	testCases := []struct {
		module   string
		status   string
		duration float64
	}{
		{"id", "success", 1.5},
		{"course", "error", 2.0},
		{"contact", "timeout", 120.0},
		{"syllabus", "success", 5.0},
	}

	for _, tc := range testCases {
		m.RecordScraper(tc.module, tc.status, tc.duration)
	}
}

// ============================================
// Cache metrics tests
// ============================================

func TestRecordCacheHit(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	modules := []string{"students", "courses", "contacts", "syllabi", "stickers"}
	for _, module := range modules {
		m.RecordCacheHit(module)
	}
}

func TestRecordCacheMiss(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	modules := []string{"students", "courses", "contacts", "syllabi", "stickers"}
	for _, module := range modules {
		m.RecordCacheMiss(module)
	}
}

func TestSetCacheSize(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	testCases := []struct {
		module string
		size   int
	}{
		{"students", 1000},
		{"contacts", 500},
		{"courses", 2000},
		{"stickers", 50},
		{"syllabi", 3000},
	}

	for _, tc := range testCases {
		m.SetCacheSize(tc.module, tc.size)
	}
}

// ============================================
// LLM metrics tests
// ============================================

func TestRecordLLM(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	testCases := []struct {
		provider  string
		operation string
		status    string
		duration  float64
	}{
		{"gemini", "nlu", "success", 0.5},
		{"gemini", "nlu", "error", 1.0},
		{"groq", "nlu", "rate_limit", 2.0},
		{"gemini", "expander", "success", 0.8},
	}

	for _, tc := range testCases {
		m.RecordLLM(tc.provider, tc.operation, tc.status, tc.duration)
	}
}

// ============================================
// Search metrics tests
// ============================================

func TestRecordSearch(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	testCases := []struct {
		searchType  string
		status      string
		duration    float64
		resultCount int
	}{
		{"bm25", "success", 0.05, 10},
		{"bm25", "success", 0.03, 20},
		{"bm25", "error", 1.0, 0},
		{"disabled", "skipped", 0.001, 0},
	}

	for _, tc := range testCases {
		m.RecordSearch(tc.searchType, tc.status, tc.duration, tc.resultCount)
	}
}

func TestSetIndexSize(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.SetIndexSize("bm25", 1000)
	m.SetIndexSize("bm25", 2000) // Update index size
}

// ============================================
// Rate Limiter metrics tests
// ============================================

func TestRecordRateLimiterDrop(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.RecordRateLimiterDrop("user")
	m.RecordRateLimiterDrop("global")
}

func TestSetRateLimiterUsers(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.SetRateLimiterUsers(100)
	m.SetRateLimiterUsers(0)
}

// ============================================
// Job metrics tests
// ============================================

func TestRecordJob(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	testCases := []struct {
		job      string
		module   string
		duration float64
	}{
		{"warmup", "id", 60.0},
		{"warmup", "course", 120.0},
		{"warmup", "total", 300.0},
		{"cleanup", "all", 10.0},
		{"sticker_refresh", "all", 30.0},
	}

	for _, tc := range testCases {
		m.RecordJob(tc.job, tc.module, tc.duration)
	}
}

// ============================================
// Alias methods tests
// ============================================

func TestRecordScraperRequest(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	// RecordScraperRequest is an alias for RecordScraper
	m.RecordScraperRequest("id", "success", 1.5)
	m.RecordScraperRequest("course", "error", 2.0)
}

func TestRecordLLMRequest(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	// RecordLLMRequest is an alias for RecordLLM with provider
	m.RecordLLMRequest("gemini", "nlu", "success", 0.5)
	m.RecordLLMRequest("groq", "nlu", "error", 1.0)
}

func TestRecordLLMFallback(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	// RecordLLMFallback records provider fallback events
	m.RecordLLMFallback("gemini", "groq", "nlu")
	m.RecordLLMFallback("groq", "gemini", "expander")
}
