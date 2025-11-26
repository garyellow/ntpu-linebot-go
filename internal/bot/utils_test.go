package bot

import (
	"testing"
)

func TestBuildKeywordRegex(t *testing.T) {
	tests := []struct {
		name     string
		keywords []string
		input    string
		expected string
	}{
		{
			name:     "Empty keywords",
			keywords: []string{},
			input:    "課程 微積分",
			expected: "",
		},
		{
			name:     "Single keyword",
			keywords: []string{"課程"},
			input:    "課程 微積分",
			expected: "課程",
		},
		{
			name:     "Multiple keywords - longest first",
			keywords: []string{"課", "課程", "課名"},
			input:    "課程 微積分",
			expected: "課程", // Should match "課程" not "課"
		},
		{
			name:     "Case insensitive",
			keywords: []string{"course", "class"},
			input:    "Course Name",
			expected: "Course",
		},
		{
			name:     "No match",
			keywords: []string{"老師", "教師"},
			input:    "課程 微積分",
			expected: "",
		},
		{
			name:     "Chinese keywords sorted by length",
			keywords: []string{"聯繫", "聯絡", "聯繫方式", "聯絡方式"},
			input:    "聯絡方式 資工系",
			expected: "聯絡方式", // Longest match first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regex := BuildKeywordRegex(tt.keywords)
			got := regex.FindString(tt.input)
			if got != tt.expected {
				t.Errorf("BuildKeywordRegex(%v).FindString(%q) = %q, want %q",
					tt.keywords, tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractSearchTerm(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		keyword  string
		expected string
	}{
		{
			name:     "Keyword at beginning",
			text:     "課程 微積分",
			keyword:  "課程",
			expected: "微積分",
		},
		{
			name:     "Keyword at beginning with extra space",
			text:     "課程  微積分",
			keyword:  "課程",
			expected: "微積分",
		},
		{
			name:     "Keyword at end",
			text:     "微積分課程",
			keyword:  "課程",
			expected: "微積分",
		},
		{
			name:     "Keyword in middle",
			text:     "查詢課程微積分",
			keyword:  "課程",
			expected: "查詢微積分",
		},
		{
			name:     "Empty keyword",
			text:     "課程 微積分",
			keyword:  "",
			expected: "課程 微積分",
		},
		{
			name:     "Keyword not in text",
			text:     "微積分",
			keyword:  "課程",
			expected: "微積分",
		},
		{
			name:     "Only keyword",
			text:     "課程",
			keyword:  "課程",
			expected: "",
		},
		{
			name:     "Keyword with spaces around",
			text:     "  課程 微積分  ",
			keyword:  "課程",
			expected: "微積分",
		},
		{
			name:     "English keyword at beginning",
			text:     "teacher John",
			keyword:  "teacher",
			expected: "John",
		},
		{
			name:     "English keyword at end",
			text:     "John teacher",
			keyword:  "teacher",
			expected: "John",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSearchTerm(tt.text, tt.keyword)
			if got != tt.expected {
				t.Errorf("ExtractSearchTerm(%q, %q) = %q, want %q",
					tt.text, tt.keyword, got, tt.expected)
			}
		})
	}
}
