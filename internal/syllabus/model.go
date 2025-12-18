// Package syllabus provides syllabus data extraction and management
// for course smart search functionality.
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
// Uses show_info=all format which provides complete data in a consistent structure
type Fields struct {
	Objectives string // 教學目標 Course Objectives (combined CN+EN content)
	Outline    string // 內容綱要/Course Outline (combined CN+EN content)
	Schedule   string // 教學進度 (weekly schedule content)
}

// ContentForIndexing returns a single document string for BM25 search indexing.
// Combines all syllabus fields into one document with the course title as prefix.
func (f *Fields) ContentForIndexing(courseTitle string) string {
	var content strings.Builder

	if s := strings.TrimSpace(f.Objectives); s != "" {
		content.WriteString("教學目標：")
		content.WriteString(s)
		content.WriteString("\n")
	}

	if s := strings.TrimSpace(f.Outline); s != "" {
		content.WriteString("內容綱要：")
		content.WriteString(s)
		content.WriteString("\n")
	}

	if s := strings.TrimSpace(f.Schedule); s != "" {
		content.WriteString("教學進度：")
		content.WriteString(s)
	}

	trimmedContent := strings.TrimSpace(content.String())
	if trimmedContent == "" {
		return ""
	}

	if courseTitle != "" {
		return "【" + courseTitle + "】\n" + trimmedContent
	}

	return trimmedContent
}

// IsEmpty returns true if all fields are empty or whitespace-only
func (f *Fields) IsEmpty() bool {
	return strings.TrimSpace(f.Objectives) == "" &&
		strings.TrimSpace(f.Outline) == "" &&
		strings.TrimSpace(f.Schedule) == ""
}
