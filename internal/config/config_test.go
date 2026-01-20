package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Set required environment variables
	t.Setenv(EnvLineChannelAccessToken, "test_token")
	t.Setenv(EnvLineChannelSecret, "test_secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check required fields
	if cfg.LineChannelToken != "test_token" {
		t.Errorf("Expected token 'test_token', got '%s'", cfg.LineChannelToken)
	}

	if cfg.LineChannelSecret != "test_secret" {
		t.Errorf("Expected secret 'test_secret', got '%s'", cfg.LineChannelSecret)
	}

	// Check defaults
	if cfg.Port != "10000" {
		t.Errorf("Expected default port '10000', got '%s'", cfg.Port)
	}

	if cfg.ScraperMaxRetries != 10 {
		t.Errorf("Expected default max retries 10, got %d", cfg.ScraperMaxRetries)
	}
}

func TestLoad_MissingCredentials(t *testing.T) {
	// Cannot use t.Parallel() here: t.Setenv panics if called after t.Parallel().
	// Explicitly unset LINE credentials to ensure test isolation from system env.
	t.Setenv(EnvLineChannelAccessToken, "")
	t.Setenv(EnvLineChannelSecret, "")

	_, err := Load()
	if err == nil {
		t.Error("Load() should fail when LINE credentials are missing")
	}
	if !contains(err.Error(), "NTPU_LINE_CHANNEL_ACCESS_TOKEN") {
		t.Errorf("Load() error = %v, want error containing 'NTPU_LINE_CHANNEL_ACCESS_TOKEN'", err)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel() // Safe: pure struct validation, no env vars.
	tests := []struct {
		name        string
		cfg         *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			cfg: &Config{
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				DataDir:           "/data",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
				Bot:               newTestBotConfig(),
			},
			wantErr: false,
		},
		{
			name: "missing token",
			cfg: &Config{
				LineChannelSecret: "secret",
				Port:              "10000",
				DataDir:           "/data",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
				Bot:               newTestBotConfig(),
			},
			wantErr:     true,
			errContains: "NTPU_LINE_CHANNEL_ACCESS_TOKEN",
		},
		{
			name: "missing secret",
			cfg: &Config{
				LineChannelToken:  "token",
				Port:              "10000",
				DataDir:           "/data",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
				Bot:               newTestBotConfig(),
			},
			wantErr:     true,
			errContains: "NTPU_LINE_CHANNEL_SECRET",
		},
		{
			name: "missing DataDir",
			cfg: &Config{
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
				Bot:               newTestBotConfig(),
			},
			wantErr:     true,
			errContains: "NTPU_DATA_DIR",
		},
		{
			name: "negative retries",
			cfg: &Config{
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				DataDir:           "/data",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: -1,
				Bot:               newTestBotConfig(),
			},
			wantErr:     true,
			errContains: "NTPU_SCRAPER_MAX_RETRIES",
		},
		{
			name: "WaitForWarmup with zero grace period",
			cfg: &Config{
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				DataDir:           "/data",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
				WaitForWarmup:     true,
				WarmupGracePeriod: 0,
				Bot:               newTestBotConfig(),
			},
			wantErr:     true,
			errContains: "NTPU_WARMUP_GRACE_PERIOD",
		},
		// R2 Tests
		{
			name: "R2 enabled but missing access key",
			cfg: &Config{
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				DataDir:           "/data",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
				Bot:               newTestBotConfig(),
				R2Enabled:         true,
				R2AccountID:       "test_account",
				// Missing fields
				R2AccessKeyID:  "",
				R2BucketName:   "bucket",
				R2SecretKey:    "secret",
				R2LockTTL:      30 * time.Minute,
				R2PollInterval: 5 * time.Minute,
			},
			wantErr:     true,
			errContains: "NTPU_R2_ACCESS_KEY_ID",
		},
		{
			name: "R2 valid",
			cfg: &Config{
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				DataDir:           "/data",
				CacheTTL:          168 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
				Bot:               newTestBotConfig(),
				// Complete R2 config
				R2Enabled:      true,
				R2AccountID:    "test_account",
				R2AccessKeyID:  "access",
				R2SecretKey:    "secret",
				R2BucketName:   "bucket",
				R2LockTTL:      30 * time.Minute,
				R2PollInterval: 5 * time.Minute,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestConfig_FeatureEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        *Config
		check      func(*Config) bool
		want       bool
		methodName string
	}{
		// LLM
		{"LLM disabled", &Config{}, func(c *Config) bool { return c.IsLLMEnabled() }, false, "IsLLMEnabled"},
		{"LLM enabled", &Config{LLMEnabled: true}, func(c *Config) bool { return c.IsLLMEnabled() }, true, "IsLLMEnabled"},

		// R2
		{"R2 disabled", &Config{}, func(c *Config) bool { return c.IsR2Enabled() }, false, "IsR2Enabled"},
		{"R2 enabled", &Config{R2Enabled: true}, func(c *Config) bool { return c.IsR2Enabled() }, true, "IsR2Enabled"},

		// Sentry
		{"Sentry disabled", &Config{}, func(c *Config) bool { return c.IsSentryEnabled() }, false, "IsSentryEnabled"},
		{"Sentry enabled", &Config{SentryEnabled: true}, func(c *Config) bool { return c.IsSentryEnabled() }, true, "IsSentryEnabled"},

		// Better Stack
		{"BetterStack disabled", &Config{}, func(c *Config) bool { return c.IsBetterStackEnabled() }, false, "IsBetterStackEnabled"},
		{"BetterStack enabled", &Config{BetterStackEnabled: true}, func(c *Config) bool { return c.IsBetterStackEnabled() }, true, "IsBetterStackEnabled"},

		// Metrics Auth
		{"MetricsAuth disabled", &Config{}, func(c *Config) bool { return c.IsMetricsAuthEnabled() }, false, "IsMetricsAuthEnabled"},
		{"MetricsAuth enabled", &Config{MetricsAuthEnabled: true}, func(c *Config) bool { return c.IsMetricsAuthEnabled() }, true, "IsMetricsAuthEnabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.check(tt.cfg); got != tt.want {
				t.Errorf("%s() = %v, want %v", tt.methodName, got, tt.want)
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}()))
}

func TestGetBoolEnv(t *testing.T) {
	// Cannot use t.Parallel(): t.Setenv panics after t.Parallel().
	tests := []struct {
		name         string
		key          string
		value        string
		defaultValue bool
		want         bool
	}{
		{
			name:         "true lowercase",
			key:          "TEST_BOOL",
			value:        "true",
			defaultValue: false,
			want:         true,
		},
		{
			name:         "TRUE uppercase",
			key:          "TEST_BOOL",
			value:        "TRUE",
			defaultValue: false,
			want:         true,
		},
		// ... existing boolean tests ...
		{
			name:         "invalid value returns default",
			key:          "TEST_BOOL",
			value:        "invalid",
			defaultValue: true,
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				t.Setenv(tt.key, tt.value)
			}
			got := getBoolEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getBoolEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDurationEnv(t *testing.T) {
	// Cannot use t.Parallel(): t.Setenv panics after t.Parallel().
	tests := []struct {
		name         string
		key          string
		value        string
		defaultValue time.Duration
		want         time.Duration
	}{
		{
			name:         "valid duration",
			key:          "TEST_DURATION",
			value:        "5s",
			defaultValue: 1 * time.Second,
			want:         5 * time.Second,
		},
		{
			name:         "invalid duration",
			key:          "TEST_DURATION",
			value:        "invalid",
			defaultValue: 1 * time.Second,
			want:         1 * time.Second,
		},
		{
			name:         "empty value",
			key:          "TEST_DURATION",
			value:        "",
			defaultValue: 1 * time.Second,
			want:         1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				t.Setenv(tt.key, tt.value)
			}
			got := getDurationEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getDurationEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
