package genai

import (
	"context"
	"strings"
	"testing"
)

func TestQueryExpansionPrompt(t *testing.T) {
	t.Parallel()
	query := "我想學 AWS"
	prompt := QueryExpansionPrompt(query)

	// Prompt should contain Think-then-Expand structure
	if !strings.Contains(prompt, "意圖分析") {
		t.Error("Prompt should contain intent analysis instruction '意圖分析'")
	}
	if !strings.Contains(prompt, "關鍵詞") {
		t.Error("Prompt should contain keyword generation instruction '關鍵詞'")
	}
	// Prompt should contain structured output format
	if !strings.Contains(prompt, "分析：") {
		t.Error("Prompt should contain structured output format '分析：'")
	}
	if !strings.Contains(prompt, "關鍵詞：") {
		t.Error("Prompt should contain structured output format '關鍵詞：'")
	}
	// Prompt should contain expansion examples
	if !strings.Contains(prompt, "statistics") {
		t.Error("Prompt should contain bilingual expansion examples")
	}
	// Prompt should contain the original query
	if !strings.Contains(prompt, query) {
		t.Error("Prompt should contain the original query")
	}
	// Prompt should contain cross-disciplinary example
	if !strings.Contains(prompt, "資工") && !strings.Contains(prompt, "金融") {
		t.Error("Prompt should contain cross-disciplinary example")
	}
}

func TestQueryExpansionPrompt_IntentAnalysis(t *testing.T) {
	t.Parallel()
	prompt := QueryExpansionPrompt("我是資工系的，但我對金融領域有興趣")

	// Should contain intent analysis step
	if !strings.Contains(prompt, "第一步") {
		t.Error("Prompt should have step 1 (intent analysis)")
	}
	if !strings.Contains(prompt, "第二步") {
		t.Error("Prompt should have step 2 (keyword generation)")
	}
	// Should emphasize cross-disciplinary awareness
	if !strings.Contains(prompt, "跨領域") {
		t.Error("Prompt should mention cross-disciplinary awareness")
	}
	if !strings.Contains(prompt, "允許受控推論") {
		t.Error("Prompt should explicitly allow bounded inference for keyword expansion")
	}
}

func TestParseExpandedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "structured output with analysis and keywords",
			input:    "分析：使用者想學統計學相關知識\n關鍵詞：統計 statistics 統計學 機率 probability",
			expected: "統計 statistics 統計學 機率 probability",
		},
		{
			name:     "structured output with colon variant",
			input:    "分析:想學金融\n關鍵詞:金融 finance 投資 investment",
			expected: "金融 finance 投資 investment",
		},
		{
			name:     "simplified Chinese variant",
			input:    "分析：想学统计\n关键词：统计 statistics 概率 probability",
			expected: "统计 statistics 概率 probability",
		},
		{
			name:     "unstructured keyword line is accepted",
			input:    "統計 statistics 統計學 機率 probability",
			expected: "統計 statistics 統計學 機率 probability",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   \n  ",
			expected: "",
		},
		{
			name:     "analysis without keyword marker - fallback to text after analysis",
			input:    "分析：使用者想學統計\n統計 statistics 機率 probability",
			expected: "統計 statistics 機率 probability",
		},
		{
			name:     "extra text after keywords line is stripped",
			input:    "分析：想學AI\n關鍵詞：人工智慧 AI machine learning\n\n這些是推薦的關鍵詞",
			expected: "人工智慧 AI machine learning",
		},
		{
			name:     "cross-disciplinary query result",
			input:    "分析：資工背景想跨入金融，應找金融相關且偏重量化分析與程式應用的課程\n關鍵詞：金融科技 FinTech 量化分析 quantitative analysis 財務工程 financial engineering",
			expected: "金融科技 FinTech 量化分析 quantitative analysis 財務工程 financial engineering",
		},
		{
			name:     "markdown keyword label is normalized",
			input:    "- 關鍵詞：金融科技、FinTech、量化分析、quantitative analysis",
			expected: "金融科技 FinTech 量化分析 quantitative analysis",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := ParseExpandedOutput(tc.input)
			if result != tc.expected {
				t.Errorf("ParseExpandedOutput(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestBuildExpandedQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		expanded string
		expected string
	}{
		{
			name:     "keeps original signal as compact terms",
			query:    "我是資工系的想學點金融知識推薦修哪些課",
			expanded: "金融科技 FinTech 量化分析 quantitative analysis 財務工程 financial engineering",
			expected: "資工系 金融 金融科技 FinTech 量化分析 quantitative analysis 財務工程 financial engineering",
		},
		{
			name:     "drops echoed raw query before merging",
			query:    "我是資工系的想學點金融知識推薦修哪些課",
			expanded: "我是資工系的想學點金融知識推薦修哪些課 金融科技 FinTech 量化分析 quantitative analysis",
			expected: "資工系 金融 金融科技 FinTech 量化分析 quantitative analysis",
		},
		{
			name:     "short keyword query still preserved",
			query:    "AWS",
			expanded: "Amazon Web Services 雲端服務 cloud computing",
			expected: "AWS Amazon Web Services 雲端服務 cloud computing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := BuildExpandedQuery(tc.query, tc.expanded)
			if result != tc.expected {
				t.Errorf("BuildExpandedQuery(%q, %q) = %q, want %q", tc.query, tc.expanded, result, tc.expected)
			}
		})
	}
}

func TestQueryExpanderNil(t *testing.T) {
	t.Parallel()
	var e *geminiQueryExpander
	result, err := e.Expand(context.Background(), "test query")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test query" {
		t.Errorf("Expand() = %q, want %q", result, "test query")
	}
}
