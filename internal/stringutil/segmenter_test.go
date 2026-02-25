package stringutil

import "testing"

// sharedSeg is initialized once to avoid gse's concurrent map write issue
// during parallel test execution (gse uses global state for HMM model loading).
var sharedSeg = NewSegmenter()

func TestNewSegmenter(t *testing.T) {
	t.Parallel()
	if sharedSeg == nil {
		t.Fatal("NewSegmenter() returned nil")
	}
}

func TestCutSearch(t *testing.T) {
	t.Parallel()
	seg := sharedSeg

	tests := []struct {
		name      string
		input     string
		expectLen int      // Minimum number of expected tokens
		expectAll []string // Tokens that MUST be present
	}{
		{
			name:      "Chinese compound word",
			input:     "雲端運算",
			expectLen: 1,
			expectAll: []string{"雲端"},
		},
		{
			name:      "English word lowercase",
			input:     "AWS",
			expectLen: 1,
			expectAll: []string{"aws"},
		},
		{
			name:      "Mixed Chinese and English",
			input:     "雲端運算 cloud computing",
			expectLen: 3,
			expectAll: []string{"cloud", "computing"},
		},
		{
			name:      "Empty string",
			input:     "",
			expectLen: 0,
		},
		{
			name:      "Whitespace only",
			input:     "   ",
			expectLen: 0,
		},
		{
			name:      "Punctuation stripped",
			input:     "Hello, 世界!",
			expectLen: 2,
			expectAll: []string{"hello"},
		},
		{
			name:      "Numbers preserved",
			input:     "test123",
			expectLen: 1,
			expectAll: []string{"test123"},
		},
		{
			name:      "Deduplication",
			input:     "雲端 雲端",
			expectLen: 1,
			expectAll: []string{"雲端"},
		},
		{
			name:      "Course title segmentation",
			input:     "線性代數",
			expectLen: 1,
			expectAll: []string{"線性"},
		},
		{
			name:      "Multi-word Chinese",
			input:     "微積分課程",
			expectLen: 1,
			expectAll: []string{"微積分"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := seg.CutSearch(tt.input)
			if len(result) < tt.expectLen {
				t.Errorf("CutSearch(%q) returned %d tokens %v, want at least %d",
					tt.input, len(result), result, tt.expectLen)
				return
			}
			for _, expected := range tt.expectAll {
				found := false
				for _, token := range result {
					if token == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("CutSearch(%q) = %v, missing expected token %q",
						tt.input, result, expected)
				}
			}
		})
	}
}

func TestCutSearchCJKRanges(t *testing.T) {
	t.Parallel()
	seg := sharedSeg

	// CJK Extension A range (U+3400-U+4DBF)
	result := seg.CutSearch("㐀")
	if len(result) == 0 {
		t.Error("CutSearch should handle CJK Extension A characters")
	}
}
