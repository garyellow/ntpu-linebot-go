package bot

import (
	"regexp"
	"sort"
	"strings"
)

// PostbackSplitChar is the delimiter used to separate fields in postback data.
// This ensures consistency across all bot modules when constructing postback strings.
// Example: "action$data1$data2" where "$" is the split character.
const PostbackSplitChar = "$"

// BuildKeywordRegex creates a regex pattern from keywords that matches at the START of text.
// Keywords are sorted by length (longest first) to ensure correct alternation matching.
// For example, "課程" should match before "課" to prevent partial matches.
//
// The regex uses ^ anchor to ensure keywords only match at the beginning of text.
// This prevents false positives like "我想找課程" matching "課程".
//
// Panics if keywords is empty, as this indicates a programming error.
//
// Usage:
//
//	keywords := []string{"課", "課程", "課名"}
//	regex := BuildKeywordRegex(keywords)
//	match := regex.FindString("課程 微積分") // Returns "課程"
//	match := regex.FindString("微積分課程") // Returns "" (no match - keyword not at start)
func BuildKeywordRegex(keywords []string) *regexp.Regexp {
	if len(keywords) == 0 {
		panic("BuildKeywordRegex: keywords cannot be empty")
	}

	// Create a copy to avoid modifying the original slice
	sorted := make([]string, len(keywords))
	copy(sorted, keywords)

	// Sort by length in descending order (longest first)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i]) > len(sorted[j])
	})

	// Use ^ anchor to match only at the start of text
	// (?i) for case-insensitive matching
	pattern := "(?i)^(" + strings.Join(sorted, "|") + ")"
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

// ContainsAllRunes checks if string s contains all runes from string chars,
// counting character occurrences (e.g., "aa" requires at least 2 'a's in s).
// Example: ContainsAllRunes("資訊工程學系", "資工系") returns true
// because all characters in "資工系" exist in "資訊工程學系".
// This is case-insensitive for ASCII characters.
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
