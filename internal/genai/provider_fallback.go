// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains public fallback wrappers for intent parsing and query expansion.
package genai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// FallbackIntentParser routes NLU parsing across the configured LLM model chain.
type FallbackIntentParser struct {
	chain *fallbackChain[*ParseResult]
}

// NewFallbackIntentParser creates a fallback-enabled intent parser.
func NewFallbackIntentParser(cfg RetryConfig, parsers ...IntentParser) *FallbackIntentParser {
	return newFallbackIntentParserWithCooldowns(cfg, globalModelCooldownStore, parsers...)
}

func newFallbackIntentParserWithCooldowns(cfg RetryConfig, cooldowns *modelCooldownStore, parsers ...IntentParser) *FallbackIntentParser {
	steps := make([]chainStep[*ParseResult], 0, len(parsers))
	for _, parser := range parsers {
		if parser == nil {
			continue
		}
		p := parser
		steps = append(steps, chainStep[*ParseResult]{
			endpoint: p,
			call:     p.Parse,
		})
	}

	return &FallbackIntentParser{
		chain: newFallbackChain(chainPolicy{
			operation:      operationNLU,
			retryConfig:    cfg,
			attemptTimeout: cfg.AttemptTimeout,
		}, cooldowns, steps),
	}
}

// Parse analyzes text and returns the first successful parsed intent.
func (f *FallbackIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if f == nil || !f.chain.isConfigured() {
		return nil, errors.New("intent parser not configured")
	}
	return f.chain.run(ctx, text)
}

// IsEnabled returns true if at least one parser is enabled.
func (f *FallbackIntentParser) IsEnabled() bool {
	if f == nil || !f.chain.isConfigured() {
		return false
	}
	for _, step := range f.chain.steps {
		parser, ok := step.endpoint.(IntentParser)
		if ok && parser.IsEnabled() {
			return true
		}
	}
	return false
}

// Provider returns the provider type of the first configured model.
func (f *FallbackIntentParser) Provider() Provider {
	if f == nil {
		return ""
	}
	return f.chain.provider()
}

// Model returns the model name of the first configured model.
func (f *FallbackIntentParser) Model() string {
	if f == nil {
		return ""
	}
	return f.chain.model()
}

// Close closes all parser clients.
func (f *FallbackIntentParser) Close() error {
	if f == nil {
		return nil
	}
	return f.chain.close()
}

// FallbackQueryExpander routes query expansion across the configured LLM model chain.
type FallbackQueryExpander struct {
	chain *fallbackChain[string]
}

// NewFallbackQueryExpander creates a fallback-enabled query expander.
func NewFallbackQueryExpander(cfg RetryConfig, expanders ...QueryExpander) *FallbackQueryExpander {
	return newFallbackQueryExpanderWithCooldowns(cfg, globalModelCooldownStore, expanders...)
}

func newFallbackQueryExpanderWithCooldowns(cfg RetryConfig, cooldowns *modelCooldownStore, expanders ...QueryExpander) *FallbackQueryExpander {
	steps := make([]chainStep[string], 0, len(expanders))
	for _, expander := range expanders {
		if expander == nil {
			continue
		}
		e := expander
		steps = append(steps, chainStep[string]{
			endpoint: e,
			call:     e.Expand,
		})
	}

	return &FallbackQueryExpander{
		chain: newFallbackChain(chainPolicy{
			operation:      operationExpander,
			retryConfig:    cfg,
			attemptTimeout: cfg.AttemptTimeout,
		}, cooldowns, steps),
	}
}

// Expand expands a query. If the feature is disabled, the original query is returned.
func (f *FallbackQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if f == nil || !f.chain.isConfigured() {
		return query, nil
	}
	result, err := f.chain.run(ctx, query)
	if err != nil {
		return query, err
	}
	return result, nil
}

// Provider returns the provider type of the first configured model.
func (f *FallbackQueryExpander) Provider() Provider {
	if f == nil {
		return ""
	}
	return f.chain.provider()
}

// Model returns the model name of the first configured model.
func (f *FallbackQueryExpander) Model() string {
	if f == nil {
		return ""
	}
	return f.chain.model()
}

// Close closes all expander clients.
func (f *FallbackQueryExpander) Close() error {
	if f == nil {
		return nil
	}
	return f.chain.close()
}

func recordCooldownEvent(provider Provider, model string, kind RateLimitKind, action string) {
	if metrics.LLMCooldownTotal == nil {
		return
	}
	metrics.LLMCooldownTotal.WithLabelValues(string(provider), model, string(kind), action).Inc()
}

func recordLLMSuccess(provider Provider, model, operation string, start time.Time) {
	if metrics.LLMTotal == nil || metrics.LLMDuration == nil {
		return
	}
	metrics.LLMTotal.WithLabelValues(string(provider), model, operation, "success").Inc()
	metrics.LLMDuration.WithLabelValues(string(provider), model, operation).Observe(time.Since(start).Seconds())
}

func recordLLMError(provider Provider, model, operation string, err error, start time.Time) {
	if metrics.LLMTotal == nil || metrics.LLMDuration == nil {
		return
	}
	metrics.LLMTotal.WithLabelValues(string(provider), model, operation, classifyErrorType(err)).Inc()
	metrics.LLMDuration.WithLabelValues(string(provider), model, operation).Observe(time.Since(start).Seconds())
}

func recordFallback(fromProvider Provider, fromModel string, toProvider Provider, toModel, operation string) {
	if metrics.LLMFallbackTotal == nil {
		return
	}
	metrics.LLMFallbackTotal.WithLabelValues(
		string(fromProvider),
		fromModel,
		string(toProvider),
		toModel,
		operation,
	).Inc()
}

func classifyErrorType(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		switch {
		case llmErr.StatusCode == http.StatusTooManyRequests:
			if classifyRateLimitKind(err) == RateLimitExhausted {
				return "quota_exhausted"
			}
			return "rate_limit"
		case llmErr.StatusCode >= 500:
			return "server_error"
		case llmErr.StatusCode == http.StatusUnauthorized || llmErr.StatusCode == http.StatusForbidden:
			return "auth_error"
		case llmErr.StatusCode == http.StatusNotFound:
			return "model_not_found"
		case llmErr.StatusCode == http.StatusBadRequest:
			return "invalid_request"
		}
	}

	action := ClassifyError(err)
	switch action {
	case ActionFallback:
		errStr := strings.ToLower(err.Error())
		if containsAny(errStr, "401", "unauthorized", "unauthenticated", "invalid api key", "invalid_api_key",
			"403", "forbidden", "permission denied") {
			return "auth_error"
		}
		if containsAny(errStr, "404", "not found") {
			return "model_not_found"
		}
		return "quota_exhausted"
	case ActionRetry:
		return "transient_error"
	default:
		return "error"
	}
}
