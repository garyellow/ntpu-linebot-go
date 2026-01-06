// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains factory functions for creating LLM providers.
package genai

import (
	"context"
	"log/slog"
)

// CreateIntentParser creates an IntentParser based on the provided configuration.
// It returns a FallbackIntentParser that handles model-to-model and provider-to-provider fallback.
//
// Provider chain logic (3-layer fallback):
//  1. For each provider in cfg.Providers order:
//     - Each model in provider's IntentModels is tried
//     - Each model has retry logic (configured in RetryConfig)
//  2. Providers without API keys are skipped
//  3. Returns nil if no providers/models are configured.
func CreateIntentParser(ctx context.Context, cfg LLMConfig) (IntentParser, error) {
	parsers := []IntentParser{}

	// Iterate through providers in configured order
	for _, provider := range cfg.Providers {
		if !cfg.HasProvider(provider) {
			continue
		}

		providerCfg := cfg.GetProviderConfig(provider)
		if providerCfg == nil {
			continue
		}

		// Get models for this provider (use defaults if not configured)
		models := providerCfg.IntentModels
		if len(models) == 0 {
			models = getDefaultIntentModels(provider)
		}

		// Create parsers for each model
		for _, model := range models {
			p, err := createIntentParserForProvider(ctx, provider, providerCfg.APIKey, model)
			if err != nil {
				slog.WarnContext(ctx, "failed to create intent parser",
					"provider", provider,
					"model", model,
					"error", err)
				continue
			}
			if p != nil {
				parsers = append(parsers, p)
			}
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

// createIntentParserForProvider creates an IntentParser for a specific provider.
func createIntentParserForProvider(ctx context.Context, provider Provider, apiKey, model string) (IntentParser, error) {
	switch provider {
	case ProviderGemini:
		return newGeminiIntentParser(ctx, apiKey, model)
	case ProviderGroq, ProviderCerebras:
		// OpenAI-compatible providers
		return newOpenAIIntentParser(ctx, provider, apiKey, model)
	default:
		return nil, nil
	}
}

// CreateQueryExpander creates a QueryExpander based on the provided configuration.
// Similar to CreateIntentParser, it handles model-to-model and provider-to-provider fallback.
func CreateQueryExpander(ctx context.Context, cfg LLMConfig) (QueryExpander, error) {
	expanders := []QueryExpander{}

	// Iterate through providers in configured order
	for _, provider := range cfg.Providers {
		if !cfg.HasProvider(provider) {
			continue
		}

		providerCfg := cfg.GetProviderConfig(provider)
		if providerCfg == nil {
			continue
		}

		// Get models for this provider (use defaults if not configured)
		models := providerCfg.ExpanderModels
		if len(models) == 0 {
			models = getDefaultExpanderModels(provider)
		}

		// Create expanders for each model
		for _, model := range models {
			e, err := createExpanderForProvider(ctx, provider, providerCfg.APIKey, model)
			if err != nil {
				slog.WarnContext(ctx, "failed to create query expander",
					"provider", provider,
					"model", model,
					"error", err)
				continue
			}
			if e != nil {
				expanders = append(expanders, e)
			}
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

// createExpanderForProvider creates a QueryExpander for a specific provider.
func createExpanderForProvider(ctx context.Context, provider Provider, apiKey, model string) (QueryExpander, error) {
	switch provider {
	case ProviderGemini:
		return newGeminiQueryExpander(ctx, apiKey, model)
	case ProviderGroq, ProviderCerebras:
		// OpenAI-compatible providers
		return newOpenAIQueryExpander(ctx, provider, apiKey, model)
	default:
		return nil, nil
	}
}

// getDefaultIntentModels returns the default intent models for a provider.
func getDefaultIntentModels(provider Provider) []string {
	switch provider {
	case ProviderGemini:
		return DefaultGeminiIntentModels
	case ProviderGroq:
		return DefaultGroqIntentModels
	case ProviderCerebras:
		return DefaultCerebrasIntentModels
	default:
		return nil
	}
}

// getDefaultExpanderModels returns the default expander models for a provider.
func getDefaultExpanderModels(provider Provider) []string {
	switch provider {
	case ProviderGemini:
		return DefaultGeminiExpanderModels
	case ProviderGroq:
		return DefaultGroqExpanderModels
	case ProviderCerebras:
		return DefaultCerebrasExpanderModels
	default:
		return nil
	}
}

// DefaultLLMConfig returns a default LLM configuration.
// API keys must be provided separately.
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Providers: DefaultProviders,
		Gemini: ProviderConfig{
			IntentModels:   DefaultGeminiIntentModels,
			ExpanderModels: DefaultGeminiExpanderModels,
		},
		Groq: ProviderConfig{
			IntentModels:   DefaultGroqIntentModels,
			ExpanderModels: DefaultGroqExpanderModels,
		},
		Cerebras: ProviderConfig{
			IntentModels:   DefaultCerebrasIntentModels,
			ExpanderModels: DefaultCerebrasExpanderModels,
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
