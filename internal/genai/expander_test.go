package genai

import (
	"context"
	"strings"
	"testing"
)

func TestContainsAbbreviation(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"AWS", true},
		{"aws", true},
		{"我想學 AWS", true},
		{"AI 課程", true},
		{"機器學習 ML", true},
		{"程式設計", false},
		{"微積分", false},
		{"data analysis", false},
		{"LLM RAG", true},
		{"SQL database", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := containsAbbreviation(tt.query)
			if result != tt.expected {
				t.Errorf("containsAbbreviation(%q) = %v, want %v", tt.query, result, tt.expected)
			}
		})
	}
}

func TestBuildExpansionPrompt(t *testing.T) {
	query := "我想學 AWS"
	prompt := buildExpansionPrompt(query)

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
	var e *QueryExpander
	result, err := e.Expand(context.Background(), "test query")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test query" {
		t.Errorf("Expand() = %q, want %q", result, "test query")
	}
}
