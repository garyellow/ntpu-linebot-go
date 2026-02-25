// Package stringutil provides common string manipulation utilities.
package stringutil

import "strings"

// SanitizeText performs complete text sanitization:
// 1. Trim spaces
// 2. Normalize whitespace
// 3. Remove punctuation
// 4. Final normalization
func SanitizeText(text string) string {
	text = strings.TrimSpace(text)
	text = NormalizeWhitespace(text)
	text = RemovePunctuation(text)
	return NormalizeWhitespace(text)
}

// NormalizeWhitespace collapses all whitespace sequences into a single space.
func NormalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// RemovePunctuation removes all punctuation, keeping only ASCII alphanumeric,
// spaces, CJK Unified Ideographs (U+4E00-U+9FFF), and CJK Extension A (U+3400-U+4DBF).
// CJK fullwidth space (U+3000) is converted to ASCII space.
func RemovePunctuation(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == ' ',
			r >= 0x4E00 && r <= 0x9FFF,
			r >= 0x3400 && r <= 0x4DBF:
			result.WriteRune(r)
		case r >= 0x3000 && r <= 0x303F:
			if r == 0x3000 {
				result.WriteRune(' ')
			}
		default:
		}
	}
	return result.String()
}

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
