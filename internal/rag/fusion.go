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

// ToSearchResults converts HybridResults to SearchResults for compatibility.
//
// RRF-based confidence scoring:
//
// Key insight: BM25 scores are unbounded and not comparable to vector similarity.
// RRF deliberately ignores raw scores and only uses rankings.
// Therefore, we derive a "confidence" score from the RRF score itself,
// NOT from trying to normalize/combine BM25 and vector scores.
//
// The confidence score represents "how strongly this result was endorsed
// by both retrieval methods" rather than a true similarity measure.
//
// Confidence calculation:
//   - Normalize RRF score relative to top result (0-1 range)
//   - Apply source bonus: results found by BOTH methods get a boost
//   - Vector similarity acts as a tiebreaker when available
//
// This avoids the conceptual error of treating BM25 scores as similarities.
func ToSearchResults(hybridResults []HybridResult) []SearchResult {
	if len(hybridResults) == 0 {
		return nil
	}

	// RRF scores are already rank-based, normalize to 0-1 using max score
	maxRRFScore := hybridResults[0].RRFScore
	if maxRRFScore <= 0 {
		maxRRFScore = 1 // Prevent division by zero
	}

	results := make([]SearchResult, len(hybridResults))
	for i, hr := range hybridResults {
		confidence := computeConfidence(hr, maxRRFScore)

		results[i] = SearchResult{
			UID:        hr.UID,
			Title:      hr.Title,
			Teachers:   hr.Teachers,
			Year:       hr.Year,
			Term:       hr.Term,
			Content:    hr.Content,
			Similarity: confidence,
		}
	}

	return results
}

// computeConfidence calculates a confidence score for hybrid search results.
//
// - RRF score reflects ranking consensus, not semantic similarity
// - We convert RRF to a 0-1 "confidence" scale for UX purposes
// - Results appearing in BOTH sources get higher confidence
// - Vector similarity provides additional signal when available
//
// This is conceptually different from "similarity" - it represents
// "retrieval confidence" rather than "semantic closeness".
func computeConfidence(hr HybridResult, maxRRFScore float64) float32 {
	hasBM25 := hr.BM25Rank > 0
	hasVector := hr.VectorRank > 0

	// Base confidence from normalized RRF score
	// RRF scores typically range from 0 to ~0.03, normalize to 0-1
	baseConfidence := float32(hr.RRFScore / maxRRFScore)

	switch {
	case hasBM25 && hasVector:
		// Both sources agree - highest confidence
		// Use vector similarity to refine, as it's the only true similarity measure
		// Formula: 70% RRF-based + 30% vector similarity
		combined := 0.7*baseConfidence + 0.3*hr.VectorSim
		// Boost for appearing in both (up to 10% bonus for lower scores)
		boost := 0.1 * (1.0 - combined)
		return clampSimilarity(combined + float32(boost))

	case hasVector:
		// Vector only - use vector similarity directly (true similarity)
		// But weight by RRF position
		return clampSimilarity(0.4*baseConfidence + 0.6*hr.VectorSim)

	case hasBM25:
		// BM25 only - use RRF confidence since BM25 has no similarity concept
		// Apply rank-based decay: rank 1 → 0.9, rank 10 → 0.5
		rankDecay := float32(1.0 / (1.0 + 0.05*float64(hr.BM25Rank)))
		return clampSimilarity(0.5*baseConfidence + 0.5*rankDecay)

	default:
		return baseConfidence
	}
}

// clampSimilarity ensures similarity is within valid range [0, 1]
func clampSimilarity(s float32) float32 {
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}
