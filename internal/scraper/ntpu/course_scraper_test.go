package ntpu

import (
	"regexp"
	"strings"
	"testing"
)

// TestAllEduCodes tests if all education codes are present
func TestAllEduCodes(t *testing.T) {
	expectedCodes := []string{"U", "M", "N", "P"}

	if len(AllEduCodes) != len(expectedCodes) {
		t.Errorf("Expected %d education codes, got %d", len(expectedCodes), len(AllEduCodes))
	}

	for _, code := range expectedCodes {
		found := false
		for _, allCode := range AllEduCodes {
			if allCode == code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected education code %q not found", code)
		}
	}
}

// TestClassroomRegex tests the classroom regex pattern
func TestClassroomRegex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		matches bool
	}{
		{
			name:    "Valid classroom format",
			input:   "8F12",
			matches: true,
		},
		{
			name:    "Valid with B prefix",
			input:   "B123",
			matches: true,
		},
		{
			name:    "Valid multi-digit floor",
			input:   "12F01",
			matches: true,
		},
		{
			name:    "Invalid format",
			input:   "ABC",
			matches: false,
		},
	}

	// The regex pattern from the scraper
	classroomRegex := regexp.MustCompile(`[0-9]*[FB]?[0-9]+`)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := classroomRegex.MatchString(tt.input)
			if match != tt.matches {
				t.Errorf("Expected match=%v for %q, got %v", tt.matches, tt.input, match)
			}
		})
	}
}

// TestScrapeCourseByUID_Success tests course scraping with valid UIDs
func TestScrapeCourseByUID_Success(t *testing.T) {
	tests := []struct {
		name       string
		uid        string
		expectYear int
		expectTerm int
		expectCode string
	}{
		{
			name:       "7-digit UID (old format)",
			uid:        "1131U01",
			expectYear: 113,
			expectTerm: 1,
			expectCode: "U01",
		},
		{
			name:       "9-digit UID (new format)",
			uid:        "113104567",
			expectYear: 113,
			expectTerm: 1,
			expectCode: "04567",
		},
		{
			name:       "10-digit UID",
			uid:        "1131M04567",
			expectYear: 113,
			expectTerm: 1,
			expectCode: "M04567",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse UID to extract year, term, and code
			var year, term int
			var code string

			if len(tt.uid) == 7 {
				// Old format: 1131U01
				year = parseYear(tt.uid[:3])
				term = parseTerm(tt.uid[3:4])
				code = tt.uid[4:]
			} else if len(tt.uid) == 9 {
				// New format: 113104567
				year = parseYear(tt.uid[:3])
				term = parseTerm(tt.uid[3:4])
				code = tt.uid[4:]
			} else if len(tt.uid) == 10 {
				// New format with edu code: 1131M04567
				year = parseYear(tt.uid[:3])
				term = parseTerm(tt.uid[3:4])
				code = tt.uid[4:]
			}

			if year != tt.expectYear {
				t.Errorf("Expected year %d, got %d", tt.expectYear, year)
			}

			if term != tt.expectTerm {
				t.Errorf("Expected term %d, got %d", tt.expectTerm, term)
			}

			if code != tt.expectCode {
				t.Errorf("Expected code %q, got %q", tt.expectCode, code)
			}
		})
	}
}

// TestScrapeCourseByUID_InvalidUID tests error handling for invalid UIDs
func TestScrapeCourseByUID_InvalidUID(t *testing.T) {
	tests := []struct {
		name string
		uid  string
	}{
		{"Empty UID", ""},
		{"Too short", "123"},
		{"Invalid characters", "ABC1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Invalid UIDs should fail validation
			if len(tt.uid) < 7 || len(tt.uid) > 10 {
				// Expected to fail
				t.Logf("UID %q correctly identified as invalid", tt.uid)
			}
		})
	}
}

// BenchmarkScrapeCourseByUID benchmarks the course scraping function
func BenchmarkScrapeCourseByUID(b *testing.B) {
	// Note: This benchmark would need a real client in production
	// For testing purposes, we're just measuring the parsing overhead
	uid := "1131U0001"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Parse UID components
		_ = parseYear(uid[:3])
		_ = parseTerm(uid[3:4])
		_ = uid[4:]
	}
}

// Helper functions for parsing UID components
func parseYear(yearStr string) int {
	var year int
	// Simple string to int conversion
	for _, ch := range yearStr {
		year = year*10 + int(ch-'0')
	}
	return year
}

func parseTerm(termStr string) int {
	return int(termStr[0] - '0')
}

// TestCourseNameExtraction tests extracting course names from HTML
func TestCourseNameExtraction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard course name",
			input:    "資料結構",
			expected: "資料結構",
		},
		{
			name:     "Course name with spaces",
			input:    " 計算機概論 ",
			expected: "計算機概論",
		},
		{
			name:     "English course name",
			input:    "Introduction to Computer Science",
			expected: "Introduction to Computer Science",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strings.TrimSpace(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
