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

// TestSentryTimeouts verifies sentry-related timeout constants
func TestSentryTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"SentryHTTPTimeout", SentryHTTPTimeout, 5 * time.Second},
		{"SentryFlushTimeout", SentryFlushTimeout, 5 * time.Second},
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
		{"HotSwapCloseGracePeriod", HotSwapCloseGracePeriod, 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestR2Timeouts verifies R2-related timeout constants
func TestR2Timeouts(t *testing.T) {
	if R2RequestTimeout != 60*time.Second {
		t.Errorf("R2RequestTimeout = %v, want %v", R2RequestTimeout, 60*time.Second)
	}
}

// TestBackgroundJobIntervals verifies background job intervals
func TestBackgroundJobIntervals(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"DataRefreshIntervalDefault", DataRefreshIntervalDefault, 24 * time.Hour},
		{"DataCleanupIntervalDefault", DataCleanupIntervalDefault, 24 * time.Hour},
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

// TestWarmupTimeouts verifies warmup timeout constants
func TestWarmupTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"WarmupStickerFetch", WarmupStickerFetch, 5 * time.Second},
		{"WarmupProactive", WarmupProactive, 2 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestSmartSearchTimeouts verifies smart search and readiness timeout constants
func TestSmartSearchTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"SmartSearchTimeout", SmartSearchTimeout, 30 * time.Second},
		{"ReadinessCheckTimeout", ReadinessCheckTimeout, 3 * time.Second},
		{"ReadinessWarmupTimeout", ReadinessWarmupTimeout, 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestGracefulShutdownTimeout verifies graceful shutdown timeout constant
func TestGracefulShutdownTimeout(t *testing.T) {
	if GracefulShutdown != 70*time.Second {
		t.Errorf("GracefulShutdown = %v, want %v", GracefulShutdown, 70*time.Second)
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
