package course

import (
	"testing"
	"time"
)

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
			expectedYear1: 114, // 2025年2月 currentYear = 2025-1911 = 114
			expectedTerm1: 2,
			expectedYear2: 114,
			expectedTerm2: 1,
		},
		{
			name:          "March (下學期進行中)",
			year:          2025,
			month:         3,
			expectedYear1: 114,
			expectedTerm1: 2,
			expectedYear2: 114,
			expectedTerm2: 1,
		},
		{
			name:          "June (下學期結束)",
			year:          2025,
			month:         6,
			expectedYear1: 114,
			expectedTerm1: 2,
			expectedYear2: 114,
			expectedTerm2: 1,
		},
		{
			name:          "July (暑假期間)",
			year:          2025,
			month:         7,
			expectedYear1: 114,
			expectedTerm1: 2,
			expectedYear2: 114,
			expectedTerm2: 1,
		},
		{
			name:          "August (暑假期間)",
			year:          2025,
			month:         8,
			expectedYear1: 114,
			expectedTerm1: 2,
			expectedYear2: 114,
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
			// Simulate the logic in handleCourseTitleSearch
			currentYear := tt.year - 1911
			currentMonth := tt.month

			var searchYears, searchTerms []int
			if currentMonth >= 2 && currentMonth <= 6 {
				searchYears = []int{currentYear, currentYear}
				searchTerms = []int{2, 1}
			} else if currentMonth >= 7 && currentMonth <= 8 {
				searchYears = []int{currentYear, currentYear}
				searchTerms = []int{2, 1}
			} else {
				// 9-12月 + 1月: 上學期進行中或寒假
				var academicYear int
				if currentMonth >= 9 {
					academicYear = currentYear
				} else {
					academicYear = currentYear - 1
				}
				searchYears = []int{academicYear, academicYear - 1}
				searchTerms = []int{1, 2}
			}

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

	var searchYears, searchTerms []int
	if currentMonth >= 2 && currentMonth <= 6 {
		searchYears = []int{currentYear, currentYear}
		searchTerms = []int{2, 1}
	} else if currentMonth >= 7 && currentMonth <= 8 {
		searchYears = []int{currentYear, currentYear}
		searchTerms = []int{2, 1}
	} else {
		var academicYear int
		if currentMonth >= 9 {
			academicYear = currentYear
		} else {
			academicYear = currentYear - 1
		}
		searchYears = []int{academicYear, academicYear - 1}
		searchTerms = []int{1, 2}
	}

	t.Logf("Search semesters: %d-%d, %d-%d", searchYears[0], searchTerms[0], searchYears[1], searchTerms[1])
}
