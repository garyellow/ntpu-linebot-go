// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains shared types, interfaces, and configuration for NLU intent parsing
// and query expansion with multi-provider fallback support.
package genai

import (
	"context"
	"time"
)

// Provider represents an LLM provider.
type Provider string

const (
	// ProviderGemini represents Google's Gemini API.
	ProviderGemini Provider = "gemini"
	// ProviderGroq represents Groq's API (fast inference).
	ProviderGroq Provider = "groq"
)

// String returns the string representation of the provider.
func (p Provider) String() string {
	return string(p)
}

// IntentParser defines the interface for NLU intent parsing.
// Implementations include Gemini and Groq providers.
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
// Implementations include Gemini and Groq providers.
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
	// Module is the target module (course, id, contact, help, direct_reply)
	Module string

	// Intent is the specific intent within the module
	Intent string

	// Params contains the extracted parameters
	Params map[string]string

	// FunctionName is the raw function name from the model (for debugging)
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

	// IntentModel is the model name for intent parsing.
	IntentModel string

	// IntentFallbackModel is the fallback model for intent parsing.
	IntentFallbackModel string

	// ExpanderModel is the model name for query expansion.
	ExpanderModel string

	// ExpanderFallbackModel is the fallback model for query expansion.
	ExpanderFallbackModel string
}

// LLMConfig holds configuration for all LLM providers.
type LLMConfig struct {
	// PrimaryProvider is the first provider to try.
	// Default: "gemini" if GEMINI_API_KEY is set, otherwise "groq"
	PrimaryProvider Provider

	// FallbackProvider is the provider to try if primary fails.
	// Default: "groq" if primary is "gemini", otherwise "gemini"
	FallbackProvider Provider

	// Gemini configuration
	Gemini ProviderConfig

	// Groq configuration
	Groq ProviderConfig

	// RetryConfig for retry behavior
	RetryConfig RetryConfig
}

// Default model constants
const (
	// DefaultGeminiIntentModel is the default model for Gemini intent parsing.
	// gemini-2.5-flash offers excellent function calling with fast inference.
	DefaultGeminiIntentModel = "gemini-2.5-flash"
	// DefaultGeminiIntentFallbackModel is the fallback model for Gemini intent parsing.
	// gemini-2.5-flash-lite provides faster, cost-efficient function calling.
	DefaultGeminiIntentFallbackModel = "gemini-2.5-flash-lite"
	// DefaultGeminiExpanderModel is the default model for Gemini query expansion.
	DefaultGeminiExpanderModel = "gemini-2.5-flash"
	// DefaultGeminiExpanderFallbackModel is the fallback model for Gemini query expansion.
	// gemini-2.5-flash-lite provides faster, cost-efficient text generation.
	DefaultGeminiExpanderFallbackModel = "gemini-2.5-flash-lite"

	// DefaultGroqIntentModel is the default model for Groq intent parsing.
	// Llama 4 Maverick (Preview) offers excellent function calling and intent classification with fast inference (~900 TPS).
	DefaultGroqIntentModel = "meta-llama/llama-4-maverick-17b-128e-instruct"
	// DefaultGroqIntentFallbackModel is the fallback model for Groq intent parsing.
	// llama-3.3-70b-versatile is Production-grade and provides reliable fallback with strong accuracy.
	DefaultGroqIntentFallbackModel = "llama-3.3-70b-versatile"
	// DefaultGroqExpanderModel is the default model for Groq query expansion.
	// Llama 4 Scout (Preview) offers efficient query expansion with fast inference (~750 TPS).
	DefaultGroqExpanderModel = "meta-llama/llama-4-scout-17b-16e-instruct"
	// DefaultGroqExpanderFallbackModel is the fallback model for Groq query expansion.
	// llama-3.1-8b-instant is Production-grade and provides fast, cost-efficient fallback.
	DefaultGroqExpanderFallbackModel = "llama-3.1-8b-instant"

	// Retry configuration defaults
	DefaultMaxRetryAttempts  = 2
	DefaultInitialRetryDelay = 500 * time.Millisecond
	DefaultMaxRetryDelay     = 3 * time.Second
)

// HasAnyProvider returns true if at least one provider is configured.
func (c *LLMConfig) HasAnyProvider() bool {
	return c.Gemini.APIKey != "" || c.Groq.APIKey != ""
}

// HasProvider returns true if the specified provider is configured.
func (c *LLMConfig) HasProvider(p Provider) bool {
	switch p {
	case ProviderGemini:
		return c.Gemini.APIKey != ""
	case ProviderGroq:
		return c.Groq.APIKey != ""
	default:
		return false
	}
}

// GetFallbackProvider returns the fallback provider (the other one).
func (c *LLMConfig) GetFallbackProvider() Provider {
	switch c.PrimaryProvider {
	case ProviderGemini:
		if c.Groq.APIKey != "" {
			return ProviderGroq
		}
	case ProviderGroq:
		if c.Gemini.APIKey != "" {
			return ProviderGemini
		}
	}
	return ""
}
