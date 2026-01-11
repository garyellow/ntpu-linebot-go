package course

import (
	"sync"
	"time"
)

// Semester represents an academic semester with year and term.
type Semester struct {
	Year int // ROC year (e.g., 113)
	Term int // 1 (Fall) or 2 (Spring)
}

// SemesterCache stores detected semesters for user queries.
// Updated by warmup, used by handlers for course searches.
//
// Design Philosophy:
// - Warmup probes actual data sources (scraper) to find available semesters
// - User queries use cached semesters (never probe, always fast)
// - Cache is updated during warmup/daily refresh
type SemesterCache struct {
	mu sync.RWMutex

	// semesters stores the 4 most recent semesters with data.
	// Order: newest first (e.g., [114-2, 114-1, 113-2, 113-1])
	semesters []Semester
}

// NewSemesterCache creates a new empty SemesterCache.
func NewSemesterCache() *SemesterCache {
	return &SemesterCache{}
}

// Update replaces cached semesters with new data.
// Called by warmup after probing data sources.
func (c *SemesterCache) Update(semesters []Semester) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.semesters = semesters
}

// GetRecentSemesters returns the 2 most recent semesters.
// Returns (years, terms) slices for easy use by handlers.
func (c *SemesterCache) GetRecentSemesters() ([]int, []int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.semesters) >= 2 {
		return []int{c.semesters[0].Year, c.semesters[1].Year},
			[]int{c.semesters[0].Term, c.semesters[1].Term}
	}
	// Fallback: no cached data yet, use calendar-based estimate
	return getCalendarBasedSemesters(2)
}

// GetExtendedSemesters returns the 3rd and 4th semesters.
// Used for "更多學期" (More Semesters) search.
func (c *SemesterCache) GetExtendedSemesters() ([]int, []int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.semesters) >= 4 {
		return []int{c.semesters[2].Year, c.semesters[3].Year},
			[]int{c.semesters[2].Term, c.semesters[3].Term}
	}
	if len(c.semesters) > 2 {
		years := make([]int, len(c.semesters)-2)
		terms := make([]int, len(c.semesters)-2)
		for i := 2; i < len(c.semesters); i++ {
			years[i-2] = c.semesters[i].Year
			terms[i-2] = c.semesters[i].Term
		}
		return years, terms
	}
	// Fallback: no extended semesters available (cache has less than 3 semesters)
	return []int{}, []int{}
}

// GetAllSemesters returns all cached semesters.
func (c *SemesterCache) GetAllSemesters() ([]int, []int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.semesters) > 0 {
		years := make([]int, len(c.semesters))
		terms := make([]int, len(c.semesters))
		for i, sem := range c.semesters {
			years[i] = sem.Year
			terms[i] = sem.Term
		}
		return years, terms
	}
	// Fallback to calendar-based
	return getCalendarBasedSemesters(4)
}

// HasData returns true if the cache has semester data.
func (c *SemesterCache) HasData() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.semesters) > 0
}

// getCalendarBasedSemesters returns n semesters based on current date.
// This is a fallback for when no cached data is available.
// Uses Taiwan academic calendar logic:
// - Semester 1 (Fall): September - January
// - Semester 2 (Spring): February - June
// - Summer Break: July - August
func getCalendarBasedSemesters(count int) ([]int, []int) {
	now := time.Now()
	currentYear := now.Year() - 1911 // Convert AD to ROC year
	currentMonth := int(now.Month())

	var startYear, startTerm int

	switch {
	case currentMonth >= 2 && currentMonth <= 8:
		// Spring semester or summer break (Feb-Aug)
		// Current academic year = previous calendar year
		startYear = currentYear - 1
		startTerm = 2

	case currentMonth >= 9:
		// Fall semester (Sep-Dec)
		startYear = currentYear
		startTerm = 1

	default:
		// January - winter break, fall semester just ended
		startYear = currentYear - 1
		startTerm = 1
	}

	return generateSemestersBackward(startYear, startTerm, count)
}

// generateSemestersBackward generates n semesters going backwards from the given start point.
// Term alternates: 2 → 1 (same year) → 2 (prev year) → 1 → ...
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

// GetWarmupProbeStart returns the starting semester for warmup probing.
// Warmup should start from current ROC year term 2 and probe backwards
// until it finds 4 semesters with actual data.
//
// Example: In January 2026 (ROC 115), returns (115, 2) as the start point.
// Warmup will then probe: 115-2 → 115-1 → 114-2 → 114-1 → ... until 4 found.
func GetWarmupProbeStart() (year, term int) {
	now := time.Now()
	currentYear := now.Year() - 1911 // Convert AD to ROC year
	return currentYear, 2
}

// GenerateProbeSequence generates a sequence of semesters for probing.
// Starts from the given semester and goes backwards.
// Used by warmup to determine which semesters to check for data.
func GenerateProbeSequence(startYear, startTerm, maxCount int) []Semester {
	semesters := make([]Semester, maxCount)

	year := startYear
	term := startTerm

	for i := range maxCount {
		semesters[i] = Semester{Year: year, Term: term}

		// Go to previous semester
		if term == 1 {
			term = 2
			year--
		} else {
			term = 1
		}
	}

	return semesters
}
