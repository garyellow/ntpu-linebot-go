package course

import "time"

// getSemestersToSearch returns the semesters to search based on current date.
func getSemestersToSearch() ([]int, []int) {
	return getSemestersForDate(time.Now())
}

// getSemestersForDate returns the semesters to search based on a specific date.
// This implements the Taiwan academic calendar logic where:
// - Semester 1 (Fall): September - January
// - Semester 2 (Spring): February - June
// - Summer Break: July - August
//
// Returns: (years []int, terms []int) where years[i] and terms[i] form a semester pair.
// The function always returns 2 semesters: the current/most recent and the previous one.
//
// Search strategy by month:
//   - Feb-Jun (2-6月): Current year both terms (term 2 then term 1) - Spring semester in progress
//   - Jul-Aug (7-8月): Current year both terms (term 2 then term 1) - Summer break
//   - Sep-Dec & Jan (9-12月 + 1月): Academic year term 1 + previous year term 2 - Fall semester in progress or winter break
//
// Examples:
//   - 2025/03 → Search [113-2, 113-1] (Spring semester 113 in progress)
//   - 2025/07 → Search [113-2, 113-1] (Summer break, both semesters ended)
//   - 2025/11 → Search [114-1, 113-2] (Fall semester 114 in progress, academic year starts Sep 2024)
//   - 2025/01 → Search [113-1, 112-2] (Winter break, Fall semester 113 just ended)
func getSemestersForDate(date time.Time) ([]int, []int) {
	currentYear := date.Year() - 1911 // Convert AD to ROC year
	currentMonth := int(date.Month())

	var years, terms []int

	switch {
	case currentMonth >= 2 && currentMonth <= 8:
		// Spring semester in progress (2-6月) or summer break (7-8月)
		// Search: Current year term 2 (most recent) + Current year term 1
		// Note: Academic year starts in previous calendar year
		// Example: Feb 2025 is 113-2 (Academic Year 113 started Sep 2024)
		// currentYear (AD-1911) for 2025 is 114, so we need currentYear - 1
		searchYear := currentYear - 1
		years = []int{searchYear, searchYear}
		terms = []int{2, 1}

	default:
		// Fall semester in progress (9-12月) or winter break (1月)
		// Academic year calculation: September starts new academic year
		// Example: 2024/9 → 114學年度, 2025/1 → still 113學年度 (started 2024/9)
		var academicYear int
		if currentMonth >= 9 {
			// September onwards: current year is the academic year
			academicYear = currentYear
		} else {
			// January: previous year is the academic year (started last September)
			academicYear = currentYear - 1
		}

		// Search: Academic year term 1 + Previous academic year term 2
		// This covers the most recent semester and the one before
		years = []int{academicYear, academicYear - 1}
		terms = []int{1, 2}
	}

	return years, terms
}
