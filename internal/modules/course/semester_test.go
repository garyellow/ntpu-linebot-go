package course

import (
	"testing"
	"time"
)

// TestSemesterCache tests the SemesterCache functionality
func TestSemesterCache(t *testing.T) {
	t.Parallel()

	t.Run("NewSemesterCache creates empty cache", func(t *testing.T) {
		t.Parallel()
		cache := NewSemesterCache()
		if cache.HasData() {
			t.Error("Expected new cache to have no data")
		}
	})

	t.Run("Update stores semesters", func(t *testing.T) {
		t.Parallel()
		cache := NewSemesterCache()
		semesters := []Semester{
			{Year: 114, Term: 2},
			{Year: 114, Term: 1},
			{Year: 113, Term: 2},
			{Year: 113, Term: 1},
		}
		cache.Update(semesters)

		if !cache.HasData() {
			t.Error("Expected cache to have data after Update")
		}
	})

	t.Run("GetRecentSemesters returns first 2 semesters", func(t *testing.T) {
		t.Parallel()
		cache := NewSemesterCache()
		semesters := []Semester{
			{Year: 114, Term: 2},
			{Year: 114, Term: 1},
			{Year: 113, Term: 2},
			{Year: 113, Term: 1},
		}
		cache.Update(semesters)

		years, terms := cache.GetRecentSemesters()
		if len(years) != 2 || len(terms) != 2 {
			t.Fatalf("Expected 2 semesters, got %d years and %d terms", len(years), len(terms))
		}
		if years[0] != 114 || terms[0] != 2 {
			t.Errorf("First semester: expected 114-2, got %d-%d", years[0], terms[0])
		}
		if years[1] != 114 || terms[1] != 1 {
			t.Errorf("Second semester: expected 114-1, got %d-%d", years[1], terms[1])
		}
	})

	t.Run("GetExtendedSemesters returns 3rd and 4th semesters", func(t *testing.T) {
		t.Parallel()
		cache := NewSemesterCache()
		semesters := []Semester{
			{Year: 114, Term: 2},
			{Year: 114, Term: 1},
			{Year: 113, Term: 2},
			{Year: 113, Term: 1},
		}
		cache.Update(semesters)

		years, terms := cache.GetExtendedSemesters()
		if len(years) != 2 || len(terms) != 2 {
			t.Fatalf("Expected 2 semesters, got %d years and %d terms", len(years), len(terms))
		}
		if years[0] != 113 || terms[0] != 2 {
			t.Errorf("Third semester: expected 113-2, got %d-%d", years[0], terms[0])
		}
		if years[1] != 113 || terms[1] != 1 {
			t.Errorf("Fourth semester: expected 113-1, got %d-%d", years[1], terms[1])
		}
	})

	t.Run("GetAllSemesters returns all semesters", func(t *testing.T) {
		t.Parallel()
		cache := NewSemesterCache()
		semesters := []Semester{
			{Year: 114, Term: 2},
			{Year: 114, Term: 1},
			{Year: 113, Term: 2},
			{Year: 113, Term: 1},
		}
		cache.Update(semesters)

		years, terms := cache.GetAllSemesters()
		if len(years) != 4 || len(terms) != 4 {
			t.Fatalf("Expected 4 semesters, got %d years and %d terms", len(years), len(terms))
		}
	})

	t.Run("Fallback to calendar-based when no cached data", func(t *testing.T) {
		t.Parallel()
		cache := NewSemesterCache()

		// Should return calendar-based semesters when no data
		years, terms := cache.GetRecentSemesters()
		if len(years) != 2 || len(terms) != 2 {
			t.Errorf("Expected 2 fallback semesters, got %d years and %d terms", len(years), len(terms))
		}
	})
}

// TestGetCalendarBasedSemesters tests calendar-based semester calculation
func TestGetCalendarBasedSemesters(t *testing.T) {
	t.Parallel()

	// Test with current date
	years, terms := getCalendarBasedSemesters(4)
	if len(years) != 4 || len(terms) != 4 {
		t.Errorf("Expected 4 semesters, got %d years and %d terms", len(years), len(terms))
	}

	// Verify order is newest first
	for i := 0; i < len(years)-1; i++ {
		currKey := years[i]*10 + terms[i]
		nextKey := years[i+1]*10 + terms[i+1]
		if currKey <= nextKey {
			t.Errorf("Semesters not in descending order at index %d: %d-%d vs %d-%d",
				i, years[i], terms[i], years[i+1], terms[i+1])
		}
	}

	t.Logf("Calendar-based semesters: %d-%d, %d-%d, %d-%d, %d-%d",
		years[0], terms[0], years[1], terms[1], years[2], terms[2], years[3], terms[3])
}

// TestGenerateSemestersBackward tests backward semester generation
func TestGenerateSemestersBackward(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startYear int
		startTerm int
		count     int
		expected  []struct{ year, term int }
	}{
		{
			name:      "From term 2",
			startYear: 114,
			startTerm: 2,
			count:     4,
			expected: []struct{ year, term int }{
				{114, 2}, {114, 1}, {113, 2}, {113, 1},
			},
		},
		{
			name:      "From term 1",
			startYear: 114,
			startTerm: 1,
			count:     4,
			expected: []struct{ year, term int }{
				{114, 1}, {113, 2}, {113, 1}, {112, 2},
			},
		},
		{
			name:      "Small count",
			startYear: 113,
			startTerm: 2,
			count:     2,
			expected: []struct{ year, term int }{
				{113, 2}, {113, 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			years, terms := generateSemestersBackward(tt.startYear, tt.startTerm, tt.count)

			if len(years) != tt.count || len(terms) != tt.count {
				t.Fatalf("Expected %d semesters, got %d years and %d terms",
					tt.count, len(years), len(terms))
			}

			for i, exp := range tt.expected {
				if years[i] != exp.year || terms[i] != exp.term {
					t.Errorf("Semester %d: expected %d-%d, got %d-%d",
						i, exp.year, exp.term, years[i], terms[i])
				}
			}
		})
	}
}

// TestGetWarmupProbeStart tests warmup probe starting point
func TestGetWarmupProbeStart(t *testing.T) {
	t.Parallel()

	year, term := GetWarmupProbeStart()

	// Should return current ROC year, term 2
	now := time.Now()
	expectedYear := now.Year() - 1911

	if year != expectedYear {
		t.Errorf("Expected year %d, got %d", expectedYear, year)
	}
	if term != 2 {
		t.Errorf("Expected term 2, got %d", term)
	}

	t.Logf("Warmup probe start: %d-%d", year, term)
}

// TestGenerateProbeSequence tests probe sequence generation
func TestGenerateProbeSequence(t *testing.T) {
	t.Parallel()

	semesters := GenerateProbeSequence(115, 2, 6)

	if len(semesters) != 6 {
		t.Fatalf("Expected 6 semesters, got %d", len(semesters))
	}

	expected := []Semester{
		{Year: 115, Term: 2},
		{Year: 115, Term: 1},
		{Year: 114, Term: 2},
		{Year: 114, Term: 1},
		{Year: 113, Term: 2},
		{Year: 113, Term: 1},
	}

	for i, exp := range expected {
		if semesters[i].Year != exp.Year || semesters[i].Term != exp.Term {
			t.Errorf("Semester %d: expected %d-%d, got %d-%d",
				i, exp.Year, exp.Term, semesters[i].Year, semesters[i].Term)
		}
	}
}

// TestSemesterCacheConcurrency tests concurrent access to SemesterCache
func TestSemesterCacheConcurrency(t *testing.T) {
	t.Parallel()

	cache := NewSemesterCache()
	semesters := []Semester{
		{Year: 114, Term: 2},
		{Year: 114, Term: 1},
		{Year: 113, Term: 2},
		{Year: 113, Term: 1},
	}

	// Run concurrent reads and writes
	done := make(chan bool)

	// Writer
	go func() {
		for i := 0; i < 100; i++ {
			cache.Update(semesters)
		}
		done <- true
	}()

	// Reader 1
	go func() {
		for i := 0; i < 100; i++ {
			cache.GetRecentSemesters()
		}
		done <- true
	}()

	// Reader 2
	go func() {
		for i := 0; i < 100; i++ {
			cache.GetExtendedSemesters()
		}
		done <- true
	}()

	// Reader 3
	go func() {
		for i := 0; i < 100; i++ {
			cache.HasData()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}
}
