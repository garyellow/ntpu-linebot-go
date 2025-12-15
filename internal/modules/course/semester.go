package course

import (
	"context"
	"time"
)

// Semester represents an academic semester with year and term.
type Semester struct {
	Year int // ROC year (e.g., 113)
	Term int // 1 (Fall) or 2 (Spring)
}

// SemesterDetector provides intelligent semester detection for course queries.
// It can detect the current active semester by checking actual course data availability.
type SemesterDetector struct {
	// countFunc is a function that counts courses for a given semester.
	// This allows dependency injection for testing and different data sources.
	countFunc func(ctx context.Context, year, term int) (int, error)
}

// NewSemesterDetector creates a new SemesterDetector with the given course count function.
// The countFunc should return the number of courses available for a given year and term.
func NewSemesterDetector(countFunc func(ctx context.Context, year, term int) (int, error)) *SemesterDetector {
	return &SemesterDetector{countFunc: countFunc}
}

// getSemestersToSearch returns 4 semesters to search based on current date.
// This is the simple calendar-based version.
// For intelligent detection that checks actual data, use SemesterDetector.DetectActiveSemesters.
func getSemestersToSearch() ([]int, []int) {
	return getSemestersForDate(time.Now())
}

// getSemestersForDate returns 4 semesters to search based on a specific date.
// This implements the Taiwan academic calendar logic where:
// - Semester 1 (Fall): September - January
// - Semester 2 (Spring): February - June
// - Summer Break: July - August
//
// Returns: (years []int, terms []int) where years[i] and terms[i] form a semester pair.
// The function returns 4 semesters: starting from the potential newest and going backwards.
//
// Search strategy by month:
//   - Feb-Aug (2-8月): Start from current academic year term 2, go back 4 semesters
//   - Sep-Dec (9-12月): Start from next academic year term 1 (may not exist yet), go back 4 semesters
//   - Jan (1月): Start from current academic year term 1, go back 4 semesters
//
// Examples (returning 4 semesters, newest first):
//   - 2025/03 → [113-2, 113-1, 112-2, 112-1] (Spring semester 113 in progress)
//   - 2025/07 → [113-2, 113-1, 112-2, 112-1] (Summer break)
//   - 2025/09 → [114-1, 113-2, 113-1, 112-2] (Fall semester 114 just started or about to start)
//   - 2025/11 → [114-1, 113-2, 113-1, 112-2] (Fall semester 114 in progress)
//   - 2026/01 → [114-1, 113-2, 113-1, 112-2] (Winter break, Fall semester 114 just ended)
func getSemestersForDate(date time.Time) ([]int, []int) {
	currentYear := date.Year() - 1911 // Convert AD to ROC year
	currentMonth := int(date.Month())

	var startYear, startTerm int

	switch {
	case currentMonth >= 2 && currentMonth <= 8:
		// Spring semester in progress (2-6月) or summer break (7-8月)
		// Start from: current academic year term 2
		// Academic year for Feb-Aug: previous calendar year
		// Example: Feb 2025 → 113-2 (Academic Year 113 started Sep 2024)
		startYear = currentYear - 1
		startTerm = 2

	case currentMonth >= 9:
		// Fall semester just started or in progress (9-12月)
		// Start from: current calendar year as academic year, term 1
		// Example: Sep 2025 → 114-1 (new academic year starting)
		startYear = currentYear
		startTerm = 1

	default:
		// January (month == 1)
		// Winter break, fall semester just ended
		// Start from: previous calendar year as academic year, term 1
		// Example: Jan 2026 → 114-1 (Academic Year 114 started Sep 2025)
		startYear = currentYear - 1
		startTerm = 1
	}

	// Generate 4 semesters going backwards from start point
	return generateSemestersBackward(startYear, startTerm, 4)
}

// generateSemestersBackward generates n semesters going backwards from the given start point.
// Term alternates: 1 → 2 (prev year) → 1 (prev year) → 2 (prev-prev year) → ...
func generateSemestersBackward(startYear, startTerm, count int) ([]int, []int) {
	years := make([]int, count)
	terms := make([]int, count)

	year := startYear
	term := startTerm

	for i := range count {
		years[i] = year
		terms[i] = term

		// Go to previous semester
		if term == 1 {
			term = 2
			year--
		} else {
			term = 1
			// year stays the same (term 2 → term 1 of same year)
		}
	}

	return years, terms
}

// DetectActiveSemesters intelligently detects which semesters have data available.
// Strategy:
// 1. Calculate the potential newest semester based on calendar (may be future)
// 2. Check if that semester has any data (> 0 courses)
// 3. If yes → use it as starting point
// 4. If no → shift back one semester and use that
//
// This handles:
// - Early data upload (e.g., Aug 25: 114-1 already available)
// - Delayed upload (e.g., Sep 15: 114-1 not ready yet)
// - Pre-registration period (next semester opens early)
//
// Returns 4 semesters starting from the detected active semester.
// The minCoursesThreshold is kept for backwards compatibility but typically set to 0.
func (d *SemesterDetector) DetectActiveSemesters(ctx context.Context, minCoursesThreshold int) ([]int, []int) {
	// Get calendar-based semesters as starting point
	baseYears, baseTerms := getSemestersForDate(time.Now())

	if d.countFunc == nil {
		// No count function provided, use calendar-based detection
		return baseYears, baseTerms
	}

	// Check if the newest semester has enough courses
	newestCount, err := d.countFunc(ctx, baseYears[0], baseTerms[0])
	if err != nil {
		// On error, fall back to calendar-based detection
		return baseYears, baseTerms
	}

	if newestCount >= minCoursesThreshold {
		// Newest semester has data, use calendar-based semesters
		return baseYears, baseTerms
	}

	// Newest semester has no/insufficient data, shift back by one semester
	// This handles cases like:
	// - Sep 1st: new semester started but no data uploaded yet
	// - Early in registration period
	shiftedYears, shiftedTerms := generateSemestersBackward(baseYears[1], baseTerms[1], 4)

	return shiftedYears, shiftedTerms
}

// DetectWarmupSemesters intelligently detects which 4 semesters should be warmed up.
// It returns the semesters that should have course data based on actual data availability,
// not just calendar dates. This ensures we always warmup the most relevant semesters.
//
// Strategy (DATA-FIRST approach):
// 1. Check if "next potential semester" has data (e.g., early upload before semester starts)
// 2. If yes, use it as the newest semester
// 3. If no, use calendar-based newest semester
// 4. Generate 4 semesters backward from the newest
//
// This approach handles edge cases like:
// - Early uploads: If next semester data is uploaded early, use it immediately
// - Late uploads: If current semester data not ready, fall back to previous
// - Pre-registration periods: Naturally detects when new semester opens
//
// Key insight: We trust data availability over calendar calculations.
func (d *SemesterDetector) DetectWarmupSemesters(ctx context.Context, minCoursesThreshold int) []Semester {
	// Use DetectActiveSemesters to find which 4 semesters have data
	years, terms := d.DetectActiveSemesters(ctx, minCoursesThreshold)

	// Convert to Semester structs (should always be 4 semesters)
	result := make([]Semester, len(years))
	for i := range years {
		result[i] = Semester{
			Year: years[i],
			Term: terms[i],
		}
	}

	return result
}
