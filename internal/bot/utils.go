package bot

import (
	"regexp"
	"slices"
	"strings"
)

// PostbackSplitChar is the delimiter used to separate fields in postback data.
// This ensures consistency across all bot modules when constructing postback strings.
// Example: "action$data1$data2" where "$" is the split character.
const PostbackSplitChar = "$"

// BuildKeywordRegex creates a regex pattern matching keywords at the START of text.
// Keywords are sorted by length (longest first) to prevent partial matches.
// Uses ^ anchor to match only at beginning. Panics if keywords is empty.
//
// IMPORTANT: Keywords must be followed by a space or be the entire text.
// This prevents false matches like "課程表" triggering "課程".
// Use MatchKeyword() to get the matched keyword without trailing space.
//
// Example:
//
//	MatchKeyword(BuildKeywordRegex([]string{"課", "課程"}), "課程 微積分") // Returns "課程"
//	MatchKeyword(BuildKeywordRegex([]string{"課", "課程"}), "課程")      // Returns "課程"
//	MatchKeyword(BuildKeywordRegex([]string{"課", "課程"}), "課程微積分") // Returns "" (no space)
func BuildKeywordRegex(keywords []string) *regexp.Regexp {
	if len(keywords) == 0 {
		panic("BuildKeywordRegex: keywords cannot be empty")
	}

	// Create a copy to avoid modifying the original slice
	sorted := make([]string, len(keywords))
	copy(sorted, keywords)

	// Sort by length in descending order (longest first)
	slices.SortFunc(sorted, func(a, b string) int {
		return len(b) - len(a)
	})

	// Use ^ anchor to match only at the start of text
	// (?i) for case-insensitive matching
	// Group 1 captures the keyword, (?:\s|$) requires space or end after keyword
	// This prevents false matches like "課程表" triggering "課程"
	pattern := "(?i)^(" + strings.Join(sorted, "|") + ")(?:\\s|$)"
	return regexp.MustCompile(pattern)
}

// MatchKeyword returns the matched keyword from text using the given regex.
// Returns empty string if no match. The keyword is returned without trailing space.
//
// Example:
//
//	regex := BuildKeywordRegex([]string{"課程", "課"})
//	MatchKeyword(regex, "課程 微積分") // Returns "課程"
//	MatchKeyword(regex, "課程微積分")  // Returns "" (no space after keyword)
func MatchKeyword(regex *regexp.Regexp, text string) string {
	match := regex.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return match[1] // Group 1 is the keyword without trailing space
}

// ExtractSearchTerm extracts the search term by removing the matched keyword.
// Handles keyword at beginning, end, or middle of text. Returns trimmed result.
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
