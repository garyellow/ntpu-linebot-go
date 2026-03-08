package rag

import (
	"errors"
	"math"
)

// defaultK1 is the BM25 term frequency saturation parameter.
// Lower values cause faster TF saturation; 1.2 is the industry standard
// used by Lucene, Elasticsearch, and Azure AI Search.
const defaultK1 = 1.2

// defaultB is the BM25 document length normalization parameter.
// 0.75 is the standard default; higher values normalize more aggressively.
// Used by Lucene, Elasticsearch (k1=1.2, b=0.75), and Stanford IR textbook.
const defaultB = 0.75

// bm25Engine is an efficient BM25 Okapi implementation using a pre-built inverted index.
//
// # Why we don't use github.com/iwilltry42/bm25-go
//
// That library has a fundamental performance bug in GetScores: it converts the
// pre-tokenized corpus ([][]string) back to a joined string, then calls the tokenizer
// again for every (query-term, document) pair. With an expensive tokenizer (gse HMM),
// this means O(N_query_tokens × N_docs) tokenizer calls per query — roughly 75,000
// CutSearch calls for a typical expanded query over 2 semesters × 2500 docs each.
//
// # Design
//
// Build phase (once per semester, amortized over many queries):
//   - Tokenize each document exactly once
//   - Build inverted index: term → []posting{docID, termFreq}
//   - Pre-compute IDF for every term
//
// Query phase (per user request):
//   - Zero tokenizer calls
//   - For each query token, walk its posting list (already knew result)
//   - Compute BM25 Okapi score: O(|queryTokens| × avgPostingsPerTerm)
//
// This is the standard approach used by Lucene/Elasticsearch.
type bm25Engine struct {
	// invertedIndex maps a term to its posting list.
	// Posting list is sorted by docID (ascending) for cache-friendly access.
	invertedIndex map[string][]docPosting

	// idfValues caches IDF for each term (computed once at build time).
	idfValues map[string]float64

	// docLengths[i] = number of tokens in document i.
	docLengths []int

	avgDocLen  float64
	corpusSize int

	// BM25 Okapi parameters (industry standard defaults)
	k1 float64 // term frequency saturation (1.2)
	b  float64 // document length normalization (0.75)
}

// docPosting records one document's term frequency for a given term.
type docPosting struct {
	docID int
	tf    int // how many times the term appears in this document
}

// newBM25Engine builds a BM25 Okapi engine from a pre-tokenized corpus.
// Each element of tokenizedCorpus is the token list for one document.
// Documents in tokenizedCorpus must correspond 1-to-1 with uidList in semesterIndex.
func newBM25Engine(tokenizedCorpus [][]string) (*bm25Engine, error) {
	if len(tokenizedCorpus) == 0 {
		return nil, errors.New("bm25: corpus cannot be empty")
	}

	e := &bm25Engine{
		invertedIndex: make(map[string][]docPosting, 1024),
		docLengths:    make([]int, len(tokenizedCorpus)),
		corpusSize:    len(tokenizedCorpus),
		k1:            defaultK1,
		b:             defaultB,
	}

	// termDocFreq[term] = number of documents containing term (used for IDF).
	termDocFreq := make(map[string]int, 1024)

	var totalDocLen int
	for docID, tokens := range tokenizedCorpus {
		e.docLengths[docID] = len(tokens)
		totalDocLen += len(tokens)

		// Compute term frequencies for this document in a single pass.
		localTF := make(map[string]int, len(tokens))
		for _, t := range tokens {
			localTF[t]++
		}

		// Update inverted index and document frequency counts.
		for term, tf := range localTF {
			e.invertedIndex[term] = append(e.invertedIndex[term], docPosting{docID: docID, tf: tf})
			termDocFreq[term]++
		}
	}

	if totalDocLen > 0 {
		e.avgDocLen = float64(totalDocLen) / float64(e.corpusSize)
	} else {
		e.avgDocLen = 1.0 // Guard: all documents empty (prevents division by zero in GetScores)
	}

	// Pre-compute IDF for all terms (BM25 Okapi variant, same formula as the old library).
	e.idfValues = make(map[string]float64, len(termDocFreq))
	n := float64(e.corpusSize)
	for term, df := range termDocFreq {
		// Lucene IDF variant: log(1 + (N-df+0.5)/(df+0.5))
		// Because df ≤ N, the quantity inside the log is always ≥ 1+0.5/(N+0.5) > 1,
		// so the log value (IDF) is always positive. The guard is retained for defense-in-depth.
		idf := math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1.0)
		if idf > 0 {
			e.idfValues[term] = idf
		}
	}

	return e, nil
}

// GetScores returns the BM25 Okapi score for every document in the corpus.
// Complexity: O(|queryTokens| × avgPostingsPerTerm) — no tokenizer calls.
func (e *bm25Engine) GetScores(queryTokens []string) ([]float64, error) {
	if len(queryTokens) == 0 {
		return nil, errors.New("bm25: query cannot be empty")
	}

	scores := make([]float64, e.corpusSize)

	for _, q := range queryTokens {
		idf, ok := e.idfValues[q]
		if !ok || idf <= 0 {
			// Term not in corpus; contributes nothing.
			continue
		}

		for _, p := range e.invertedIndex[q] {
			tf := float64(p.tf)
			dl := float64(e.docLengths[p.docID])
			// BM25 Okapi TF normalization
			k := e.k1 * (1.0 - e.b + e.b*dl/e.avgDocLen)
			scores[p.docID] += idf * (tf * (e.k1 + 1.0)) / (tf + k)
		}
	}

	return scores, nil
}
