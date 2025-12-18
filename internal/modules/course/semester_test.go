package course

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestGetSemestersToSearchLive tests with current date (for verification only)
func TestGetSemestersToSearchLive(t *testing.T) {
	years, terms := getSemestersToSearch()

	// Now returns 2 semesters (default for user queries)
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
	t.Logf("Search semesters (2 recent): %d-%d, %d-%d",
		years[0], terms[0], years[1], terms[1])
}

// TestGetExtendedSemesters tests extended search (4 semesters)
func TestGetExtendedSemesters(t *testing.T) {
	years, terms := getExtendedSemesters()

	// Extended search returns 4 semesters
	if len(years) != 4 {
		t.Errorf("Expected 4 years for extended search, got %d", len(years))
	}
	if len(terms) != 4 {
		t.Errorf("Expected 4 terms for extended search, got %d", len(terms))
	}

	t.Logf("Extended search semesters (4): %d-%d, %d-%d, %d-%d, %d-%d",
		years[0], terms[0], years[1], terms[1], years[2], terms[2], years[3], terms[3])
}

// TestRecentSemestersForDate tests the 2-semester retrieval
func TestRecentSemestersForDate(t *testing.T) {
	tests := []struct {
		name     string
		year     int
		month    int
		expected []struct {
			year int
			term int
		}
	}{
		{
			name:  "March - returns 2 recent semesters",
			year:  2025,
			month: 3,
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1},
			},
		},
		{
			name:  "September - returns 2 recent semesters",
			year:  2025,
			month: 9,
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2},
			},
		},
		{
			name:  "January - returns 2 recent semesters",
			year:  2025,
			month: 1,
			expected: []struct{ year, term int }{
				{113, 1}, {112, 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDate := time.Date(tt.year, time.Month(tt.month), 15, 0, 0, 0, 0, time.Local)
			years, terms := getRecentSemestersForDate(testDate)

			if len(years) != 2 || len(terms) != 2 {
				t.Errorf("Expected 2 semesters, got %d years and %d terms", len(years), len(terms))
				return
			}

			for i := 0; i < 2; i++ {
				if years[i] != tt.expected[i].year || terms[i] != tt.expected[i].term {
					t.Errorf("Semester %d: expected %d-%d, got %d-%d",
						i+1, tt.expected[i].year, tt.expected[i].term, years[i], terms[i])
				}
			}
		})
	}
}

// TestSemesterDetectionLogic tests the semester detection logic for course queries
func TestSemesterDetectionLogic(t *testing.T) {
	tests := []struct {
		name     string
		year     int // Western year
		month    int
		expected []struct {
			year int
			term int
		}
	}{
		{
			name:  "January (寒假期間，上學年第1學期)",
			year:  2025,
			month: 1,
			// Jan 2025 → 113-1 started Sep 2024, so start from 113-1
			expected: []struct{ year, term int }{
				{113, 1}, {112, 2}, {112, 1}, {111, 2},
			},
		},
		{
			name:  "February (下學期開始)",
			year:  2025,
			month: 2,
			// Feb 2025 → 113-2 (Academic Year 113 started Sep 2024)
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1}, {112, 2}, {112, 1},
			},
		},
		{
			name:  "March (下學期進行中)",
			year:  2025,
			month: 3,
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1}, {112, 2}, {112, 1},
			},
		},
		{
			name:  "June (下學期結束)",
			year:  2025,
			month: 6,
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1}, {112, 2}, {112, 1},
			},
		},
		{
			name:  "July (暑假期間)",
			year:  2025,
			month: 7,
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1}, {112, 2}, {112, 1},
			},
		},
		{
			name:  "August (暑假期間)",
			year:  2025,
			month: 8,
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1}, {112, 2}, {112, 1},
			},
		},
		{
			name:  "September (新學年第1學期開始)",
			year:  2025,
			month: 9,
			// Sep 2025 → 114-1 (new academic year starting)
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2}, {113, 1}, {112, 2},
			},
		},
		{
			name:  "November (上學期進行中)",
			year:  2025,
			month: 11,
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2}, {113, 1}, {112, 2},
			},
		},
		{
			name:  "December (上學期進行中)",
			year:  2025,
			month: 12,
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2}, {113, 1}, {112, 2},
			},
		},
		{
			name:  "January next year (寒假)",
			year:  2026,
			month: 1,
			// Jan 2026 → 114-1 (started Sep 2025)
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2}, {113, 1}, {112, 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a time object for the test case
			testDate := time.Date(tt.year, time.Month(tt.month), 15, 0, 0, 0, 0, time.Local)

			searchYears, searchTerms := getSemestersForDate(testDate)

			// Verify we get 4 semesters
			if len(searchYears) != 4 || len(searchTerms) != 4 {
				t.Errorf("Expected 4 semesters, got %d years and %d terms", len(searchYears), len(searchTerms))
				return
			}

			// Verify each semester
			for i := 0; i < 4; i++ {
				if searchYears[i] != tt.expected[i].year {
					t.Errorf("Semester %d: expected year %d, got %d", i+1, tt.expected[i].year, searchYears[i])
				}
				if searchTerms[i] != tt.expected[i].term {
					t.Errorf("Semester %d: expected term %d, got %d", i+1, tt.expected[i].term, searchTerms[i])
				}
			}
		})
	}
}

// TestGenerateSemestersBackward tests the helper function
func TestGenerateSemestersBackward(t *testing.T) {
	tests := []struct {
		name      string
		startYear int
		startTerm int
		count     int
		expected  []struct{ year, term int }
	}{
		{
			name:      "Start from term 1",
			startYear: 114,
			startTerm: 1,
			count:     4,
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2}, {113, 1}, {112, 2},
			},
		},
		{
			name:      "Start from term 2",
			startYear: 113,
			startTerm: 2,
			count:     4,
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1}, {112, 2}, {112, 1},
			},
		},
		{
			name:      "Generate 6 semesters",
			startYear: 114,
			startTerm: 1,
			count:     6,
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2}, {113, 1}, {112, 2}, {112, 1}, {111, 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			years, terms := generateSemestersBackward(tt.startYear, tt.startTerm, tt.count)

			if len(years) != tt.count || len(terms) != tt.count {
				t.Errorf("Expected %d semesters, got %d years and %d terms", tt.count, len(years), len(terms))
				return
			}

			for i := 0; i < tt.count; i++ {
				if years[i] != tt.expected[i].year || terms[i] != tt.expected[i].term {
					t.Errorf("Semester %d: expected %d-%d, got %d-%d",
						i+1, tt.expected[i].year, tt.expected[i].term, years[i], terms[i])
				}
			}
		})
	}
}

// TestSemesterDetector tests the intelligent semester detection
func TestSemesterDetector(t *testing.T) {
	t.Run("DetectActiveSemesters with data in newest semester", func(t *testing.T) {
		// Mock count function that returns 1000 courses for all semesters
		mockCount := func(ctx context.Context, year, term int) (int, error) {
			return 1000, nil
		}

		detector := NewSemesterDetector(mockCount)
		years, _ := detector.DetectActiveSemesters(context.Background())

		// Should return calendar-based semesters since data exists
		if len(years) != 4 {
			t.Errorf("Expected 4 semesters, got %d", len(years))
		}
	})

	t.Run("DetectActiveSemesters with no data in newest semester", func(t *testing.T) {
		// Mock count function that returns 0 for newest semester
		callCount := 0
		mockCount := func(ctx context.Context, year, term int) (int, error) {
			callCount++
			if callCount == 1 {
				return 0, nil // First call (newest semester) returns 0
			}
			return 1000, nil
		}

		detector := NewSemesterDetector(mockCount)

		// Get expected base semesters first
		baseYears, baseTerms := getSemestersForDate(time.Now())

		years, terms := detector.DetectActiveSemesters(context.Background())

		// Should shift back by one semester since newest has no data
		expectedYears, expectedTerms := generateSemestersBackward(baseYears[1], baseTerms[1], 4)

		for i := 0; i < 4; i++ {
			if years[i] != expectedYears[i] || terms[i] != expectedTerms[i] {
				t.Errorf("Semester %d: expected %d-%d, got %d-%d",
					i+1, expectedYears[i], expectedTerms[i], years[i], terms[i])
			}
		}
	})

	t.Run("DetectActiveSemesters with nil count function", func(t *testing.T) {
		detector := NewSemesterDetector(nil)
		years, terms := detector.DetectActiveSemesters(context.Background())

		// Should return calendar-based semesters
		expectedYears, expectedTerms := getSemestersForDate(time.Now())

		for i := 0; i < 4; i++ {
			if years[i] != expectedYears[i] || terms[i] != expectedTerms[i] {
				t.Errorf("Semester %d: expected %d-%d, got %d-%d",
					i+1, expectedYears[i], expectedTerms[i], years[i], terms[i])
			}
		}
	})
}

// TestDetectWarmupSemesters tests the warmup semester detection
func TestDetectWarmupSemesters(t *testing.T) {
	t.Run("With data available", func(t *testing.T) {
		mockCount := func(ctx context.Context, year, term int) (int, error) {
			return 1000, nil
		}

		detector := NewSemesterDetector(mockCount)
		semesters := detector.DetectWarmupSemesters(context.Background())

		if len(semesters) != 4 {
			t.Errorf("Expected 4 semesters, got %d", len(semesters))
		}

		// Semesters should be in descending order (newest first)
		for i := 0; i < len(semesters)-1; i++ {
			curr := semesters[i].Year*10 + semesters[i].Term
			next := semesters[i+1].Year*10 + semesters[i+1].Term
			if curr <= next {
				t.Errorf("Semesters not in descending order: %v", semesters)
				break
			}
		}
	})

	t.Run("With no data in newest semester", func(t *testing.T) {
		mockCount := func(ctx context.Context, year, term int) (int, error) {
			return 0, nil // No data
		}

		detector := NewSemesterDetector(mockCount)
		semesters := detector.DetectWarmupSemesters(context.Background())

		if len(semesters) != 4 {
			t.Errorf("Expected 4 semesters, got %d", len(semesters))
		}
	})

	t.Run("Term 2 scenario - has term 1 data only", func(t *testing.T) {
		// Simulate February (term 2): term 2 has no data yet, but term 1 exists
		mockCount := func(ctx context.Context, year, term int) (int, error) {
			if term == 1 {
				return 1500, nil // Term 1 has data
			}
			return 0, nil // Term 2 has no data yet
		}

		detector := NewSemesterDetector(mockCount)
		semesters := detector.DetectWarmupSemesters(context.Background())

		if len(semesters) != 4 {
			t.Errorf("Expected 4 semesters, got %d", len(semesters))
		}

		// Log semesters for debugging
		t.Logf("Semesters returned: %v", semesters)
	})

	t.Run("Error handling", func(t *testing.T) {
		mockCount := func(ctx context.Context, year, term int) (int, error) {
			return 0, errors.New("database error")
		}

		detector := NewSemesterDetector(mockCount)
		semesters := detector.DetectWarmupSemesters(context.Background())

		// Should fall back to calendar-based semesters
		if len(semesters) != 4 {
			t.Errorf("Expected 4 semesters on error, got %d", len(semesters))
		}
	})

	t.Run("Calendar-based detection with data", func(t *testing.T) {
		// Returns data for all requested semesters
		mockCount := func(ctx context.Context, year, term int) (int, error) {
			// Mock returns data for any recent semester
			if year >= 112 {
				return 1000, nil
			}
			return 0, nil
		}

		detector := NewSemesterDetector(mockCount)
		semesters := detector.DetectWarmupSemesters(context.Background())

		if len(semesters) != 4 {
			t.Errorf("Expected 4 semesters, got %d", len(semesters))
		}

		// Verify all returned semesters are recent
		for _, sem := range semesters {
			if sem.Year < 112 {
				t.Errorf("Expected recent semester (>=112), got %d-%d", sem.Year, sem.Term)
			}
		}
	})
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

	t.Logf("Search semesters: %d-%d, %d-%d, %d-%d, %d-%d",
		searchYears[0], searchTerms[0], searchYears[1], searchTerms[1],
		searchYears[2], searchTerms[2], searchYears[3], searchTerms[3])
}
