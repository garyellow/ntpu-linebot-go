package config

import (
	"testing"
	"time"
)

// TestWebhookTimeouts verifies webhook-related timeout constants
func TestWebhookTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"WebhookProcessing", WebhookProcessing, 60 * time.Second},
		{"WebhookHTTPRead", WebhookHTTPRead, 10 * time.Second},
		{"WebhookHTTPWrite", WebhookHTTPWrite, 65 * time.Second},
		{"WebhookHTTPIdle", WebhookHTTPIdle, 120 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestScraperTimeouts verifies scraper-related timeout constants
func TestScraperTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"ScraperRequest", ScraperRequest, 60 * time.Second},
		{"ScraperRetryInitial", ScraperRetryInitial, 4 * time.Second},
		{"ScraperRateLimit", ScraperRateLimit, 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestDatabaseTimeouts verifies database-related timeout constants
func TestDatabaseTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"DatabaseBusyTimeout", DatabaseBusyTimeout, 30 * time.Second},
		{"DatabaseConnMaxLifetime", DatabaseConnMaxLifetime, time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestBackgroundJobIntervals verifies background job intervals
func TestBackgroundJobIntervals(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"CacheCleanupInterval", CacheCleanupInterval, 24 * time.Hour},
		{"MetricsUpdateInterval", MetricsUpdateInterval, 5 * time.Minute},
		{"RateLimiterCleanupInterval", RateLimiterCleanupInterval, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestBackgroundJobScheduleHours verifies background job schedule hours (Taiwan time)
func TestBackgroundJobScheduleHours(t *testing.T) {
	tests := []struct {
		name     string
		got      int
		expected int
	}{
		{"WarmupHour", WarmupHour, 3},             // 3:00 AM - warmup cache
		{"CacheCleanupHour", CacheCleanupHour, 4}, // 4:00 AM - cleanup after warmup
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestScheduleOrderIsLogical verifies jobs run in logical order
func TestScheduleOrderIsLogical(t *testing.T) {
	// Warmup should happen before cache cleanup
	if WarmupHour >= CacheCleanupHour {
		t.Errorf("WarmupHour (%d) should be < CacheCleanupHour (%d) to avoid deleting fresh data",
			WarmupHour, CacheCleanupHour)
	}

	// All should be in early morning (0-6 AM)
	if WarmupHour < 0 || WarmupHour > 6 {
		t.Errorf("WarmupHour (%d) should be in early morning (0-6 AM)", WarmupHour)
	}
	if CacheCleanupHour < 0 || CacheCleanupHour > 6 {
		t.Errorf("CacheCleanupHour (%d) should be in early morning (0-6 AM)", CacheCleanupHour)
	}
}

// TestTimeoutRelationships verifies that timeouts have proper relationships
func TestTimeoutRelationships(t *testing.T) {
	// WebhookHTTPWrite should be greater than WebhookProcessing
	if WebhookHTTPWrite <= WebhookProcessing {
		t.Errorf("WebhookHTTPWrite (%v) should be > WebhookProcessing (%v)",
			WebhookHTTPWrite, WebhookProcessing)
	}

	// WebhookHTTPIdle should be greater than WebhookHTTPWrite
	if WebhookHTTPIdle <= WebhookHTTPWrite {
		t.Errorf("WebhookHTTPIdle (%v) should be > WebhookHTTPWrite (%v)",
			WebhookHTTPIdle, WebhookHTTPWrite)
	}

	// ScraperRequest should be greater than ScraperRetryInitial
	if ScraperRequest <= ScraperRetryInitial {
		t.Errorf("ScraperRequest (%v) should be > ScraperRetryInitial (%v)",
			ScraperRequest, ScraperRetryInitial)
	}

	// SmartSearchTimeout should be less than WebhookProcessing
	// to allow time for other operations after smart search
	if SmartSearchTimeout >= WebhookProcessing {
		t.Errorf("SmartSearchTimeout (%v) should be < WebhookProcessing (%v)",
			SmartSearchTimeout, WebhookProcessing)
	}
}
