// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains the fallback wrapper for cross-provider failover.
package genai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
)

// FallbackIntentParser wraps a primary and fallback IntentParser.
// It implements three-layer fallback:
// 1. Model retry with backoff (same provider)
// 2. Provider fallback (primary → fallback provider)
// 3. Graceful degradation (return nil result)
type FallbackIntentParser struct {
	primary     IntentParser
	fallback    IntentParser
	retryConfig RetryConfig
}

// NewFallbackIntentParser creates a new fallback-enabled intent parser.
// If fallback is nil, only retry logic is applied to the primary provider.
func NewFallbackIntentParser(primary, fallback IntentParser, cfg RetryConfig) *FallbackIntentParser {
	return &FallbackIntentParser{
		primary:     primary,
		fallback:    fallback,
		retryConfig: cfg,
	}
}

// Parse tries the primary parser first with retry, then falls back if needed.
func (f *FallbackIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if f == nil || f.primary == nil {
		return nil, errors.New("intent parser not configured")
	}

	start := time.Now()
	provider := f.primary.Provider()

	// Try primary with retry
	result, err := f.parseWithRetry(ctx, f.primary, text)
	if err == nil {
		recordIntentSuccess(provider, start)
		return result, nil
	}

	// Check if we should fallback
	action := ClassifyError(err)
	slog.WarnContext(ctx, "primary intent parser failed",
		"provider", provider,
		"error", err,
		"action", action,
		"duration", time.Since(start))

	// If error is not recoverable or no fallback, return error
	if action == ActionFail || f.fallback == nil {
		recordIntentError(provider, err)
		return nil, err
	}

	// Try fallback provider
	slog.InfoContext(ctx, "falling back to secondary provider",
		"from", provider,
		"to", f.fallback.Provider())

	fallbackStart := time.Now()
	fallbackProvider := f.fallback.Provider()

	result, err = f.parseWithRetry(ctx, f.fallback, text)
	if err == nil {
		recordIntentSuccess(fallbackProvider, fallbackStart)
		recordFallback(provider, fallbackProvider, "nlu", time.Since(start))
		return result, nil
	}

	// Both providers failed
	recordIntentError(fallbackProvider, err)
	slog.ErrorContext(ctx, "all intent parsers failed",
		"primary", provider,
		"fallback", fallbackProvider,
		"error", err)

	return nil, fmt.Errorf("all providers failed: %w", err)
}

// parseWithRetry attempts parsing with retry logic.
func (f *FallbackIntentParser) parseWithRetry(ctx context.Context, parser IntentParser, text string) (*ParseResult, error) {
	var lastErr error

	for attempt := range f.retryConfig.MaxAttempts {
		// Check context before attempting
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		result, err := parser.Parse(ctx, text)
		if err == nil {
			return result, nil
		}

		lastErr = err
		action := ClassifyError(err)

		// Don't retry if error is not retryable
		if action != ActionRetry {
			return nil, err
		}

		// Last attempt, don't sleep
		if attempt == f.retryConfig.MaxAttempts-1 {
			break
		}

		// Calculate backoff with jitter
		backoff := CalculateBackoff(attempt+1, f.retryConfig.InitialDelay, f.retryConfig.MaxDelay)

		// Check remaining time budget with actual backoff
		if !HasSufficientBudget(ctx, backoff) {
			// Insufficient time remaining, return last error
			return nil, fmt.Errorf("timeout during retry: %w", lastErr)
		}

		slog.DebugContext(ctx, "retrying intent parse",
			"provider", parser.Provider(),
			"attempt", attempt+1,
			"backoff", backoff,
			"error", err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, lastErr
}

// IsEnabled returns true if at least one parser is enabled.
func (f *FallbackIntentParser) IsEnabled() bool {
	if f == nil {
		return false
	}
	return (f.primary != nil && f.primary.IsEnabled()) ||
		(f.fallback != nil && f.fallback.IsEnabled())
}

// Provider returns the primary provider type.
func (f *FallbackIntentParser) Provider() Provider {
	if f == nil || f.primary == nil {
		return ""
	}
	return f.primary.Provider()
}

// Close closes both parsers.
func (f *FallbackIntentParser) Close() error {
	if f == nil {
		return nil
	}

	var errs []error
	if f.primary != nil {
		if err := f.primary.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.fallback != nil {
		if err := f.fallback.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// FallbackQueryExpander wraps a primary and fallback QueryExpander.
type FallbackQueryExpander struct {
	primary     QueryExpander
	fallback    QueryExpander
	retryConfig RetryConfig
}

// NewFallbackQueryExpander creates a new fallback-enabled query expander.
func NewFallbackQueryExpander(primary, fallback QueryExpander, cfg RetryConfig) *FallbackQueryExpander {
	return &FallbackQueryExpander{
		primary:     primary,
		fallback:    fallback,
		retryConfig: cfg,
	}
}

// Expand tries the primary expander first with retry, then falls back if needed.
// On complete failure, returns the original query (graceful degradation).
func (f *FallbackQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if f == nil || f.primary == nil {
		return query, nil // Graceful degradation
	}

	start := time.Now()
	provider := f.primary.Provider()

	// Try primary with retry
	result, err := f.expandWithRetry(ctx, f.primary, query)
	if err == nil {
		recordExpanderSuccess(provider, start)
		return result, nil
	}

	// Check if we should fallback
	action := ClassifyError(err)
	slog.WarnContext(ctx, "primary query expander failed",
		"provider", provider,
		"error", err,
		"action", action,
		"duration", time.Since(start))

	// If error is not recoverable or no fallback, degrade gracefully
	if action == ActionFail || f.fallback == nil {
		recordExpanderError(provider, err)
		// Graceful degradation: return original query
		return query, nil
	}

	// Try fallback provider
	slog.InfoContext(ctx, "falling back to secondary expander",
		"from", provider,
		"to", f.fallback.Provider())

	fallbackStart := time.Now()
	fallbackProvider := f.fallback.Provider()

	result, err = f.expandWithRetry(ctx, f.fallback, query)
	if err == nil {
		recordExpanderSuccess(fallbackProvider, fallbackStart)
		recordFallback(provider, fallbackProvider, "expander", time.Since(start))
		return result, nil
	}

	// Both providers failed - graceful degradation
	recordExpanderError(fallbackProvider, err)
	slog.WarnContext(ctx, "all expanders failed, using original query",
		"primary", provider,
		"fallback", fallbackProvider,
		"query", query)

	return query, nil // Always return original query on failure
}

// expandWithRetry attempts expansion with retry logic.
func (f *FallbackQueryExpander) expandWithRetry(ctx context.Context, expander QueryExpander, query string) (string, error) {
	var lastErr error

	for attempt := range f.retryConfig.MaxAttempts {
		if ctx.Err() != nil {
			return query, ctx.Err()
		}

		if !HasSufficientBudget(ctx, f.retryConfig.InitialDelay) {
			if lastErr != nil {
				return query, lastErr
			}
			return query, ctx.Err()
		}

		result, err := expander.Expand(ctx, query)
		if err == nil {
			return result, nil
		}

		lastErr = err
		action := ClassifyError(err)

		if action != ActionRetry {
			return query, err
		}

		if attempt == f.retryConfig.MaxAttempts-1 {
			break
		}

		backoff := CalculateBackoff(attempt+1, f.retryConfig.InitialDelay, f.retryConfig.MaxDelay)

		slog.DebugContext(ctx, "retrying query expansion",
			"provider", expander.Provider(),
			"attempt", attempt+1,
			"backoff", backoff,
			"error", err)

		select {
		case <-ctx.Done():
			return query, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return query, lastErr
}

// Provider returns the primary provider type.
func (f *FallbackQueryExpander) Provider() Provider {
	if f == nil || f.primary == nil {
		return ""
	}
	return f.primary.Provider()
}

// Close closes both expanders.
func (f *FallbackQueryExpander) Close() error {
	if f == nil {
		return nil
	}

	var errs []error
	if f.primary != nil {
		if err := f.primary.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if f.fallback != nil {
		if err := f.fallback.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// Helper functions for metrics recording

func recordIntentSuccess(provider Provider, start time.Time) {
	if metrics.LLMTotal == nil || metrics.LLMDuration == nil {
		return
	}
	metrics.LLMTotal.WithLabelValues(string(provider), "nlu", "success").Inc()
	metrics.LLMDuration.WithLabelValues(string(provider), "nlu").Observe(time.Since(start).Seconds())
}

func recordIntentError(provider Provider, err error) {
	if metrics.LLMTotal == nil {
		return
	}
	errType := classifyErrorType(err)
	metrics.LLMTotal.WithLabelValues(string(provider), "nlu", errType).Inc()
}

func recordExpanderSuccess(provider Provider, start time.Time) {
	if metrics.LLMTotal == nil || metrics.LLMDuration == nil {
		return
	}
	metrics.LLMTotal.WithLabelValues(string(provider), "expander", "success").Inc()
	metrics.LLMDuration.WithLabelValues(string(provider), "expander").Observe(time.Since(start).Seconds())
}

func recordExpanderError(provider Provider, err error) {
	if metrics.LLMTotal == nil {
		return
	}
	errType := classifyErrorType(err)
	metrics.LLMTotal.WithLabelValues(string(provider), "expander", errType).Inc()
}

func recordFallback(fromProvider, toProvider Provider, operation string, totalDuration time.Duration) {
	if metrics.LLMFallbackTotal == nil {
		return
	}
	metrics.LLMFallbackTotal.WithLabelValues(
		string(fromProvider),
		string(toProvider),
		operation,
	).Inc()

	// Record additional latency introduced by fallback
	if metrics.LLMFallbackLatency != nil {
		metrics.LLMFallbackLatency.WithLabelValues(
			string(fromProvider),
			string(toProvider),
			operation,
		).Observe(totalDuration.Seconds())
	}
}

// classifyErrorType maps error to a metric status label.
// Provides fine-grained error classification for better observability.
func classifyErrorType(err error) string {
	if err == nil {
		return "success"
	}

	// Check for context errors first
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	// Check for wrapped LLMError with status code
	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		switch {
		case llmErr.StatusCode == http.StatusTooManyRequests:
			return "rate_limit"
		case llmErr.StatusCode >= 500:
			return "server_error"
		case llmErr.StatusCode == http.StatusUnauthorized || llmErr.StatusCode == http.StatusForbidden:
			return "auth_error"
		case llmErr.StatusCode == http.StatusBadRequest:
			return "invalid_request"
		}
	}

	// Fall back to action-based classification
	action := ClassifyError(err)
	switch action {
	case ActionFallback:
		return "quota_exhausted"
	case ActionRetry:
		return "transient_error"
	default:
		return "error"
	}
}
