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
// Only includes fields used for RAG embedding: 教學目標, 內容綱要, 教學進度
type Fields struct {
	Objectives string // 教學目標
	Outline    string // 內容綱要
	Schedule   string // 教學進度
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
func (f *Fields) ChunksForEmbedding(courseTitle string) []Chunk {
	var chunks []Chunk
	prefix := ""
	if courseTitle != "" {
		prefix = "【" + courseTitle + "】\n"
	}

	// Chunk 1: Objectives (most important for "what will I learn" queries)
	// Use strings.TrimSpace to skip whitespace-only content
	if strings.TrimSpace(f.Objectives) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeObjectives,
			Content: prefix + "教學目標：" + f.Objectives,
		})
	}

	// Chunk 2: Outline (important for topic/content queries)
	if strings.TrimSpace(f.Outline) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeOutline,
			Content: prefix + "內容綱要：" + f.Outline,
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

// IsEmpty returns true if all fields are empty or whitespace-only
func (f *Fields) IsEmpty() bool {
	return strings.TrimSpace(f.Objectives) == "" && strings.TrimSpace(f.Outline) == "" && strings.TrimSpace(f.Schedule) == ""
}
