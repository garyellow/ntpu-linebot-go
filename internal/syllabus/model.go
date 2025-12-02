// Package syllabus provides syllabus data extraction and management
// for course semantic search functionality.
package syllabus

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ChunkType identifies the type of content in a chunk
type ChunkType string

// Chunk types for syllabus embedding
const (
	ChunkTypeObjectives ChunkType = "objectives" // 教學目標
	ChunkTypeOutline    ChunkType = "outline"    // 內容綱要
	ChunkTypeSchedule   ChunkType = "schedule"   // 教學進度
)

// Chunk represents a single chunk of syllabus content for embedding
type Chunk struct {
	Type    ChunkType // Type of chunk content
	Content string    // The actual content for embedding
}

// ComputeContentHash calculates SHA256 hash of the content.
// Used for incremental update detection - only re-embed if content changed.
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

// ChunksForEmbedding returns separate chunks for better asymmetric semantic search.
// Each chunk includes the course title as context prefix.
// This improves retrieval accuracy for short queries against long documents.
//
// Strategy (2025 best practices):
// - Each syllabus field is already a semantically coherent unit
// - No truncation needed as Gemini embedding supports 2048 tokens (~8000 chars)
// - Full content preserved for maximum retrieval accuracy
// - Whitespace-only fields are skipped (no value for embedding)
// - Chinese and English content are merged when both exist for better semantic coverage
func (f *Fields) ChunksForEmbedding(courseTitle string) []Chunk {
	var chunks []Chunk
	prefix := ""
	if courseTitle != "" {
		prefix = "【" + courseTitle + "】\n"
	}

	// Chunk 1: Objectives (merge CN + EN if both exist)
	// Most important for "what will I learn" queries
	objectives := mergeContent(f.ObjectivesCN, f.ObjectivesEN)
	if strings.TrimSpace(objectives) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeObjectives,
			Content: prefix + "教學目標：" + objectives,
		})
	}

	// Chunk 2: Outline (merge CN + EN if both exist)
	// Important for topic/content queries
	outline := mergeContent(f.OutlineCN, f.OutlineEN)
	if strings.TrimSpace(outline) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeOutline,
			Content: prefix + "內容綱要：" + outline,
		})
	}

	// Chunk 3: Schedule (full content, may contain useful info like exam weeks)
	if strings.TrimSpace(f.Schedule) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeSchedule,
			Content: prefix + "教學進度：" + f.Schedule,
		})
	}

	return chunks
}

// mergeContent merges Chinese and English content with proper formatting.
// If only one exists, return that one. If both exist, combine with newline separator.
func mergeContent(cn, en string) string {
	cn = strings.TrimSpace(cn)
	en = strings.TrimSpace(en)

	if cn == "" && en == "" {
		return ""
	}
	if cn == "" {
		return en
	}
	if en == "" {
		return cn
	}
	// Both exist - combine them
	return cn + "\n" + en
}

// IsEmpty returns true if all fields are empty or whitespace-only
func (f *Fields) IsEmpty() bool {
	return strings.TrimSpace(f.ObjectivesCN) == "" &&
		strings.TrimSpace(f.ObjectivesEN) == "" &&
		strings.TrimSpace(f.OutlineCN) == "" &&
		strings.TrimSpace(f.OutlineEN) == "" &&
		strings.TrimSpace(f.Schedule) == ""
}
