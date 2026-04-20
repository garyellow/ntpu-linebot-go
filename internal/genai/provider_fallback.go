// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains the fallback wrapper for cross-provider failover.
package genai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
	cooldowns   *modelCooldownStore
}

// NewFallbackIntentParser creates a new fallback-enabled intent parser.
// It tries parsers in the order they are provided.
func NewFallbackIntentParser(cfg RetryConfig, parsers ...IntentParser) *FallbackIntentParser {
	return newFallbackIntentParserWithCooldowns(cfg, globalModelCooldownStore, parsers...)
}

func newFallbackIntentParserWithCooldowns(cfg RetryConfig, cooldowns *modelCooldownStore, parsers ...IntentParser) *FallbackIntentParser {
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
		cooldowns:   cooldowns,
	}
}

// Parse tries the parsers in order with retry, then falls back if needed.
func (f *FallbackIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if f == nil || len(f.parsers) == 0 {
		return nil, errors.New("intent parser not configured")
	}

	var lastErr error

	for i, parser := range f.parsers {
		start := time.Now()
		provider := parser.Provider()
		model := parser.Model()

		if cooldown, cooling := f.cooldowns.Get(provider, model); cooling && f.hasAvailableIntentParser(i+1) {
			slog.DebugContext(ctx, "Skipping cooled-down intent parser",
				"provider", provider,
				"model", model,
				"cooldown_kind", cooldown.Kind,
				"cooldown_remaining_ms", cooldown.Remaining(time.Now()).Milliseconds())
			recordCooldownEvent(provider, cooldown.Kind, "skipped")
			continue
		}

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
					recordFallback(prevProvider, provider, "nlu")
				}
			}
			return result, nil
		}

		lastErr = err
		if cooldown, applied := applyCooldown(f.cooldowns, provider, model, err); applied {
			slog.InfoContext(ctx, "Applied model cooldown after intent parser rate limit",
				"provider", provider,
				"model", model,
				"cooldown_kind", cooldown.Kind,
				"cooldown_remaining_ms", cooldown.Remaining(time.Now()).Milliseconds())
			recordCooldownEvent(provider, cooldown.Kind, "applied")
		}
		action := ClassifyError(err)

		slog.WarnContext(ctx, "Intent parser failed",
			"provider", provider,
			"model", model,
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
		slog.DebugContext(ctx, "Falling back to next intent parser",
			"from_index", i,
			"from_provider", provider,
			"to_index", i+1,
			"to_provider", f.parsers[i+1].Provider())
	}

	// Defensive fallback: loop above is expected to return on all paths
	return nil, fmt.Errorf("all intent parsers failed: %w", lastErr)
}

func (f *FallbackIntentParser) hasAvailableIntentParser(start int) bool {
	if f == nil {
		return false
	}
	for _, parser := range f.parsers[start:] {
		if parser == nil {
			continue
		}
		if _, cooling := f.cooldowns.Get(parser.Provider(), parser.Model()); !cooling {
			return true
		}
	}
	return false
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

		slog.DebugContext(ctx, "Retrying intent parse",
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

// Model returns the model name of the current/first parser.
func (f *FallbackIntentParser) Model() string {
	if f == nil || len(f.parsers) == 0 {
		return ""
	}
	return f.parsers[0].Model()
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
	cooldowns   *modelCooldownStore
}

// NewFallbackQueryExpander creates a new fallback-enabled query expander.
func NewFallbackQueryExpander(cfg RetryConfig, expanders ...QueryExpander) *FallbackQueryExpander {
	return newFallbackQueryExpanderWithCooldowns(cfg, globalModelCooldownStore, expanders...)
}

func newFallbackQueryExpanderWithCooldowns(cfg RetryConfig, cooldowns *modelCooldownStore, expanders ...QueryExpander) *FallbackQueryExpander {
	activeExpanders := make([]QueryExpander, 0, len(expanders))
	for _, e := range expanders {
		if e != nil {
			activeExpanders = append(activeExpanders, e)
		}
	}
	return &FallbackQueryExpander{
		expanders:   activeExpanders,
		retryConfig: cfg,
		cooldowns:   cooldowns,
	}
}

// Expand tries the expanders in order with retry, then falls back if needed.
// On complete failure, returns an error so the caller can handle it explicitly.
func (f *FallbackQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if f == nil || len(f.expanders) == 0 {
		return query, nil // feature disabled, not a failure
	}

	for i, expander := range f.expanders {
		start := time.Now()
		provider := expander.Provider()
		model := expander.Model()

		if cooldown, cooling := f.cooldowns.Get(provider, model); cooling && f.hasAvailableQueryExpander(i+1) {
			slog.DebugContext(ctx, "Skipping cooled-down query expander",
				"provider", provider,
				"model", model,
				"cooldown_kind", cooldown.Kind,
				"cooldown_remaining_ms", cooldown.Remaining(time.Now()).Milliseconds())
			recordCooldownEvent(provider, cooldown.Kind, "skipped")
			continue
		}

		// Try current expander with retry
		result, err := f.expandWithRetry(ctx, expander, query)
		if err == nil {
			recordExpanderSuccess(provider, start)
			// Only record provider-level fallback when the provider actually changes
			if i > 0 {
				prevProvider := f.expanders[i-1].Provider()
				if prevProvider != provider {
					recordFallback(prevProvider, provider, "expander")
				}
			}
			return result, nil
		}

		// Check if we should fallback
		if cooldown, applied := applyCooldown(f.cooldowns, provider, model, err); applied {
			slog.InfoContext(ctx, "Applied model cooldown after query expander rate limit",
				"provider", provider,
				"model", model,
				"cooldown_kind", cooldown.Kind,
				"cooldown_remaining_ms", cooldown.Remaining(time.Now()).Milliseconds())
			recordCooldownEvent(provider, cooldown.Kind, "applied")
		}
		action := ClassifyError(err)
		slog.WarnContext(ctx, "Query expander failed",
			"provider", provider,
			"model", model,
			"index", i,
			"error", err,
			"action", action,
			"duration_ms", time.Since(start).Milliseconds())

		// If error is not recoverable or no more fallbacks, propagate error to caller
		if action == ActionFail || i == len(f.expanders)-1 {
			recordExpanderError(provider, err)
			return query, err
		}

		// Falling back to next expander
		slog.DebugContext(ctx, "Falling back to next query expander",
			"from_index", i,
			"from_provider", provider,
			"to_index", i+1,
			"to_provider", f.expanders[i+1].Provider())
	}

	// unreachable: loop always returns via ActionFail or last-expander check above
	return query, fmt.Errorf("all %d query expanders failed", len(f.expanders))
}

func (f *FallbackQueryExpander) hasAvailableQueryExpander(start int) bool {
	if f == nil {
		return false
	}
	for _, expander := range f.expanders[start:] {
		if expander == nil {
			continue
		}
		if _, cooling := f.cooldowns.Get(expander.Provider(), expander.Model()); !cooling {
			return true
		}
	}
	return false
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

		slog.DebugContext(ctx, "Retrying query expansion",
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

// Model returns the model name of the current/first expander.
func (f *FallbackQueryExpander) Model() string {
	if f == nil || len(f.expanders) == 0 {
		return ""
	}
	return f.expanders[0].Model()
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

func recordCooldownEvent(provider Provider, kind RateLimitKind, action string) {
	if metrics.LLMCooldownTotal == nil {
		return
	}
	metrics.LLMCooldownTotal.WithLabelValues(string(provider), string(kind), action).Inc()
}

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

func recordFallback(fromProvider, toProvider Provider, operation string) {
	if metrics.LLMFallbackTotal == nil {
		return
	}
	metrics.LLMFallbackTotal.WithLabelValues(
		string(fromProvider),
		string(toProvider),
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
		case llmErr.StatusCode == http.StatusBadRequest:
			return "invalid_request"
		}
	}

	action := ClassifyError(err)
	switch action {
	case ActionFallback:
		// Distinguish the three reasons that all produce ActionFallback.
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
