package genai

import (
	"context"
	"testing"
)

func TestNewGroqIntentParser_NilWithEmptyKey(t *testing.T) {
	parser, err := newGroqIntentParser(context.Background(), "", "")
	if err != nil {
		t.Errorf("newGroqIntentParser() error = %v, want nil", err)
	}
	if parser != nil {
		t.Error("newGroqIntentParser() should return nil for empty key")
	}
}

func TestGroqIntentParser_IsEnabled(t *testing.T) {
	// nil parser
	var nilParser *groqIntentParser
	if nilParser.IsEnabled() {
		t.Error("nil parser should return false for IsEnabled")
	}

	// parser with nil client
	parserWithNilClient := &groqIntentParser{client: nil}
	if parserWithNilClient.IsEnabled() {
		t.Error("parser with nil client should return false for IsEnabled")
	}
}

func TestGroqIntentParser_ParseNil(t *testing.T) {
	var nilParser *groqIntentParser
	_, err := nilParser.Parse(context.Background(), "test")
	if err == nil {
		t.Error("Parse() should return error for nil parser")
	}
}

func TestGroqIntentParser_Provider(t *testing.T) {
	parser := &groqIntentParser{}
	if got := parser.Provider(); got != ProviderGroq {
		t.Errorf("Provider() = %v, want %v", got, ProviderGroq)
	}
}

func TestGroqIntentParser_Close(t *testing.T) {
	// nil parser
	var nilParser *groqIntentParser
	if err := nilParser.Close(); err != nil {
		t.Errorf("nil parser.Close() should not error, got: %v", err)
	}

	// normal parser (groq-go doesn't require cleanup)
	parser := &groqIntentParser{}
	if err := parser.Close(); err != nil {
		t.Errorf("parser.Close() should not error, got: %v", err)
	}
}

func TestBuildGroqTools(t *testing.T) {
	tools := buildGroqTools()

	if len(tools) == 0 {
		t.Error("buildGroqTools() should return non-empty slice")
	}

	// Check that all tools have required fields
	for _, tool := range tools {
		if tool.Function.Name == "" {
			t.Error("tool function name should not be empty")
		}
		if tool.Function.Description == "" {
			t.Error("tool function description should not be empty")
		}
	}

	// Count should match IntentModuleMap
	expectedCount := len(IntentModuleMap)
	if len(tools) != expectedCount {
		t.Errorf("buildGroqTools() returned %d tools, want %d", len(tools), expectedCount)
	}
}
