// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains the Groq implementation of query expansion.
package genai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/conneroisu/groq-go"
)

// groqQueryExpander expands user queries for better smart search results.
// Uses Groq LLM to add synonyms, translations, and related concepts.
// It implements the QueryExpander interface.
type groqQueryExpander struct {
	client *groq.Client
	model  groq.ChatModel
}

// newGroqQueryExpander creates a new Groq-based query expander.
// Returns nil if apiKey is empty (expansion disabled).
func newGroqQueryExpander(_ context.Context, apiKey, model string) (*groqQueryExpander, error) {
	if apiKey == "" {
		return nil, nil //nolint:nilnil // Intentional: feature disabled when no API key
	}

	if model == "" {
		model = DefaultGroqExpanderModel
	}

	client, err := groq.NewClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create groq client: %w", err)
	}

	return &groqQueryExpander{
		client: client,
		model:  groq.ChatModel(model),
	}, nil
}

// Expand expands a query with synonyms and related terms for better search.
// Returns the expanded query string.
func (e *groqQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if e == nil || e.client == nil {
		return query, nil
	}

	// Skip expansion for very long queries (already descriptive enough)
	if len([]rune(query)) > MinQueryLengthForExpansion && !containsAbbreviation(query) {
		return query, nil
	}

	prompt := buildExpansionPrompt(query)

	req := groq.ChatCompletionRequest{
		Model: e.model,
		Messages: []groq.ChatCompletionMessage{
			{
				Role:    groq.RoleUser,
				Content: prompt,
			},
		},
		Temperature: 0.3, // Low temperature for consistent expansion
		MaxTokens:   200,
	}

	start := time.Now()
	resp, err := e.client.ChatCompletion(ctx, req)
	duration := time.Since(start)

	if err != nil {
		slog.WarnContext(ctx, "query expansion API call failed",
			"provider", "groq",
			"model", e.model,
			"query_length", len(query),
			"duration_ms", duration.Milliseconds(),
			"error", err)
		// Return error for retry/fallback decision
		return query, fmt.Errorf("chat completion failed: %w", err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return query, nil
	}

	result := strings.TrimSpace(resp.Choices[0].Message.Content)
	if result == "" {
		return query, nil
	}

	// Ensure original query is preserved (prepend if not present)
	if !strings.Contains(result, query) {
		result = query + " " + result
	}

	// Log success with token usage
	if resp.Usage.TotalTokens > 0 {
		slog.DebugContext(ctx, "query expansion completed",
			"provider", "groq",
			"model", e.model,
			"input_tokens", resp.Usage.PromptTokens,
			"output_tokens", resp.Usage.CompletionTokens,
			"total_tokens", resp.Usage.TotalTokens,
			"duration_ms", duration.Milliseconds(),
			"original_length", len(query),
			"expanded_length", len(result))
	}

	return result, nil
}

// Provider returns the provider type for this expander.
func (e *groqQueryExpander) Provider() Provider {
	return ProviderGroq
}

// Close releases resources.
// Safe to call on nil receiver.
func (e *groqQueryExpander) Close() error {
	if e == nil {
		return nil
	}
	// groq-go client doesn't require cleanup
	return nil
}
