// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains factory functions for creating LLM providers.
package genai

import (
	"context"
	"log/slog"
)

// CreateIntentParser creates an IntentParser based on the provided configuration.
// It returns nil if no providers/models are configured.
func CreateIntentParser(ctx context.Context, cfg LLMConfig) (IntentParser, error) {
	parsers := []IntentParser{}
	for _, spec := range buildModelChain(cfg, func(provider Provider, providerCfg *ProviderConfig) []string {
		models := providerCfg.IntentModels
		if len(models) == 0 {
			models = getDefaultIntentModels(provider)
		}
		return models
	}) {
		p, err := createIntentParserForProvider(ctx, spec.provider, spec.providerCfg, spec.model)
		if err != nil {
			slog.WarnContext(ctx, "Failed to create intent parser",
				"provider", spec.provider,
				"model", spec.model,
				"error", err)
			continue
		}
		if p != nil {
			parsers = append(parsers, p)
		}
	}

	// No providers available
	if len(parsers) == 0 {
		slog.InfoContext(ctx, "No LLM provider configured for intent parsing")
		return nil, nil
	}

	// Log configuration
	slog.InfoContext(ctx, "Intent parser configured",
		"primary", parsers[0].Provider(),
		"chainSize", len(parsers))

	return NewFallbackIntentParser(cfg.RetryConfig, parsers...), nil
}

// createIntentParserForProvider creates an IntentParser for a specific provider.
func createIntentParserForProvider(ctx context.Context, provider Provider, cfg *ProviderConfig, model string) (IntentParser, error) {
	switch provider {
	case ProviderGemini:
		return newGeminiIntentParser(ctx, cfg.APIKey, model)
	case ProviderGroq, ProviderCerebras:
		// OpenAI-compatible providers with fixed endpoints
		return newOpenAIIntentParser(ctx, provider, cfg.APIKey, model, "")
	case ProviderOpenAI:
		// OpenAI-compatible with custom endpoint
		return newOpenAIIntentParser(ctx, provider, cfg.APIKey, model, cfg.Endpoint)
	default:
		return nil, nil
	}
}

// CreateQueryExpander creates a QueryExpander based on the provided configuration.
// It returns nil if no providers/models are configured.
func CreateQueryExpander(ctx context.Context, cfg LLMConfig) (QueryExpander, error) {
	expanders := []QueryExpander{}
	for _, spec := range buildModelChain(cfg, func(provider Provider, providerCfg *ProviderConfig) []string {
		models := providerCfg.ExpanderModels
		if len(models) == 0 {
			models = getDefaultExpanderModels(provider)
		}
		return models
	}) {
		e, err := createExpanderForProvider(ctx, spec.provider, spec.providerCfg, spec.model)
		if err != nil {
			slog.WarnContext(ctx, "Failed to create query expander",
				"provider", spec.provider,
				"model", spec.model,
				"error", err)
			continue
		}
		if e != nil {
			expanders = append(expanders, e)
		}
	}

	// No providers available
	if len(expanders) == 0 {
		slog.InfoContext(ctx, "No LLM provider configured for query expansion")
		return nil, nil
	}

	slog.InfoContext(ctx, "Query expander configured",
		"primary", expanders[0].Provider(),
		"chainSize", len(expanders))

	return NewFallbackQueryExpander(cfg.RetryConfig, expanders...), nil
}

type modelSpec struct {
	provider    Provider
	providerCfg *ProviderConfig
	model       string
}

type providerModelSet struct {
	provider    Provider
	providerCfg *ProviderConfig
	models      []string
}

func buildModelChain(cfg LLMConfig, selectModels func(Provider, *ProviderConfig) []string) []modelSpec {
	providers := make([]providerModelSet, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		if !cfg.HasProvider(provider) {
			continue
		}
		providerCfg := cfg.GetProviderConfig(provider)
		if providerCfg == nil {
			continue
		}
		models := selectModels(provider, providerCfg)
		if len(models) == 0 {
			continue
		}
		providers = append(providers, providerModelSet{
			provider:    provider,
			providerCfg: providerCfg,
			models:      models,
		})
	}

	maxModels := 0
	for _, provider := range providers {
		if len(provider.models) > maxModels {
			maxModels = len(provider.models)
		}
	}

	chain := make([]modelSpec, 0)
	for modelIndex := range maxModels {
		for _, provider := range providers {
			if modelIndex >= len(provider.models) {
				continue
			}
			chain = append(chain, modelSpec{
				provider:    provider.provider,
				providerCfg: provider.providerCfg,
				model:       provider.models[modelIndex],
			})
		}
	}
	return chain
}

// createExpanderForProvider creates a QueryExpander for a specific provider.
func createExpanderForProvider(ctx context.Context, provider Provider, cfg *ProviderConfig, model string) (QueryExpander, error) {
	switch provider {
	case ProviderGemini:
		return newGeminiQueryExpander(ctx, cfg.APIKey, model)
	case ProviderGroq, ProviderCerebras:
		// OpenAI-compatible providers with fixed endpoints
		return newOpenAIQueryExpander(ctx, provider, cfg.APIKey, model, "")
	case ProviderOpenAI:
		// OpenAI-compatible with custom endpoint
		return newOpenAIQueryExpander(ctx, provider, cfg.APIKey, model, cfg.Endpoint)
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
	case ProviderOpenAI:
		// OpenAI-compatible custom endpoint has no default models
		return nil
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
	case ProviderOpenAI:
		// OpenAI-compatible custom endpoint has no default models
		return nil
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
		MaxAttempts:    DefaultMaxRetryAttempts,
		InitialDelay:   DefaultInitialRetryDelay,
		MaxDelay:       DefaultMaxRetryDelay,
		AttemptTimeout: DefaultLLMAttemptTimeout,
	}
}
