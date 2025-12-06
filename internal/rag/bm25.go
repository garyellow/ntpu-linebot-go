// Package rag provides Retrieval-Augmented Generation functionality.
// Uses BM25 keyword search with LLM-based query expansion for course retrieval.
package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/crawlab-team/bm25"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
)

// SearchResult represents a search result with confidence score.
// Confidence is derived from BM25 rank position, not semantic similarity.
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
type BM25Index struct {
	bm25Okapi   *bm25.BM25Okapi
	corpus      []string           // Tokenized document content
	rawDocs     []string           // Original document content
	uidToDocIDs map[string][]int   // UID -> document indices (multiple chunks per UID)
	docIDToUID  map[int]string     // Document index -> UID
	metadata    map[string]docMeta // UID -> metadata
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
		uidToDocIDs: make(map[string][]int),
		docIDToUID:  make(map[int]string),
		metadata:    make(map[string]docMeta),
		logger:      log,
	}
}

// Initialize builds the BM25 index from syllabi
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

	// Build corpus from syllabi chunks
	var corpus []string
	var rawDocs []string
	idx.uidToDocIDs = make(map[string][]int)
	idx.docIDToUID = make(map[int]string)
	idx.metadata = make(map[string]docMeta)

	docIndex := 0
	for _, syl := range syllabi {
		// Store metadata
		idx.metadata[syl.UID] = docMeta{
			Title:    syl.Title,
			Teachers: syl.Teachers,
			Year:     syl.Year,
			Term:     syl.Term,
		}

		// Create Fields and generate chunks
		fields := &syllabus.Fields{
			ObjectivesCN: syl.ObjectivesCN,
			ObjectivesEN: syl.ObjectivesEN,
			OutlineCN:    syl.OutlineCN,
			OutlineEN:    syl.OutlineEN,
			Schedule:     syl.Schedule,
		}
		chunks := fields.ChunksForIndexing(syl.Title)

		for _, chunk := range chunks {
			if strings.TrimSpace(chunk.Content) == "" {
				continue
			}
			// Store raw content
			rawDocs = append(rawDocs, chunk.Content)
			// Tokenize and store
			corpus = append(corpus, chunk.Content)
			// Map indices
			idx.uidToDocIDs[syl.UID] = append(idx.uidToDocIDs[syl.UID], docIndex)
			idx.docIDToUID[docIndex] = syl.UID
			docIndex++
		}
	}

	if len(corpus) == 0 {
		idx.initialized = true
		return nil
	}

	idx.corpus = corpus
	idx.rawDocs = rawDocs

	// Create BM25 index with Chinese tokenizer
	// k1=1.5, b=0.75 are standard BM25 parameters
	bm25Okapi, err := bm25.NewBM25Okapi(corpus, tokenizeChinese, 1.5, 0.75, nil)
	if err != nil {
		return fmt.Errorf("failed to create BM25 index: %w", err)
	}
	idx.bm25Okapi = bm25Okapi
	idx.initialized = true

	idx.logger.WithField("docs", len(corpus)).Info("BM25 index initialized")
	return nil
}

// AddSyllabi adds new syllabi to the BM25 index incrementally.
// This is called during warmup to update the index with new data.
// For simplicity, this re-initializes the entire index with all syllabi.
// Context parameter is for API compatibility.
func (idx *BM25Index) AddSyllabi(_ context.Context, syllabi []*storage.Syllabus) error {
	if len(syllabi) == 0 {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// For incremental updates, we need to rebuild the entire index
	// since BM25 requires all documents for IDF calculation
	// Get existing syllabi from metadata
	var allSyllabi []*storage.Syllabus
	existingUIDs := make(map[string]bool)

	for uid, meta := range idx.metadata {
		existingUIDs[uid] = true
		// Reconstruct syllabus from metadata (basic info only)
		allSyllabi = append(allSyllabi, &storage.Syllabus{
			UID:      uid,
			Title:    meta.Title,
			Teachers: meta.Teachers,
			Year:     meta.Year,
			Term:     meta.Term,
		})
	}

	// Add new syllabi (avoid duplicates)
	for _, syl := range syllabi {
		if !existingUIDs[syl.UID] {
			allSyllabi = append(allSyllabi, syl)
		}
	}

	// Reinitialize with all syllabi
	idx.initialized = false
	idx.corpus = nil
	idx.rawDocs = nil
	idx.uidToDocIDs = make(map[string][]int)
	idx.docIDToUID = make(map[int]string)
	idx.metadata = make(map[string]docMeta)
	idx.bm25Okapi = nil
	idx.mu.Unlock()

	err := idx.Initialize(allSyllabi)
	idx.mu.Lock() // Re-lock for deferred unlock
	return err
}

// Search performs BM25 keyword search
// Returns results sorted by BM25 score (descending)
func (idx *BM25Index) Search(query string, topN int) ([]BM25Result, error) {
	if idx == nil || !idx.initialized || idx.bm25Okapi == nil {
		return nil, nil
	}

	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Tokenize query
	tokenizedQuery := tokenizeChinese(query)
	if len(tokenizedQuery) == 0 {
		return nil, nil
	}

	// Get BM25 scores for all documents
	scores, err := idx.bm25Okapi.GetScores(tokenizedQuery)
	if err != nil {
		return nil, fmt.Errorf("BM25 scoring failed: %w", err)
	}

	// Create scored results
	type scoredDoc struct {
		docID int
		score float64
	}
	var scoredDocs []scoredDoc
	for docID, score := range scores {
		if score > 0 {
			scoredDocs = append(scoredDocs, scoredDoc{docID: docID, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})

	// Deduplicate by UID, keeping highest score
	uidBest := make(map[string]scoredDoc)
	for _, sd := range scoredDocs {
		uid := idx.docIDToUID[sd.docID]
		if uid == "" {
			continue
		}
		if existing, exists := uidBest[uid]; !exists || sd.score > existing.score {
			uidBest[uid] = sd
		}
	}

	// Convert to results and sort
	results := make([]BM25Result, 0, len(uidBest))
	for uid, sd := range uidBest {
		meta := idx.metadata[uid]
		results = append(results, BM25Result{
			UID:      uid,
			Title:    meta.Title,
			Teachers: meta.Teachers,
			Year:     meta.Year,
			Term:     meta.Term,
			Score:    sd.score,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Assign ranks and limit results
	for i := range results {
		results[i].Rank = i + 1
	}

	if topN > 0 && len(results) > topN {
		results = results[:topN]
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
// Note: BM25 doesn't support incremental updates, so this triggers a full rebuild.
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

	// Generate chunks for new syllabus
	fields := &syllabus.Fields{
		ObjectivesCN: syl.ObjectivesCN,
		ObjectivesEN: syl.ObjectivesEN,
		OutlineCN:    syl.OutlineCN,
		OutlineEN:    syl.OutlineEN,
		Schedule:     syl.Schedule,
	}
	chunks := fields.ChunksForIndexing(syl.Title)

	docIndex := len(idx.corpus)
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		idx.rawDocs = append(idx.rawDocs, chunk.Content)
		idx.corpus = append(idx.corpus, chunk.Content)
		idx.uidToDocIDs[syl.UID] = append(idx.uidToDocIDs[syl.UID], docIndex)
		idx.docIDToUID[docIndex] = syl.UID
		docIndex++
	}

	// Rebuild BM25 index from updated corpus
	if len(idx.corpus) == 0 {
		return nil
	}

	bm25Okapi, err := bm25.NewBM25Okapi(idx.corpus, tokenizeChinese, 1.5, 0.75, nil)
	if err != nil {
		return fmt.Errorf("failed to rebuild BM25 index: %w", err)
	}
	idx.bm25Okapi = bm25Okapi
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
	return idx.initialized && idx.bm25Okapi != nil
}

// Count returns the number of documents in the index
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
// 3. Generate character bigrams for Chinese text (handles no-space languages)
// 4. Keep individual characters for short queries
func tokenizeChinese(text string) []string {
	text = strings.ToLower(text)

	var tokens []string
	var currentWord strings.Builder

	runes := []rune(text)
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			// Check if this is a CJK character
			if isCJK(r) {
				// Flush any pending non-CJK word
				if currentWord.Len() > 0 {
					tokens = append(tokens, currentWord.String())
					currentWord.Reset()
				}
				// Add individual character
				tokens = append(tokens, string(r))
				// Add bigram with next character if exists
				if i+1 < len(runes) && isCJK(runes[i+1]) {
					tokens = append(tokens, string(r)+string(runes[i+1]))
				}
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
