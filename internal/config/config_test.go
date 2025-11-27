package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Set required environment variables
	_ = os.Setenv("LINE_CHANNEL_ACCESS_TOKEN", "test_token")
	_ = os.Setenv("LINE_CHANNEL_SECRET", "test_secret")
	defer func() { _ = os.Unsetenv("LINE_CHANNEL_ACCESS_TOKEN") }()
	defer func() { _ = os.Unsetenv("LINE_CHANNEL_SECRET") }()

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

	if cfg.ScraperMaxRetries != 5 {
		t.Errorf("Expected default max retries 5, got %d", cfg.ScraperMaxRetries)
	}
}

func TestLoadForMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        ValidationMode
		setupEnv    func()
		cleanupEnv  func()
		wantErr     bool
		errContains string
	}{
		{
			name: "server mode - valid config",
			mode: ServerMode,
			setupEnv: func() {
				_ = os.Setenv("LINE_CHANNEL_ACCESS_TOKEN", "test_token")
				_ = os.Setenv("LINE_CHANNEL_SECRET", "test_secret")
			},
			cleanupEnv: func() {
				_ = os.Unsetenv("LINE_CHANNEL_ACCESS_TOKEN")
				_ = os.Unsetenv("LINE_CHANNEL_SECRET")
			},
			wantErr: false,
		},
		{
			name: "server mode - missing credentials",
			mode: ServerMode,
			setupEnv: func() {
				// No LINE credentials set
			},
			cleanupEnv:  func() {},
			wantErr:     true,
			errContains: "LINE_CHANNEL_ACCESS_TOKEN",
		},
		{
			name: "warmup mode - no credentials required",
			mode: WarmupMode,
			setupEnv: func() {
				// No LINE credentials needed
			},
			cleanupEnv: func() {},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanupEnv()

			cfg, err := LoadForMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadForMode(%v) error = %v, wantErr %v", tt.mode, err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("LoadForMode() error = %v, want error containing %q", err, tt.errContains)
				}
			}

			if !tt.wantErr && cfg == nil {
				t.Error("LoadForMode() returned nil config without error")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				LineChannelToken:        "token",
				LineChannelSecret:       "secret",
				Port:                    "10000",
				SQLitePath:              "/data/cache.db",
				CacheTTL:                168 * time.Hour,
				SoftTTL:                 120 * time.Hour,
				ScraperTimeout:          60 * time.Second,
				ScraperMaxRetries:       3,
				WebhookTimeout:          25 * time.Second,
				UserRateLimitTokens:     10,
				UserRateLimitRefillRate: 0.33,
			},
			wantErr: false,
		},
		{
			name: "missing token",
			cfg: &Config{
				LineChannelSecret:       "secret",
				Port:                    "10000",
				SQLitePath:              "/data/cache.db",
				CacheTTL:                168 * time.Hour,
				SoftTTL:                 120 * time.Hour,
				ScraperTimeout:          60 * time.Second,
				ScraperMaxRetries:       3,
				WebhookTimeout:          25 * time.Second,
				UserRateLimitTokens:     10,
				UserRateLimitRefillRate: 0.33,
			},
			wantErr: true,
		},
		{
			name: "negative retries",
			cfg: &Config{
				LineChannelToken:        "token",
				LineChannelSecret:       "secret",
				Port:                    "10000",
				SQLitePath:              "/data/cache.db",
				CacheTTL:                168 * time.Hour,
				SoftTTL:                 120 * time.Hour,
				ScraperTimeout:          60 * time.Second,
				ScraperMaxRetries:       -1,
				WebhookTimeout:          25 * time.Second,
				UserRateLimitTokens:     10,
				UserRateLimitRefillRate: 0.33,
			},
			wantErr: true,
		},
		{
			name: "SoftTTL >= CacheTTL",
			cfg: &Config{
				LineChannelToken:        "token",
				LineChannelSecret:       "secret",
				Port:                    "10000",
				SQLitePath:              "/data/cache.db",
				CacheTTL:                168 * time.Hour,
				SoftTTL:                 168 * time.Hour, // Same as CacheTTL - invalid
				ScraperTimeout:          60 * time.Second,
				ScraperMaxRetries:       3,
				WebhookTimeout:          25 * time.Second,
				UserRateLimitTokens:     10,
				UserRateLimitRefillRate: 0.33,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateForMode(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		mode        ValidationMode
		wantErr     bool
		errContains string
	}{
		{
			name: "server mode - valid config",
			cfg: &Config{
				LineChannelToken:        "token",
				LineChannelSecret:       "secret",
				Port:                    "10000",
				SQLitePath:              "/data/cache.db",
				CacheTTL:                168 * time.Hour,
				SoftTTL:                 120 * time.Hour,
				ScraperTimeout:          60 * time.Second,
				ScraperMaxRetries:       3,
				WebhookTimeout:          25 * time.Second,
				UserRateLimitTokens:     10,
				UserRateLimitRefillRate: 0.33,
			},
			mode:    ServerMode,
			wantErr: false,
		},
		{
			name: "server mode - missing token",
			cfg: &Config{
				LineChannelSecret:       "secret",
				Port:                    "10000",
				SQLitePath:              "/data/cache.db",
				CacheTTL:                168 * time.Hour,
				SoftTTL:                 120 * time.Hour,
				ScraperTimeout:          60 * time.Second,
				ScraperMaxRetries:       3,
				WebhookTimeout:          25 * time.Second,
				UserRateLimitTokens:     10,
				UserRateLimitRefillRate: 0.33,
			},
			mode:        ServerMode,
			wantErr:     true,
			errContains: "LINE_CHANNEL_ACCESS_TOKEN",
		},
		{
			name: "warmup mode - missing LINE credentials OK",
			cfg: &Config{
				SQLitePath:        "/data/cache.db",
				CacheTTL:          168 * time.Hour,
				SoftTTL:           120 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
			},
			mode:    WarmupMode,
			wantErr: false,
		},
		{
			name: "warmup mode - missing SQLite path",
			cfg: &Config{
				CacheTTL:          168 * time.Hour,
				SoftTTL:           120 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: 3,
			},
			mode:        WarmupMode,
			wantErr:     true,
			errContains: "SQLITE_PATH",
		},
		{
			name: "warmup mode - negative retries",
			cfg: &Config{
				SQLitePath:        "/data/cache.db",
				CacheTTL:          168 * time.Hour,
				SoftTTL:           120 * time.Hour,
				ScraperTimeout:    60 * time.Second,
				ScraperMaxRetries: -1,
			},
			mode:        WarmupMode,
			wantErr:     true,
			errContains: "SCRAPER_MAX_RETRIES",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateForMode(tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForMode(%v) error = %v, wantErr %v", tt.mode, err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateForMode() error = %v, want error containing %q", err, tt.errContains)
				}
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

func TestGetDurationEnv(t *testing.T) {
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
				_ = os.Setenv(tt.key, tt.value)
				defer func() { _ = os.Unsetenv(tt.key) }()
			}

			got := getDurationEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getDurationEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
