// Package rag provides Retrieval-Augmented Generation functionality
// using chromem-go for vector storage and Gemini for embeddings.
package rag

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	chromem "github.com/philippgille/chromem-go"

	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
)

const (
	// SyllabusCollectionName is the name of the syllabus collection in chromem
	SyllabusCollectionName = "syllabi"

	// DefaultSearchResults is the default number of results for semantic search
	DefaultSearchResults = 10

	// MaxSearchResults is the maximum number of results for semantic search
	// With the new 10-multiple logic, we may return more than this
	MaxSearchResults = 100

	// MinSimilarityThreshold is the minimum cosine similarity score to include a result
	// Results below this threshold are considered not relevant enough
	// Range: 0.0 to 1.0 (cosine similarity)
	//
	// Note: We use asymmetric semantic search (short query vs chunked document)
	// With chunking, similarity scores should be higher than whole-document embedding
	// 0.3 filters out low-quality matches while keeping reasonably relevant results
	MinSimilarityThreshold float32 = 0.3

	// HighRelevanceThreshold is the similarity score threshold for highly relevant results
	// Results with similarity >= 0.7 (70%) are considered highly relevant
	// All highly relevant results are returned, then padded to next multiple of 10
	HighRelevanceThreshold float32 = 0.7
)

// VectorDB wraps chromem-go database for course syllabus semantic search
type VectorDB struct {
	db            *chromem.DB
	collection    *chromem.Collection
	embeddingFunc chromem.EmbeddingFunc
	logger        *logger.Logger
	mu            sync.RWMutex
	initialized   bool
}

// SearchResult represents a semantic search result
type SearchResult struct {
	UID        string   // Course UID
	Title      string   // Course title
	Teachers   []string // Course teachers
	Year       int      // Academic year
	Term       int      // Semester
	Content    string   // Syllabus content
	Similarity float32  // Cosine similarity score (0-1)
}

// NewVectorDB creates a new vector database for syllabus search
// persistDir should be the base data directory (e.g., "./data")
// Returns nil if apiKey is empty (feature disabled)
func NewVectorDB(persistDir, apiKey string, log *logger.Logger) (*VectorDB, error) {
	if apiKey == "" {
		log.Info("Gemini API key not configured, semantic search disabled")
		return nil, nil
	}

	// Create embedding function using Gemini API
	embeddingFunc := genai.NewEmbeddingFunc(apiKey)

	// Persistence path for chromem
	chromemPath := filepath.Join(persistDir, "chromem", "syllabi")

	// Create chromem database with persistence
	db, err := chromem.NewPersistentDB(chromemPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create chromem database: %w", err)
	}

	vdb := &VectorDB{
		db:            db,
		embeddingFunc: embeddingFunc,
		logger:        log,
		initialized:   false,
	}

	return vdb, nil
}

// Initialize loads existing syllabi into the vector store
// Call this after creating the VectorDB
func (v *VectorDB) Initialize(ctx context.Context, syllabi []*storage.Syllabus) error {
	if v == nil {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Get or create collection
	collection, err := v.db.GetOrCreateCollection(SyllabusCollectionName, nil, v.embeddingFunc)
	if err != nil {
		return fmt.Errorf("failed to get/create collection: %w", err)
	}
	v.collection = collection

	// Check if we have documents already
	existingCount := collection.Count()
	if existingCount > 0 {
		v.logger.WithField("count", existingCount).Info("Loaded existing syllabi embeddings from disk")
		v.initialized = true
		return nil
	}

	// Add syllabi to collection
	if len(syllabi) > 0 {
		if err := v.addSyllabiInternal(ctx, syllabi); err != nil {
			return fmt.Errorf("failed to add syllabi: %w", err)
		}
		v.logger.WithField("count", len(syllabi)).Info("Indexed syllabi for semantic search")
	}

	v.initialized = true
	return nil
}

// AddSyllabus adds a single syllabus to the vector store
func (v *VectorDB) AddSyllabus(ctx context.Context, syllabus *storage.Syllabus) error {
	if v == nil || v.collection == nil {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	return v.addSyllabusInternal(ctx, syllabus)
}

// AddSyllabi adds multiple syllabi to the vector store
func (v *VectorDB) AddSyllabi(ctx context.Context, syllabi []*storage.Syllabus) error {
	if v == nil || v.collection == nil || len(syllabi) == 0 {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	return v.addSyllabiInternal(ctx, syllabi)
}

// addSyllabusInternal adds a single syllabus (internal, assumes lock held)
// Uses chunking strategy: each syllabus field becomes a separate document
// Document IDs are formatted as "{UID}_{chunk_type}" for deduplication during search
func (v *VectorDB) addSyllabusInternal(ctx context.Context, syl *storage.Syllabus) error {
	// Skip if all fields are empty or whitespace-only
	if strings.TrimSpace(syl.ObjectivesCN) == "" &&
		strings.TrimSpace(syl.ObjectivesEN) == "" &&
		strings.TrimSpace(syl.OutlineCN) == "" &&
		strings.TrimSpace(syl.OutlineEN) == "" &&
		strings.TrimSpace(syl.Schedule) == "" {
		return nil
	}

	// Create Fields from Syllabus and generate chunks
	fields := &syllabus.Fields{
		ObjectivesCN: syl.ObjectivesCN,
		ObjectivesEN: syl.ObjectivesEN,
		OutlineCN:    syl.OutlineCN,
		OutlineEN:    syl.OutlineEN,
		Schedule:     syl.Schedule,
	}
	chunks := fields.ChunksForEmbedding(syl.Title)

	if len(chunks) == 0 {
		return nil
	}

	docs := make([]chromem.Document, 0, len(chunks))
	for _, chunk := range chunks {
		docID := fmt.Sprintf("%s_%s", syl.UID, chunk.Type)
		docs = append(docs, chromem.Document{
			ID:      docID,
			Content: chunk.Content,
			Metadata: map[string]string{
				"uid":        syl.UID,
				"title":      syl.Title,
				"teachers":   strings.Join(syl.Teachers, ", "),
				"year":       fmt.Sprintf("%d", syl.Year),
				"term":       fmt.Sprintf("%d", syl.Term),
				"chunk_type": string(chunk.Type),
			},
		})
	}

	if err := v.collection.AddDocuments(ctx, docs, 4); err != nil {
		return fmt.Errorf("failed to add document chunks for %s: %w", syl.UID, err)
	}

	return nil
}

// addSyllabiInternal adds multiple syllabi (internal, assumes lock held)
// Uses chunking strategy: each syllabus produces multiple documents
func (v *VectorDB) addSyllabiInternal(ctx context.Context, syllabi []*storage.Syllabus) error {
	docs := make([]chromem.Document, 0, len(syllabi)*3) // Estimate 3 chunks per syllabus

	for _, syl := range syllabi {
		// Skip if all fields are empty or whitespace-only
		if strings.TrimSpace(syl.ObjectivesCN) == "" &&
			strings.TrimSpace(syl.ObjectivesEN) == "" &&
			strings.TrimSpace(syl.OutlineCN) == "" &&
			strings.TrimSpace(syl.OutlineEN) == "" &&
			strings.TrimSpace(syl.Schedule) == "" {
			continue
		}

		// Create Fields from Syllabus and generate chunks
		fields := &syllabus.Fields{
			ObjectivesCN: syl.ObjectivesCN,
			ObjectivesEN: syl.ObjectivesEN,
			OutlineCN:    syl.OutlineCN,
			OutlineEN:    syl.OutlineEN,
			Schedule:     syl.Schedule,
		}
		chunks := fields.ChunksForEmbedding(syl.Title)

		for _, chunk := range chunks {
			docID := fmt.Sprintf("%s_%s", syl.UID, chunk.Type)
			docs = append(docs, chromem.Document{
				ID:      docID,
				Content: chunk.Content,
				Metadata: map[string]string{
					"uid":        syl.UID,
					"title":      syl.Title,
					"teachers":   strings.Join(syl.Teachers, ", "),
					"year":       fmt.Sprintf("%d", syl.Year),
					"term":       fmt.Sprintf("%d", syl.Term),
					"chunk_type": string(chunk.Type),
				},
			})
		}
	}

	if len(docs) == 0 {
		return nil
	}

	if err := v.collection.AddDocuments(ctx, docs, 4); err != nil { // 4 concurrent embeddings
		return fmt.Errorf("failed to add documents: %w", err)
	}

	return nil
}

// Search performs semantic search for courses matching the query.
//
// The nResults parameter serves as a fallback count when no highly relevant results exist.
// When highly relevant results (>= 70% similarity) are found, they take priority:
//
//   - If highRelevanceCount > 0: Returns ceil(highRelevanceCount / 10) * 10 results
//     (e.g., 3 high relevance → 10, 13 high relevance → 20)
//   - If highRelevanceCount == 0: Returns up to nResults (the requested count)
//
// This ensures users always see all highly relevant matches, while nResults
// controls the fallback behavior for queries with no strong matches.
//
// Results are deduplicated by course UID, keeping the highest similarity chunk.
func (v *VectorDB) Search(ctx context.Context, query string, nResults int) ([]SearchResult, error) {
	if v == nil || v.collection == nil {
		return nil, nil
	}

	if query == "" {
		return nil, nil
	}

	if nResults <= 0 {
		nResults = DefaultSearchResults
	}
	if nResults > MaxSearchResults {
		nResults = MaxSearchResults
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	// Check collection size and adjust nResults if needed
	// chromem-go returns error if nResults > document count
	docCount := v.collection.Count()
	if docCount == 0 {
		return nil, nil // No documents to search
	}

	// Request more results than needed to account for deduplication
	// With chunking, we may get multiple chunks from the same course
	// Request enough to find all high-relevance results
	queryLimit := MaxSearchResults * 3
	if queryLimit > docCount {
		queryLimit = docCount
	}

	// Query the collection
	results, err := v.collection.Query(ctx, query, queryLimit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query collection: %w", err)
	}

	// Deduplicate by UID, keeping highest similarity for each course
	uidBestResult := make(map[string]SearchResult)

	for _, result := range results {
		// Skip results below minimum similarity threshold
		if result.Similarity < MinSimilarityThreshold {
			continue
		}

		// Extract UID from metadata (chunk documents store uid separately)
		uid := result.Metadata["uid"]
		if uid == "" {
			// Fallback: try to extract from document ID (format: "UID_chunktype")
			uid = extractUIDFromDocID(result.ID)
		}
		if uid == "" {
			continue
		}

		// Check if we already have a result for this UID
		existing, exists := uidBestResult[uid]
		if !exists || result.Similarity > existing.Similarity {
			sr := SearchResult{
				UID:        uid,
				Content:    result.Content,
				Similarity: result.Similarity,
			}

			// Extract metadata
			if title, ok := result.Metadata["title"]; ok {
				sr.Title = title
			}
			if teachers, ok := result.Metadata["teachers"]; ok && teachers != "" {
				sr.Teachers = strings.Split(teachers, ", ")
			}
			if yearStr, ok := result.Metadata["year"]; ok {
				_, _ = fmt.Sscanf(yearStr, "%d", &sr.Year)
			}
			if termStr, ok := result.Metadata["term"]; ok {
				_, _ = fmt.Sscanf(termStr, "%d", &sr.Term)
			}

			uidBestResult[uid] = sr
		}
	}

	// Convert map to slice and sort by similarity (descending)
	searchResults := make([]SearchResult, 0, len(uidBestResult))
	for _, sr := range uidBestResult {
		searchResults = append(searchResults, sr)
	}

	sort.Slice(searchResults, func(i, j int) bool {
		return searchResults[i].Similarity > searchResults[j].Similarity
	})

	// Apply the new relevance-based logic:
	// 1. Count highly relevant results (>= 70% similarity)
	// 2. All highly relevant results are always included
	// 3. Pad to next multiple of 10 (min 10 results)
	highRelevanceCount := 0
	for _, sr := range searchResults {
		if sr.Similarity >= HighRelevanceThreshold {
			highRelevanceCount++
		} else {
			break // Since sorted, no more high relevance after this
		}
	}

	// Calculate final result count
	finalCount := nResults // Default to requested count

	if highRelevanceCount > 0 {
		// Pad to next multiple of 10
		finalCount = ((highRelevanceCount + 9) / 10) * 10
		// Ensure at least 10 results
		if finalCount < 10 {
			finalCount = 10
		}
		// Don't exceed available results or max limit
		if finalCount > len(searchResults) {
			finalCount = len(searchResults)
		}
		if finalCount > MaxSearchResults {
			finalCount = MaxSearchResults
		}
	} else if finalCount > len(searchResults) {
		// No highly relevant results, just use default
		finalCount = len(searchResults)
	}

	if len(searchResults) > finalCount {
		searchResults = searchResults[:finalCount]
	}

	return searchResults, nil
}

// extractUIDFromDocID extracts UID from document ID format "UID_chunktype"
func extractUIDFromDocID(docID string) string {
	// Document ID format: "UID_chunktype" (e.g., "1131U0001_objectives")
	lastIdx := strings.LastIndex(docID, "_")
	if lastIdx > 0 {
		return docID[:lastIdx]
	}
	return ""
}

// UpdateSyllabus updates a syllabus in the vector store
// Removes all old chunks and adds new ones
func (v *VectorDB) UpdateSyllabus(ctx context.Context, syl *storage.Syllabus) error {
	if v == nil || v.collection == nil {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Delete all old chunks for this syllabus (5 chunk types)
	chunkTypes := []syllabus.ChunkType{
		syllabus.ChunkTypeObjectivesCN,
		syllabus.ChunkTypeObjectivesEN,
		syllabus.ChunkTypeOutlineCN,
		syllabus.ChunkTypeOutlineEN,
		syllabus.ChunkTypeSchedule,
	}
	for _, ct := range chunkTypes {
		docID := fmt.Sprintf("%s_%s", syl.UID, ct)
		if err := v.collection.Delete(ctx, nil, nil, docID); err != nil {
			// Ignore not found errors
			v.logger.WithError(err).WithField("docID", docID).Debug("Failed to delete old chunk")
		}
	}

	// Add new chunks
	return v.addSyllabusInternal(ctx, syl)
}

// DeleteSyllabus removes all chunks for a syllabus from the vector store
func (v *VectorDB) DeleteSyllabus(ctx context.Context, uid string) error {
	if v == nil || v.collection == nil {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Delete all chunks for this syllabus (5 chunk types)
	chunkTypes := []syllabus.ChunkType{
		syllabus.ChunkTypeObjectivesCN,
		syllabus.ChunkTypeObjectivesEN,
		syllabus.ChunkTypeOutlineCN,
		syllabus.ChunkTypeOutlineEN,
		syllabus.ChunkTypeSchedule,
	}
	for _, ct := range chunkTypes {
		docID := fmt.Sprintf("%s_%s", uid, ct)
		if err := v.collection.Delete(ctx, nil, nil, docID); err != nil {
			// Ignore errors (chunk might not exist)
			v.logger.WithError(err).WithField("docID", docID).Debug("Failed to delete chunk")
		}
	}

	return nil
}

// Count returns the number of documents in the collection
func (v *VectorDB) Count() int {
	if v == nil || v.collection == nil {
		return 0
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.collection.Count()
}

// IsEnabled returns true if the vector database is initialized and ready
func (v *VectorDB) IsEnabled() bool {
	if v == nil {
		return false
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.initialized && v.collection != nil
}

// Close closes the vector database
func (v *VectorDB) Close() error {
	if v == nil {
		return nil
	}
	// chromem-go automatically persists on operations
	// No explicit close needed
	return nil
}
