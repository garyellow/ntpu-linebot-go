package genai

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	operationNLU      = "nlu"
	operationExpander = "expander"
)

type llmEndpoint interface {
	Provider() Provider
	Model() string
	Close() error
}

type chainStep[T any] struct {
	endpoint llmEndpoint
	call     func(context.Context, string) (T, error)
}

type chainPolicy struct {
	operation      string
	retryConfig    RetryConfig
	attemptTimeout time.Duration
}

type fallbackChain[T any] struct {
	steps     []chainStep[T]
	policy    chainPolicy
	cooldowns *modelCooldownStore
}

func newFallbackChain[T any](policy chainPolicy, cooldowns *modelCooldownStore, steps []chainStep[T]) *fallbackChain[T] {
	activeSteps := make([]chainStep[T], 0, len(steps))
	for _, step := range steps {
		if step.endpoint != nil && step.call != nil {
			activeSteps = append(activeSteps, step)
		}
	}

	policy.retryConfig = normalizeRetryConfig(policy.retryConfig)
	if policy.attemptTimeout <= 0 {
		policy.attemptTimeout = DefaultLLMAttemptTimeout
	}
	if cooldowns == nil {
		cooldowns = globalModelCooldownStore
	}

	return &fallbackChain[T]{
		steps:     activeSteps,
		policy:    policy,
		cooldowns: cooldowns,
	}
}

func normalizeRetryConfig(cfg RetryConfig) RetryConfig {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.InitialDelay < 0 {
		cfg.InitialDelay = 0
	}
	if cfg.MaxDelay < 0 {
		cfg.MaxDelay = 0
	}
	if cfg.MaxDelay > 0 && cfg.InitialDelay > cfg.MaxDelay {
		cfg.InitialDelay = cfg.MaxDelay
	}
	return cfg
}

func (c *fallbackChain[T]) run(ctx context.Context, input string) (T, error) {
	var zero T
	if c == nil || len(c.steps) == 0 {
		return zero, fmt.Errorf("%s chain not configured", c.operation())
	}

	var lastErr error

	for i, step := range c.steps {
		provider := step.endpoint.Provider()
		model := step.endpoint.Model()
		if cooldown, cooling := c.cooldowns.Get(provider, model); cooling && c.hasAvailableStep(i+1) {
			slog.DebugContext(ctx, "Skipping cooled-down LLM model",
				"operation", c.operation(),
				"provider", provider,
				"model", model,
				"cooldown_kind", cooldown.Kind,
				"cooldown_remaining_ms", cooldown.Remaining(time.Now()).Milliseconds())
			recordCooldownEvent(provider, model, cooldown.Kind, "skipped")
			continue
		}

		start := time.Now()
		result, err := c.runStep(ctx, i, step, input)
		if err == nil {
			recordLLMSuccess(provider, model, c.operation(), start)
			return result, nil
		}

		lastErr = err
		recordLLMError(provider, model, c.operation(), err, start)
		if cooldown, applied := applyCooldown(c.cooldowns, provider, model, err); applied {
			slog.InfoContext(ctx, "Applied model cooldown after LLM failure",
				"operation", c.operation(),
				"provider", provider,
				"model", model,
				"cooldown_kind", cooldown.Kind,
				"cooldown_remaining_ms", cooldown.Remaining(time.Now()).Milliseconds())
			recordCooldownEvent(provider, model, cooldown.Kind, "applied")
		}

		action := ClassifyError(err)
		slog.WarnContext(ctx, "LLM model failed",
			"operation", c.operation(),
			"provider", provider,
			"model", model,
			"index", i,
			"error", err,
			"action", action,
			"duration_ms", time.Since(start).Milliseconds())

		if action == ActionFail || i == len(c.steps)-1 || ctx.Err() != nil {
			if i == len(c.steps)-1 && len(c.steps) > 1 {
				return zero, fmt.Errorf("all %d %s models failed, last error: %w", len(c.steps), c.operation(), lastErr)
			}
			return zero, lastErr
		}

		next := c.steps[i+1].endpoint
		recordFallback(provider, model, next.Provider(), next.Model(), c.operation())
		slog.DebugContext(ctx, "Falling back to next LLM model",
			"operation", c.operation(),
			"from_index", i,
			"from_provider", provider,
			"from_model", model,
			"to_index", i+1,
			"to_provider", next.Provider(),
			"to_model", next.Model())
	}

	return zero, fmt.Errorf("all %s models failed: %w", c.operation(), lastErr)
}

func (c *fallbackChain[T]) runStep(ctx context.Context, index int, step chainStep[T], input string) (T, error) {
	var zero T
	var lastErr error
	maxAttempts := c.policy.retryConfig.MaxAttempts
	if c.hasAvailableStep(index + 1) {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		attemptCtx, cancel := context.WithTimeout(ctx, c.policy.attemptTimeout)
		result, err := step.call(attemptCtx, input)
		cancel()
		if err == nil {
			return result, nil
		}

		lastErr = err
		if ClassifyError(err) != ActionRetry {
			return zero, err
		}
		if attempt == maxAttempts-1 {
			break
		}

		backoff := CalculateBackoff(attempt+1, c.policy.retryConfig.InitialDelay, c.policy.retryConfig.MaxDelay)
		requiredBudget := backoff + c.policy.attemptTimeout
		if !HasSufficientBudget(ctx, requiredBudget) {
			return zero, fmt.Errorf("timeout during retry: %w", lastErr)
		}

		slog.DebugContext(ctx, "Retrying LLM model",
			"operation", c.operation(),
			"provider", step.endpoint.Provider(),
			"model", step.endpoint.Model(),
			"attempt", attempt+1,
			"backoff_ms", backoff.Milliseconds(),
			"error", err)

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return zero, lastErr
}

func (c *fallbackChain[T]) hasAvailableStep(start int) bool {
	if c == nil {
		return false
	}
	for _, step := range c.steps[start:] {
		if step.endpoint == nil {
			continue
		}
		if _, cooling := c.cooldowns.Get(step.endpoint.Provider(), step.endpoint.Model()); !cooling {
			return true
		}
	}
	return false
}

func (c *fallbackChain[T]) isConfigured() bool {
	return c != nil && len(c.steps) > 0
}

func (c *fallbackChain[T]) provider() Provider {
	if !c.isConfigured() {
		return ""
	}
	return c.steps[0].endpoint.Provider()
}

func (c *fallbackChain[T]) model() string {
	if !c.isConfigured() {
		return ""
	}
	return c.steps[0].endpoint.Model()
}

func (c *fallbackChain[T]) close() error {
	if c == nil {
		return nil
	}

	var errs []error
	for _, step := range c.steps {
		if step.endpoint != nil {
			if err := step.endpoint.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

func (c *fallbackChain[T]) operation() string {
	if c == nil || c.policy.operation == "" {
		return "llm"
	}
	return c.policy.operation
}
