// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains factory functions for creating LLM providers.
package genai

import (
	"context"
	"fmt"
	"log/slog"
)

// CreateIntentParser creates an IntentParser based on the provided configuration.
// It returns a FallbackIntentParser that handles retry and provider fallback.
//
// Provider selection logic:
//  1. If both providers are configured, primary is used first with fallback
//  2. If only one provider is configured, it's used without fallback
//  3. Returns nil if no providers are configured
//
// The returned parser implements the IntentParser interface.
func CreateIntentParser(ctx context.Context, cfg LLMConfig) (IntentParser, error) {
	var primary, fallback IntentParser
	var err error

	// Create primary provider
	switch cfg.PrimaryProvider {
	case ProviderGemini:
		if cfg.Gemini.APIKey != "" {
			primary, err = newGeminiIntentParser(ctx, cfg.Gemini.APIKey, cfg.Gemini.IntentModel)
			if err != nil {
				return nil, fmt.Errorf("failed to create gemini intent parser: %w", err)
			}
		}
	case ProviderGroq:
		if cfg.Groq.APIKey != "" {
			primary, err = newGroqIntentParser(ctx, cfg.Groq.APIKey, cfg.Groq.IntentModel)
			if err != nil {
				return nil, fmt.Errorf("failed to create groq intent parser: %w", err)
			}
		}
	}

	// Create fallback provider (opposite of primary)
	switch cfg.FallbackProvider {
	case ProviderGemini:
		if cfg.Gemini.APIKey != "" && cfg.FallbackProvider != cfg.PrimaryProvider {
			fallback, err = newGeminiIntentParser(ctx, cfg.Gemini.APIKey, cfg.Gemini.IntentFallbackModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create gemini fallback parser", "error", err)
			}
		}
	case ProviderGroq:
		if cfg.Groq.APIKey != "" && cfg.FallbackProvider != cfg.PrimaryProvider {
			fallback, err = newGroqIntentParser(ctx, cfg.Groq.APIKey, cfg.Groq.IntentFallbackModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create groq fallback parser", "error", err)
			}
		}
	}

	// If no primary, try to use what's available
	if primary == nil {
		if cfg.Gemini.APIKey != "" {
			primary, err = newGeminiIntentParser(ctx, cfg.Gemini.APIKey, cfg.Gemini.IntentModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create gemini intent parser", "error", err)
			}
		}
		if primary == nil && cfg.Groq.APIKey != "" {
			primary, err = newGroqIntentParser(ctx, cfg.Groq.APIKey, cfg.Groq.IntentModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create groq intent parser", "error", err)
			}
		}
	}

	// No providers available
	if primary == nil {
		slog.InfoContext(ctx, "no LLM provider configured for intent parsing")
		return nil, nil
	}

	// Log configuration
	slog.InfoContext(ctx, "intent parser configured",
		"primary", primary.Provider(),
		"hasFallback", fallback != nil)

	return NewFallbackIntentParser(primary, fallback, cfg.RetryConfig), nil
}

// CreateQueryExpander creates a QueryExpander based on the provided configuration.
// Similar to CreateIntentParser, it handles retry and provider fallback.
func CreateQueryExpander(ctx context.Context, cfg LLMConfig) (QueryExpander, error) {
	var primary, fallback QueryExpander
	var err error

	// Create primary provider
	switch cfg.PrimaryProvider {
	case ProviderGemini:
		if cfg.Gemini.APIKey != "" {
			primary, err = newGeminiQueryExpander(ctx, cfg.Gemini.APIKey, cfg.Gemini.ExpanderModel)
			if err != nil {
				return nil, fmt.Errorf("failed to create gemini query expander: %w", err)
			}
		}
	case ProviderGroq:
		if cfg.Groq.APIKey != "" {
			primary, err = newGroqQueryExpander(ctx, cfg.Groq.APIKey, cfg.Groq.ExpanderModel)
			if err != nil {
				return nil, fmt.Errorf("failed to create groq query expander: %w", err)
			}
		}
	}

	// Create fallback provider
	switch cfg.FallbackProvider {
	case ProviderGemini:
		if cfg.Gemini.APIKey != "" && cfg.FallbackProvider != cfg.PrimaryProvider {
			fallback, err = newGeminiQueryExpander(ctx, cfg.Gemini.APIKey, cfg.Gemini.ExpanderFallbackModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create gemini fallback expander", "error", err)
			}
		}
	case ProviderGroq:
		if cfg.Groq.APIKey != "" && cfg.FallbackProvider != cfg.PrimaryProvider {
			fallback, err = newGroqQueryExpander(ctx, cfg.Groq.APIKey, cfg.Groq.ExpanderFallbackModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create groq fallback expander", "error", err)
			}
		}
	}

	// If no primary, try to use what's available
	if primary == nil {
		if cfg.Gemini.APIKey != "" {
			primary, err = newGeminiQueryExpander(ctx, cfg.Gemini.APIKey, cfg.Gemini.ExpanderModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create gemini query expander", "error", err)
			}
		}
		if primary == nil && cfg.Groq.APIKey != "" {
			primary, err = newGroqQueryExpander(ctx, cfg.Groq.APIKey, cfg.Groq.ExpanderModel)
			if err != nil {
				slog.WarnContext(ctx, "failed to create groq query expander", "error", err)
			}
		}
	}

	// No providers available
	if primary == nil {
		slog.InfoContext(ctx, "no LLM provider configured for query expansion")
		return nil, nil
	}

	slog.InfoContext(ctx, "query expander configured",
		"primary", primary.Provider(),
		"hasFallback", fallback != nil)

	return NewFallbackQueryExpander(primary, fallback, cfg.RetryConfig), nil
}

// DefaultLLMConfig returns a default LLM configuration.
// API keys must be provided separately.
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		PrimaryProvider:  ProviderGemini,
		FallbackProvider: ProviderGroq,
		Gemini: ProviderConfig{
			IntentModel:           DefaultGeminiIntentModel,
			IntentFallbackModel:   DefaultGeminiIntentFallbackModel,
			ExpanderModel:         DefaultGeminiExpanderModel,
			ExpanderFallbackModel: DefaultGeminiExpanderFallbackModel,
		},
		Groq: ProviderConfig{
			IntentModel:           DefaultGroqIntentModel,
			IntentFallbackModel:   DefaultGroqIntentFallbackModel,
			ExpanderModel:         DefaultGroqExpanderModel,
			ExpanderFallbackModel: DefaultGroqExpanderFallbackModel,
		},
		RetryConfig: DefaultRetryConfig(),
	}
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  DefaultMaxRetryAttempts,
		InitialDelay: DefaultInitialRetryDelay,
		MaxDelay:     DefaultMaxRetryDelay,
	}
}
