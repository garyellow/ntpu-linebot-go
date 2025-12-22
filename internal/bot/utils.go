package bot

import (
	"regexp"
	"slices"
	"strings"

	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
)

// PostbackSplitChar is the delimiter used to separate fields in postback data.
// This ensures consistency across all bot modules when constructing postback strings.
// Example: "action$data1$data2" where "$" is the split character.
const PostbackSplitChar = "$"

// BuildKeywordRegex creates a regex pattern matching keywords at the START of text.
// Keywords are sorted by length (longest first) to prevent partial matches.
// Uses ^ anchor to match only at beginning. Panics if keywords is empty.
//
// Example:
//
//	BuildKeywordRegex([]string{"課", "課程"}).FindString("課程 微積分") // Returns "課程"
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
	pattern := "(?i)^(" + strings.Join(sorted, "|") + ")"
	return regexp.MustCompile(pattern)
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

// ContainsAllRunes checks if s contains all runes from chars (case-insensitive for ASCII).
// Counts character occurrences: "aa" requires at least 2 'a's in s.
// Example: ContainsAllRunes("資訊工程學系", "資工系") returns true.
//
// Deprecated: Use stringutil.ContainsAllRunes instead.
func ContainsAllRunes(s, chars string) bool {
	return stringutil.ContainsAllRunes(s, chars)
}
