// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
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

// FallbackIntentParser wraps a list of IntentParsers.
// It implements multi-layer fallback:
// 1. Model retry with backoff (same model)
// 2. Sequential fallback through the provided list of models/providers
// 3. Graceful degradation (return error if all fail)
type FallbackIntentParser struct {
	parsers     []IntentParser
	retryConfig RetryConfig
}

// NewFallbackIntentParser creates a new fallback-enabled intent parser.
// It tries parsers in the order they are provided.
func NewFallbackIntentParser(cfg RetryConfig, parsers ...IntentParser) *FallbackIntentParser {
	// Filter out nil parsers
	activeParsers := make([]IntentParser, 0, len(parsers))
	for _, p := range parsers {
		if p != nil {
			activeParsers = append(activeParsers, p)
		}
	}
	return &FallbackIntentParser{
		parsers:     activeParsers,
		retryConfig: cfg,
	}
}

// Parse tries the parsers in order with retry, then falls back if needed.
func (f *FallbackIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if f == nil || len(f.parsers) == 0 {
		return nil, errors.New("intent parser not configured")
	}

	totalStart := time.Now()
	var lastErr error

	for i, parser := range f.parsers {
		start := time.Now()
		provider := parser.Provider()

		// Try current parser with retry
		result, err := f.parseWithRetry(ctx, parser, text)
		if err == nil {
			recordIntentSuccess(provider, start)
			// Only record provider-level fallback when the provider actually changes.
			// This avoids misleading metrics like Gemini→Gemini when falling back between
			// multiple models of the same provider.
			if i > 0 {
				prevProvider := f.parsers[i-1].Provider()
				if prevProvider != provider {
					recordFallback(prevProvider, provider, "nlu", time.Since(totalStart))
				}
			}
			return result, nil
		}

		lastErr = err
		action := ClassifyError(err)

		slog.WarnContext(ctx, "intent parser failed",
			"provider", provider,
			"index", i,
			"error", err,
			"action", action,
			"duration_ms", time.Since(start).Milliseconds())

		// If error is not recoverable or no more fallbacks, record error and stop
		if action == ActionFail || i == len(f.parsers)-1 {
			recordIntentError(provider, err)
			if i == len(f.parsers)-1 && len(f.parsers) > 1 {
				return nil, fmt.Errorf("all %d parsers failed, last error: %w", len(f.parsers), lastErr)
			}
			return nil, lastErr
		}

		// Falling back to next parser
		slog.InfoContext(ctx, "falling back to next intent parser",
			"from_index", i,
			"from_provider", provider,
			"to_index", i+1,
			"to_provider", f.parsers[i+1].Provider())
	}

	return nil, fmt.Errorf("all intent parsers failed: %w", lastErr)
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
			"backoff_ms", backoff.Milliseconds(),
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
	for _, p := range f.parsers {
		if p != nil && p.IsEnabled() {
			return true
		}
	}
	return false
}

// Provider returns the provider type of the current/first parser.
func (f *FallbackIntentParser) Provider() Provider {
	if f == nil || len(f.parsers) == 0 {
		return ""
	}
	return f.parsers[0].Provider()
}

// Close closes all parsers.
func (f *FallbackIntentParser) Close() error {
	if f == nil {
		return nil
	}

	var errs []error
	for _, p := range f.parsers {
		if p != nil {
			if err := p.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// FallbackQueryExpander wraps a list of QueryExpanders.
type FallbackQueryExpander struct {
	expanders   []QueryExpander
	retryConfig RetryConfig
}

// NewFallbackQueryExpander creates a new fallback-enabled query expander.
func NewFallbackQueryExpander(cfg RetryConfig, expanders ...QueryExpander) *FallbackQueryExpander {
	activeExpanders := make([]QueryExpander, 0, len(expanders))
	for _, e := range expanders {
		if e != nil {
			activeExpanders = append(activeExpanders, e)
		}
	}
	return &FallbackQueryExpander{
		expanders:   activeExpanders,
		retryConfig: cfg,
	}
}

// Expand tries the expanders in order with retry, then falls back if needed.
// On complete failure, returns the original query (graceful degradation).
func (f *FallbackQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if f == nil || len(f.expanders) == 0 {
		return query, nil // Graceful degradation
	}

	totalStart := time.Now()

	for i, expander := range f.expanders {
		start := time.Now()
		provider := expander.Provider()

		// Try current expander with retry
		result, err := f.expandWithRetry(ctx, expander, query)
		if err == nil {
			recordExpanderSuccess(provider, start)
			// Only record provider-level fallback when the provider actually changes
			if i > 0 {
				prevProvider := f.expanders[i-1].Provider()
				if prevProvider != provider {
					recordFallback(prevProvider, provider, "expander", time.Since(totalStart))
				}
			}
			return result, nil
		}

		// Check if we should fallback
		action := ClassifyError(err)
		slog.WarnContext(ctx, "query expander failed",
			"provider", provider,
			"index", i,
			"error", err,
			"action", action,
			"duration_ms", time.Since(start).Milliseconds())

		// If error is not recoverable or no more fallbacks, degrade gracefully
		if action == ActionFail || i == len(f.expanders)-1 {
			recordExpanderError(provider, err)
			// Graceful degradation: return original query
			return query, nil
		}

		// Falling back to next expander
		slog.InfoContext(ctx, "falling back to next query expander",
			"from_index", i,
			"from_provider", provider,
			"to_index", i+1,
			"to_provider", f.expanders[i+1].Provider())
	}

	return query, nil // Always return original query on failure
}

// expandWithRetry attempts expansion with retry logic.
func (f *FallbackQueryExpander) expandWithRetry(ctx context.Context, expander QueryExpander, query string) (string, error) {
	var lastErr error

	for attempt := range f.retryConfig.MaxAttempts {
		if ctx.Err() != nil {
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

		// Check if we have sufficient timeout budget for this backoff
		if !HasSufficientBudget(ctx, backoff) {
			return query, fmt.Errorf("timeout during retry: %w", lastErr)
		}

		slog.DebugContext(ctx, "retrying query expansion",
			"provider", expander.Provider(),
			"attempt", attempt+1,
			"backoff_ms", backoff.Milliseconds(),
			"error", err)

		select {
		case <-ctx.Done():
			return query, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return query, lastErr
}

// Provider returns the provider type of the current/first expander.
func (f *FallbackQueryExpander) Provider() Provider {
	if f == nil || len(f.expanders) == 0 {
		return ""
	}
	return f.expanders[0].Provider()
}

// Close closes all expanders.
func (f *FallbackQueryExpander) Close() error {
	if f == nil {
		return nil
	}

	var errs []error
	for _, e := range f.expanders {
		if e != nil {
			if err := e.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// Helper functions for metrics recording
// (keeping these as they were)

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
			return "rate_limit"
		case llmErr.StatusCode >= 500:
			return "server_error"
		case llmErr.StatusCode == http.StatusUnauthorized || llmErr.StatusCode == http.StatusForbidden:
			return "auth_error"
		case llmErr.StatusCode == http.StatusBadRequest:
			return "invalid_request"
		}
	}

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
