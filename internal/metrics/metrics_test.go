package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNew(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	if m == nil {
		t.Fatal("New() returned nil")
	}

	// Verify all metric fields are initialized
	if m.ScraperRequestsTotal == nil {
		t.Error("ScraperRequestsTotal is nil")
	}
	if m.ScraperDurationSeconds == nil {
		t.Error("ScraperDurationSeconds is nil")
	}
	if m.CacheHitsTotal == nil {
		t.Error("CacheHitsTotal is nil")
	}
	if m.CacheMissesTotal == nil {
		t.Error("CacheMissesTotal is nil")
	}
	if m.CacheSize == nil {
		t.Error("CacheSize is nil")
	}
	if m.WebhookDurationSeconds == nil {
		t.Error("WebhookDurationSeconds is nil")
	}
	if m.WebhookRequestsTotal == nil {
		t.Error("WebhookRequestsTotal is nil")
	}
	if m.HTTPErrorsTotal == nil {
		t.Error("HTTPErrorsTotal is nil")
	}
	if m.CourseDataIntegrity == nil {
		t.Error("CourseDataIntegrity is nil")
	}
	if m.RateLimiterDropped == nil {
		t.Error("RateLimiterDropped is nil")
	}
	if m.RateLimiterActiveUsers == nil {
		t.Error("RateLimiterActiveUsers is nil")
	}
	if m.RateLimiterCleaned == nil {
		t.Error("RateLimiterCleaned is nil")
	}
	if m.WarmupTasksTotal == nil {
		t.Error("WarmupTasksTotal is nil")
	}
	if m.WarmupDuration == nil {
		t.Error("WarmupDuration is nil")
	}
	// Semantic search metrics
	if m.SemanticSearchTotal == nil {
		t.Error("SemanticSearchTotal is nil")
	}
	if m.SemanticSearchDuration == nil {
		t.Error("SemanticSearchDuration is nil")
	}
	if m.SemanticSearchResults == nil {
		t.Error("SemanticSearchResults is nil")
	}
	if m.VectorDBSize == nil {
		t.Error("VectorDBSize is nil")
	}
}

func TestRecordScraperRequest(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordScraperRequest("id", "success", 1.5)
	m.RecordScraperRequest("course", "error", 2.0)
	m.RecordScraperRequest("contact", "timeout", 120.0)
}

func TestRecordCacheHit(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordCacheHit("id")
	m.RecordCacheHit("course")
	m.RecordCacheHit("contact")
}

func TestRecordCacheMiss(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordCacheMiss("id")
	m.RecordCacheMiss("course")
	m.RecordCacheMiss("contact")
}

func TestRecordWebhook(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordWebhook("message", "success", 0.5)
	m.RecordWebhook("postback", "error", 1.0)
	m.RecordWebhook("follow", "success", 0.1)
}

func TestRecordHTTPError(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordHTTPError("timeout", "webhook")
	m.RecordHTTPError("rate_limit", "scraper")
	m.RecordHTTPError("invalid_signature", "webhook")
}

func TestRecordCourseIntegrityIssue(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordCourseIntegrityIssue("missing_no")
	m.RecordCourseIntegrityIssue("empty_title")
}

func TestRecordRateLimiterDrop(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordRateLimiterDrop("user")
	m.RecordRateLimiterDrop("global")
}

func TestRecordWarmupTask(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordWarmupTask("id", "success")
	m.RecordWarmupTask("course", "error")
	m.RecordWarmupTask("contact", "success")
}

func TestRecordWarmupDuration(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordWarmupDuration(60.0)
	m.RecordWarmupDuration(300.0)
}

func TestSetCacheSize(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.SetCacheSize("students", 1000)
	m.SetCacheSize("contacts", 500)
	m.SetCacheSize("courses", 2000)
	m.SetCacheSize("stickers", 50)
}

func TestRecordSemanticSearch(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordSemanticSearch("success", 1.5, 10, "direct")
	m.RecordSemanticSearch("error", 0.5, 0, "direct")
	m.RecordSemanticSearch("fallback", 2.0, 5, "fallback")
	m.RecordSemanticSearch("disabled", 0.0, 0, "direct")
}

func TestRecordEmbeddingLatency(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordEmbeddingLatency(0.5)
	m.RecordEmbeddingLatency(1.0)
	m.RecordEmbeddingLatency(2.5)
}

func TestSetVectorDBSize(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.SetVectorDBSize(0)
	m.SetVectorDBSize(100)
	m.SetVectorDBSize(5000)
}

func TestSetRateLimiterActiveUsers(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.SetRateLimiterActiveUsers(0)
	m.SetRateLimiterActiveUsers(10)
	m.SetRateLimiterActiveUsers(100)
}

func TestRecordRateLimiterCleanup(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Should not panic
	m.RecordRateLimiterCleanup(0)
	m.RecordRateLimiterCleanup(5)
	m.RecordRateLimiterCleanup(50)
}

func TestMetrics_WithDefaultRegistry(t *testing.T) {
	// Test that metrics can be created with a new registry
	// without conflicting with default registry
	registry := prometheus.NewRegistry()
	m := New(registry)

	// Record some metrics
	m.RecordScraperRequest("test", "success", 1.0)
	m.RecordCacheHit("test")
	m.RecordWebhook("message", "success", 0.5)

	// Gather metrics to verify they were recorded
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Should have metrics registered
	if len(metricFamilies) == 0 {
		t.Error("No metrics were gathered")
	}

	// Check for specific metric names
	expectedMetrics := map[string]bool{
		"ntpu_scraper_requests_total":   false,
		"ntpu_scraper_duration_seconds": false,
		"ntpu_cache_hits_total":         false,
		"ntpu_webhook_requests_total":   false,
		"ntpu_webhook_duration_seconds": false,
	}

	for _, mf := range metricFamilies {
		if _, ok := expectedMetrics[mf.GetName()]; ok {
			expectedMetrics[mf.GetName()] = true
		}
	}

	for name, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %q not found", name)
		}
	}
}
