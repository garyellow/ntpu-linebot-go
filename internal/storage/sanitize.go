package storage

import "strings"

// sanitizeSearchTerm escapes SQLite LIKE special characters to prevent SQL injection
// SQLite LIKE special characters: % (matches any sequence of characters)
//
//	_ (matches any single character)
//	\ (escape character when specified)
func sanitizeSearchTerm(term string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\", // Escape backslash first
		"%", "\\%", // Escape percent
		"_", "\\_", // Escape underscore
	)
	return replacer.Replace(term)
}
