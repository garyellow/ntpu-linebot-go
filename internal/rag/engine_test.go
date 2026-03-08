package rag

import (
	"math"
	"testing"
)

// TestNewBM25Engine_EmptyCorpus verifies that building an engine with no documents returns an error.
func TestNewBM25Engine_EmptyCorpus(t *testing.T) {
	t.Parallel()

	_, err := newBM25Engine(nil, defaultK1, defaultB)
	if err == nil {
		t.Error("expected error for nil corpus, got nil")
	}

	_, err = newBM25Engine([][]string{}, defaultK1, defaultB)
	if err == nil {
		t.Error("expected error for empty corpus slice, got nil")
	}
}

// TestNewBM25Engine_SingleDocument verifies a single-document corpus builds without error.
func TestNewBM25Engine_SingleDocument(t *testing.T) {
	t.Parallel()

	corpus := [][]string{{"雲端", "運算"}}
	e, err := newBM25Engine(corpus, defaultK1, defaultB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.corpusSize != 1 {
		t.Errorf("corpusSize = %d, want 1", e.corpusSize)
	}
}

// TestNewBM25Engine_IDFFormula verifies the Lucene IDF formula:
// IDF = log(1 + (N - df + 0.5) / (df + 0.5))
//
// With this formula, IDF is always > 0 because:
// - The log argument = 1 + (N-df+0.5)/(df+0.5) > 1 whenever df ≤ N
// - log(x) > 0 for x > 1
func TestNewBM25Engine_IDFFormula(t *testing.T) {
	t.Parallel()

	// 3-document corpus: "a" in 2 docs (common), "b"/"c"/"d" in 1 doc each (rare).
	corpus := [][]string{
		{"a", "b"},
		{"a", "c"},
		{"b", "d"},
	}
	e, err := newBM25Engine(corpus, defaultK1, defaultB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	n := float64(3)
	wantIDF := func(df float64) float64 {
		return math.Log((n-df+0.5)/(df+0.5) + 1.0)
	}

	const tol = 1e-10
	if got := e.idfValues["a"]; math.Abs(got-wantIDF(2)) > tol {
		t.Errorf("IDF('a') = %f, want %f", got, wantIDF(2))
	}
	if got := e.idfValues["c"]; math.Abs(got-wantIDF(1)) > tol {
		t.Errorf("IDF('c') = %f, want %f", got, wantIDF(1))
	}

	// Rare terms (df=1) must have strictly higher IDF than common terms (df=2).
	if e.idfValues["c"] <= e.idfValues["a"] {
		t.Errorf("rare term 'c' IDF (%f) should exceed common term 'a' IDF (%f)",
			e.idfValues["c"], e.idfValues["a"])
	}
}

// TestNewBM25Engine_IDFAlwaysPositive verifies that even for a term appearing in ALL
// documents, the Lucene +1 formula keeps IDF positive.
func TestNewBM25Engine_IDFAlwaysPositive(t *testing.T) {
	t.Parallel()

	corpus := [][]string{
		{"a", "b"},
		{"a", "c"},
		{"a", "d"}, // "a" is in every document
	}
	e, err := newBM25Engine(corpus, defaultK1, defaultB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for term, idf := range e.idfValues {
		if idf <= 0 {
			t.Errorf("IDF(%q) = %f, want > 0 (Lucene formula guarantees positivity)", term, idf)
		}
	}
}

// TestGetScores_EmptyQuery verifies that GetScores rejects empty token slices.
func TestGetScores_EmptyQuery(t *testing.T) {
	t.Parallel()

	corpus := [][]string{{"a", "b"}, {"c", "d"}}
	e, _ := newBM25Engine(corpus, defaultK1, defaultB)

	if _, err := e.GetScores(nil); err == nil {
		t.Error("expected error for nil query tokens")
	}
	if _, err := e.GetScores([]string{}); err == nil {
		t.Error("expected error for empty query tokens")
	}
}

// TestGetScores_AlwaysNonNegative verifies that BM25 Okapi with Lucene IDF
// never produces negative document scores.
func TestGetScores_AlwaysNonNegative(t *testing.T) {
	t.Parallel()

	corpus := [][]string{
		{"雲端", "運算", "資料"},
		{"機器", "學習", "演算法"},
		{"資料", "庫", "設計"},
	}
	e, err := newBM25Engine(corpus, defaultK1, defaultB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scores, err := e.GetScores([]string{"雲端", "資料"})
	if err != nil {
		t.Fatalf("GetScores returned error: %v", err)
	}

	for i, s := range scores {
		if s < 0 {
			t.Errorf("scores[%d] = %f, want >= 0 (BM25 Okapi is non-negative)", i, s)
		}
	}
}

// TestGetScores_ScoreLength verifies GetScores returns one score per document.
func TestGetScores_ScoreLength(t *testing.T) {
	t.Parallel()

	corpus := [][]string{{"a"}, {"b"}, {"c"}, {"d"}}
	e, _ := newBM25Engine(corpus, defaultK1, defaultB)
	scores, err := e.GetScores([]string{"a"})
	if err != nil {
		t.Fatalf("GetScores error: %v", err)
	}
	if len(scores) != 4 {
		t.Errorf("len(scores) = %d, want 4", len(scores))
	}
}

// TestGetScores_UnknownTermZeroScore verifies that a query term absent from the
// corpus contributes zero to every document score.
func TestGetScores_UnknownTermZeroScore(t *testing.T) {
	t.Parallel()

	corpus := [][]string{{"a", "b"}, {"c", "d"}}
	e, _ := newBM25Engine(corpus, defaultK1, defaultB)

	scores, err := e.GetScores([]string{"xyz_not_in_corpus"})
	if err != nil {
		t.Fatalf("GetScores error: %v", err)
	}
	for i, s := range scores {
		if s != 0 {
			t.Errorf("scores[%d] = %f for unknown term, want 0", i, s)
		}
	}
}

// TestGetScores_TFOrdering verifies that a document with higher term frequency
// scores higher than one with lower TF (all else equal).
func TestGetScores_TFOrdering(t *testing.T) {
	t.Parallel()

	// doc0 contains "雲端" twice, doc1 once → doc0 should rank higher.
	corpus := [][]string{
		{"雲端", "雲端"}, // TF = 2
		{"雲端"},       // TF = 1
		{"資料", "結構"}, // no "雲端"
	}
	e, err := newBM25Engine(corpus, defaultK1, defaultB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scores, err := e.GetScores([]string{"雲端"})
	if err != nil {
		t.Fatalf("GetScores error: %v", err)
	}

	if scores[0] <= scores[1] {
		t.Errorf("doc with TF=2 (%.4f) should score > doc with TF=1 (%.4f)", scores[0], scores[1])
	}
	if scores[2] != 0 {
		t.Errorf("doc with no matching term should score 0, got %.4f", scores[2])
	}
}

// TestGetScores_MultiTermOrdering verifies that more matching terms → higher score.
func TestGetScores_MultiTermOrdering(t *testing.T) {
	t.Parallel()

	corpus := [][]string{
		{"a", "b", "c"}, // matches all three query terms
		{"a", "b"},      // matches two
		{"a"},           // matches one
		{"z"},           // matches none
	}
	e, err := newBM25Engine(corpus, defaultK1, defaultB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scores, err := e.GetScores([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("GetScores error: %v", err)
	}

	if scores[0] <= scores[1] {
		t.Errorf("3-term match (%.4f) should score > 2-term match (%.4f)", scores[0], scores[1])
	}
	if scores[1] <= scores[2] {
		t.Errorf("2-term match (%.4f) should score > 1-term match (%.4f)", scores[1], scores[2])
	}
	if scores[3] != 0 {
		t.Errorf("no-match doc should score 0, got %.4f", scores[3])
	}
}
