package metrics

import (
	"slices"
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
		// HTTP metrics
		{"HTTPServerRequestsTotal", func() bool { return m.HTTPServerRequestsTotal != nil }},
		{"HTTPServerRequestDuration", func() bool { return m.HTTPServerRequestDuration != nil }},

		// Webhook metrics
		{"WebhookBatchTotal", func() bool { return m.WebhookBatchTotal != nil }},
		{"WebhookTotal", func() bool { return m.WebhookTotal != nil }},
		{"WebhookDuration", func() bool { return m.WebhookDuration != nil }},
		{"LineReplyTotal", func() bool { return m.LineReplyTotal != nil }},
		{"LineReplyDuration", func() bool { return m.LineReplyDuration != nil }},

		// Scraper metrics
		{"ScraperTotal", func() bool { return m.ScraperTotal != nil }},
		{"ScraperDuration", func() bool { return m.ScraperDuration != nil }},

		// Cache metrics
		{"CacheOperations", func() bool { return m.CacheOperations != nil }},
		{"CacheSize", func() bool { return m.CacheSize != nil }},

		// LLM metrics
		{"LLMTotal", func() bool { return m.LLMTotal != nil }},
		{"LLMDuration", func() bool { return m.LLMDuration != nil }},
		{"LLMFallbackTotal", func() bool { return m.LLMFallbackTotal != nil }},
		{"LLMCooldownTotal", func() bool { return m.LLMCooldownTotal != nil }},

		// Search metrics
		{"SearchTotal", func() bool { return m.SearchTotal != nil }},
		{"SearchDuration", func() bool { return m.SearchDuration != nil }},
		{"SearchResults", func() bool { return m.SearchResults != nil }},
		{"IndexSize", func() bool { return m.IndexSize != nil }},

		// Intent Distribution metrics
		{"IntentTotal", func() bool { return m.IntentTotal != nil }},

		// Rate limiter metrics
		{"RateLimiterDropped", func() bool { return m.RateLimiterDropped != nil }},
		{"RateLimiterUsers", func() bool { return m.RateLimiterUsers != nil }},
		{"LLMRateLimiterUsers", func() bool { return m.LLMRateLimiterUsers != nil }},

		// Job metrics
		{"JobTotal", func() bool { return m.JobTotal != nil }},
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

func TestRecordHTTPServerRequest(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.RecordHTTPServerRequest("GET", "/health", 200, 0.01)
	m.RecordHTTPServerRequest("POST", "/webhook", 503, 0.25)
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

func TestRecordLineReply(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.RecordLineReply("success", 0.2)
	m.RecordLineReply("invalid_token", 0.0)
	m.RecordLineReply("rate_limited", 1.0)
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
		model     string
		operation string
		status    string
		duration  float64
	}{
		{"gemini", "gemma-4-31b-it", "nlu", "success", 0.5},
		{"gemini", "gemma-4-31b-it", "nlu", "error", 1.0},
		{"groq", "openai/gpt-oss-120b", "nlu", "rate_limit", 2.0},
		{"gemini", "gemma-4-31b-it", "expander", "success", 0.8},
	}

	for _, tc := range testCases {
		m.RecordLLM(tc.provider, tc.model, tc.operation, tc.status, tc.duration)
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
		searchType string
		status     string
		duration   float64
	}{
		{"bm25", "success", 0.05},
		{"bm25", "success", 0.03},
		{"bm25", "error", 1.0},
		{"disabled", "skipped", 0.001},
	}

	for _, tc := range testCases {
		m.RecordSearch(tc.searchType, tc.status, tc.duration)
	}
}

func TestRecordSearchResults(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.RecordSearchResults("bm25", 0)
	m.RecordSearchResults("bm25", 7)
}

func TestSetIndexSize(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.SetIndexSize("bm25", 1000)
	m.SetIndexSize("bm25", 2000) // Update index size
}

// ============================================
// Intent Distribution metrics tests
// ============================================

func TestRecordIntent(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	testCases := []struct {
		module string
		intent string
		source string
	}{
		{"course", "search", "keyword"},
		{"course", "smart", "nlu"},
		{"contact", "search", "keyword"},
		{"id", "student_id", "nlu"},
		{"program", "list", "nlu"},
		{"course", "", "keyword"},
	}

	for _, tc := range testCases {
		m.RecordIntent(tc.module, tc.intent, tc.source)
	}
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
		{"refresh", "id", 60.0},
		{"refresh", "course", 120.0},
		{"refresh", "total", 300.0},
		{"cleanup", "all", 10.0},
		{"sticker_refresh", "all", 30.0},
	}

	for _, tc := range testCases {
		m.RecordJob(tc.job, tc.module, tc.duration)
	}
}

func TestRecordJobRun(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.RecordJobRun("refresh", "total", "success", 300.0)
	m.RecordJobRun("refresh", "total", "error", 12.0)
	m.RecordJobRun("refresh", "total", "skipped", 0.1)
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
	m.RecordLLMRequest("gemini", "gemma-4-31b-it", "nlu", "success", 0.5)
	m.RecordLLMRequest("groq", "openai/gpt-oss-120b", "nlu", "error", 1.0)
}

func TestRecordLLMFallback(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	// RecordLLMFallback records provider fallback events
	m.RecordLLMFallback("gemini", "gemma-4-31b-it", "groq", "openai/gpt-oss-120b", "nlu")
	m.RecordLLMFallback("groq", "openai/gpt-oss-120b", "gemini", "gemma-4-26b-a4b-it", "expander")
}

func TestMetricNamesRegisteredAfterUse(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	m := New(registry)

	m.RecordHTTPServerRequest("GET", "/health", 200, 0.01)
	m.RecordWebhookBatch("accepted")
	m.RecordWebhook("message", "success", 0.5)
	m.RecordLineReply("success", 0.2)
	m.RecordScraper("course", "success", 1.5)
	m.RecordCacheHit("course")
	m.SetCacheSize("courses", 100)
	m.RecordLLM("gemini", "gemma-4-31b-it", "nlu", "success", 0.5)
	m.RecordLLMFallback("gemini", "gemma-4-31b-it", "groq", "openai/gpt-oss-120b", "nlu")
	m.LLMCooldownTotal.WithLabelValues("gemini", "gemma-4-31b-it", "burst", "applied").Inc()
	m.RecordSearch("bm25", "success", 0.05)
	m.RecordSearchResults("bm25", 3)
	m.SetIndexSize("bm25", 1000)
	m.RecordIntent("course", "smart", "nlu")
	m.RecordRateLimiterDrop("user")
	m.SetRateLimiterUsers(10)
	m.SetLLMRateLimiterUsers(2)
	m.RecordJobRun("refresh", "total", "success", 120)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	got := make([]string, 0, len(families))
	for _, family := range families {
		got = append(got, family.GetName())
	}

	want := []string{
		"ntpu_http_server_requests_total",
		"ntpu_http_server_request_duration_seconds",
		"ntpu_webhook_batch_total",
		"ntpu_webhook_total",
		"ntpu_webhook_duration_seconds",
		"ntpu_line_reply_total",
		"ntpu_line_reply_duration_seconds",
		"ntpu_scraper_total",
		"ntpu_scraper_duration_seconds",
		"ntpu_cache_operations_total",
		"ntpu_cache_size",
		"ntpu_llm_total",
		"ntpu_llm_duration_seconds",
		"ntpu_llm_fallback_total",
		"ntpu_llm_cooldown_total",
		"ntpu_search_total",
		"ntpu_search_duration_seconds",
		"ntpu_search_results",
		"ntpu_index_size",
		"ntpu_intent_total",
		"ntpu_rate_limiter_dropped_total",
		"ntpu_rate_limiter_users",
		"ntpu_llm_rate_limiter_users",
		"ntpu_job_total",
		"ntpu_job_duration_seconds",
	}

	for _, name := range want {
		if !slices.Contains(got, name) {
			t.Errorf("metric %q not gathered; got %v", name, got)
		}
	}
}
