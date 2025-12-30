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
	// Ensure no LINE credentials are set
	// Note: t.Parallel() runs this in parallel with others.
	// t.Setenv sets env vars for the test process (or test container?), but t.Parallel() implies
	// they run concurrently.
	// WAIT. t.Setenv in Go 1.17+ sets the env var for the *duration of the test*.
	// However, os.Setenv affects the whole process.
	// If tests run in parallel, one test verifying "MissingCredentials" (expecting unset)
	// while another sets "TOKEN", they might race if they run in the same process.
	// Go's t.Setenv handles this by NOT allowing t.Parallel() to mix with Setenv?
	// ACTUALLY: "If a test calls Setenv and T.Parallel, Setenv panics." (Go documentation)
	// SO I CANNOT USE t.Setenv AND t.Parallel() TOGETHER if I am modifying the same keys?
	// Let's check docs.
	// "TestContext.Setenv ... cannot be used with Parallel." -> This was true in older versions or specific contexts?
	// Go 1.22+ might allow it?
	// Checking recent docs: "You cannot call t.Setenv() on a test that has called t.Parallel()."
	// Correct.
	// So I CANNOT parallelize these tests if they use env vars.
	// Unless I mock the env loader.
	// But `Load()` calls `os.Getenv` directly?
	// Yes.
	// So I CANNOT add t.Parallel() to `config_test.go`.
	// I will convert to t.Setenv for cleanup safety, but invoke t.Parallel() IS NOT ALLOWED.
	//
	// Wait, I already added t.Parallel() to previous files.
	// Those files didn't use `os.Setenv`?
	// `config_test.go` DOES.
	// `app_test.go` did NOT use `os.Setenv`.
	//
	// So for `config_test.go`, I will REMOVE `t.Parallel()` and just use `t.Setenv`.
	// Or just keep it serial.
	// The instruction was "Audit... Applying t.Parallel()".
	// If I can't, I skip it.
	// I will just refactor to t.Setenv for cleanliness, but NOT add t.Parallel().

	// Wait, I can try to simply optimize `config_test.go` by refactoring `t.Setenv` which is better practice.
	// But I won't add `t.Parallel()`.

	// Ensure no LINE credentials are set
	// Since t.Setenv only affects the current test and restores afterwards, and we are running serially,
	// we just need to ensure we don't pick up dirty state.
	// Actually, if I use t.Setenv in TestLoad, it restores.
	// In TestLoad_MissingCredentials, I want them UNSET.
	// If the environment already has them (from system), this test might fail.
	// I should explicitly Unset them.
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
	t.Parallel() // This matches pure struct validation, so it IS safe for parallel.
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
	// This helper uses os.Setenv, so it CANNOT be parallel.
	// I will refactor it to use t.Setenv.
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
			// Cannot use t.Parallel() here because we are setting env vars in subtests
			// which might overlap if parallel?
			// Wait, if parent is NOT parallel, subtests CAN be parallel?
			// No, t.Setenv documentation says "panics if t.Parallel() has been called".
			// So I cannot use t.Parallel().
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
