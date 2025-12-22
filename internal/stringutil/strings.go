// Package stringutil provides common string manipulation utilities.
package stringutil

import "strings"

// IsNumeric checks if a string contains only digits.
// Returns false for empty strings.
func IsNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ContainsAllRunes checks if s contains all runes from chars (case-insensitive for ASCII).
// Counts character occurrences: "aa" requires at least 2 'a's in s.
// Supports non-contiguous character matching: "明王" matches "王小明".
//
// Example:
//
//	ContainsAllRunes("資訊工程學系", "資工系") returns true
//	ContainsAllRunes("王小明", "王明") returns true
//	ContainsAllRunes("王小明", "明王") returns true
func ContainsAllRunes(s, chars string) bool {
	if chars == "" {
		return true
	}
	if s == "" {
		return false
	}

	// Convert to lowercase for case-insensitive matching (for ASCII)
	sLower := strings.ToLower(s)
	charsLower := strings.ToLower(chars)

	// Build a map counting rune occurrences in s
	runeCount := make(map[rune]int)
	for _, r := range sLower {
		runeCount[r]++
	}

	// Build a map counting required rune occurrences in chars
	requiredCount := make(map[rune]int)
	for _, r := range charsLower {
		requiredCount[r]++
	}

	// Check if s has at least as many of each rune as required
	for r, required := range requiredCount {
		if runeCount[r] < required {
			return false
		}
	}
	return true
}
