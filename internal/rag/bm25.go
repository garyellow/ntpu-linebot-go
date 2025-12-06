// Package rag provides Retrieval-Augmented Generation functionality.
// Uses BM25 keyword search with LLM-based query expansion for course retrieval.
package rag

import (
	"context"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
	"github.com/iwilltry42/bm25-go/bm25"
)

// SearchResult represents a search result with confidence score.
// Confidence is derived from BM25 rank position, not vector similarity.
type SearchResult struct {
	UID        string   // Course UID
	Title      string   // Course title
	Teachers   []string // Course teachers
	Year       int      // Academic year
	Term       int      // Semester
	Confidence float32  // Rank-based confidence (0-1), higher = more relevant
}

// BM25Index provides keyword-based search using BM25 algorithm.
// Combined with LLM query expansion, provides effective course retrieval.
//
// Single Document Strategy (BM25 Best Practice):
// - Each course = 1 document (not chunked like embedding models)
// - BM25's length normalization (b=0.75) handles document length differences
// - More accurate IDF calculation with 1:1 course-to-document mapping
// - Simpler architecture: no deduplication logic needed
//
// Uses github.com/iwilltry42/bm25-go library (maintained by k3d-io/k3d maintainer)
type BM25Index struct {
	engine   *bm25.BM25Okapi    // External BM25 implementation
	corpus   []string           // Original document strings (for GetTopN)
	uidList  []string           // UID at each index
	metadata map[string]docMeta // UID -> metadata

	logger      *logger.Logger
	mu          sync.RWMutex
	initialized bool
}

// docMeta stores metadata for a document
type docMeta struct {
	Title    string
	Teachers []string
	Year     int
	Term     int
}

// BM25Result represents a BM25 search result
type BM25Result struct {
	UID      string
	Title    string
	Teachers []string
	Year     int
	Term     int
	Score    float64 // BM25 score (higher is better)
	Rank     int     // Rank position (1-indexed)
}

// NewBM25Index creates a new BM25 index
func NewBM25Index(log *logger.Logger) *BM25Index {
	return &BM25Index{
		metadata: make(map[string]docMeta),
		logger:   log,
	}
}

// Initialize builds the BM25 index from syllabi
// Each syllabus becomes a single document (no chunking - BM25 best practice)
func (idx *BM25Index) Initialize(syllabi []*storage.Syllabus) error {
	if idx == nil {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if len(syllabi) == 0 {
		idx.initialized = true
		return nil
	}

	// Reset index data
	idx.corpus = nil
	idx.uidList = nil
	idx.metadata = make(map[string]docMeta)

	for _, syl := range syllabi {
		// Store metadata
		idx.metadata[syl.UID] = docMeta{
			Title:    syl.Title,
			Teachers: syl.Teachers,
			Year:     syl.Year,
			Term:     syl.Term,
		}

		// Create single document from all fields
		fields := &syllabus.Fields{
			ObjectivesCN: syl.ObjectivesCN,
			ObjectivesEN: syl.ObjectivesEN,
			OutlineCN:    syl.OutlineCN,
			OutlineEN:    syl.OutlineEN,
			Schedule:     syl.Schedule,
		}
		content := fields.ContentForIndexing(syl.Title)

		if strings.TrimSpace(content) == "" {
			continue
		}

		idx.corpus = append(idx.corpus, content)
		idx.uidList = append(idx.uidList, syl.UID)
	}

	// Build BM25 engine with external library
	if len(idx.corpus) > 0 {
		var err error
		idx.engine, err = bm25.NewBM25Okapi(idx.corpus, tokenizeChinese, 1.5, 0.75, nil)
		if err != nil {
			return err
		}
	}

	idx.initialized = true
	idx.logger.WithField("courses", len(idx.corpus)).Info("BM25 index initialized")
	return nil
}

// SyllabiLoader is an interface for loading syllabi from storage.
// This allows BM25Index to reload data without direct storage dependency.
type SyllabiLoader interface {
	GetAllSyllabi(ctx context.Context) ([]*storage.Syllabus, error)
}

// RebuildFromDB reloads all syllabi from the database and rebuilds the index.
// This is called during warmup after new syllabi are saved to ensure
// the index contains complete syllabus content (not just metadata).
// BM25 requires all documents for IDF calculation, so full rebuild is necessary.
func (idx *BM25Index) RebuildFromDB(ctx context.Context, loader SyllabiLoader) error {
	if idx == nil {
		return nil
	}

	// Load all syllabi from database (includes full content)
	syllabi, err := loader.GetAllSyllabi(ctx)
	if err != nil {
		return err
	}

	// Reinitialize index with complete data
	return idx.Initialize(syllabi)
}

// Search performs BM25 keyword search
// Returns results sorted by BM25 score (descending)
// With single document strategy, no deduplication is needed (1 course = 1 document)
func (idx *BM25Index) Search(query string, topN int) ([]BM25Result, error) {
	if idx == nil || !idx.initialized || len(idx.corpus) == 0 || idx.engine == nil {
		return nil, nil
	}

	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Tokenize query
	queryTokens := tokenizeChinese(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	// Get scores from external library
	scores, err := idx.engine.GetScores(queryTokens)
	if err != nil {
		return nil, err
	}

	// Collect and sort results (filter scores > 0 OR negative but not zero)
	type scoredDoc struct {
		docID int
		score float64
	}
	var scoredDocs []scoredDoc

	for docID, score := range scores {
		// Include all non-zero scores (positive or negative)
		// Negative scores can occur when term appears in all documents (IDF edge case)
		if score != 0 {
			scoredDocs = append(scoredDocs, scoredDoc{docID: docID, score: score})
		}
	}

	// Sort by score descending using O(n log n) algorithm
	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})

	// Limit results
	if topN > 0 && len(scoredDocs) > topN {
		scoredDocs = scoredDocs[:topN]
	}

	// Convert to results
	results := make([]BM25Result, 0, len(scoredDocs))
	for rank, sd := range scoredDocs {
		if sd.docID >= len(idx.uidList) {
			continue
		}
		uid := idx.uidList[sd.docID]
		meta := idx.metadata[uid]
		results = append(results, BM25Result{
			UID:      uid,
			Title:    meta.Title,
			Teachers: meta.Teachers,
			Year:     meta.Year,
			Term:     meta.Term,
			Score:    sd.score,
			Rank:     rank + 1,
		})
	}

	return results, nil
}

// SearchCourses performs BM25 search and returns SearchResult with confidence scores.
// This is the primary search interface for course retrieval.
// Context parameter is for API compatibility (not used in BM25).
func (idx *BM25Index) SearchCourses(_ context.Context, query string, topN int) ([]SearchResult, error) {
	bm25Results, err := idx.Search(query, topN)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, len(bm25Results))
	for i, r := range bm25Results {
		results[i] = SearchResult{
			UID:        r.UID,
			Title:      r.Title,
			Teachers:   r.Teachers,
			Year:       r.Year,
			Term:       r.Term,
			Confidence: computeRankConfidence(r.Rank),
		}
	}

	return results, nil
}

// computeRankConfidence calculates confidence score from BM25 rank.
// BM25 scores are unbounded and query-dependent, so we use rank as a proxy.
//
// Formula: 1 / (1 + 0.05 * rank)
//   - rank 1 → 0.95
//   - rank 5 → 0.80
//   - rank 10 → 0.67
//   - rank 20 → 0.50
func computeRankConfidence(rank int) float32 {
	if rank <= 0 {
		return 0
	}
	return float32(1.0 / (1.0 + 0.05*float64(rank)))
}

// AddSyllabus adds a single syllabus to the index.
// Note: BM25 requires full IDF recalculation, so this rebuilds the engine.
// For batch additions, prefer collecting all syllabi and calling Initialize().
func (idx *BM25Index) AddSyllabus(syl *storage.Syllabus) error {
	if idx == nil || syl == nil {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Check if syllabus already exists (by UID)
	if _, exists := idx.metadata[syl.UID]; exists {
		// Already in index, skip (update not supported)
		return nil
	}

	// Add new syllabus metadata
	idx.metadata[syl.UID] = docMeta{
		Title:    syl.Title,
		Teachers: syl.Teachers,
		Year:     syl.Year,
		Term:     syl.Term,
	}

	// Create single document from all fields
	fields := &syllabus.Fields{
		ObjectivesCN: syl.ObjectivesCN,
		ObjectivesEN: syl.ObjectivesEN,
		OutlineCN:    syl.OutlineCN,
		OutlineEN:    syl.OutlineEN,
		Schedule:     syl.Schedule,
	}
	content := fields.ContentForIndexing(syl.Title)

	if strings.TrimSpace(content) == "" {
		return nil
	}

	// Add to corpus
	idx.corpus = append(idx.corpus, content)
	idx.uidList = append(idx.uidList, syl.UID)

	// Rebuild BM25 engine (required for IDF recalculation)
	var err error
	idx.engine, err = bm25.NewBM25Okapi(idx.corpus, tokenizeChinese, 1.5, 0.75, nil)
	if err != nil {
		return err
	}

	idx.initialized = true

	idx.logger.WithField("uid", syl.UID).Debug("Added syllabus to BM25 index")
	return nil
}

// IsEnabled returns true if the index is initialized
func (idx *BM25Index) IsEnabled() bool {
	if idx == nil {
		return false
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.initialized && len(idx.corpus) > 0
}

// Count returns the number of courses (documents) in the index
// With single document strategy, this equals the number of syllabi indexed
func (idx *BM25Index) Count() int {
	if idx == nil {
		return 0
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.corpus)
}

// tokenizeChinese performs tokenization optimized for Chinese text
// Strategy:
// 1. Lowercase for case-insensitive matching
// 2. Split on whitespace and punctuation
// 3. Keep individual CJK characters as tokens (unigrams only)
// 4. Keep non-CJK words as single tokens
func tokenizeChinese(text string) []string {
	text = strings.ToLower(text)

	var tokens []string
	var currentWord strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			// Check if this is a CJK character
			if isCJK(r) {
				// Flush any pending non-CJK word
				if currentWord.Len() > 0 {
					tokens = append(tokens, currentWord.String())
					currentWord.Reset()
				}
				// Add individual character (unigram only)
				tokens = append(tokens, string(r))
			} else {
				// Non-CJK: accumulate into word
				currentWord.WriteRune(r)
			}
		} else {
			// Separator (whitespace, punctuation)
			if currentWord.Len() > 0 {
				tokens = append(tokens, currentWord.String())
				currentWord.Reset()
			}
		}
	}

	// Don't forget trailing word
	if currentWord.Len() > 0 {
		tokens = append(tokens, currentWord.String())
	}

	return tokens
}

// isCJK returns true if the rune is a CJK character
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || // Chinese
		unicode.Is(unicode.Hiragana, r) || // Japanese Hiragana
		unicode.Is(unicode.Katakana, r) || // Japanese Katakana
		unicode.Is(unicode.Hangul, r) // Korean
}
