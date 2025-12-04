package rag

import (
	"testing"
)

func TestFuseRRF(t *testing.T) {
	bm25Results := []BM25Result{
		{UID: "course1", Title: "Course 1", Score: 10.0, Rank: 1},
		{UID: "course2", Title: "Course 2", Score: 8.0, Rank: 2},
		{UID: "course3", Title: "Course 3", Score: 5.0, Rank: 3},
	}

	vectorResults := []SearchResult{
		{UID: "course2", Title: "Course 2", Similarity: 0.9},
		{UID: "course4", Title: "Course 4", Similarity: 0.85},
		{UID: "course1", Title: "Course 1", Similarity: 0.7},
	}

	results := FuseRRFWithDefaults(bm25Results, vectorResults, 10)

	if len(results) == 0 {
		t.Fatal("FuseRRF() returned no results")
	}

	// course1 and course2 should be in top results (appear in both lists)
	topUIDs := make(map[string]bool)
	for i := 0; i < min(3, len(results)); i++ {
		topUIDs[results[i].UID] = true
	}

	// course2 should be top because it ranks high in both lists
	if results[0].UID != "course2" {
		t.Errorf("FuseRRF() top result = %s, want course2 (appears in both lists with high ranks)", results[0].UID)
	}

	// Both course1 and course2 should be in top 3
	if !topUIDs["course1"] {
		t.Error("FuseRRF() course1 should be in top 3 (appears in both lists)")
	}
	if !topUIDs["course2"] {
		t.Error("FuseRRF() course2 should be in top 3 (appears in both lists)")
	}
}

func TestFuseRRF_BM25Only(t *testing.T) {
	bm25Results := []BM25Result{
		{UID: "course1", Title: "Course 1", Score: 10.0, Rank: 1},
		{UID: "course2", Title: "Course 2", Score: 8.0, Rank: 2},
	}

	results := FuseRRFWithDefaults(bm25Results, nil, 10)

	if len(results) != 2 {
		t.Errorf("FuseRRF() with BM25 only returned %d results, want 2", len(results))
	}

	// Order should match BM25 order
	if results[0].UID != "course1" {
		t.Errorf("FuseRRF() first result = %s, want course1", results[0].UID)
	}
}

func TestFuseRRF_VectorOnly(t *testing.T) {
	vectorResults := []SearchResult{
		{UID: "course1", Title: "Course 1", Similarity: 0.9},
		{UID: "course2", Title: "Course 2", Similarity: 0.8},
	}

	results := FuseRRFWithDefaults(nil, vectorResults, 10)

	if len(results) != 2 {
		t.Errorf("FuseRRF() with vector only returned %d results, want 2", len(results))
	}

	// Order should match vector order
	if results[0].UID != "course1" {
		t.Errorf("FuseRRF() first result = %s, want course1", results[0].UID)
	}
}

func TestFuseRRF_Empty(t *testing.T) {
	results := FuseRRFWithDefaults(nil, nil, 10)

	if len(results) != 0 {
		t.Errorf("FuseRRF() with empty inputs returned %d results, want 0", len(results))
	}
}

func TestFuseRRF_TopN(t *testing.T) {
	bm25Results := make([]BM25Result, 20)
	for i := range bm25Results {
		bm25Results[i] = BM25Result{
			UID:   "course" + string(rune('A'+i)),
			Title: "Course " + string(rune('A'+i)),
			Score: float64(20 - i),
			Rank:  i + 1,
		}
	}

	results := FuseRRFWithDefaults(bm25Results, nil, 5)

	if len(results) != 5 {
		t.Errorf("FuseRRF() with topN=5 returned %d results, want 5", len(results))
	}
}

func TestFuseRRF_WeightBalance(t *testing.T) {
	// Test that BM25 weight affects ranking
	bm25Results := []BM25Result{
		{UID: "bm25_top", Title: "BM25 Top", Score: 10.0, Rank: 1},
	}

	vectorResults := []SearchResult{
		{UID: "vector_top", Title: "Vector Top", Similarity: 0.95},
	}

	// With default weights (BM25=0.4, Vector=0.6), vector_top should rank higher
	results := FuseRRFWithDefaults(bm25Results, vectorResults, 10)

	if len(results) != 2 {
		t.Fatalf("FuseRRF() returned %d results, want 2", len(results))
	}

	// With 60% vector weight, vector_top should be first
	if results[0].UID != "vector_top" {
		t.Errorf("FuseRRF() with default weights: first result = %s, want vector_top (60%% weight)", results[0].UID)
	}

	// With BM25 weight = 0.8, bm25_top should be first
	results = FuseRRF(bm25Results, vectorResults, 0.8, 10)

	if results[0].UID != "bm25_top" {
		t.Errorf("FuseRRF() with BM25 weight=0.8: first result = %s, want bm25_top", results[0].UID)
	}
}

func TestToSearchResults(t *testing.T) {
	hybridResults := []HybridResult{
		{
			UID:        "course1",
			Title:      "Course 1",
			Teachers:   []string{"Teacher 1"},
			Year:       113,
			Term:       1,
			VectorSim:  0.85,
			BM25Score:  8.5,
			RRFScore:   0.02,
			VectorRank: 1,
			BM25Rank:   2,
		},
		{
			UID:       "course2",
			Title:     "Course 2",
			BM25Score: 10.0,
			RRFScore:  0.015,
			BM25Rank:  1,
		},
	}

	results := ToSearchResults(hybridResults)

	if len(results) != 2 {
		t.Fatalf("ToSearchResults() returned %d results, want 2", len(results))
	}

	// First result should preserve vector similarity
	if results[0].Similarity != 0.85 {
		t.Errorf("ToSearchResults() first result similarity = %v, want 0.85", results[0].Similarity)
	}

	// Metadata should be preserved
	if results[0].Title != "Course 1" {
		t.Errorf("ToSearchResults() first result title = %s, want Course 1", results[0].Title)
	}

	if len(results[0].Teachers) != 1 || results[0].Teachers[0] != "Teacher 1" {
		t.Errorf("ToSearchResults() first result teachers = %v, want [Teacher 1]", results[0].Teachers)
	}
}

func TestToSearchResults_Empty(t *testing.T) {
	results := ToSearchResults(nil)

	if results != nil {
		t.Errorf("ToSearchResults(nil) = %v, want nil", results)
	}
}
