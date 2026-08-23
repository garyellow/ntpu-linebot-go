// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains shared types, interfaces, and configuration for NLU intent parsing
// and query expansion with multi-provider fallback support.
//
// Architecture:
// - Gemini: Uses google.golang.org/genai (official SDK)
// - Groq/Cerebras: Uses github.com/openai/openai-go/v3 (OpenAI-compatible API)
//
// Fallback Strategy (3-layer):
// 1. Model Call: Each provider/model call is bounded by a per-attempt timeout
// 2. Fast Fallback: Prefer the next configured provider/model over retrying the same one
// 3. Last Resort Retry: Retry the same model only when no alternative remains
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
	// ProviderOpenAI represents OpenAI-compatible endpoints (self-hosted, custom endpoint).
	ProviderOpenAI Provider = "openai"
)

// ProviderEndpoint defines the base URL for OpenAI-compatible providers.
// Gemini is not included as it uses a different SDK.
var ProviderEndpoint = map[Provider]string{
	ProviderGroq:     "https://api.groq.com/openai/v1/",
	ProviderCerebras: "https://api.cerebras.ai/v1/",
}

// IsOpenAICompatible returns true if the provider uses OpenAI-compatible API.
func (p Provider) IsOpenAICompatible() bool {
	if p == ProviderOpenAI {
		return true
	}
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
	// Model returns the configured model name for cooldown/routing decisions.
	Model() string
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
	// Model returns the configured model name for cooldown/routing decisions.
	Model() string
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
	// The fallback chain uses this only when no alternative model remains.
	// Default: 2 (1 initial + 1 last-resort retry)
	MaxAttempts int

	// InitialDelay is the base delay before first retry.
	// Default: 500ms
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	// Default: 3s
	MaxDelay time.Duration

	// AttemptTimeout bounds a single provider/model call.
	// Default: 5s
	AttemptTimeout time.Duration
}

// ProviderConfig holds configuration for a single LLM provider.
type ProviderConfig struct {
	// APIKey is the API key for the provider.
	APIKey string //nolint:gosec // G117: field name matches secret pattern but is not a secret itself

	// Endpoint is the custom base URL for OpenAI-compatible providers.
	// Only used by ProviderOpenAI; other providers use ProviderEndpoint map.
	Endpoint string

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
	// The factory interleaves providers by model rank, so each provider's first
	// model is tried before any provider's second model.
	// Default: ["gemini", "groq", "cerebras", "openai"] (only those with API keys)
	Providers []Provider

	// Gemini configuration
	Gemini ProviderConfig

	// Groq configuration (OpenAI-compatible)
	Groq ProviderConfig

	// Cerebras configuration (OpenAI-compatible)
	Cerebras ProviderConfig

	// OpenAI configuration (OpenAI-compatible, custom endpoint)
	OpenAI ProviderConfig

	// RetryConfig for retry behavior
	RetryConfig RetryConfig
}

// Default model configurations.
// First element is primary model, subsequent elements are fallbacks.
var (
	// DefaultGeminiIntentModels is the default model chain for Gemini intent parsing.
	// Keep the strongest and most consistently available Gemini Flash-Lite options first,
	// then fall back to the Gemma 4 family when needed.
	DefaultGeminiIntentModels = []string{"gemini-3.5-flash-lite", "gemini-3.1-flash-lite", "gemma-4-31b-it", "gemma-4-26b-a4b-it"}

	// DefaultGeminiExpanderModels is the default model chain for Gemini query expansion.
	DefaultGeminiExpanderModels = []string{"gemini-3.5-flash-lite", "gemini-3.1-flash-lite", "gemma-4-31b-it", "gemma-4-26b-a4b-it"}

	// DefaultGroqIntentModels is the default model chain for Groq intent parsing.
	// Keep the strongest supported model first for the same usage budget, and prefer the
	// more broadly available GPT-OSS stack ahead of the smaller 20B model.
	// Removed: deprecated Llama/Qwen variants and unsupported model IDs.
	DefaultGroqIntentModels = []string{"qwen/qwen3.6-27b", "openai/gpt-oss-120b", "openai/gpt-oss-20b"}

	// DefaultGroqExpanderModels is the default model chain for Groq query expansion.
	// Same model selection rationale as intent models.
	DefaultGroqExpanderModels = []string{"qwen/qwen3.6-27b", "openai/gpt-oss-120b", "openai/gpt-oss-20b"}

	// DefaultCerebrasIntentModels is the default model chain for Cerebras intent parsing.
	// Gemma 4 31B is the higher-quality default on Cerebras while GPT-OSS 120B remains
	// the stable fallback when the Gemma route is unavailable or rate-limited.
	DefaultCerebrasIntentModels = []string{"gemma-4-31b", "gpt-oss-120b"}

	// DefaultCerebrasExpanderModels is the default model chain for Cerebras query expansion.
	DefaultCerebrasExpanderModels = []string{"gemma-4-31b", "gpt-oss-120b"}

	// DefaultProviders is the default provider order for fallback.
	DefaultProviders = []Provider{ProviderGemini, ProviderGroq, ProviderCerebras, ProviderOpenAI}
)

// Retry configuration defaults
const (
	DefaultMaxRetryAttempts  = 2
	DefaultInitialRetryDelay = 500 * time.Millisecond
	DefaultMaxRetryDelay     = 3 * time.Second
	DefaultLLMAttemptTimeout = 5 * time.Second
)

// HasAnyProvider returns true if at least one provider is configured.
func (c *LLMConfig) HasAnyProvider() bool {
	return c.Gemini.APIKey != "" || c.Groq.APIKey != "" || c.Cerebras.APIKey != "" || (c.OpenAI.APIKey != "" && c.OpenAI.Endpoint != "")
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
	case ProviderOpenAI:
		return c.OpenAI.APIKey != "" && c.OpenAI.Endpoint != ""
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
	case ProviderOpenAI:
		return &c.OpenAI
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
