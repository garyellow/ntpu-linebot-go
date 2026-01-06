// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains shared types, interfaces, and configuration for NLU intent parsing
// and query expansion with multi-provider fallback support.
//
// Architecture:
// - Gemini: Uses google.golang.org/genai (official SDK)
// - Groq/Cerebras: Uses github.com/openai/openai-go/v3 (OpenAI-compatible API)
//
// Fallback Strategy (3-layer):
// 1. Model Retry: Same model retried with exponential backoff
// 2. Model Chain: Next model in same provider's model list
// 3. Provider Chain: Next provider in LLM_PROVIDERS list
package genai

import (
	"context"
	"time"
)

// Provider represents an LLM provider.
type Provider string

const (
	// ProviderGemini represents Google's Gemini API (non-OpenAI-compatible).
	ProviderGemini Provider = "gemini"
	// ProviderGroq represents Groq's API (OpenAI-compatible, fast inference).
	ProviderGroq Provider = "groq"
	// ProviderCerebras represents Cerebras's API (OpenAI-compatible, ultra-fast inference).
	ProviderCerebras Provider = "cerebras"
)

// ProviderEndpoint defines the base URL for OpenAI-compatible providers.
// Gemini is not included as it uses a different SDK.
var ProviderEndpoint = map[Provider]string{
	ProviderGroq:     "https://api.groq.com/openai/v1/",
	ProviderCerebras: "https://api.cerebras.ai/v1/",
}

// IsOpenAICompatible returns true if the provider uses OpenAI-compatible API.
func (p Provider) IsOpenAICompatible() bool {
	_, ok := ProviderEndpoint[p]
	return ok
}

// String returns the string representation of the provider.
func (p Provider) String() string {
	return string(p)
}

// IntentParser defines the interface for NLU intent parsing.
// Implementations include Gemini (native) and OpenAI-compatible providers (Groq, Cerebras).
// Uses forced function calling mode (ANY/required) to ensure consistent responses.
type IntentParser interface {
	// Parse analyzes user input and returns a parsed intent (always a function call).
	Parse(ctx context.Context, text string) (*ParseResult, error)
	// IsEnabled returns true if the parser is properly initialized.
	IsEnabled() bool
	// Close releases any resources held by the parser.
	Close() error
	// Provider returns the provider type for metrics.
	Provider() Provider
}

// QueryExpander defines the interface for query expansion.
// Implementations include Gemini (native) and OpenAI-compatible providers (Groq, Cerebras).
type QueryExpander interface {
	// Expand expands a query with synonyms and related terms.
	Expand(ctx context.Context, query string) (string, error)
	// Close releases any resources held by the expander.
	Close() error
	// Provider returns the provider type for metrics.
	Provider() Provider
}

// ParseResult represents the result of intent parsing.
type ParseResult struct {
	// Module is the target module.
	// Valid values: course, id, contact, program, help, direct_reply
	Module string

	// Intent is the specific intent within the module.
	// Examples: search, smart, uid (course module); search, student_id, department (id module)
	Intent string

	// Params contains the extracted parameters.
	// Key is the parameter name (e.g., "keyword", "query", "message").
	Params map[string]string

	// FunctionName is the raw function name from the model (for debugging).
	// Format: {module}_{intent} (e.g., "course_search", "direct_reply")
	FunctionName string
}

// RetryConfig defines retry behavior for LLM API calls.
// Uses AWS-recommended Full Jitter exponential backoff.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including initial).
	// Default: 2 (1 initial + 1 retry)
	MaxAttempts int

	// InitialDelay is the base delay before first retry.
	// Default: 500ms
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	// Default: 3s
	MaxDelay time.Duration
}

// ProviderConfig holds configuration for a single LLM provider.
type ProviderConfig struct {
	// APIKey is the API key for the provider.
	APIKey string

	// IntentModels is the ordered list of models for intent parsing.
	// First model is primary, rest are fallbacks tried in order.
	IntentModels []string

	// ExpanderModels is the ordered list of models for query expansion.
	// First model is primary, rest are fallbacks tried in order.
	ExpanderModels []string
}

// LLMConfig holds configuration for all LLM providers.
type LLMConfig struct {
	// Providers is the ordered list of providers to try.
	// Fallback happens in order: first provider's models, then second, etc.
	// Default: ["gemini", "groq", "cerebras"] (only those with API keys)
	Providers []Provider

	// Gemini configuration
	Gemini ProviderConfig

	// Groq configuration (OpenAI-compatible)
	Groq ProviderConfig

	// Cerebras configuration (OpenAI-compatible)
	Cerebras ProviderConfig

	// RetryConfig for retry behavior
	RetryConfig RetryConfig
}

// Default model configurations.
// First element is primary model, subsequent elements are fallbacks.
var (
	// DefaultGeminiIntentModels is the default model chain for Gemini intent parsing.
	// gemini-2.5-flash offers excellent function calling with fast inference.
	// gemini-2.5-flash-lite provides faster, cost-efficient fallback.
	DefaultGeminiIntentModels = []string{"gemini-2.5-flash", "gemini-2.5-flash-lite"}

	// DefaultGeminiExpanderModels is the default model chain for Gemini query expansion.
	DefaultGeminiExpanderModels = []string{"gemini-2.5-flash", "gemini-2.5-flash-lite"}

	// DefaultGroqIntentModels is the default model chain for Groq intent parsing.
	// Llama 4 Maverick (Preview) offers excellent function calling with fast inference (~900 TPS).
	// llama-3.3-70b-versatile is Production-grade fallback with strong accuracy.
	DefaultGroqIntentModels = []string{"meta-llama/llama-4-maverick-17b-128e-instruct", "llama-3.3-70b-versatile"}

	// DefaultGroqExpanderModels is the default model chain for Groq query expansion.
	// Llama 4 Scout (Preview) offers efficient query expansion with fast inference (~750 TPS).
	// llama-3.1-8b-instant is Production-grade fallback.
	DefaultGroqExpanderModels = []string{"meta-llama/llama-4-scout-17b-16e-instruct", "llama-3.1-8b-instant"}

	// DefaultCerebrasIntentModels is the default model chain for Cerebras intent parsing.
	// llama-3.3-70b offers strong function calling with ultra-fast inference.
	// llama-3.1-8b provides fast fallback.
	DefaultCerebrasIntentModels = []string{"llama-3.3-70b", "llama-3.1-8b"}

	// DefaultCerebrasExpanderModels is the default model chain for Cerebras query expansion.
	// llama-3.3-70b offers strong query expansion with ultra-fast inference.
	// llama-3.1-8b provides fast fallback.
	DefaultCerebrasExpanderModels = []string{"llama-3.3-70b", "llama-3.1-8b"}

	// DefaultProviders is the default provider order for fallback.
	DefaultProviders = []Provider{ProviderGemini, ProviderGroq, ProviderCerebras}
)

// Retry configuration defaults
const (
	DefaultMaxRetryAttempts  = 2
	DefaultInitialRetryDelay = 500 * time.Millisecond
	DefaultMaxRetryDelay     = 3 * time.Second
)

// HasAnyProvider returns true if at least one provider is configured.
func (c *LLMConfig) HasAnyProvider() bool {
	return c.Gemini.APIKey != "" || c.Groq.APIKey != "" || c.Cerebras.APIKey != ""
}

// HasProvider returns true if the specified provider is configured with an API key.
func (c *LLMConfig) HasProvider(p Provider) bool {
	switch p {
	case ProviderGemini:
		return c.Gemini.APIKey != ""
	case ProviderGroq:
		return c.Groq.APIKey != ""
	case ProviderCerebras:
		return c.Cerebras.APIKey != ""
	default:
		return false
	}
}

// GetProviderConfig returns the configuration for a specific provider.
func (c *LLMConfig) GetProviderConfig(p Provider) *ProviderConfig {
	switch p {
	case ProviderGemini:
		return &c.Gemini
	case ProviderGroq:
		return &c.Groq
	case ProviderCerebras:
		return &c.Cerebras
	default:
		return nil
	}
}

// ConfiguredProviders returns the list of providers with configured API keys,
// in the order specified by c.Providers.
func (c *LLMConfig) ConfiguredProviders() []Provider {
	result := make([]Provider, 0, len(c.Providers))
	for _, p := range c.Providers {
		if c.HasProvider(p) {
			result = append(result, p)
		}
	}
	return result
}
