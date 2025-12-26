package ntpu

import (
	"net/url"
	"testing"
)

// TestEncodeToBig5 tests the Big5 encoding function
func TestEncodeToBig5(t *testing.T) {
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
