package config
	{
		{"DataRefreshIntervalDefault", DataRefreshIntervalDefault, 24 * time.Hour},
		{"DataCleanupIntervalDefault", DataCleanupIntervalDefault, 24 * time.Hour},
		{"MetricsUpdateInterval", MetricsUpdateInterval, 5 * time.Minute},
		{"RateLimiterCleanupInterval", RateLimiterCleanupInterval, 5 * time.Minute},
	{
		{"DataRefreshIntervalDefault", DataRefreshIntervalDefault, 24 * time.Hour},
		{"DataCleanupIntervalDefault", DataCleanupIntervalDefault, 24 * time.Hour},
		{"MetricsUpdateInterval", MetricsUpdateInterval, 5 * time.Minute},
		{"RateLimiterCleanupInterval", RateLimiterCleanupInterval, 5 * time.Minute},
	{
		{"DataRefreshIntervalDefault", DataRefreshIntervalDefault, 24 * time.Hour},
		{"DataCleanupIntervalDefault", DataCleanupIntervalDefault, 24 * time.Hour},
		{"MetricsUpdateInterval", MetricsUpdateInterval, 5 * time.Minute},
		{"RateLimiterCleanupInterval", RateLimiterCleanupInterval, 5 * time.Minute},
	{
		{"DataRefreshIntervalDefault", DataRefreshIntervalDefault, 24 * time.Hour},
		{"DataCleanupIntervalDefault", DataCleanupIntervalDefault, 24 * time.Hour},
		{"MetricsUpdateInterval", MetricsUpdateInterval, 5 * time.Minute},
		{"RateLimiterCleanupInterval", RateLimiterCleanupInterval, 5 * time.Minute},
	}
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
