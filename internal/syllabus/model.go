// Package syllabus provides syllabus data extraction and management
// for course semantic search functionality.
package syllabus

import (
	"crypto/sha256"
	"encoding/hex"
)

// ComputeContentHash calculates SHA256 hash of the content.
// Used for incremental update detection - only re-embed if content changed.
func ComputeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// Fields represents parsed syllabus fields from the course detail page
// Only includes fields used for RAG embedding: 教學目標, 內容綱要, 教學進度
type Fields struct {
	Objectives string // 教學目標
	Outline    string // 內容綱要
	Schedule   string // 教學進度
}

// MergeForEmbedding combines relevant fields into a single string for embedding
// Only includes: 教學目標, 內容綱要, 教學進度
func (f *Fields) MergeForEmbedding() string {
	var parts []string

	if f.Objectives != "" {
		parts = append(parts, "教學目標："+f.Objectives)
	}
	if f.Outline != "" {
		parts = append(parts, "內容綱要："+f.Outline)
	}
	if f.Schedule != "" {
		parts = append(parts, "教學進度："+f.Schedule)
	}

	if len(parts) == 0 {
		return ""
	}

	result := ""
	for i, part := range parts {
		if i > 0 {
			result += "\n\n"
		}
		result += part
	}
	return result
}

// IsEmpty returns true if all fields are empty
func (f *Fields) IsEmpty() bool {
	return f.Objectives == "" && f.Outline == "" && f.Schedule == ""
}
