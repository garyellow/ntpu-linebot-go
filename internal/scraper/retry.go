package scraper

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"math"
	"math/big"
	"time"
)

// RetryWithBackoff retries a function with exponential backoff and jitter.
// Stops retrying immediately if the error is a permanentError (e.g., 404/403/401).
//
// maxRetries: maximum number of retry attempts (0 = no retry, just try once)
// initialDelay: initial delay before first retry (e.g., 1s)
//
// Backoff formula: delay = initialDelay * 2^attempt ± 25% jitter
// Example with initialDelay=1s, maxRetries=10:
//
//	attempt 0: immediate (first try)
//	attempt 1: ~1s    (0.75s - 1.25s)
//	attempt 2: ~2s    (1.5s - 2.5s)
//	attempt 3: ~4s    (3s - 5s)
//	attempt 4: ~8s    (6s - 10s)
//	attempt 5: ~16s   (12s - 20s)
//	attempt 6: ~32s   (24s - 40s)
//	attempt 7: ~64s   (48s - 80s)
//	attempt 8: ~128s  (96s - 160s)
//	attempt 9: ~256s  (192s - 320s)
//	attempt 10: ~512s (384s - 640s)
func RetryWithBackoff(ctx context.Context, maxRetries int, initialDelay time.Duration, fn func() error) error {
	var lastErr error
	startTime := time.Now()

	for attempt := 0; attempt <= maxRetries; attempt++ {
		attemptStart := time.Now()

		// Try the function
		err := fn()
		if err == nil {
			// Log success if retries were needed
			if attempt > 0 {
				slog.InfoContext(ctx, "Request succeeded after retries",
					"total_attempts", attempt+1,
					"total_duration_ms", time.Since(startTime).Milliseconds())
			}
			return nil
		}
		lastErr = err

		// Don't retry permanent errors (e.g., 404, 403, 401)
		var permErr *permanentError
		if errors.As(err, &permErr) {
			slog.DebugContext(ctx, "Permanent error, not retrying",
				"error", err,
				"attempt", attempt+1)
			return permErr.Unwrap() // Return the underlying error
		}

		// Log retry warning
		if attempt < maxRetries {
			slog.DebugContext(ctx, "Request failed, will retry",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"duration_ms", time.Since(attemptStart).Milliseconds(),
				"error", err)
		}

		// Don't delay after the last attempt
		if attempt == maxRetries {
			break
		}

		// Calculate exponential backoff delay
		delay := time.Duration(float64(initialDelay) * math.Pow(2, float64(attempt)))

		// Add jitter (±25%)
		halfDelay := int64(delay) / 2
		if halfDelay == 0 {
			halfDelay = 1 // Prevent division by zero
		}
		// Use crypto/rand.Int for statistically uniform random number without overflow risk
		jitterBig, err := rand.Int(rand.Reader, big.NewInt(halfDelay))
		if err != nil {
			// Fallback to zero jitter on crypto failure (extremely rare)
			jitterBig = big.NewInt(0)
		}
		jitter := time.Duration(jitterBig.Int64())
		delay = delay - delay/4 + jitter

		// Wait for delay or context cancellation
		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Log final failure
	slog.ErrorContext(ctx, "All retries exhausted",
		"total_attempts", maxRetries+1,
		"total_duration_ms", time.Since(startTime).Milliseconds(),
		"last_error", lastErr)

	return lastErr
}

// Sleep waits for the specified duration, respecting context cancellation
func Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
