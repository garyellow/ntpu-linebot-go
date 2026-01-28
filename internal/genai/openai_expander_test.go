package genai

import (
	"context"
	"testing"
)

func TestNewOpenAIQueryExpander_NilWithEmptyKey(t *testing.T) {
	t.Parallel()
	expander, err := newOpenAIQueryExpander(context.Background(), ProviderGroq, "", "", "")
	if err != nil {
		t.Errorf("Expected nil error for empty key, got: %v", err)
	}
	if expander != nil {
		t.Error("Expected nil expander for empty key")
	}
}

func TestNewOpenAIQueryExpander_ValidKey(t *testing.T) {
	t.Parallel()
	// Test with mock API key (won't make actual API calls)
	expander, err := newOpenAIQueryExpander(context.Background(), ProviderGroq, "test-api-key", "llama-3.1-8b-instant", "")
	if err != nil {
		t.Fatalf("Expected no error for valid config, got: %v", err)
	}
	if expander == nil {
		t.Fatal("Expected non-nil expander")
		return
	}
	if expander.provider != ProviderGroq {
		t.Errorf("Expected provider %v, got %v", ProviderGroq, expander.provider)
	}
	if expander.model != "llama-3.1-8b-instant" {
		t.Errorf("Expected model llama-3.1-8b-instant, got %v", expander.model)
	}
}

func TestNewOpenAIQueryExpander_Cerebras(t *testing.T) {
	t.Parallel()
	expander, err := newOpenAIQueryExpander(context.Background(), ProviderCerebras, "test-key", "llama-3.3-70b", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if expander == nil {
		t.Fatal("Expected non-nil expander")
		return
	}
	if expander.provider != ProviderCerebras {
		t.Errorf("Expected provider %v, got %v", ProviderCerebras, expander.provider)
	}
}

func TestOpenAIQueryExpander_ExpandNil(t *testing.T) {
	t.Parallel()
	var nilExpander *openaiQueryExpander
	result, err := nilExpander.Expand(context.Background(), "test query")
	if err != nil {
		t.Errorf("Expand() on nil expander should return original query without error, got error: %v", err)
	}
	if result != "test query" {
		t.Errorf("Expand() = %q, want %q", result, "test query")
	}
}

func TestOpenAIQueryExpander_Provider(t *testing.T) {
	t.Parallel()

	// nil expander
	var nilExpander *openaiQueryExpander
	if nilExpander.Provider() != "" {
		t.Error("nil expander should return empty string for Provider")
	}

	// expander with provider
	expander := &openaiQueryExpander{provider: ProviderGroq}
	if expander.Provider() != ProviderGroq {
		t.Errorf("Expected provider %v, got %v", ProviderGroq, expander.Provider())
	}

	expander2 := &openaiQueryExpander{provider: ProviderCerebras}
	if expander2.Provider() != ProviderCerebras {
		t.Errorf("Expected provider %v, got %v", ProviderCerebras, expander2.Provider())
	}
}

func TestOpenAIQueryExpander_Close(t *testing.T) {
	t.Parallel()

	// nil expander - should not panic
	var nilExpander *openaiQueryExpander
	err := nilExpander.Close()
	if err != nil {
		t.Errorf("Close on nil expander should return nil, got: %v", err)
	}

	// expander with valid client
	expander, _ := newOpenAIQueryExpander(context.Background(), ProviderGroq, "test-key", "", "")
	if expander != nil {
		err = expander.Close()
		if err != nil {
			t.Errorf("Close should return nil, got: %v", err)
		}
	}
}

func TestOpenAIQueryExpander_ExpandWithCancellation(t *testing.T) {
	t.Parallel()

	expander, err := newOpenAIQueryExpander(context.Background(), ProviderGroq, "test-key", "", "")
	if err != nil {
		t.Fatalf("Failed to create expander: %v", err)
	}
	if expander == nil {
		t.Skip("Expander is nil, skipping test")
	}

	// Create already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := expander.Expand(ctx, "test query")

	// On cancellation, should return original query
	if result != "test query" {
		t.Errorf("Expected original query on error, got: %q", result)
	}

	// Error should be present (context cancellation)
	if err == nil {
		t.Error("Expected error for canceled context")
	}
}

func TestOpenAIQueryExpander_ExpandEmptyQuery(t *testing.T) {
	t.Parallel()

	expander, err := newOpenAIQueryExpander(context.Background(), ProviderGroq, "test-key", "", "")
	if err != nil {
		t.Fatalf("Failed to create expander: %v", err)
	}
	if expander == nil {
		t.Skip("Expander is nil, skipping test")
	}

	// Empty query should return empty
	result, err := expander.Expand(context.Background(), "")
	if err != nil && err != context.Canceled {
		t.Logf("Got error for empty query: %v (acceptable)", err)
	}
	if result != "" {
		t.Errorf("Expected empty result for empty query, got: %q", result)
	}
}
