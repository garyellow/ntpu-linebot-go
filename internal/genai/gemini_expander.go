// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains the Gemini implementation of query expansion.
package genai

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"google.golang.org/genai"
)

// MinQueryLengthForExpansion is the minimum rune count to skip expansion
// Short queries benefit most from expansion
const MinQueryLengthForExpansion = 15

// geminiQueryExpander expands user queries for better smart search results.
// Uses LLM to add synonyms, translations, and related concepts.
// It implements the QueryExpander interface.
type geminiQueryExpander struct {
	client *genai.Client
	model  string
}

// newGeminiQueryExpander creates a new Gemini-based query expander.
// Returns nil if apiKey is empty (expansion disabled).
func newGeminiQueryExpander(ctx context.Context, apiKey, model string) (*geminiQueryExpander, error) {
	if apiKey == "" {
		return nil, nil //nolint:nilnil // Intentional: feature disabled when no API key
	}

	if model == "" {
		model = DefaultGeminiExpanderModel
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &geminiQueryExpander{
		client: client,
		model:  model,
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
func (e *geminiQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if e == nil || e.client == nil {
		return query, nil
	}

	// Skip expansion for very long queries (already descriptive enough)
	if len([]rune(query)) > MinQueryLengthForExpansion && !containsAbbreviation(query) {
		return query, nil
	}

	prompt := buildExpansionPrompt(query)

	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.3), // Low temperature for consistent expansion
		MaxOutputTokens: 200,
	}

	start := time.Now()
	resp, err := e.client.Models.GenerateContent(ctx, e.model, genai.Text(prompt), config)
	duration := time.Since(start)

	if err != nil {
		slog.WarnContext(ctx, "query expansion API call failed",
			"provider", "gemini",
			"model", e.model,
			"query_length", len(query),
			"duration_ms", duration.Milliseconds(),
			"error", err)
		// Return error for retry/fallback decision
		return query, fmt.Errorf("generate content failed: %w", err)
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

	// Log success with token usage
	if resp.UsageMetadata != nil {
		slog.DebugContext(ctx, "query expansion completed",
			"provider", "gemini",
			"model", e.model,
			"input_tokens", resp.UsageMetadata.PromptTokenCount,
			"output_tokens", resp.UsageMetadata.CandidatesTokenCount,
			"total_tokens", resp.UsageMetadata.TotalTokenCount,
			"duration_ms", duration.Milliseconds(),
			"original_length", len(query),
			"expanded_length", len(result))
	}

	return result, nil
}

// Provider returns the provider type for this expander.
func (e *geminiQueryExpander) Provider() Provider {
	return ProviderGemini
}

// Close releases resources.
// Safe to call on nil receiver.
func (e *geminiQueryExpander) Close() error {
	if e == nil {
		return nil
	}
	// Future: Add client.Close() when SDK supports it
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
