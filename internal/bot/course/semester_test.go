package course

import (
	"testing"
	"time"
)

// TestGetSemestersToSearchLive tests with current date (for verification only)
func TestGetSemestersToSearchLive(t *testing.T) {
	years, terms := getSemestersToSearch()

	// Basic validation
	if len(years) != 2 {
		t.Errorf("Expected 2 years, got %d", len(years))
	}
	if len(terms) != 2 {
		t.Errorf("Expected 2 terms, got %d", len(terms))
	}

	// Log current results for manual verification
	now := time.Now()
	t.Logf("Current date: %s (ROC year %d, month %d)",
		now.Format("2006-01-02"), now.Year()-1911, now.Month())
	t.Logf("Search semesters: %d-%d, %d-%d", years[0], terms[0], years[1], terms[1])
}

// TestSemesterDetectionLogic tests the semester detection logic for course queries
func TestSemesterDetectionLogic(t *testing.T) {
	tests := []struct {
		name          string
		year          int // Western year
		month         int
		expectedYear1 int // First search year (ROC)
		expectedTerm1 int // First search term
		expectedYear2 int // Second search year (ROC)
		expectedTerm2 int // Second search term
	}{
		{
			name:          "January (寒假期間，上學年第1學期)",
			year:          2025,
			month:         1,
			expectedYear1: 113, // 2024學年度 (2024/9~2025/1)
			expectedTerm1: 1,
			expectedYear2: 112,
			expectedTerm2: 2,
		},
		{
			name:          "February (下學期開始)",
			year:          2025,
			month:         2,
			expectedYear1: 113, // 2024學年度下學期 (2025/2~2025/6)
			expectedTerm1: 2,
			expectedYear2: 113,
			expectedTerm2: 1,
		},
		{
			name:          "March (下學期進行中)",
			year:          2025,
			month:         3,
			expectedYear1: 113,
			expectedTerm1: 2,
			expectedYear2: 113,
			expectedTerm2: 1,
		},
		{
			name:          "June (下學期結束)",
			year:          2025,
			month:         6,
			expectedYear1: 113,
			expectedTerm1: 2,
			expectedYear2: 113,
			expectedTerm2: 1,
		},
		{
			name:          "July (暑假期間)",
			year:          2025,
			month:         7,
			expectedYear1: 113,
			expectedTerm1: 2,
			expectedYear2: 113,
			expectedTerm2: 1,
		},
		{
			name:          "August (暑假期間)",
			year:          2025,
			month:         8,
			expectedYear1: 113,
			expectedTerm1: 2,
			expectedYear2: 113,
			expectedTerm2: 1,
		},
		{
			name:          "September (新學年第1學期開始)",
			year:          2025,
			month:         9,
			expectedYear1: 114, // 2025學年度 (2025/9~2026/1)
			expectedTerm1: 1,
			expectedYear2: 113,
			expectedTerm2: 2,
		},
		{
			name:          "November (上學期進行中)",
			year:          2025,
			month:         11,
			expectedYear1: 114,
			expectedTerm1: 1,
			expectedYear2: 113,
			expectedTerm2: 2,
		},
		{
			name:          "December (上學期進行中)",
			year:          2025,
			month:         12,
			expectedYear1: 114,
			expectedTerm1: 1,
			expectedYear2: 113,
			expectedTerm2: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a time object for the test case
			testDate := time.Date(tt.year, time.Month(tt.month), 15, 0, 0, 0, 0, time.Local)

			searchYears, searchTerms := getSemestersForDate(testDate)

			// Verify results
			if searchYears[0] != tt.expectedYear1 {
				t.Errorf("Year 1: expected %d, got %d", tt.expectedYear1, searchYears[0])
			}
			if searchTerms[0] != tt.expectedTerm1 {
				t.Errorf("Term 1: expected %d, got %d", tt.expectedTerm1, searchTerms[0])
			}
			if searchYears[1] != tt.expectedYear2 {
				t.Errorf("Year 2: expected %d, got %d", tt.expectedYear2, searchYears[1])
			}
			if searchTerms[1] != tt.expectedTerm2 {
				t.Errorf("Term 2: expected %d, got %d", tt.expectedTerm2, searchTerms[1])
			}
		})
	}
}

// TestCurrentMonth tests with actual current time (for debugging purposes)
func TestCurrentMonth(t *testing.T) {
	now := time.Now()
	currentYear := now.Year() - 1911
	currentMonth := int(now.Month())

	t.Logf("Current date: %s", now.Format("2006-01-02"))
	t.Logf("Current ROC year: %d", currentYear)
	t.Logf("Current month: %d", currentMonth)

	searchYears, searchTerms := getSemestersForDate(now)

	t.Logf("Search semesters: %d-%d, %d-%d", searchYears[0], searchTerms[0], searchYears[1], searchTerms[1])
}
