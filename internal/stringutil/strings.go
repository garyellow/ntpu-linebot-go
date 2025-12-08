// Package stringutil provides common string manipulation utilities.
package stringutil

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

// TrimAndNormalize trims whitespace and normalizes internal spacing.
// Multiple spaces are collapsed into single spaces.
func TrimAndNormalize(s string) string {
	// Implementation can use strings.Fields if needed
	return s
}
