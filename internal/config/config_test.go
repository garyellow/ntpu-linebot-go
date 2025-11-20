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

	if cfg.ScraperWorkers != 5 {
		t.Errorf("Expected default workers 5, got %d", cfg.ScraperWorkers)
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
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				SQLitePath:        "/data/cache.db",
				ScraperWorkers:    5,
				ScraperMaxRetries: 3,
			},
			wantErr: false,
		},
		{
			name: "missing token",
			cfg: &Config{
				LineChannelSecret: "secret",
				Port:              "10000",
				SQLitePath:        "/data/cache.db",
				ScraperWorkers:    5,
			},
			wantErr: true,
		},
		{
			name: "invalid workers",
			cfg: &Config{
				LineChannelToken:  "token",
				LineChannelSecret: "secret",
				Port:              "10000",
				SQLitePath:        "/data/cache.db",
				ScraperWorkers:    0,
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
