package genai

import (
	"context"
	"strings"
	"testing"
)

func TestQueryExpansionPrompt(t *testing.T) {
	query := "我想學 AWS"
	prompt := QueryExpansionPrompt(query)

	// Check that prompt contains essential elements
	// Prompt should contain Chinese instructions (規則, 任務, etc.)
	if !strings.Contains(prompt, "規則") {
		t.Error("Prompt should contain Chinese instructions")
	}
	// Prompt should contain English examples (AWS, AI, etc.)
	if !strings.Contains(prompt, "AWS") {
		t.Error("Prompt should contain English examples")
	}
	// Prompt should contain the original query
	if !strings.Contains(prompt, query) {
		t.Error("Prompt should contain the original query")
	}
}

func TestQueryExpanderNil(t *testing.T) {
	var e *geminiQueryExpander
	result, err := e.Expand(context.Background(), "test query")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test query" {
		t.Errorf("Expand() = %q, want %q", result, "test query")
	}
}
