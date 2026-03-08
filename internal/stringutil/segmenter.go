// Package stringutil provides common string manipulation utilities.
package stringutil

import (
	"strings"

	"github.com/go-ego/gse"
)

// Segmenter wraps gse for Chinese word segmentation.
// It is safe for concurrent use after initialization.
type Segmenter struct {
	seg gse.Segmenter
}

// NewSegmenter creates a new Chinese word segmenter with the Traditional Chinese dictionary.
// Returns a usable segmenter even if dictionary loading fails (falls back to character-level).
//
// NOTE: gse uses global state for HMM model loading, so NewSegmenter must not be
// called concurrently. In production, create a single shared instance at startup
// and inject it via constructors.
func NewSegmenter() *Segmenter {
	s := &Segmenter{}
	// Load Traditional Chinese dictionary from embedded data (no external files)
	_ = s.seg.LoadDictEmbed("zh_t")
	return s
}

// CutSearch performs search-optimized segmentation on Chinese text.
// Returns meaningful word segments suitable for search queries.
// For example: "線性代數進階" → ["線性代數", "線性", "進階", ...]
//
// Non-CJK text (English words, numbers) is kept as-is.
// Result is deduplicated to avoid redundant search terms.
// Use CutSearchAll when duplicate tokens must be preserved (e.g., BM25 document indexing).
func (s *Segmenter) CutSearch(text string) []string {
	return s.cutSearch(text, true)
}

// CutSearchAll is identical to CutSearch but preserves duplicate tokens.
// Use this for BM25 document indexing so term frequencies and document lengths
// reflect actual occurrence counts rather than unique-term counts.
func (s *Segmenter) CutSearchAll(text string) []string {
	return s.cutSearch(text, false)
}

func (s *Segmenter) cutSearch(text string, deduplicate bool) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}

	var tokens []string
	var wordBuf strings.Builder
	var cjkBuf strings.Builder

	flushWord := func() {
		if wordBuf.Len() > 0 {
			tokens = append(tokens, wordBuf.String())
			wordBuf.Reset()
		}
	}

	flushCJK := func() {
		if cjkBuf.Len() > 0 {
			segs := s.seg.CutSearch(cjkBuf.String(), true)
			for _, t := range segs {
				t = strings.TrimSpace(t)
				if t != "" {
					tokens = append(tokens, t)
				}
			}
			cjkBuf.Reset()
		}
	}

	for _, r := range text {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF, r >= 0x3400 && r <= 0x4DBF:
			flushWord()
			cjkBuf.WriteRune(r)
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			flushCJK()
			wordBuf.WriteRune(r)
		default:
			// Separator (space, punctuation)
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()

	if !deduplicate {
		return tokens
	}

	// Deduplicate
	if len(tokens) <= 1 {
		return tokens
	}
	seen := make(map[string]struct{}, len(tokens))
	deduped := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if _, exists := seen[t]; !exists {
			seen[t] = struct{}{}
			deduped = append(deduped, t)
		}
	}
	return deduped
}
