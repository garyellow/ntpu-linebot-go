// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains the Gemini implementation of query expansion.
package genai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/genai"
)

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
		model = DefaultGeminiExpanderModels[0]
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
// 1. Extract core keywords (remove instruction words like "想學", "幫我找")
// 2. Expand abbreviations (AWS → Amazon Web Services)
// 3. Add bilingual translations (Chinese ↔ English)
// 4. Include related concepts and synonyms
// 5. Natural deduplication via space-separated output format
//
// BM25 Integration:
// - Expanded query is tokenized by BM25 (unigram for CJK, words for English)
// - Space-separated format allows BM25 to weight each term independently
// - Duplicate terms are naturally merged by BM25's term frequency calculation
//
// Examples:
//   - "想學 AWS" → "AWS Amazon Web Services 雲端服務 雲端運算 cloud computing"
//   - "我想學 AI" → "AI 人工智慧 機器學習 深度學習 artificial intelligence machine learning"
//   - "程式設計課程" → "程式設計 programming 軟體開發 coding 演算法 algorithms"
func (e *geminiQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if e == nil || e.client == nil {
		return query, nil
	}

	// Let LLM handle ALL queries - it can:
	// 1. Expand abbreviations (AWS → 雲端服務)
	// 2. Add synonyms and related terms
	// 3. Clean up verbose queries to extract key concepts
	// 4. Handle mixed Chinese/English with different information density
	prompt := QueryExpansionPrompt(query)

	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr[float32](0.3), // Low temperature for consistent expansion
		MaxOutputTokens: 200,
	}

	start := time.Now()
	resp, err := e.client.Models.GenerateContent(ctx, e.model, genai.Text(prompt), config)
	duration := time.Since(start)

	if err != nil {
		slog.WarnContext(ctx, "Query expansion API call failed",
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
		slog.DebugContext(ctx, "Query expansion completed",
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
	// Note: genai.Client does not require explicit cleanup in current SDK version
	return nil
}
