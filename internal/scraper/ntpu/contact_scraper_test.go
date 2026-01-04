package ntpu

import (
	"net/url"
	"testing"
)

// TestEncodeToBig5 tests the Big5 encoding function
func TestEncodeToBig5(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		hasError bool
		checkVal string
	}{
		{
			name:     "Valid ASCII",
			input:    "test",
			hasError: false,
		},
		{
			name:     "Valid Chinese",
			input:    "王", // Specific test case for PR review
			hasError: false,
			checkVal: "%A4%FD",
		},
		{
			name:     "General Chinese",
			input:    "測試",
			hasError: false,
		},
		{
			name:     "Mixed ASCII and Chinese",
			input:    "test測試123",
			hasError: false,
		},
		{
			name:     "Empty string",
			input:    "",
			hasError: false,
		},
		{
			name:     "Special characters",
			input:    "!@#$%^&*()",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := encodeToBig5(tt.input)
			if tt.hasError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.hasError && result == "" && tt.input != "" {
				t.Error("Expected non-empty result")
			}

			// Verify specific value if provided (PR requirement)
			if tt.checkVal != "" {
				escaped := url.QueryEscape(result)
				if escaped != tt.checkVal {
					t.Errorf("Expected encoded value %s, got %s", tt.checkVal, escaped)
				}
			}
		})
	}
}

// TestGenerateUID tests the UID generation function
func TestGenerateUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "Two parts",
			parts:    []string{"行政單位", "教務處"},
			expected: "行政單位_教務處",
		},
		{
			name:     "Three parts",
			parts:    []string{"學術單位", "資訊工程學系", "資訊組"},
			expected: "學術單位_資訊工程學系_資訊組",
		},
		{
			name:     "Single part",
			parts:    []string{"單一部門"},
			expected: "單一部門",
		},
		{
			name:     "With empty string",
			parts:    []string{"行政單位", ""},
			expected: "行政單位_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := generateUID(tt.parts...)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// BenchmarkEncodeToBig5 benchmarks the Big5 encoding function
func BenchmarkEncodeToBig5(b *testing.B) {
	testString := "測試字串test123"
	for b.Loop() {
		_, _ = encodeToBig5(testString)
	}
}

// BenchmarkGenerateUID benchmarks the UID generation function
func BenchmarkGenerateUID(b *testing.B) {
	for b.Loop() {
		_ = generateUID("學術單位", "資訊工程學系")
	}
}

// TestBuildContactSearchURL tests the URL building function for contact search
func TestBuildContactSearchURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		searchTerm string
		wantEmpty  bool
		contains   string // substring that should be in the URL
	}{
		{
			name:       "Valid Chinese name",
			searchTerm: "王",
			wantEmpty:  false,
			contains:   "https://sea.cc.ntpu.edu.tw",
		},
		{
			name:       "Valid ASCII",
			searchTerm: "test",
			wantEmpty:  false,
			contains:   "?q=",
		},
		{
			name:       "Empty string",
			searchTerm: "",
			wantEmpty:  false, // Empty string should still generate URL
			contains:   "?q=",
		},
		{
			name:       "Chinese name with multiple characters",
			searchTerm: "王小明",
			wantEmpty:  false,
			contains:   "https://sea.cc.ntpu.edu.tw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := BuildContactSearchURL(tt.searchTerm)

			if tt.wantEmpty && result != "" {
				t.Errorf("Expected empty URL, got %s", result)
			}
			if !tt.wantEmpty && result == "" {
				t.Error("Expected non-empty URL, got empty")
			}
			if tt.contains != "" && result != "" {
				if !containsSubstring(result, tt.contains) {
					t.Errorf("Expected URL to contain %q, got %s", tt.contains, result)
				}
			}
		})
	}
}

// containsSubstring is a helper to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
