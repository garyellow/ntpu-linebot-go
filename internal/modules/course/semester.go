package course

import (
	"context"
	"sync"
	"time"
)

// Semester represents an academic semester with year and term.
type Semester struct {
	Year int // ROC year (e.g., 113)
	Term int // 1 (Fall) or 2 (Spring)
}

// SemesterDetector provides data-driven semester detection for course queries.
// It detects available semesters by checking actual course data in the database,
// rather than relying on calendar-based calculations.
//
// Design Philosophy:
// - All semester decisions should be based on actual data availability
// - Calendar-based calculation is only used as initial probe for warmup/detection
// - User queries always use cached semester data from last detection
//
// Usage:
// 1. Create with NewSemesterDetector(countFunc)
// 2. Call RefreshSemesters(ctx) during warmup or periodically
// 3. Use GetRecentSemesters() / GetExtendedSemesters() for user queries
type SemesterDetector struct {
	// countFunc is a function that counts courses for a given semester.
	// This allows dependency injection for testing and different data sources.
	countFunc func(ctx context.Context, year, term int) (int, error)

	// mu protects concurrent access to cachedYears and cachedTerms.
	// RefreshSemesters writes to these fields while GetRecentSemesters,
	// GetExtendedSemesters, HasData, and GetAllSemesters read from them.
	mu sync.RWMutex

	// cachedYears and cachedTerms store the 4 most recent semesters with data.
	// Updated by RefreshSemesters(), used by GetRecentSemesters()/GetExtendedSemesters().
	cachedYears []int
	cachedTerms []int
}

// NewSemesterDetector creates a new SemesterDetector with the given course count function.
// The countFunc should return the number of courses available for a given year and term.
func NewSemesterDetector(countFunc func(ctx context.Context, year, term int) (int, error)) *SemesterDetector {
	return &SemesterDetector{countFunc: countFunc}
}

// getSemestersToSearch returns 2 recent semesters based on current date (calendar-based).
// This is a fallback for when SemesterDetector has no cached data.
//
// Prefer using SemesterDetector.GetRecentSemesters() for data-driven semester detection.
func getSemestersToSearch() ([]int, []int) {
	return getRecentSemestersForDate(time.Now())
}

// getExtendedSemesters returns 2 additional semesters (3rd and 4th) based on current date (calendar-based).
// This is a fallback for when SemesterDetector has no cached data.
//
// Prefer using SemesterDetector.GetExtendedSemesters() for data-driven semester detection.
func getExtendedSemesters() ([]int, []int) {
	years, terms := getSemestersForDate(time.Now())
	// Skip first 2 semesters (already in regular search), return 3rd and 4th
	if len(years) >= 4 {
		return years[2:4], terms[2:4]
	}
	// Fallback: if less than 4 semesters available, return what's beyond the first 2
	if len(years) > 2 {
		return years[2:], terms[2:]
	}
	// Edge case: no additional semesters available
	return []int{}, []int{}
}

// getRecentSemestersForDate returns 2 recent semesters based on a specific date.
// This is optimized for user queries - most students only need current + previous semester.
//
// Examples (returning 2 semesters, newest first):
//   - 2025/03 → [113-2, 113-1] (Spring semester in progress)
//   - 2025/09 → [114-1, 113-2] (Fall semester just started)
//   - 2026/01 → [114-1, 113-2] (Winter break)
func getRecentSemestersForDate(date time.Time) ([]int, []int) {
	years, terms := getSemestersForDate(date)
	// Return only first 2 semesters
	if len(years) >= 2 {
		return years[:2], terms[:2]
	}
	return years, terms
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
// Term alternates: 1 → 2 (prev year) → 1 (same year as that term 2) → 2 (prev year) → ...
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

// DetectActiveSemesters returns the 4 most recent semesters with course data.
// It checks if the calendar-based newest semester has any data; if not, shifts back one semester.
// This ensures we always return semesters with actual course availability.
func (d *SemesterDetector) DetectActiveSemesters(ctx context.Context) ([]int, []int) {
	// Get calendar-based semesters as starting point
	baseYears, baseTerms := getSemestersForDate(time.Now())

	if d.countFunc == nil {
		// No count function provided, use calendar-based detection
		return baseYears, baseTerms
	}

	// Check if the newest semester has any data
	newestCount, err := d.countFunc(ctx, baseYears[0], baseTerms[0])
	if err != nil {
		// On error, fall back to calendar-based detection
		return baseYears, baseTerms
	}

	if newestCount > 0 {
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

// DetectWarmupSemesters returns the 4 most recent semesters with course data.
// This is used during warmup to determine which semesters should be cached.
// Also updates the cached semesters for user queries.
func (d *SemesterDetector) DetectWarmupSemesters(ctx context.Context) []Semester {
	// Use DetectActiveSemesters to find which 4 semesters have data
	years, terms := d.DetectActiveSemesters(ctx)

	// Cache the detected semesters for user queries (protected by mutex)
	d.mu.Lock()
	d.cachedYears = years
	d.cachedTerms = terms
	d.mu.Unlock()

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

// RefreshSemesters detects and caches the 4 most recent semesters with data.
// This should be called during warmup or periodically to update the cache.
// User queries use the cached values via GetRecentSemesters/GetExtendedSemesters.
func (d *SemesterDetector) RefreshSemesters(ctx context.Context) {
	years, terms := d.DetectActiveSemesters(ctx)
	d.mu.Lock()
	d.cachedYears = years
	d.cachedTerms = terms
	d.mu.Unlock()
}

// GetRecentSemesters returns the 2 most recent semesters with data.
// This is data-driven: returns semesters from the last RefreshSemesters/DetectWarmupSemesters call.
// If no cached data, falls back to calendar-based detection.
//
// Returns: (years []int, terms []int) where years[i] and terms[i] form a semester pair.
func (d *SemesterDetector) GetRecentSemesters() ([]int, []int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.cachedYears) >= 2 {
		return d.cachedYears[:2], d.cachedTerms[:2]
	}
	// Fallback to calendar-based (should not happen in normal operation)
	return getRecentSemestersForDate(time.Now())
}

// GetExtendedSemesters returns the 3rd and 4th semesters with data.
// This is data-driven: returns semesters from the last RefreshSemesters/DetectWarmupSemesters call.
// Used for "更多學期" (More Semesters) search.
//
// Returns: (years []int, terms []int) for semesters 3-4, or empty if not available.
func (d *SemesterDetector) GetExtendedSemesters() ([]int, []int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.cachedYears) >= 4 {
		return d.cachedYears[2:4], d.cachedTerms[2:4]
	}
	if len(d.cachedYears) > 2 {
		return d.cachedYears[2:], d.cachedTerms[2:]
	}
	// Fallback to calendar-based (should not happen in normal operation)
	return getExtendedSemesters()
}

// HasData returns true if the detector has cached semester data.
// This indicates whether RefreshSemesters or DetectWarmupSemesters has been called.
func (d *SemesterDetector) HasData() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.cachedYears) > 0
}

// GetAllSemesters returns all 4 cached semesters with data.
// This is data-driven: returns semesters from the last RefreshSemesters/DetectWarmupSemesters call.
func (d *SemesterDetector) GetAllSemesters() ([]int, []int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.cachedYears) > 0 {
		return d.cachedYears, d.cachedTerms
	}
	// Fallback to calendar-based
	return getSemestersForDate(time.Now())
}
