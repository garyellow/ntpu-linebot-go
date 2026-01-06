package genai

import (
	"context"
	"testing"
)

func TestDefaultLLMConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultLLMConfig()

	// Check default provider order (slice-based)
	if len(cfg.Providers) != len(DefaultProviders) {
		t.Errorf("Providers length = %v, want %v", len(cfg.Providers), len(DefaultProviders))
	}
	for i, p := range cfg.Providers {
		if p != DefaultProviders[i] {
			t.Errorf("Providers[%d] = %v, want %v", i, p, DefaultProviders[i])
		}
	}

	// Check Gemini defaults (now using slice-based model chains)
	if len(cfg.Gemini.IntentModels) != len(DefaultGeminiIntentModels) {
		t.Errorf("Gemini.IntentModels length = %v, want %v", len(cfg.Gemini.IntentModels), len(DefaultGeminiIntentModels))
	}
	for i, model := range cfg.Gemini.IntentModels {
		if model != DefaultGeminiIntentModels[i] {
			t.Errorf("Gemini.IntentModels[%d] = %v, want %v", i, model, DefaultGeminiIntentModels[i])
		}
	}
	if len(cfg.Gemini.ExpanderModels) != len(DefaultGeminiExpanderModels) {
		t.Errorf("Gemini.ExpanderModels length = %v, want %v", len(cfg.Gemini.ExpanderModels), len(DefaultGeminiExpanderModels))
	}

	// Check Groq defaults
	if len(cfg.Groq.IntentModels) != len(DefaultGroqIntentModels) {
		t.Errorf("Groq.IntentModels length = %v, want %v", len(cfg.Groq.IntentModels), len(DefaultGroqIntentModels))
	}
	if len(cfg.Groq.ExpanderModels) != len(DefaultGroqExpanderModels) {
		t.Errorf("Groq.ExpanderModels length = %v, want %v", len(cfg.Groq.ExpanderModels), len(DefaultGroqExpanderModels))
	}

	// Check Cerebras defaults
	if len(cfg.Cerebras.IntentModels) != len(DefaultCerebrasIntentModels) {
		t.Errorf("Cerebras.IntentModels length = %v, want %v", len(cfg.Cerebras.IntentModels), len(DefaultCerebrasIntentModels))
	}
	if len(cfg.Cerebras.ExpanderModels) != len(DefaultCerebrasExpanderModels) {
		t.Errorf("Cerebras.ExpanderModels length = %v, want %v", len(cfg.Cerebras.ExpanderModels), len(DefaultCerebrasExpanderModels))
	}

	// Check retry config defaults
	if cfg.RetryConfig.MaxAttempts != DefaultMaxRetryAttempts {
		t.Errorf("RetryConfig.MaxAttempts = %v, want %v", cfg.RetryConfig.MaxAttempts, DefaultMaxRetryAttempts)
	}
	if cfg.RetryConfig.InitialDelay != DefaultInitialRetryDelay {
		t.Errorf("RetryConfig.InitialDelay = %v, want %v", cfg.RetryConfig.InitialDelay, DefaultInitialRetryDelay)
	}
	if cfg.RetryConfig.MaxDelay != DefaultMaxRetryDelay {
		t.Errorf("RetryConfig.MaxDelay = %v, want %v", cfg.RetryConfig.MaxDelay, DefaultMaxRetryDelay)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultRetryConfig()

	if cfg.MaxAttempts != DefaultMaxRetryAttempts {
		t.Errorf("MaxAttempts = %v, want %v", cfg.MaxAttempts, DefaultMaxRetryAttempts)
	}
	if cfg.InitialDelay != DefaultInitialRetryDelay {
		t.Errorf("InitialDelay = %v, want %v", cfg.InitialDelay, DefaultInitialRetryDelay)
	}
	if cfg.MaxDelay != DefaultMaxRetryDelay {
		t.Errorf("MaxDelay = %v, want %v", cfg.MaxDelay, DefaultMaxRetryDelay)
	}
}

func TestLLMConfig_HasAnyProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      LLMConfig
		expected bool
	}{
		{
			name:     "no providers",
			cfg:      LLMConfig{},
			expected: false,
		},
		{
			name: "gemini only",
			cfg: LLMConfig{
				Gemini: ProviderConfig{APIKey: "test-key"},
			},
			expected: true,
		},
		{
			name: "groq only",
			cfg: LLMConfig{
				Groq: ProviderConfig{APIKey: "test-key"},
			},
			expected: true,
		},
		{
			name: "both providers",
			cfg: LLMConfig{
				Gemini: ProviderConfig{APIKey: "gemini-key"},
				Groq:   ProviderConfig{APIKey: "groq-key"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.HasAnyProvider(); got != tt.expected {
				t.Errorf("HasAnyProvider() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLLMConfig_HasProvider(t *testing.T) {
	t.Parallel()
	cfg := LLMConfig{
		Gemini: ProviderConfig{APIKey: "gemini-key"},
	}

	if !cfg.HasProvider(ProviderGemini) {
		t.Error("HasProvider(Gemini) should return true")
	}
	if cfg.HasProvider(ProviderGroq) {
		t.Error("HasProvider(Groq) should return false")
	}
	if cfg.HasProvider("unknown") {
		t.Error("HasProvider(unknown) should return false")
	}
}

func TestLLMConfig_ConfiguredProviders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      LLMConfig
		expected []Provider
	}{
		{
			name: "gemini and groq configured",
			cfg: LLMConfig{
				Providers: DefaultProviders,
				Gemini:    ProviderConfig{APIKey: "gemini-key"},
				Groq:      ProviderConfig{APIKey: "groq-key"},
			},
			expected: []Provider{ProviderGemini, ProviderGroq},
		},
		{
			name: "groq only configured",
			cfg: LLMConfig{
				Providers: DefaultProviders,
				Groq:      ProviderConfig{APIKey: "groq-key"},
			},
			expected: []Provider{ProviderGroq},
		},
		{
			name: "all three configured",
			cfg: LLMConfig{
				Providers: DefaultProviders,
				Gemini:    ProviderConfig{APIKey: "gemini-key"},
				Groq:      ProviderConfig{APIKey: "groq-key"},
				Cerebras:  ProviderConfig{APIKey: "cerebras-key"},
			},
			expected: []Provider{ProviderGemini, ProviderGroq, ProviderCerebras},
		},
		{
			name: "none configured",
			cfg: LLMConfig{
				Providers: DefaultProviders,
			},
			expected: []Provider{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.cfg.ConfiguredProviders()
			if len(got) != len(tt.expected) {
				t.Errorf("ConfiguredProviders() length = %v, want %v", len(got), len(tt.expected))
				return
			}
			for i, p := range got {
				if p != tt.expected[i] {
					t.Errorf("ConfiguredProviders()[%d] = %v, want %v", i, p, tt.expected[i])
				}
			}
		})
	}
}

func TestCreateIntentParser_NoProviders(t *testing.T) {
	t.Parallel()
	cfg := DefaultLLMConfig()
	// No API keys set

	parser, err := CreateIntentParser(context.Background(), cfg)
	if err != nil {
		t.Errorf("CreateIntentParser() error = %v, want nil", err)
	}
	if parser != nil {
		t.Error("CreateIntentParser() should return nil when no providers configured")
	}
}

func TestCreateQueryExpander_NoProviders(t *testing.T) {
	t.Parallel()
	cfg := DefaultLLMConfig()
	// No API keys set

	expander, err := CreateQueryExpander(context.Background(), cfg)
	if err != nil {
		t.Errorf("CreateQueryExpander() error = %v, want nil", err)
	}
	if expander != nil {
		t.Error("CreateQueryExpander() should return nil when no providers configured")
	}
}

func TestProviderString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		provider Provider
		expected string
	}{
		{ProviderGemini, "gemini"},
		{ProviderGroq, "groq"},
		{Provider("custom"), "custom"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			t.Parallel()
			if got := tt.provider.String(); got != tt.expected {
				t.Errorf("Provider.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
