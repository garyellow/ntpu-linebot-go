// Package syllabus provides syllabus data extraction and management
// for course semantic search functionality.
package syllabus

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ComputeContentHash calculates SHA256 hash of the content.
// Used for incremental update detection - only re-index if content changed.
func ComputeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// Fields represents parsed syllabus fields from the course detail page
// Supports both separated (5 fields) and merged (3 fields) formats:
// - Separated: 教學目標, Course Objectives, 內容綱要, Course Outline, 教學進度
// - Merged: 教學目標 Course Objectives, 內容綱要/Course Outline, 教學進度
type Fields struct {
	ObjectivesCN string // 教學目標 (Chinese)
	ObjectivesEN string // Course Objectives (English, may be empty if merged)
	OutlineCN    string // 內容綱要 (Chinese)
	OutlineEN    string // Course Outline (English, may be empty if merged)
	Schedule     string // 教學進度 (only the schedule content, not metadata)
}

// ContentForIndexing returns a single document string for BM25 search indexing.
// Combines all syllabus fields into one document with the course title as prefix.
//
// BM25 Best Practice:
// - Single document per course: More accurate IDF calculation
// - No chunking needed: BM25's length normalization (b=0.75) handles document length
// - Simpler mapping: 1 course = 1 document, no deduplication needed
// - Better for small corpus: ~2000 courses doesn't need embedding-style chunking
//
// For embedding models, chunking is necessary due to token limits and semantic compression.
// BM25 doesn't have these constraints - it uses term frequency statistics directly.
func (f *Fields) ContentForIndexing(courseTitle string) string {
	// Build content without title first
	var content strings.Builder

	// Objectives (教學目標)
	if s := strings.TrimSpace(f.ObjectivesCN); s != "" {
		content.WriteString("教學目標：")
		content.WriteString(s)
		content.WriteString("\n")
	}
	if s := strings.TrimSpace(f.ObjectivesEN); s != "" {
		content.WriteString("Course Objectives: ")
		content.WriteString(s)
		content.WriteString("\n")
	}

	// Outline (內容綱要)
	if s := strings.TrimSpace(f.OutlineCN); s != "" {
		content.WriteString("內容綱要：")
		content.WriteString(s)
		content.WriteString("\n")
	}
	if s := strings.TrimSpace(f.OutlineEN); s != "" {
		content.WriteString("Course Outline: ")
		content.WriteString(s)
		content.WriteString("\n")
	}

	// Schedule (教學進度)
	if s := strings.TrimSpace(f.Schedule); s != "" {
		content.WriteString("教學進度：")
		content.WriteString(s)
	}

	// If no content, return empty (don't add title for empty content)
	trimmedContent := strings.TrimSpace(content.String())
	if trimmedContent == "" {
		return ""
	}

	// Prepend course title if provided
	if courseTitle != "" {
		return "【" + courseTitle + "】\n" + trimmedContent
	}

	return trimmedContent
}

// IsEmpty returns true if all fields are empty or whitespace-only
func (f *Fields) IsEmpty() bool {
	return strings.TrimSpace(f.ObjectivesCN) == "" &&
		strings.TrimSpace(f.ObjectivesEN) == "" &&
		strings.TrimSpace(f.OutlineCN) == "" &&
		strings.TrimSpace(f.OutlineEN) == "" &&
		strings.TrimSpace(f.Schedule) == ""
}
