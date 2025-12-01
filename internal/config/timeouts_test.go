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
		{"CacheCleanupInterval", CacheCleanupInterval, 12 * time.Hour},
		{"CacheCleanupInitialDelay", CacheCleanupInitialDelay, 5 * time.Minute},
		{"StickerRefreshInterval", StickerRefreshInterval, 24 * time.Hour},
		{"StickerRefreshInitialDelay", StickerRefreshInitialDelay, 1 * time.Hour},
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

	// SemanticSearchTimeout should be less than WebhookProcessing
	// to allow time for other operations after semantic search
	if SemanticSearchTimeout >= WebhookProcessing {
		t.Errorf("SemanticSearchTimeout (%v) should be < WebhookProcessing (%v)",
			SemanticSearchTimeout, WebhookProcessing)
	}
}
