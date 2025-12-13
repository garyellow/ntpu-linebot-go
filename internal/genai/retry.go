// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains retry logic with exponential backoff and jitter.
package genai

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"time"
)

// CalculateBackoff calculates the delay before the next retry attempt.
// Uses AWS-recommended Full Jitter algorithm:
//
//	delay = random(0, min(maxDelay, initialDelay * 2^attempt))
//
// Full Jitter provides:
//   - Lower contention than Equal Jitter or Exponential Backoff
//   - Faster completion time under high load
//   - Better distribution of retry attempts
//
// Reference: https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/
func CalculateBackoff(attempt int, initial, max time.Duration) time.Duration {
	if attempt <= 0 {
		return 0 // No delay on first attempt
	}

	// Calculate exponential delay: initial * 2^(attempt-1)
	exp := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(initial) * exp)

	// Cap at maximum
	if delay > max {
		delay = max
	}

	// Apply Full Jitter: random(0, delay)
	if delay <= 0 {
		return 0
	}

	// Use crypto/rand for uniform distribution without bias
	maxNs := big.NewInt(int64(delay))
	jitterBig, err := rand.Int(rand.Reader, maxNs)
	if err != nil {
		// Fallback to half delay on crypto failure (extremely rare)
		return delay / 2
	}

	return time.Duration(jitterBig.Int64())
}

// Sleep waits for the specified duration, respecting context cancellation.
// Returns ctx.Err() if context is cancelled during sleep.
func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WithRetry executes a function with retry logic using exponential backoff.
// The function is retried on transient errors up to cfg.MaxAttempts times.
//
// Parameters:
//   - ctx: Context for cancellation and deadline
//   - cfg: Retry configuration
//   - fn: Function to execute (should return nil on success)
//
// Returns the last error if all attempts fail, or nil on success.
func WithRetry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Check context before attempting
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success
		}
		lastErr = err

		// Check if error is permanent (don't retry)
		if IsPermanent(err) {
			return err
		}

		// Don't sleep after the last attempt
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		// Calculate backoff delay
		delay := CalculateBackoff(attempt+1, cfg.InitialDelay, cfg.MaxDelay)

		// Sleep with context cancellation support
		if err := Sleep(ctx, delay); err != nil {
			return err // Context cancelled
		}
	}

	return lastErr
}

// WithRetryAndMetrics executes a function with retry logic and records metrics.
// This is a convenience wrapper that tracks retry attempts.
//
// Parameters:
//   - ctx: Context for cancellation and deadline
//   - cfg: Retry configuration
//   - onRetry: Callback called before each retry (for metrics/logging)
//   - fn: Function to execute (should return nil on success)
func WithRetryAndMetrics(ctx context.Context, cfg RetryConfig, onRetry func(attempt int, err error), fn func() error) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Check context before attempting
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success
		}
		lastErr = err

		// Check if error is permanent (don't retry)
		if IsPermanent(err) {
			return err
		}

		// Don't sleep after the last attempt
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		// Record retry attempt
		if onRetry != nil {
			onRetry(attempt+1, err)
		}

		// Calculate backoff delay
		delay := CalculateBackoff(attempt+1, cfg.InitialDelay, cfg.MaxDelay)

		// Sleep with context cancellation support
		if err := Sleep(ctx, delay); err != nil {
			return err // Context cancelled
		}
	}

	return lastErr
}

// RemainingBudget calculates how much time is left in the context deadline.
// Returns 0 if no deadline is set, or negative if deadline has passed.
func RemainingBudget(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0 // No deadline
	}
	return time.Until(deadline)
}

// HasSufficientBudget checks if there's enough time remaining for an operation.
// This helps prevent starting operations that are likely to timeout.
func HasSufficientBudget(ctx context.Context, required time.Duration) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true // No deadline means unlimited budget
	}
	return time.Until(deadline) >= required
}
