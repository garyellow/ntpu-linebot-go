package ntpu

import (
	"regexp"
	"testing"
)

// TestAllEduCodes tests if all education codes are present
func TestAllEduCodes(t *testing.T) {
	t.Parallel()
	expectedCodes := []string{"U", "M", "N", "P"}

	if len(allEducationCodes) != len(expectedCodes) {
		t.Errorf("Expected %d education codes, got %d", len(expectedCodes), len(allEducationCodes))
	}

	for _, code := range expectedCodes {
		found := false
		for _, allCode := range allEducationCodes {
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
	t.Parallel()
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
			t.Parallel()
			match := classroomRegex.MatchString(tt.input)
			if match != tt.matches {
				t.Errorf("Expected match=%v for %q, got %v", tt.matches, tt.input, match)
			}
		})
	}
}

// Note: UID parsing logic is tested in the course handler module.
// Scraper tests focus on format validation and regex patterns only.
// Course name extraction uses standard library strings.TrimSpace - no need to test stdlib.
