package genai

import (
	"context"
	"strings"
	"testing"
)

func TestQueryExpansionPrompt(t *testing.T) {
	t.Parallel()
	query := "我想學 AWS"
	prompt := QueryExpansionPrompt(query)

	// Check that prompt contains essential elements
	// Prompt should contain Chinese step instructions (步驟, 擴展, etc.)
	if !strings.Contains(prompt, "步驟") && !strings.Contains(prompt, "擴展") {
		t.Error("Prompt should contain Chinese instructions (步驟 or 擴展)")
	}
	// Prompt should contain expansion examples
	if !strings.Contains(prompt, "AI") && !strings.Contains(prompt, "行銷") {
		t.Error("Prompt should contain expansion examples")
	}
	// Prompt should contain the original query
	if !strings.Contains(prompt, query) {
		t.Error("Prompt should contain the original query")
	}
}

func TestQueryExpanderNil(t *testing.T) {
	t.Parallel()
	var e *geminiQueryExpander
	result, err := e.Expand(context.Background(), "test query")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test query" {
		t.Errorf("Expand() = %q, want %q", result, "test query")
	}
}
