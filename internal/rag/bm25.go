// Package rag provides Retrieval-Augmented Generation functionality
package rag

import (
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

// BM25Index provides keyword-based search using BM25 algorithm
// Complements vector search for hybrid search functionality
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
		chunks := fields.ChunksForEmbedding(syl.Title)

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

// AddSyllabus adds a single syllabus to the index
// Note: This rebuilds the entire index (BM25 doesn't support incremental updates)
func (idx *BM25Index) AddSyllabus(syl *storage.Syllabus) error {
	// For now, we don't support incremental updates
	// The index should be rebuilt during warmup
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
