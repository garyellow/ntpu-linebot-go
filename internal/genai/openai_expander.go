// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains the unified OpenAI-compatible implementation of query expansion.
// It works with any OpenAI-compatible provider (Groq, Cerebras) via custom BaseURL.
package genai

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// openaiQueryExpander expands user queries for better smart search results.
// Uses OpenAI-compatible LLM to add synonyms, translations, and related concepts.
// It implements the QueryExpander interface.
type openaiQueryExpander struct {
	client   openai.Client
	model    string
	provider Provider
}

// newOpenAIQueryExpander creates a new OpenAI-compatible query expander.
// Returns nil if apiKey is empty (expansion disabled).
//
// Parameters:
//   - provider: The provider type (ProviderGroq, ProviderCerebras)
//   - apiKey: The API key for the provider
//   - model: The model name to use (uses provider defaults if empty)
func newOpenAIQueryExpander(_ context.Context, provider Provider, apiKey, model string) (*openaiQueryExpander, error) {
	if apiKey == "" {
		return nil, nil //nolint:nilnil // Intentional: feature disabled when no API key
	}

	// Get the base URL for the provider
	baseURL, ok := ProviderEndpoint[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported OpenAI-compatible provider: %s", provider)
	}

	// Use default model if not specified
	if model == "" {
		switch provider {
		case ProviderGroq:
			model = DefaultGroqExpanderModels[0]
		case ProviderCerebras:
			model = DefaultCerebrasExpanderModels[0]
		default:
			return nil, fmt.Errorf("no default model for provider: %s", provider)
		}
	}

	// Create client with custom base URL
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	return &openaiQueryExpander{
		client:   client,
		model:    model,
		provider: provider,
	}, nil
}

// Expand expands a query with synonyms and related terms for better search.
// Returns the expanded query string.
func (e *openaiQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if e == nil {
		return query, nil
	}

	// Let LLM handle ALL queries - it can:
	// 1. Expand abbreviations (AWS → 雲端服務)
	// 2. Add synonyms and related terms
	// 3. Clean up verbose queries to extract key concepts
	// 4. Handle mixed Chinese/English with different information density
	prompt := QueryExpansionPrompt(query)

	params := openai.ChatCompletionNewParams{
		Model: e.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Temperature: openai.Float(0.3), // Low temperature for consistent expansion
		MaxTokens:   openai.Int(200),
	}

	start := time.Now()
	resp, err := e.client.Chat.Completions.New(ctx, params)
	duration := time.Since(start)

	if err != nil {
		slog.WarnContext(ctx, "query expansion API call failed",
			"provider", e.provider,
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
			"provider", e.provider,
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
func (e *openaiQueryExpander) Provider() Provider {
	if e == nil {
		return ""
	}
	return e.provider
}

// Close releases resources.
// Safe to call on nil receiver.
func (e *openaiQueryExpander) Close() error {
	if e == nil {
		return nil
	}
	// openai-go client doesn't require cleanup
	return nil
}
