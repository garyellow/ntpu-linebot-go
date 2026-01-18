// Package syllabus provides syllabus data extraction and management
package syllabus

import (
	"strings"
	"unicode"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// MatchProgramTypes matches full program names from syllabus page with
// raw program requirements from list page using fuzzy matching.
// Returns program requirements with full names and correct course types.
//
// Strategy (multi-tier for robustness):
//  1. Exact match (after normalization)
//  2. Full name contains raw name (raw is substring)
//  3. Core name matching (remove degree prefixes/suffixes)
//  4. Character similarity (Jaccard, high threshold)
//
// If no match found, defaults to "選" (elective).
func MatchProgramTypes(fullNames []string, rawReqs []storage.RawProgramReq) []storage.ProgramRequirement {
	result := make([]storage.ProgramRequirement, 0, len(fullNames))

	for _, fullName := range fullNames {
		// Only process items ending with "學程"
		if !strings.HasSuffix(fullName, "學程") {
			continue
		}

		// Find matching raw requirement
		courseType := findMatchingType(fullName, rawReqs)

		result = append(result, storage.ProgramRequirement{
			ProgramName: fullName,
			CourseType:  courseType,
		})
	}

	return result
}

// findMatchingType finds the course type for a full program name
// by matching against raw requirements from the list page.
// Uses multi-tier matching strategy for robustness.
func findMatchingType(fullName string, rawReqs []storage.RawProgramReq) string {
	fullNorm := normalizeName(fullName)

	// Tier 1: Exact match (after normalization)
	for _, raw := range rawReqs {
		if normalizeName(raw.Name) == fullNorm {
			return raw.CourseType
		}
	}

	// Tier 2: Full name contains raw name (raw is substring)
	// e.g., fullName="商業智慧與大數據分析學士學分學程" contains raw="商業智慧與大數據分析學程"
	for _, raw := range rawReqs {
		rawNorm := normalizeName(raw.Name)
		if rawNorm != "" && strings.Contains(fullNorm, rawNorm) {
			return raw.CourseType
		}
	}

	// Tier 3: Core name matching (handles heavily abbreviated cases)
	// Extract core: remove degree prefixes (學士/碩士) and suffixes (學分學程/微學程)
	fullCore := extractCore(fullName)
	for _, raw := range rawReqs {
		rawCore := extractCore(raw.Name)
		if rawCore != "" && fullCore != "" {
			// Check bidirectional containment
			if strings.Contains(fullCore, rawCore) || strings.Contains(rawCore, fullCore) {
				return raw.CourseType
			}
		}
	}

	// Tier 4: Character-based similarity (Jaccard on characters)
	// Only use if similarity is very high (> 0.7) to avoid false positives
	bestMatch := ""
	bestScore := 0.0
	for _, raw := range rawReqs {
		// Only compare with items that might be programs (contain "學程")
		if !strings.Contains(raw.Name, "學程") {
			continue
		}
		score := jaccardSimilarity(fullName, raw.Name)
		if score > bestScore && score > 0.7 {
			bestScore = score
			bestMatch = raw.CourseType
		}
	}
	if bestMatch != "" {
		return bestMatch
	}

	// Default: 選 (elective) if no match found
	return "選"
}

// normalizeName normalizes a program name for comparison.
// Removes all whitespace for consistent matching.
func normalizeName(s string) string {
	var result []rune
	for _, r := range s {
		if !unicode.IsSpace(r) {
			result = append(result, r)
		}
	}
	return string(result)
}

// extractCore extracts the core program name without degree/type prefixes and suffixes.
// e.g., "商業智慧與大數據分析學士學分學程" -> "商業智慧與大數據分析"
func extractCore(s string) string {
	// Remove common suffixes (order matters: longer suffixes first)
	suffixes := []string{
		"學士暨碩士學分學程",
		"學士學分學程",
		"碩士學分學程",
		"學士暨碩士微學程",
		"學士微學程",
		"碩士微學程",
		"學分學程",
		"微學程",
		"學程",
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			s = strings.TrimSuffix(s, suffix)
			break
		}
	}

	// Remove common prefixes
	prefixes := []string{"學士暨碩士", "學士", "碩士"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimPrefix(s, prefix)
			break
		}
	}

	return strings.TrimSpace(s)
}

// jaccardSimilarity computes Jaccard similarity between two strings based on characters.
// Returns a value between 0 and 1, where 1 is identical character sets.
func jaccardSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}

	setA := make(map[rune]bool)
	setB := make(map[rune]bool)

	for _, r := range a {
		if !unicode.IsSpace(r) {
			setA[r] = true
		}
	}
	for _, r := range b {
		if !unicode.IsSpace(r) {
			setB[r] = true
		}
	}

	// Intersection
	intersection := 0
	for r := range setA {
		if setB[r] {
			intersection++
		}
	}

	// Union
	union := len(setA) + len(setB) - intersection

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}
