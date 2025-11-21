package storage

import (
	"strings"
	"testing"
)

func TestSanitizeSearchTerm(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal text",
			input:    "資訊工程",
			expected: "資訊工程",
		},
		{
			name:     "text with wildcard %",
			input:    "test%value",
			expected: "test\\%value",
		},
		{
			name:     "text with wildcard _",
			input:    "test_value",
			expected: "test\\_value",
		},
		{
			name:     "text with backslash",
			input:    "test\\value",
			expected: "test\\\\value",
		},
		{
			name:     "multiple special characters",
			input:    "test%_value\\test",
			expected: "test\\%\\_value\\\\test",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "%_\\",
			expected: "\\%\\_\\\\",
		},
		{
			name:     "Chinese characters with special chars",
			input:    "資工%系_名\\稱",
			expected: "資工\\%系\\_名\\\\稱",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeSearchTerm(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeSearchTerm(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeSearchTermSQLInjection(t *testing.T) {
	// Test potential SQL injection attempts
	sqlInjectionTests := []string{
		"'; DROP TABLE students; --",
		"1' OR '1'='1",
		"admin'--",
		"' UNION SELECT * FROM users--",
	}

	for _, input := range sqlInjectionTests {
		t.Run("SQL injection: "+input, func(t *testing.T) {
			result := sanitizeSearchTerm(input)

			// Note: sanitizeSearchTerm preserves SQL keywords but escapes wildcards
			// Actual SQL injection protection comes from parameterized queries
			_ = result // Use result to avoid unused variable warning			// Verify wildcards are escaped
			if strings.Contains(input, "%") {
				if !strings.Contains(result, "\\%") {
					t.Error("Expected % to be escaped")
				}
			}
			if strings.Contains(input, "_") {
				if !strings.Contains(result, "\\_") {
					t.Error("Expected _ to be escaped")
				}
			}
		})
	}
}

func TestSanitizeSearchTermUnicode(t *testing.T) {
	// Test Unicode characters
	unicodeTests := []struct {
		name  string
		input string
	}{
		{"Chinese", "資訊工程學系"},
		{"Japanese", "情報工学科"},
		{"Korean", "정보공학과"},
		{"Emoji", "👨‍💻 程式設計"},
		{"Mixed", "Computer 科學 123"},
	}

	for _, tt := range unicodeTests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeSearchTerm(tt.input)
			// Unicode should pass through unchanged (unless it contains special chars)
			if !strings.Contains(result, "%") && !strings.Contains(result, "_") && !strings.Contains(result, "\\") {
				if result != tt.input {
					t.Errorf("Expected Unicode to pass through unchanged")
				}
			}
		})
	}
}

func TestSanitizeSearchTermPerformance(t *testing.T) {
	// Test with large input
	input := strings.Repeat("test%_value\\", 1000)
	result := sanitizeSearchTerm(input)

	// Verify all instances are escaped
	expectedOccurrences := 1000
	if strings.Count(result, "\\%") != expectedOccurrences {
		t.Errorf("Expected %d occurrences of \\%%, got %d", expectedOccurrences, strings.Count(result, "\\%"))
	}
	if strings.Count(result, "\\_") != expectedOccurrences {
		t.Errorf("Expected %d occurrences of \\_, got %d", expectedOccurrences, strings.Count(result, "\\_"))
	}
	// Backslash appears twice: once in original and once as escape
	if strings.Count(result, "\\\\") != expectedOccurrences {
		t.Errorf("Expected %d occurrences of \\\\, got %d", expectedOccurrences, strings.Count(result, "\\\\"))
	}
}
