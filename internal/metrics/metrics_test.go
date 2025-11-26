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
	if m.WarmupTasksTotal == nil {
		t.Error("WarmupTasksTotal is nil")
	}
	if m.WarmupDuration == nil {
		t.Error("WarmupDuration is nil")
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
