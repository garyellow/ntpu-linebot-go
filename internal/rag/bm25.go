// Package rag provides Retrieval-Augmented Generation capabilities
// using BM25 keyword search with LLM query expansion.
package rag

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
)

// MaxSearchResults is the maximum number of results to return per semester.
// Each semester independently gets up to this many results.
const (
	MaxSearchResults = 10

	// MinConfidence is the minimum relative confidence score for a search result
	// to be included. Results below this threshold are filtered out as noise.
	//
	// Rationale (small corpus, ~2000-5000 docs):
	//   - BM25 absolute scores are query-dependent and not comparable across queries
	//   - We use relative scoring (score/maxScore) which is the academic consensus approach
	//     (Arampatzis et al., 2009: Normal-Exponential mixture model for BM25 score distributions)
	//   - Score ratio ≥ 0.25 filters out tail noise while retaining partially relevant results
	//   - Per-semester relative scoring ensures fair cutoff within each semester's IDF context
	MinConfidence = 0.25
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
	engine   *bm25Engine        // BM25 engine for this semester only
	uidList  []string           // UID at each index (Corresponds to engine internal doc IDs)
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
// Optimization Notes:
//   - Does NOT store raw text corpus in memory (significant savings)
//   - Loads syllabi semester-by-semester during initialization to minimize peak memory
//
// Uses an in-house BM25 Okapi engine (internal/rag/engine.go) with inverted index;
// documents are tokenized exactly once at index build time, so queries involve
// zero tokenizer calls.
type BM25Index struct {
	semesterIndexes map[SemesterKey]*semesterIndex // Per-semester BM25 indexes
	allSemesters    []SemesterKey                  // All semesters sorted (newest first)

	seg         *stringutil.Segmenter // Chinese word segmenter (shared)
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

// NewBM25Index creates a new BM25 index with shared Chinese segmenter.
// The segmenter must be pre-initialized and non-nil.
func NewBM25Index(log *logger.Logger, seg *stringutil.Segmenter) *BM25Index {
	if seg == nil {
		panic("bm25: segmenter must not be nil")
	}
	return &BM25Index{
		semesterIndexes: make(map[SemesterKey]*semesterIndex),
		seg:             seg,
		logger:          log,
	}
}

// Initialize builds BM25 indexes from the database.
//
// Concurrency design:
//   - All CPU-heavy work (tokenization, inverted-index build) happens WITHOUT holding
//     any lock, so ongoing SearchCourses calls continue using the previous index.
//   - Only the pointer swap at the end requires a brief write lock (microseconds).
//
// Memory strategy (one semester at a time):
//   - Syllabi are loaded per-semester so the full corpus is never in memory at once.
//   - Local syllabi slices go out of scope after each semester, letting GC reclaim them.
func (idx *BM25Index) Initialize(ctx context.Context, db *storage.DB) error {
	if idx == nil {
		return nil
	}

	// ── Build phase (no lock) ─────────────────────────────────────────────────
	// All expensive tokenization and index construction happens here,
	// while the existing index (if any) remains available to concurrent readers.

	// Step 1: Get all semesters that have data
	semesters, err := db.GetDistinctSemesters(ctx)
	if err != nil {
		return err
	}

	newIndexes := make(map[SemesterKey]*semesterIndex)
	var newSemesters []SemesterKey
	totalCourses := 0

	// Step 2: Chunked loading - process one semester at a time
	var pendingTokens []storage.SyllabusTokenEntry
	for _, sem := range semesters {
		key := SemesterKey{Year: sem.Year, Term: sem.Term}

		// Load syllabi ONLY for this semester
		syllabi, err := db.GetSyllabiByYearTerm(ctx, sem.Year, sem.Term)
		if err != nil {
			idx.logger.WithError(err).WithField("year", sem.Year).WithField("term", sem.Term).Warn("Failed to load syllabi for semester")
			continue
		}

		if len(syllabi) == 0 {
			continue
		}

		// Load pre-tokenized token cache for this semester's UIDs.
		// Cache hits (content_hash matches current syllabi row) skip the gse call entirely.
		uids := make([]string, len(syllabi))
		for i, s := range syllabi {
			uids[i] = s.UID
		}
		tokenCache, err := db.GetSyllabusTokensBatch(ctx, uids)
		if err != nil {
			idx.logger.WithError(err).WithField("year", sem.Year).WithField("term", sem.Term).Warn("Failed to load syllabus token cache")
			tokenCache = nil // degrade gracefully: tokenize everything from scratch
		}

		// Build index for this semester
		semIdx, count, newEntries, err := idx.buildSemesterIndex(syllabi, tokenCache)
		if err != nil {
			idx.logger.WithError(err).WithField("year", sem.Year).WithField("term", sem.Term).Warn("Failed to build index for semester")
			continue
		}

		if count > 0 {
			newIndexes[key] = semIdx
			newSemesters = append(newSemesters, key)
			totalCourses += count
		}
		pendingTokens = append(pendingTokens, newEntries...)
	}

	// Sort semesters (newest first)
	slices.SortFunc(newSemesters, func(a, b SemesterKey) int {
		if a.Year != b.Year {
			return b.Year - a.Year // Descending by year
		}
		return b.Term - a.Term // Descending by term
	})

	// ── Atomic swap phase (brief lock, O(1)) ──────────────────────────────────
	// Replaces the live index in one pointer swap; readers see either the old
	// or the new index atomically — never a partial rebuild state.
	idx.mu.Lock()
	idx.semesterIndexes = newIndexes
	idx.allSemesters = newSemesters
	idx.initialized = true
	idx.mu.Unlock()

	// Persist newly-tokenized entries so future restarts hit the cache.
	// Done after the swap so a save failure does not block the live index.
	if len(pendingTokens) > 0 {
		if err := db.SaveSyllabusTokensBatch(ctx, pendingTokens); err != nil {
			idx.logger.WithError(err).Warn("Failed to persist syllabus token cache")
		}
	}

	idx.logger.WithField("courses", totalCourses).
		WithField("semester_count", len(newIndexes)).
		WithField("token_cache_misses", len(pendingTokens)).
		Info("BM25 index initialized")

	return nil
}

// buildSemesterIndex creates a semesterIndex from a slice of syllabi.
// tokenCache maps uid → pre-tokenized tokens (may be nil for first run).
// Returns the index, document count, newly-tokenized entries to persist, and error.
// The provided syllabi slice is NOT retained.
func (idx *BM25Index) buildSemesterIndex(syllabi []*storage.Syllabus, tokenCache map[string]storage.SyllabusTokenEntry) (*semesterIndex, int, []storage.SyllabusTokenEntry, error) {
	semIdx := &semesterIndex{
		metadata: make(map[string]docMeta),
	}

	type corpusEntry struct {
		uid         string
		contentHash string
		content     string
	}
	entries := make([]corpusEntry, 0, len(syllabi))

	for _, syl := range syllabi {
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

		entries = append(entries, corpusEntry{uid: syl.UID, contentHash: syl.ContentHash, content: content})
		semIdx.uidList = append(semIdx.uidList, syl.UID)
	}

	if len(entries) == 0 {
		return nil, 0, nil, nil
	}

	// Resolve tokens: use cache if available, otherwise tokenize in parallel.
	// Parallel tokenization uses a GOMAXPROCS-bounded goroutine pool.
	// gse Segmenter is read-only after NewSegmenter() and is safe for concurrent use.
	//
	// Note: use tokenizeDoc (no dedup) so repeated terms in a syllabus are counted
	// correctly — both TF and document length must reflect actual occurrence counts.
	// Query tokens still use idx.Tokenize (dedup) since a query term appearing twice
	// carries no additional signal.
	tokenizedCorpus := make([][]string, len(entries))
	needTokenize := make([]int, 0, len(entries)) // indices that are cache misses

	for i, e := range entries {
		if cached, ok := tokenCache[e.uid]; ok {
			tokenizedCorpus[i] = cached.Tokens
		} else {
			needTokenize = append(needTokenize, i)
		}
	}

	if len(needTokenize) > 0 {
		workers := runtime.GOMAXPROCS(0)
		sem := make(chan struct{}, workers)
		var wg sync.WaitGroup
		for _, i := range needTokenize {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, doc string) {
				defer wg.Done()
				defer func() { <-sem }()
				tokenizedCorpus[i] = idx.tokenizeDoc(doc)
			}(i, entries[i].content)
		}
		wg.Wait()
	}

	// Collect newly-tokenized entries to persist (cache misses only).
	var pendingTokens []storage.SyllabusTokenEntry
	for _, i := range needTokenize {
		pendingTokens = append(pendingTokens, storage.SyllabusTokenEntry{
			UID:         entries[i].uid,
			ContentHash: entries[i].contentHash,
			Tokens:      tokenizedCorpus[i],
		})
	}

	// Build BM25 engine for this semester (independent IDF).
	//
	// BM25 Parameters (industry standard defaults - Lucene/Elasticsearch/Azure):
	//   k1 = 1.2: Term frequency saturation. Lower values mean faster saturation,
	//             suitable for expanded queries (8-16 terms) where most terms appear once.
	//   b  = 0.75: Document length normalization. Standard default, appropriate for
	//             variable-length documents (title + objectives + outline + schedule).
	//
	// References: Stanford IR textbook, Elasticsearch docs, Azure AI Search defaults,
	// bilingual Chinese/English experiments (KDD05), Korean biomedical TREC system.
	engine, err := newBM25Engine(tokenizedCorpus)
	if err != nil {
		return nil, 0, nil, err
	}
	semIdx.engine = engine

	return semIdx, len(entries), pendingTokens, nil
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
func (semIdx *semesterIndex) search(query string, topN int, tokenizer func(string) []string) []BM25Result {
	if semIdx == nil || semIdx.engine == nil {
		return nil
	}

	// Tokenize query
	queryTokens := tokenizer(query)
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
		semResults := semIdx.search(query, topN, idx.Tokenize)
		if len(semResults) == 0 {
			continue
		}

		// Best result is first since search() returns sorted by score descending
		maxScore := semResults[0].Score

		// Calculate relative confidence within this semester
		for _, r := range semResults {
			confidence := computeRelativeConfidence(r.Score, maxScore)

			// Filter out low-confidence results (noise)
			if confidence < MinConfidence {
				continue
			}

			results = append(results, SearchResult{
				UID:        r.UID,
				Title:      r.Title,
				Teachers:   r.Teachers,
				Year:       r.Year,
				Term:       r.Term,
				Confidence: confidence,
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
// Classification thresholds (defined in handler):
//   - Confidence >= 0.8: "最佳匹配" (Best Match) - Top 20% relative score range
//   - Confidence >= 0.6: "高度相關" (Highly Relevant) - Top 40% relative score range
//   - Confidence < 0.6: "部分相關" (Partially Relevant) - Remaining results
func computeRelativeConfidence(score, maxScore float64) float32 {
	// BM25 Okapi with Lucene IDF (log(1 + x)) guarantees non-negative scores.
	// This function is only called when maxScore > 0 and score ≥ 0.
	// The clamps below are retained as defense-in-depth.
	if maxScore <= 0 {
		return 0
	}
	confidence := score / maxScore
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0 {
		confidence = 0
	}
	return float32(confidence)
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
		total += len(semIdx.uidList)
	}
	return total
}

// Tokenize performs tokenization for search queries using the shared Chinese segmenter.
// Duplicates are removed because the same query term appearing twice carries no
// additional signal for BM25 scoring.
func (idx *BM25Index) Tokenize(text string) []string {
	return idx.seg.CutSearch(text)
}

// tokenizeDoc performs tokenization for document indexing without deduplication.
// Preserving duplicate tokens is essential for correct BM25 TF and document-length
// normalization: a syllabus mentioning "雲端" five times should rank higher than
// one that mentions it once.
func (idx *BM25Index) tokenizeDoc(text string) []string {
	return idx.seg.CutSearchAll(text)
}
