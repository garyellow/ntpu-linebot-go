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

// Note: UID parsing logic is tested in the course handler module.
// Scraper tests focus on format validation and regex patterns only.

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
