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

// Chunk types for syllabus indexing (BM25 search)
const (
	ChunkTypeObjectivesCN ChunkType = "objectives_cn" // 教學目標（中文）
	ChunkTypeObjectivesEN ChunkType = "objectives_en" // Course Objectives（英文）
	ChunkTypeOutlineCN    ChunkType = "outline_cn"    // 內容綱要（中文）
	ChunkTypeOutlineEN    ChunkType = "outline_en"    // Course Outline（英文）
	ChunkTypeSchedule     ChunkType = "schedule"      // 教學進度
)

// Chunk represents a single chunk of syllabus content for indexing
type Chunk struct {
	Type    ChunkType // Type of chunk content
	Content string    // The actual content for indexing
}

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

// ChunksForIndexing returns separate chunks for BM25 search indexing.
// Each chunk includes the course title as context prefix.
// This improves retrieval accuracy for short queries against long documents.
//
// Strategy:
// - 5 separate chunks: ObjectivesCN, ObjectivesEN, OutlineCN, OutlineEN, Schedule
// - Chinese and English are kept separate for cleaner matching
// - Whitespace-only fields are skipped (no value for indexing)
func (f *Fields) ChunksForIndexing(courseTitle string) []Chunk {
	var chunks []Chunk
	prefix := ""
	if courseTitle != "" {
		prefix = "【" + courseTitle + "】\n"
	}

	// Chunk 1: Objectives CN (教學目標 - 中文)
	if strings.TrimSpace(f.ObjectivesCN) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeObjectivesCN,
			Content: prefix + "教學目標：" + strings.TrimSpace(f.ObjectivesCN),
		})
	}

	// Chunk 2: Objectives EN (Course Objectives - English)
	if strings.TrimSpace(f.ObjectivesEN) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeObjectivesEN,
			Content: prefix + "Course Objectives: " + strings.TrimSpace(f.ObjectivesEN),
		})
	}

	// Chunk 3: Outline CN (內容綱要 - 中文)
	if strings.TrimSpace(f.OutlineCN) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeOutlineCN,
			Content: prefix + "內容綱要：" + strings.TrimSpace(f.OutlineCN),
		})
	}

	// Chunk 4: Outline EN (Course Outline - English)
	if strings.TrimSpace(f.OutlineEN) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeOutlineEN,
			Content: prefix + "Course Outline: " + strings.TrimSpace(f.OutlineEN),
		})
	}

	// Chunk 5: Schedule (教學進度)
	if strings.TrimSpace(f.Schedule) != "" {
		chunks = append(chunks, Chunk{
			Type:    ChunkTypeSchedule,
			Content: prefix + "教學進度：" + strings.TrimSpace(f.Schedule),
		})
	}

	return chunks
}

// IsEmpty returns true if all fields are empty or whitespace-only
func (f *Fields) IsEmpty() bool {
	return strings.TrimSpace(f.ObjectivesCN) == "" &&
		strings.TrimSpace(f.ObjectivesEN) == "" &&
		strings.TrimSpace(f.OutlineCN) == "" &&
		strings.TrimSpace(f.OutlineEN) == "" &&
		strings.TrimSpace(f.Schedule) == ""
}
