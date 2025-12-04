// Package genai provides integration with Google's Generative AI APIs.
// This file contains the QueryExpander for semantic search query expansion.
package genai

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/genai"
)

const (
	// ExpanderModel is the model used for query expansion
	ExpanderModel = "gemini-2.0-flash-lite"

	// ExpanderTimeout is the timeout for query expansion calls
	ExpanderTimeout = 8 * time.Second

	// MinQueryLengthForExpansion is the minimum rune count to skip expansion
	// Short queries benefit most from expansion
	MinQueryLengthForExpansion = 15
)

// QueryExpander expands user queries for better semantic search results.
// Uses LLM to add synonyms, translations, and related concepts.
type QueryExpander struct {
	client *genai.Client
	model  string
}

// NewQueryExpander creates a new query expander
func NewQueryExpander(ctx context.Context, apiKey string) (*QueryExpander, error) {
	if apiKey == "" {
		return nil, nil
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &QueryExpander{
		client: client,
		model:  ExpanderModel,
	}, nil
}

// Expand expands a query with synonyms and related terms for better search.
// Returns the expanded query string.
//
// Strategy:
// 1. Detect technical terms, abbreviations, and English words
// 2. Add Chinese translations for English terms
// 3. Add English translations for Chinese terms
// 4. Add related concepts and synonyms
//
// Examples:
//   - "AWS" → "AWS Amazon Web Services 雲端服務 雲端運算 cloud computing"
//   - "我想學 AI" → "我想學 AI 人工智慧 機器學習 深度學習 artificial intelligence machine learning"
//   - "程式設計" → "程式設計 programming 軟體開發 coding 程式開發"
func (e *QueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if e == nil || e.client == nil {
		return query, nil
	}

	// Skip expansion for very long queries (already descriptive enough)
	if len([]rune(query)) > MinQueryLengthForExpansion && !containsAbbreviation(query) {
		return query, nil
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, ExpanderTimeout)
	defer cancel()

	prompt := buildExpansionPrompt(query)

	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.3), // Low temperature for consistent expansion
		MaxOutputTokens: 200,
	}

	resp, err := e.client.Models.GenerateContent(ctx, e.model, genai.Text(prompt), config)
	if err != nil {
		// On error, return original query (graceful degradation)
		return query, nil
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return query, nil
	}

	// Extract expanded query from response
	var expanded strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" {
			expanded.WriteString(part.Text)
		}
	}

	result := strings.TrimSpace(expanded.String())
	if result == "" {
		return query, nil
	}

	// Ensure original query is preserved (prepend if not present)
	if !strings.Contains(result, query) {
		result = query + " " + result
	}

	return result, nil
}

// Close releases resources (no-op for current API version)
func (e *QueryExpander) Close() error {
	// Current genai.Client doesn't have a Close method
	return nil
}

// buildExpansionPrompt creates the prompt for query expansion
func buildExpansionPrompt(query string) string {
	return fmt.Sprintf(`你是課程搜尋查詢擴展助手。擴展以下查詢以提高課程搜尋效果。

## 任務
將使用者查詢擴展為包含同義詞、翻譯和相關概念的搜尋詞。

## 規則
1. 保留原始查詢詞
2. 英文縮寫必須加上全稱（AWS→Amazon Web Services）
3. 英文術語必須加上中文翻譯（AI→人工智慧）
4. 中文術語必須加上英文翻譯（機器學習→machine learning）
5. 加入2-3個相關概念
6. 只輸出擴展後的關鍵詞，用空格分隔
7. 不要輸出任何解釋或標點符號

## 範例
輸入: AWS
輸出: AWS Amazon Web Services 雲端服務 雲端運算 cloud computing EC2 S3

輸入: 我想學 AI
輸出: AI 人工智慧 artificial intelligence 機器學習 machine learning 深度學習

輸入: 程式設計
輸出: 程式設計 programming 軟體開發 coding 程式語言 software development

輸入: 資料分析
輸出: 資料分析 data analysis 數據分析 統計 statistics 資料科學 data science

## 查詢
%s

## 輸出`, query)
}

// containsAbbreviation checks if query contains technical abbreviations
var abbreviationRegex = regexp.MustCompile(`(?i)\b(AWS|AI|ML|DL|API|SDK|SQL|DB|UI|UX|IoT|AR|VR|NLP|CV|LLM|GPT|RAG|ETL|CI|CD|K8S|GCP|AZURE)\b`)

func containsAbbreviation(query string) bool {
	return abbreviationRegex.MatchString(query)
}
