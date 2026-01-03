// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains factory functions for creating LLM providers.
package genai

import (
	"context"
	"log/slog"
)

// CreateIntentParser creates an IntentParser based on the provided configuration.
// It returns a FallbackIntentParser that handles model-to-model and provider-to-provider fallback.
//
// Provider selection logic:
//  1. Primary provider's models are tried first in the specified order.
//  2. If all primary models fail, fallback provider's models are tried.
//  3. Each model is tried with retry logic (configured in RetryConfig).
//  4. Returns nil if no providers/models are configured.
func CreateIntentParser(ctx context.Context, cfg LLMConfig) (IntentParser, error) {
	parsers := []IntentParser{}

	// Helper to add parsers for a provider
	addParsers := func(provider Provider) {
		switch provider {
		case ProviderGemini:
			if cfg.Gemini.APIKey != "" {
				for _, m := range cfg.Gemini.IntentModels {
					p, err := newGeminiIntentParser(ctx, cfg.Gemini.APIKey, m)
					if err == nil {
						parsers = append(parsers, p)
					} else {
						slog.WarnContext(ctx, "failed to create gemini intent parser", "model", m, "error", err)
					}
				}
			}
		case ProviderGroq:
			if cfg.Groq.APIKey != "" {
				for _, m := range cfg.Groq.IntentModels {
					p, err := newGroqIntentParser(ctx, cfg.Groq.APIKey, m)
					if err == nil {
						parsers = append(parsers, p)
					} else {
						slog.WarnContext(ctx, "failed to create groq intent parser", "model", m, "error", err)
					}
				}
			}
		}
	}

	// Add primary followed by fallback
	addParsers(cfg.PrimaryProvider)
	if cfg.FallbackProvider != cfg.PrimaryProvider {
		addParsers(cfg.FallbackProvider)
	}

	// If list is empty, try to find ANY configured provider
	if len(parsers) == 0 {
		if cfg.Gemini.APIKey != "" && len(cfg.Gemini.IntentModels) > 0 {
			addParsers(ProviderGemini)
		}
		if len(parsers) == 0 && cfg.Groq.APIKey != "" && len(cfg.Groq.IntentModels) > 0 {
			addParsers(ProviderGroq)
		}
	}

	// No providers available
	if len(parsers) == 0 {
		slog.InfoContext(ctx, "no LLM provider configured for intent parsing")
		return nil, nil
	}

	// Log configuration
	slog.InfoContext(ctx, "intent parser configured",
		"primary", parsers[0].Provider(),
		"chainSize", len(parsers))

	return NewFallbackIntentParser(cfg.RetryConfig, parsers...), nil
}

// CreateQueryExpander creates a QueryExpander based on the provided configuration.
// Similar to CreateIntentParser, it handles model-to-model and provider-to-provider fallback.
func CreateQueryExpander(ctx context.Context, cfg LLMConfig) (QueryExpander, error) {
	expanders := []QueryExpander{}

	// Helper to add expanders for a provider
	addExpanders := func(provider Provider) {
		switch provider {
		case ProviderGemini:
			if cfg.Gemini.APIKey != "" {
				for _, m := range cfg.Gemini.ExpanderModels {
					e, err := newGeminiQueryExpander(ctx, cfg.Gemini.APIKey, m)
					if err == nil {
						expanders = append(expanders, e)
					} else {
						slog.WarnContext(ctx, "failed to create gemini query expander", "model", m, "error", err)
					}
				}
			}
		case ProviderGroq:
			if cfg.Groq.APIKey != "" {
				for _, m := range cfg.Groq.ExpanderModels {
					e, err := newGroqQueryExpander(ctx, cfg.Groq.APIKey, m)
					if err == nil {
						expanders = append(expanders, e)
					} else {
						slog.WarnContext(ctx, "failed to create groq query expander", "model", m, "error", err)
					}
				}
			}
		}
	}

	// Add primary followed by fallback
	addExpanders(cfg.PrimaryProvider)
	if cfg.FallbackProvider != cfg.PrimaryProvider {
		addExpanders(cfg.FallbackProvider)
	}

	// If list is empty, try to find ANY configured provider
	if len(expanders) == 0 {
		if cfg.Gemini.APIKey != "" && len(cfg.Gemini.ExpanderModels) > 0 {
			addExpanders(ProviderGemini)
		}
		if len(expanders) == 0 && cfg.Groq.APIKey != "" && len(cfg.Groq.ExpanderModels) > 0 {
			addExpanders(ProviderGroq)
		}
	}

	// No providers available
	if len(expanders) == 0 {
		slog.InfoContext(ctx, "no LLM provider configured for query expansion")
		return nil, nil
	}

	slog.InfoContext(ctx, "query expander configured",
		"primary", expanders[0].Provider(),
		"chainSize", len(expanders))

	return NewFallbackQueryExpander(cfg.RetryConfig, expanders...), nil
}

// DefaultLLMConfig returns a default LLM configuration.
// API keys must be provided separately.
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		PrimaryProvider:  ProviderGemini,
		FallbackProvider: ProviderGroq,
		Gemini: ProviderConfig{
			IntentModels:   DefaultGeminiIntentModels,
			ExpanderModels: DefaultGeminiExpanderModels,
		},
		Groq: ProviderConfig{
			IntentModels:   DefaultGroqIntentModels,
			ExpanderModels: DefaultGroqExpanderModels,
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
