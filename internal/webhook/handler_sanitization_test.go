package webhook

import (
	"strings"
	"testing"
)

// TestNormalizeWhitespace tests whitespace normalization
func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Single space", "hello world", "hello world"},
		{"Multiple spaces", "hello  world", "hello world"},
		{"Tabs", "hello\tworld", "hello world"},
		{"Newlines", "hello\nworld", "hello world"},
		{"Mixed whitespace", "hello \t\n world", "hello world"},
		{"Leading/trailing spaces", "  hello world  ", "hello world"},
		{"CJK with spaces", "你好  世界", "你好 世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWhitespace(tt.input)
			if got != tt.want {
				t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestRemovePunctuation tests punctuation removal (matching Python regex)
func TestRemovePunctuation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Basic punctuation
		{"Brackets", "[test]", "test"},
		{"Exclamation", "hello!", "hello"},
		{"Question", "hello?", "hello"},
		{"Comma", "hello, world", "hello world"},
		{"Period", "hello.", "hello"},
		{"Colon/semicolon", "hello:world;", "helloworld"},

		// Mixed punctuation
		{"Multiple punctuation", "hello!@#$%world", "helloworld"},
		{"Parentheses", "(hello)(world)", "helloworld"},
		{"Quotes", "\"hello\" 'world'", "hello world"},

		// CJK characters should be preserved
		{"CJK only", "你好世界", "你好世界"},
		{"CJK with punctuation", "你好！世界？", "你好世界"},
		{"Mixed CJK and English", "Hello你好World世界", "Hello你好World世界"},

		// Alphanumeric should be preserved
		{"Numbers", "test123", "test123"},
		{"Mixed alphanumeric", "abc123def456", "abc123def456"},

		// Empty and whitespace
		{"Empty", "", ""},
		{"Only spaces", "   ", "   "},
		{"Only punctuation", "!@#$%", ""},

		// Real user queries (matching Python behavior)
		{"Student ID with keyword", "學號 412345678", "學號 412345678"},
		{"Course with punctuation", "課程：微積分", "課程微積分"},
		{"Contact with comma", "聯絡,資工系", "聯絡資工系"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removePunctuation(tt.input)
			if got != tt.want {
				t.Errorf("removePunctuation(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSanitizationPipeline tests the combined effect of normalization + punctuation removal
func TestSanitizationPipeline(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Real user queries
		{"Query with extra spaces", "學號  412345678", "學號 412345678"},
		{"Query with punctuation", "課程：微積分！", "課程微積分"},
		// Multiple spaces may remain after punctuation removal; handler logic handles final normalization
		{"Query with mixed whitespace and punctuation", "  聯絡 ,  資工系  ", "聯絡  資工系"},
		{"Complex query", "  老師  「王小明」  ", "老師 王小明"},

		// Edge cases (final trim happens in handler logic, not in sanitization functions)
		{"Only punctuation and spaces", "  !!!  ???  ", ""}, // Empty after trim
		{"Empty after sanitization", "   【】   ", ""},        // Empty after trim
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply the full pipeline (matching webhook handler logic)
			step1 := normalizeWhitespace(tt.input)
			step2 := removePunctuation(step1)
			got := strings.TrimSpace(step2) // Final trim (as done in handler)

			if got != tt.want {
				t.Errorf("Pipeline(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
