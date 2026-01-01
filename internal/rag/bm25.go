// Package rag provides Retrieval-Augmented Generation capabilities
// using BM25 keyword search with LLM query expansion.
package rag

import (
	"context"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
	"github.com/iwilltry42/bm25-go/bm25"
)

// MaxSearchResults is the maximum number of results to return per semester.
// Each semester independently gets up to this many results.
const (
	MaxSearchResults = 10
)

// SearchResult represents a search result with confidence score.
// Confidence is derived from relative BM25 score within the same semester.
type SearchResult struct {
	UID        string   // Course UID
	Title      string   // Course title
	Teachers   []string // Course teachers
	Year       int      // Academic year
	Term       int      // Semester
	Confidence float32  // Relative score (0-1), score / maxScore within same semester
}

// SemesterKey uniquely identifies a semester for indexing.
type SemesterKey struct {
	Year int
	Term int
}

// semesterIndex holds BM25 index for a single semester.
// Each semester has its own IDF calculation, ensuring independent relevance scoring.
type semesterIndex struct {
	engine   *bm25.BM25Okapi    // BM25 engine for this semester only
	corpus   []string           // Document strings for this semester
	uidList  []string           // UID at each index
	metadata map[string]docMeta // UID -> metadata
}

// BM25Index provides keyword-based search using BM25 algorithm.
// Uses per-semester indexing strategy for independent relevance scoring.
//
// Per-Semester Index Strategy:
//   - Each semester has its own BM25 engine with independent IDF calculation
//   - Term importance is calculated relative to courses in the same semester
//   - A term common in semester A but rare in semester B will have different weights
//   - Ensures fair ranking within each semester without cross-semester influence
//
// Uses github.com/iwilltry42/bm25-go library (maintained by k3d-io/k3d maintainer)
type BM25Index struct {
	semesterIndexes map[SemesterKey]*semesterIndex // Per-semester BM25 indexes
	allSemesters    []SemesterKey                  // All semesters sorted (newest first)

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

// BM25Result represents a BM25 search result (internal use)
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
		semesterIndexes: make(map[SemesterKey]*semesterIndex),
		logger:          log,
	}
}

// Initialize builds per-semester BM25 indexes from syllabi.
// Each semester gets its own index with independent IDF calculation.
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
	idx.semesterIndexes = make(map[SemesterKey]*semesterIndex)
	idx.allSemesters = nil

	// Step 1: Group syllabi by semester
	semesterGroups := make(map[SemesterKey][]*storage.Syllabus)
	for _, syl := range syllabi {
		key := SemesterKey{Year: syl.Year, Term: syl.Term}
		semesterGroups[key] = append(semesterGroups[key], syl)
	}

	// Step 2: Collect and sort all semesters (newest first)
	for key := range semesterGroups {
		idx.allSemesters = append(idx.allSemesters, key)
	}
	slices.SortFunc(idx.allSemesters, func(a, b SemesterKey) int {
		if a.Year != b.Year {
			return b.Year - a.Year // Descending by year
		}
		return b.Term - a.Term // Descending by term
	})

	// Step 3: Build independent index for each semester
	totalCourses := 0
	for sem, syllabiInSem := range semesterGroups {
		semIdx := &semesterIndex{
			metadata: make(map[string]docMeta),
		}

		for _, syl := range syllabiInSem {
			// Store metadata
			semIdx.metadata[syl.UID] = docMeta{
				Title:    syl.Title,
				Teachers: syl.Teachers,
				Year:     syl.Year,
				Term:     syl.Term,
			}

			// Create single document from all fields
			fields := &syllabus.Fields{
				Objectives: syl.Objectives,
				Outline:    syl.Outline,
				Schedule:   syl.Schedule,
			}
			content := fields.ContentForIndexing(syl.Title)

			if strings.TrimSpace(content) == "" {
				continue
			}

			semIdx.corpus = append(semIdx.corpus, content)
			semIdx.uidList = append(semIdx.uidList, syl.UID)
		}

		// Build BM25 engine for this semester (independent IDF)
		if len(semIdx.corpus) > 0 {
			var err error
			semIdx.engine, err = bm25.NewBM25Okapi(semIdx.corpus, tokenizeChinese, 1.5, 0.75, nil)
			if err != nil {
				return err
			}
			idx.semesterIndexes[sem] = semIdx
			totalCourses += len(semIdx.corpus)
		}
	}

	idx.initialized = true
	idx.logger.WithField("courses", totalCourses).
		WithField("semesters", len(idx.semesterIndexes)).
		Info("BM25 index initialized (per-semester)")
	return nil
}

// RebuildFromDB reloads all syllabi from the database and rebuilds the index.
// This is called during warmup after new syllabi are saved to ensure
// the index contains complete syllabus content (not just metadata).
func (idx *BM25Index) RebuildFromDB(ctx context.Context, db *storage.DB) error {
	if idx == nil {
		return nil
	}

	// Load all syllabi from database (includes full content)
	syllabi, err := db.GetAllSyllabi(ctx)
	if err != nil {
		return err
	}

	// Reinitialize index with complete data
	return idx.Initialize(syllabi)
}

// getNewestTwoSemesters returns the newest 2 semesters from the index.
func (idx *BM25Index) getNewestTwoSemesters() []SemesterKey {
	if len(idx.allSemesters) == 0 {
		return nil
	}
	count := min(2, len(idx.allSemesters))
	return idx.allSemesters[:count]
}

// searchSemester performs BM25 search on a specific semester's index.
func (semIdx *semesterIndex) search(query string, topN int) []BM25Result {
	if semIdx == nil || semIdx.engine == nil || len(semIdx.corpus) == 0 {
		return nil
	}

	// Tokenize query
	queryTokens := tokenizeChinese(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Get scores from BM25 engine
	scores, err := semIdx.engine.GetScores(queryTokens)
	if err != nil {
		return nil
	}

	// Collect and sort results
	type scoredDoc struct {
		docID int
		score float64
	}
	var scoredDocs []scoredDoc

	for docID, score := range scores {
		if score != 0 {
			scoredDocs = append(scoredDocs, scoredDoc{docID: docID, score: score})
		}
	}

	// Sort by score descending
	slices.SortFunc(scoredDocs, func(a, b scoredDoc) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}
		return 0
	})

	// Limit results by Top-K
	if topN > 0 && len(scoredDocs) > topN {
		scoredDocs = scoredDocs[:topN]
	}

	// Convert to results
	results := make([]BM25Result, 0, len(scoredDocs))
	for rank, sd := range scoredDocs {
		if sd.docID >= len(semIdx.uidList) {
			continue
		}
		uid := semIdx.uidList[sd.docID]
		meta := semIdx.metadata[uid]
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

	return results
}

// SearchCourses performs BM25 search on the newest 2 semesters independently.
// Each semester gets its own Top-K results with confidence calculated relative to
// that semester's best match. This ensures fair representation of both semesters.
//
// Per-Semester Independent Search:
//   - Finds the newest 2 semesters in the index
//   - Searches each semester's index independently
//   - Each semester gets up to topN results
//   - Confidence is calculated within each semester (best match = 1.0)
//   - Results from both semesters are combined and returned
func (idx *BM25Index) SearchCourses(_ context.Context, query string, topN int) ([]SearchResult, error) {
	if idx == nil || !idx.initialized || len(idx.semesterIndexes) == 0 {
		return nil, nil
	}

	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Get newest 2 semesters
	newestTwo := idx.getNewestTwoSemesters()
	if len(newestTwo) == 0 {
		return nil, nil
	}

	// Search each semester independently
	var results []SearchResult
	for _, sem := range newestTwo {
		semIdx := idx.semesterIndexes[sem]
		if semIdx == nil {
			continue
		}

		// Search this semester's index
		semResults := semIdx.search(query, topN)
		if len(semResults) == 0 {
			continue
		}

		// Find max score within this semester for confidence calculation
		maxScore := semResults[0].Score
		for _, r := range semResults[1:] {
			if (maxScore >= 0 && r.Score > maxScore) || (maxScore < 0 && r.Score > maxScore) {
				maxScore = r.Score
			}
		}

		// Calculate relative confidence within this semester
		for _, r := range semResults {
			results = append(results, SearchResult{
				UID:        r.UID,
				Title:      r.Title,
				Teachers:   r.Teachers,
				Year:       r.Year,
				Term:       r.Term,
				Confidence: computeRelativeConfidence(r.Score, maxScore),
			})
		}
	}

	return results, nil
}

// computeRelativeConfidence calculates confidence as relative BM25 score.
// This is the standard approach for BM25 result classification.
//
// BM25 Score Distribution (Academic Research - Arampatzis et al., 2009):
//   - Relevant documents: Normal (Gaussian) distribution at high scores
//   - Non-relevant documents: Exponential distribution at low scores
//   - This "Normal-Exponential mixture model" is the standard for BM25
//
// Why use relative score (score/maxScore):
//   - Absolute thresholds are not comparable across queries
//   - Relative thresholds work better than absolute ones (academic consensus)
//   - With per-semester indexing, confidence is relative within the same semester
//
// Formula: score / maxScore
//   - Best result in each semester always has confidence = 1.0
//   - Other results are relative to the best match in the same semester
//
// Classification thresholds (in handler):
//   - >= 0.8: "最佳匹配" (Best Match) - Normal distribution core
//   - >= 0.6: "高度相關" (Highly Relevant) - Mixed region
//   - < 0.6: "部分相關" (Partially Relevant) - Exponential tail
func computeRelativeConfidence(score, maxScore float64) float32 {
	// Handle zero or invalid maxScore
	if maxScore == 0 {
		return 0
	}

	// Both positive: normal case (higher score = higher confidence)
	if maxScore > 0 && score > 0 {
		confidence := score / maxScore
		if confidence > 1.0 {
			confidence = 1.0
		}
		return float32(confidence)
	}

	// Both negative: inverse case (less negative = higher confidence)
	if maxScore < 0 && score < 0 {
		confidence := maxScore / score // Note: inverted division for negative scores
		if confidence > 1.0 {
			confidence = 1.0
		}
		if confidence < 0 {
			confidence = 0
		}
		return float32(confidence)
	}

	// Mixed signs (unusual): treat as 0 confidence
	return 0
}

// AddSyllabus adds a single syllabus to the appropriate semester index.
// Note: BM25 requires full IDF recalculation, so this rebuilds the semester's engine.
// For batch additions, prefer collecting all syllabi and calling Initialize().
func (idx *BM25Index) AddSyllabus(syl *storage.Syllabus) error {
	if idx == nil || syl == nil {
		return nil
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	sem := SemesterKey{Year: syl.Year, Term: syl.Term}

	// Get or create semester index
	semIdx := idx.semesterIndexes[sem]
	if semIdx == nil {
		semIdx = &semesterIndex{
			metadata: make(map[string]docMeta),
		}
		idx.semesterIndexes[sem] = semIdx

		// Add to allSemesters if new
		idx.allSemesters = append(idx.allSemesters, sem)
		slices.SortFunc(idx.allSemesters, func(a, b SemesterKey) int {
			if a.Year != b.Year {
				return b.Year - a.Year
			}
			return b.Term - a.Term
		})
	}

	// Check if syllabus already exists (by UID)
	if _, exists := semIdx.metadata[syl.UID]; exists {
		return nil
	}

	// Add new syllabus metadata
	semIdx.metadata[syl.UID] = docMeta{
		Title:    syl.Title,
		Teachers: syl.Teachers,
		Year:     syl.Year,
		Term:     syl.Term,
	}

	// Create single document from all fields
	fields := &syllabus.Fields{
		Objectives: syl.Objectives,
		Outline:    syl.Outline,
		Schedule:   syl.Schedule,
	}
	content := fields.ContentForIndexing(syl.Title)

	if strings.TrimSpace(content) == "" {
		return nil
	}

	// Add to corpus
	semIdx.corpus = append(semIdx.corpus, content)
	semIdx.uidList = append(semIdx.uidList, syl.UID)

	// Rebuild BM25 engine for this semester (required for IDF recalculation)
	var err error
	semIdx.engine, err = bm25.NewBM25Okapi(semIdx.corpus, tokenizeChinese, 1.5, 0.75, nil)
	if err != nil {
		return err
	}

	idx.initialized = true

	idx.logger.WithField("uid", syl.UID).
		WithField("semester", sem).
		Debug("Added syllabus to BM25 index")
	return nil
}

// IsEnabled returns true if the index is initialized
func (idx *BM25Index) IsEnabled() bool {
	if idx == nil {
		return false
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.initialized && len(idx.semesterIndexes) > 0
}

// Count returns the total number of courses (documents) across all semesters
func (idx *BM25Index) Count() int {
	if idx == nil {
		return 0
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	total := 0
	for _, semIdx := range idx.semesterIndexes {
		total += len(semIdx.corpus)
	}
	return total
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
