// Package rag provides Retrieval-Augmented Generation functionality
package rag

import (
	"context"
	"sync"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// HybridSearcher combines BM25 keyword search and vector semantic search
// using Reciprocal Rank Fusion (RRF) for improved retrieval
type HybridSearcher struct {
	vectorDB  *VectorDB
	bm25Index *BM25Index
	logger    *logger.Logger
}

// NewHybridSearcher creates a new hybrid searcher
// If vectorDB is nil, only BM25 search will be used
// If bm25Index is nil, only vector search will be used
func NewHybridSearcher(vectorDB *VectorDB, bm25Index *BM25Index, log *logger.Logger) *HybridSearcher {
	return &HybridSearcher{
		vectorDB:  vectorDB,
		bm25Index: bm25Index,
		logger:    log,
	}
}

// Search performs hybrid search combining BM25 and vector search
// Returns results ranked by RRF fusion of both methods
//
// The search process:
// 1. Run BM25 keyword search in parallel with vector search
// 2. Combine results using Reciprocal Rank Fusion (RRF)
// 3. Return top results sorted by combined RRF score
//
// Fallback behavior:
// - If both are disabled, returns empty results
// - If only one is available, uses that method alone with 100% weight
func (h *HybridSearcher) Search(ctx context.Context, query string, topN int) ([]SearchResult, error) {
	if h == nil {
		return nil, nil
	}

	vectorEnabled := h.vectorDB != nil && h.vectorDB.IsEnabled()
	bm25Enabled := h.bm25Index != nil && h.bm25Index.IsEnabled()

	if !vectorEnabled && !bm25Enabled {
		return nil, nil
	}

	// Fetch more results for better RRF fusion
	fetchN := topN * 3
	if fetchN < 30 {
		fetchN = 30
	}

	var (
		bm25Results   []BM25Result
		vectorResults []SearchResult
		bm25Err       error
		vectorErr     error
		wg            sync.WaitGroup
	)

	// Run searches in parallel
	if bm25Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bm25Results, bm25Err = h.bm25Index.Search(query, fetchN)
		}()
	}

	if vectorEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vectorResults, vectorErr = h.vectorDB.Search(ctx, query, fetchN)
		}()
	}

	wg.Wait()

	// Log any errors but continue with available results
	if bm25Err != nil {
		h.logger.WithError(bm25Err).Warn("BM25 search failed")
	}
	if vectorErr != nil {
		h.logger.WithError(vectorErr).Warn("Vector search failed")
	}

	// Handle single-source scenarios
	if !bm25Enabled || len(bm25Results) == 0 {
		// Vector only - vector results already have true similarity scores
		if len(vectorResults) > topN {
			vectorResults = vectorResults[:topN]
		}
		return vectorResults, nil
	}

	if !vectorEnabled || len(vectorResults) == 0 {
		// BM25 only - convert to SearchResult format
		// BM25 has no similarity concept, use rank-based confidence
		results := make([]SearchResult, 0, min(len(bm25Results), topN))
		for _, r := range bm25Results {
			if len(results) >= topN {
				break
			}
			// Rank-based confidence: top ranks get higher scores
			// rank 1 → ~0.95, rank 5 → ~0.80, rank 10 → ~0.67
			confidence := computeBM25Confidence(r.Rank)
			results = append(results, SearchResult{
				UID:        r.UID,
				Title:      r.Title,
				Teachers:   r.Teachers,
				Year:       r.Year,
				Term:       r.Term,
				Similarity: confidence,
			})
		}
		return results, nil
	}

	// Both available - use RRF fusion
	hybridResults := FuseRRFWithDefaults(bm25Results, vectorResults, topN)

	// Log fusion details for debugging
	h.logger.WithFields(map[string]any{
		"bm25_count":   len(bm25Results),
		"vector_count": len(vectorResults),
		"fused_count":  len(hybridResults),
		"query":        query,
	}).Debug("Hybrid search completed")

	return ToSearchResults(hybridResults), nil
}

// computeBM25Confidence calculates confidence score for BM25-only results.
//
// BM25 scores are unbounded and query-dependent, so they cannot be
// converted to a meaningful "similarity" measure. Instead, we use
// rank position as a proxy for relevance confidence.
//
// Formula: 1 / (1 + 0.05 * rank)
// - rank 1 → 0.95
// - rank 5 → 0.80
// - rank 10 → 0.67
// - rank 20 → 0.50
//
// This gives users a reasonable confidence indicator without
// falsely implying semantic similarity.
func computeBM25Confidence(rank int) float32 {
	if rank <= 0 {
		return 0
	}
	return float32(1.0 / (1.0 + 0.05*float64(rank)))
}

// Initialize initializes both BM25 and vector indexes with syllabi data
func (h *HybridSearcher) Initialize(ctx context.Context, syllabi []*storage.Syllabus) error {
	if h == nil {
		return nil
	}

	// Initialize BM25 index (synchronous, CPU-only)
	if h.bm25Index != nil {
		if err := h.bm25Index.Initialize(syllabi); err != nil {
			return err
		}
	}

	// Initialize vector DB (may involve API calls for embedding)
	if h.vectorDB != nil {
		if err := h.vectorDB.Initialize(ctx, syllabi); err != nil {
			return err
		}
	}

	return nil
}

// IsEnabled returns true if at least one search method is available
func (h *HybridSearcher) IsEnabled() bool {
	if h == nil {
		return false
	}
	vectorEnabled := h.vectorDB != nil && h.vectorDB.IsEnabled()
	bm25Enabled := h.bm25Index != nil && h.bm25Index.IsEnabled()
	return vectorEnabled || bm25Enabled
}

// VectorDB returns the underlying vector database (for compatibility)
func (h *HybridSearcher) VectorDB() *VectorDB {
	if h == nil {
		return nil
	}
	return h.vectorDB
}

// BM25Index returns the underlying BM25 index
func (h *HybridSearcher) BM25Index() *BM25Index {
	if h == nil {
		return nil
	}
	return h.bm25Index
}
