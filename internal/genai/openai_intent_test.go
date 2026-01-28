package genai

import (
	"context"
	"testing"
)

func TestNewOpenAIIntentParser_NilWithEmptyKey(t *testing.T) {
	t.Parallel()
	parser, err := newOpenAIIntentParser(context.Background(), ProviderGroq, "", "", "")
	if err != nil {
		t.Errorf("Expected nil error for empty key, got: %v", err)
	}
	if parser != nil {
		t.Error("Expected nil parser for empty key")
	}
}

func TestNewOpenAIIntentParser_ValidKey(t *testing.T) {
	t.Parallel()
	// Test with mock API key (won't make actual API calls)
	parser, err := newOpenAIIntentParser(context.Background(), ProviderGroq, "test-api-key", "llama-3.3-70b", "")
	if err != nil {
		t.Fatalf("Expected no error for valid config, got: %v", err)
	}
	if parser == nil {
		t.Fatal("Expected non-nil parser")
		return
	}
	if parser.provider != ProviderGroq {
		t.Errorf("Expected provider %v, got %v", ProviderGroq, parser.provider)
	}
	if parser.model != "llama-3.3-70b" {
		t.Errorf("Expected model llama-3.3-70b, got %v", parser.model)
	}
}

func TestNewOpenAIIntentParser_Cerebras(t *testing.T) {
	t.Parallel()
	parser, err := newOpenAIIntentParser(context.Background(), ProviderCerebras, "test-key", "llama-3.3-70b", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if parser == nil {
		t.Fatal("Expected non-nil parser")
		return
	}
	if parser.provider != ProviderCerebras {
		t.Errorf("Expected provider %v, got %v", ProviderCerebras, parser.provider)
	}
	if parser.model != "llama-3.3-70b" {
		t.Errorf("Expected model llama-3.3-70b, got %v", parser.model)
	}
}

func TestNewOpenAIIntentParser_OpenAIRequiresEndpoint(t *testing.T) {
	t.Parallel()
	parser, err := newOpenAIIntentParser(context.Background(), ProviderOpenAI, "test-key", "gpt-4o-mini", "")
	if err == nil {
		t.Fatal("Expected error for missing OpenAI endpoint")
	}
	if parser != nil {
		t.Error("Expected nil parser on error")
	}
}

func TestNewOpenAIIntentParser_OpenAIRequiresModel(t *testing.T) {
	t.Parallel()
	parser, err := newOpenAIIntentParser(context.Background(), ProviderOpenAI, "test-key", "", "http://localhost:1234/v1/")
	if err == nil {
		t.Fatal("Expected error for missing OpenAI model")
	}
	if parser != nil {
		t.Error("Expected nil parser on error")
	}
}

func TestOpenAIIntentParser_ParseNil(t *testing.T) {
	t.Parallel()
	var nilParser *openaiIntentParser
	_, err := nilParser.Parse(context.Background(), "test")
	if err == nil {
		t.Error("Expected error for nil parser")
	}
	if err.Error() != "intent parser is nil" {
		t.Errorf("Expected 'intent parser is nil' error, got: %v", err)
	}
}

func TestOpenAIIntentParser_Provider(t *testing.T) {
	t.Parallel()

	// nil parser
	var nilParser *openaiIntentParser
	if nilParser.Provider() != "" {
		t.Error("nil parser should return empty string for Provider")
	}

	// parser with provider
	parser := &openaiIntentParser{provider: ProviderGroq}
	if parser.Provider() != ProviderGroq {
		t.Errorf("Expected provider %v, got %v", ProviderGroq, parser.Provider())
	}

	parser2 := &openaiIntentParser{provider: ProviderCerebras}
	if parser2.Provider() != ProviderCerebras {
		t.Errorf("Expected provider %v, got %v", ProviderCerebras, parser2.Provider())
	}
}

func TestOpenAIIntentParser_Close(t *testing.T) {
	t.Parallel()

	// nil parser - should not panic
	var nilParser *openaiIntentParser
	err := nilParser.Close()
	if err != nil {
		t.Errorf("Close on nil parser should return nil, got: %v", err)
	}

	// parser with valid client
	parser, _ := newOpenAIIntentParser(context.Background(), ProviderGroq, "test-key", "", "")
	if parser != nil {
		err = parser.Close()
		if err != nil {
			t.Errorf("Close should return nil, got: %v", err)
		}
	}
}

func TestOpenAIIntentParser_ParseWithCancellation(t *testing.T) {
	t.Parallel()

	parser, err := newOpenAIIntentParser(context.Background(), ProviderGroq, "test-key", "", "")
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	if parser == nil {
		t.Skip("Parser is nil, skipping test")
	}

	// Create already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = parser.Parse(ctx, "test query")
	if err == nil {
		t.Error("Expected error for canceled context")
	}
	// Should fail due to context cancellation before API call
	if err != context.Canceled {
		t.Logf("Got error: %v (expected context.Canceled, but API may not have been called)", err)
	}
}

func TestGetProviderEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		wantURL  string
		wantOK   bool
	}{
		{
			name:     "Groq provider",
			provider: ProviderGroq,
			wantURL:  "https://api.groq.com/openai/v1/",
			wantOK:   true,
		},
		{
			name:     "Cerebras provider",
			provider: ProviderCerebras,
			wantURL:  "https://api.cerebras.ai/v1/",
			wantOK:   true,
		},
		{
			name:     "Gemini provider (not OpenAI-compatible)",
			provider: ProviderGemini,
			wantURL:  "",
			wantOK:   false,
		},
		{
			name:     "Unknown provider",
			provider: Provider("unknown"),
			wantURL:  "",
			wantOK:   false,
		},
		{
			name:     "Empty provider",
			provider: Provider(""),
			wantURL:  "",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotURL, gotOK := ProviderEndpoint[tt.provider]
			if gotOK != tt.wantOK {
				t.Errorf("ProviderEndpoint[%v] ok = %v, want %v", tt.provider, gotOK, tt.wantOK)
			}
			if gotURL != tt.wantURL {
				t.Errorf("ProviderEndpoint[%v] = %q, want %q", tt.provider, gotURL, tt.wantURL)
			}
		})
	}
}

func TestProvider_IsOpenAICompatible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{
			name:     "Gemini is not OpenAI-compatible",
			provider: ProviderGemini,
			want:     false,
		},
		{
			name:     "Groq is OpenAI-compatible",
			provider: ProviderGroq,
			want:     true,
		},
		{
			name:     "Cerebras is OpenAI-compatible",
			provider: ProviderCerebras,
			want:     true,
		},
		{
			name:     "Unknown provider is not OpenAI-compatible",
			provider: Provider("unknown"),
			want:     false,
		},
		{
			name:     "Empty provider is not OpenAI-compatible",
			provider: Provider(""),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.provider.IsOpenAICompatible()
			if got != tt.want {
				t.Errorf("Provider.IsOpenAICompatible() = %v, want %v", got, tt.want)
			}
		})
	}
}
