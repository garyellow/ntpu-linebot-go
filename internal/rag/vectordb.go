// Package rag provides Retrieval-Augmented Generation functionality
// using chromem-go for vector storage and Gemini for embeddings.
package rag

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	chromem "github.com/philippgille/chromem-go"

	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

const (
	// SyllabusCollectionName is the name of the syllabus collection in chromem
	SyllabusCollectionName = "syllabi"

	// DefaultSearchResults is the default number of results for semantic search
	DefaultSearchResults = 10

	// MaxSearchResults is the maximum number of results for semantic search
	MaxSearchResults = 20
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
func (v *VectorDB) addSyllabusInternal(ctx context.Context, syllabus *storage.Syllabus) error {
	if syllabus.Content == "" {
		return nil // Skip empty content
	}

	doc := chromem.Document{
		ID:      syllabus.UID,
		Content: syllabus.Content,
		Metadata: map[string]string{
			"title":    syllabus.Title,
			"teachers": strings.Join(syllabus.Teachers, ", "),
			"year":     fmt.Sprintf("%d", syllabus.Year),
			"term":     fmt.Sprintf("%d", syllabus.Term),
		},
	}

	if err := v.collection.AddDocument(ctx, doc); err != nil {
		return fmt.Errorf("failed to add document %s: %w", syllabus.UID, err)
	}

	return nil
}

// addSyllabiInternal adds multiple syllabi (internal, assumes lock held)
func (v *VectorDB) addSyllabiInternal(ctx context.Context, syllabi []*storage.Syllabus) error {
	docs := make([]chromem.Document, 0, len(syllabi))

	for _, syllabus := range syllabi {
		if syllabus.Content == "" {
			continue
		}

		docs = append(docs, chromem.Document{
			ID:      syllabus.UID,
			Content: syllabus.Content,
			Metadata: map[string]string{
				"title":    syllabus.Title,
				"teachers": strings.Join(syllabus.Teachers, ", "),
				"year":     fmt.Sprintf("%d", syllabus.Year),
				"term":     fmt.Sprintf("%d", syllabus.Term),
			},
		})
	}

	if len(docs) == 0 {
		return nil
	}

	if err := v.collection.AddDocuments(ctx, docs, 4); err != nil { // 4 concurrent embeddings
		return fmt.Errorf("failed to add documents: %w", err)
	}

	return nil
}

// Search performs semantic search for courses matching the query
// Returns up to nResults courses sorted by similarity
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

	// Query the collection
	results, err := v.collection.Query(ctx, query, nResults, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query collection: %w", err)
	}

	// Convert to SearchResult
	searchResults := make([]SearchResult, 0, len(results))
	for _, result := range results {
		sr := SearchResult{
			UID:        result.ID,
			Content:    result.Content,
			Similarity: result.Similarity,
		}

		// Extract metadata
		if title, ok := result.Metadata["title"]; ok {
			sr.Title = title
		}
		if teachers, ok := result.Metadata["teachers"]; ok {
			sr.Teachers = strings.Split(teachers, ", ")
		}
		if yearStr, ok := result.Metadata["year"]; ok {
			_, _ = fmt.Sscanf(yearStr, "%d", &sr.Year)
		}
		if termStr, ok := result.Metadata["term"]; ok {
			_, _ = fmt.Sscanf(termStr, "%d", &sr.Term)
		}

		searchResults = append(searchResults, sr)
	}

	return searchResults, nil
}

// UpdateSyllabus updates a syllabus in the vector store
// Removes old embedding and adds new one
func (v *VectorDB) UpdateSyllabus(ctx context.Context, syllabus *storage.Syllabus) error {
	if v == nil || v.collection == nil {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Delete old document
	if err := v.collection.Delete(ctx, nil, nil, syllabus.UID); err != nil {
		// Ignore not found errors
		v.logger.WithError(err).WithField("uid", syllabus.UID).Debug("Failed to delete old syllabus")
	}

	// Add new document
	return v.addSyllabusInternal(ctx, syllabus)
}

// DeleteSyllabus removes a syllabus from the vector store
func (v *VectorDB) DeleteSyllabus(ctx context.Context, uid string) error {
	if v == nil || v.collection == nil {
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	return v.collection.Delete(ctx, nil, nil, uid)
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
