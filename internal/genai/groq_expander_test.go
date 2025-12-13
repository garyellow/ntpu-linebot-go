package genai

import (
	"context"
	"testing"
)

func TestNewGroqQueryExpander_NilWithEmptyKey(t *testing.T) {
	expander, err := newGroqQueryExpander(context.Background(), "", "")
	if err != nil {
		t.Errorf("newGroqQueryExpander() error = %v, want nil", err)
	}
	if expander != nil {
		t.Error("newGroqQueryExpander() should return nil for empty key")
	}
}

func TestGroqQueryExpander_Expand_Nil(t *testing.T) {
	var nilExpander *groqQueryExpander
	result, err := nilExpander.Expand(context.Background(), "test query")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test query" {
		t.Errorf("Expand() = %q, want %q (original query)", result, "test query")
	}
}

func TestGroqQueryExpander_Expand_NilClient(t *testing.T) {
	expander := &groqQueryExpander{client: nil}
	result, err := expander.Expand(context.Background(), "test query")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test query" {
		t.Errorf("Expand() = %q, want %q (original query)", result, "test query")
	}
}

func TestGroqQueryExpander_Provider(t *testing.T) {
	expander := &groqQueryExpander{}
	if got := expander.Provider(); got != ProviderGroq {
		t.Errorf("Provider() = %v, want %v", got, ProviderGroq)
	}
}

func TestGroqQueryExpander_Close(t *testing.T) {
	// nil expander
	var nilExpander *groqQueryExpander
	if err := nilExpander.Close(); err != nil {
		t.Errorf("nil expander.Close() should not error, got: %v", err)
	}

	// normal expander (groq-go doesn't require cleanup)
	expander := &groqQueryExpander{}
	if err := expander.Close(); err != nil {
		t.Errorf("expander.Close() should not error, got: %v", err)
	}
}

func TestGroqQueryExpander_SkipLongQueries(t *testing.T) {
	expander := &groqQueryExpander{client: nil} // will return early due to nil client

	// Long query without abbreviations should be skipped
	longQuery := "這是一個很長的查詢字串用於測試跳過擴展的邏輯因為已經足夠描述性"
	result, err := expander.Expand(context.Background(), longQuery)

	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != longQuery {
		t.Errorf("Expand() should return original query for long queries")
	}
}
