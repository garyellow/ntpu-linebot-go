package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Set required environment variables
	t.Setenv("LINE_CHANNEL_ACCESS_TOKEN", "test_token")
	t.Setenv("LINE_CHANNEL_SECRET", "test_secret")

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
	t.Setenv("LINE_CHANNEL_ACCESS_TOKEN", "")
	t.Setenv("LINE_CHANNEL_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Error("Load() should fail when LINE credentials are missing")
	}
	if !contains(err.Error(), "LINE_CHANNEL_ACCESS_TOKEN") {
		t.Errorf("Load() error = %v, want error containing 'LINE_CHANNEL_ACCESS_TOKEN'", err)
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
			errContains: "LINE_CHANNEL_ACCESS_TOKEN",
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
			errContains: "LINE_CHANNEL_SECRET",
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
			errContains: "DATA_DIR",
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
			errContains: "SCRAPER_MAX_RETRIES",
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
			// Cannot use t.Parallel(): t.Setenv panics after t.Parallel().
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
