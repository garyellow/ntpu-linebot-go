package bot

import (
	"regexp"
	"sort"
	"strings"
)

// BuildKeywordRegex creates a regex pattern from keywords.
// Keywords are sorted by length (longest first) to ensure correct alternation matching.
// For example, "課程" should match before "課" to prevent partial matches.
//
// Usage:
//
//	keywords := []string{"課", "課程", "課名"}
//	regex := BuildKeywordRegex(keywords)
//	match := regex.FindString("課程 微積分") // Returns "課程"
func BuildKeywordRegex(keywords []string) *regexp.Regexp {
	if len(keywords) == 0 {
		// Return a regex that never matches - use impossible pattern
		return regexp.MustCompile(`\A\z.`) // Matches nothing (start, end, then any char - impossible)
	}

	// Create a copy to avoid modifying the original slice
	sorted := make([]string, len(keywords))
	copy(sorted, keywords)

	// Sort by length in descending order (longest first)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i]) > len(sorted[j])
	})

	pattern := "(?i)" + strings.Join(sorted, "|")
	return regexp.MustCompile(pattern)
}

// ExtractSearchTerm extracts the search term from text by removing the matched keyword.
// Handles three cases:
//   - Keyword at beginning: "課程 微積分" → "微積分"
//   - Keyword at end: "微積分課程" → "微積分"
//   - Keyword in middle: "查詢課程微積分" → "查詢微積分"
//
// Returns the trimmed search term.
func ExtractSearchTerm(text, keyword string) string {
	if keyword == "" {
		return strings.TrimSpace(text)
	}

	text = strings.TrimSpace(text)

	// Determine position and extract accordingly
	switch {
	case strings.HasPrefix(text, keyword):
		// Keyword at beginning
		return strings.TrimSpace(strings.TrimPrefix(text, keyword))
	case strings.HasSuffix(text, keyword):
		// Keyword at end
		return strings.TrimSpace(strings.TrimSuffix(text, keyword))
	default:
		// Keyword in middle: remove first occurrence
		return strings.TrimSpace(strings.Replace(text, keyword, "", 1))
	}
}
