package genai

import (
	"context"
	"testing"
)

func TestDefaultLLMConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultLLMConfig()

	// Check default provider order
	if cfg.PrimaryProvider != ProviderGemini {
		t.Errorf("PrimaryProvider = %v, want %v", cfg.PrimaryProvider, ProviderGemini)
	}
	if cfg.FallbackProvider != ProviderGroq {
		t.Errorf("FallbackProvider = %v, want %v", cfg.FallbackProvider, ProviderGroq)
	}

	// Check Gemini defaults
	if cfg.Gemini.IntentModel != DefaultGeminiIntentModel {
		t.Errorf("Gemini.IntentModel = %v, want %v", cfg.Gemini.IntentModel, DefaultGeminiIntentModel)
	}
	if cfg.Gemini.IntentFallbackModel != DefaultGeminiIntentFallbackModel {
		t.Errorf("Gemini.IntentFallbackModel = %v, want %v", cfg.Gemini.IntentFallbackModel, DefaultGeminiIntentFallbackModel)
	}
	if cfg.Gemini.ExpanderModel != DefaultGeminiExpanderModel {
		t.Errorf("Gemini.ExpanderModel = %v, want %v", cfg.Gemini.ExpanderModel, DefaultGeminiExpanderModel)
	}

	// Check Groq defaults
	if cfg.Groq.IntentModel != DefaultGroqIntentModel {
		t.Errorf("Groq.IntentModel = %v, want %v", cfg.Groq.IntentModel, DefaultGroqIntentModel)
	}
	if cfg.Groq.IntentFallbackModel != DefaultGroqIntentFallbackModel {
		t.Errorf("Groq.IntentFallbackModel = %v, want %v", cfg.Groq.IntentFallbackModel, DefaultGroqIntentFallbackModel)
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

func TestLLMConfig_GetFallbackProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cfg      LLMConfig
		expected Provider
	}{
		{
			name: "gemini primary with groq available",
			cfg: LLMConfig{
				PrimaryProvider: ProviderGemini,
				Gemini:          ProviderConfig{APIKey: "gemini-key"},
				Groq:            ProviderConfig{APIKey: "groq-key"},
			},
			expected: ProviderGroq,
		},
		{
			name: "groq primary with gemini available",
			cfg: LLMConfig{
				PrimaryProvider: ProviderGroq,
				Gemini:          ProviderConfig{APIKey: "gemini-key"},
				Groq:            ProviderConfig{APIKey: "groq-key"},
			},
			expected: ProviderGemini,
		},
		{
			name: "gemini primary without groq",
			cfg: LLMConfig{
				PrimaryProvider: ProviderGemini,
				Gemini:          ProviderConfig{APIKey: "gemini-key"},
			},
			expected: "",
		},
		{
			name: "groq primary without gemini",
			cfg: LLMConfig{
				PrimaryProvider: ProviderGroq,
				Groq:            ProviderConfig{APIKey: "groq-key"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.GetFallbackProvider(); got != tt.expected {
				t.Errorf("GetFallbackProvider() = %v, want %v", got, tt.expected)
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
