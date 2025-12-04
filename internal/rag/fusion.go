// Package rag provides Retrieval-Augmented Generation functionality
package rag

import (
	"sort"
)

const (
	// RRFConstant is the constant used in RRF formula: 1 / (k + rank)
	// Standard value is 60, which provides a good balance between
	// giving weight to top-ranked documents while not ignoring lower-ranked ones
	RRFConstant = 60

	// DefaultBM25Weight is the default weight for BM25 results in RRF fusion
	// 0.4 means BM25 contributes 40% and vector search contributes 60%
	DefaultBM25Weight = 0.4

	// DefaultVectorWeight is the default weight for vector search results
	DefaultVectorWeight = 0.6
)

// HybridResult represents a result from hybrid search (BM25 + Vector)
type HybridResult struct {
	UID        string
	Title      string
	Teachers   []string
	Year       int
	Term       int
	Content    string  // From vector search
	BM25Score  float64 // BM25 score (0 if not found in BM25)
	VectorSim  float32 // Vector similarity (0 if not found in vector)
	RRFScore   float64 // Combined RRF score
	BM25Rank   int     // Rank in BM25 results (0 if not found)
	VectorRank int     // Rank in vector results (0 if not found)
}

// FuseRRF combines BM25 and vector search results using Reciprocal Rank Fusion
//
// RRF formula: score(d) = Σ (w_i / (k + rank_i))
// where k is RRFConstant (60), rank_i is the rank in each source,
// and w_i is the weight for each source
//
// Parameters:
//   - bm25Results: Results from BM25 keyword search
//   - vectorResults: Results from vector similarity search
//   - bm25Weight: Weight for BM25 (0-1), vector weight is (1 - bm25Weight)
//   - topN: Maximum number of results to return
//
// Returns combined results sorted by RRF score (descending)
func FuseRRF(bm25Results []BM25Result, vectorResults []SearchResult, bm25Weight float64, topN int) []HybridResult {
	if bm25Weight < 0 {
		bm25Weight = 0
	}
	if bm25Weight > 1 {
		bm25Weight = 1
	}
	vectorWeight := 1.0 - bm25Weight

	// Map to store combined results by UID
	resultMap := make(map[string]*HybridResult)

	// Process BM25 results
	for i, r := range bm25Results {
		rank := i + 1 // 1-indexed rank
		score := bm25Weight / float64(RRFConstant+rank)

		if existing, ok := resultMap[r.UID]; ok {
			existing.BM25Score = r.Score
			existing.BM25Rank = rank
			existing.RRFScore += score
		} else {
			resultMap[r.UID] = &HybridResult{
				UID:       r.UID,
				Title:     r.Title,
				Teachers:  r.Teachers,
				Year:      r.Year,
				Term:      r.Term,
				BM25Score: r.Score,
				BM25Rank:  rank,
				RRFScore:  score,
			}
		}
	}

	// Process vector results
	for i, r := range vectorResults {
		rank := i + 1 // 1-indexed rank
		score := vectorWeight / float64(RRFConstant+rank)

		if existing, ok := resultMap[r.UID]; ok {
			existing.VectorSim = r.Similarity
			existing.VectorRank = rank
			existing.Content = r.Content
			existing.RRFScore += score
			// Update metadata if missing
			if existing.Title == "" {
				existing.Title = r.Title
			}
			if len(existing.Teachers) == 0 {
				existing.Teachers = r.Teachers
			}
			if existing.Year == 0 {
				existing.Year = r.Year
			}
			if existing.Term == 0 {
				existing.Term = r.Term
			}
		} else {
			resultMap[r.UID] = &HybridResult{
				UID:        r.UID,
				Title:      r.Title,
				Teachers:   r.Teachers,
				Year:       r.Year,
				Term:       r.Term,
				Content:    r.Content,
				VectorSim:  r.Similarity,
				VectorRank: rank,
				RRFScore:   score,
			}
		}
	}

	// Convert map to slice
	results := make([]HybridResult, 0, len(resultMap))
	for _, r := range resultMap {
		results = append(results, *r)
	}

	// Sort by RRF score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].RRFScore > results[j].RRFScore
	})

	// Limit results
	if topN > 0 && len(results) > topN {
		results = results[:topN]
	}

	return results
}

// FuseRRFWithDefaults uses default weights for BM25 (0.4) and Vector (0.6)
func FuseRRFWithDefaults(bm25Results []BM25Result, vectorResults []SearchResult, topN int) []HybridResult {
	return FuseRRF(bm25Results, vectorResults, DefaultBM25Weight, topN)
}

// ToSearchResults converts HybridResults to SearchResults for compatibility
// Uses RRF score as a normalized similarity (scaled to 0-1 range approximately)
func ToSearchResults(hybridResults []HybridResult) []SearchResult {
	if len(hybridResults) == 0 {
		return nil
	}

	// Find max RRF score for normalization
	maxScore := hybridResults[0].RRFScore

	results := make([]SearchResult, len(hybridResults))
	for i, hr := range hybridResults {
		// Normalize RRF score to 0-1 range
		// Use original vector similarity if available, otherwise scale RRF
		var similarity float32
		if hr.VectorSim > 0 {
			// Prefer actual vector similarity for display
			similarity = hr.VectorSim
		} else if maxScore > 0 {
			// Scale RRF score to approximate similarity
			similarity = float32(hr.RRFScore / maxScore)
		}

		results[i] = SearchResult{
			UID:        hr.UID,
			Title:      hr.Title,
			Teachers:   hr.Teachers,
			Year:       hr.Year,
			Term:       hr.Term,
			Content:    hr.Content,
			Similarity: similarity,
		}
	}

	return results
}
