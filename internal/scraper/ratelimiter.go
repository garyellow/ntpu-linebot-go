package scraper

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
	"time"
)

// RetryWithBackoff retries a function with exponential backoff and jitter
// Only used for FAILED requests. Success path is handled separately.
//
// maxRetries: maximum number of retry attempts (0 = no retry, just try once)
// initialDelay: initial delay before first retry (e.g., 4s)
//
// Backoff formula: delay = initialDelay * 2^attempt ± 25% jitter
// Example with initialDelay=4s, maxRetries=5:
//
//	attempt 0: immediate (first try)
//	attempt 1: ~4s  (3s - 5s)
//	attempt 2: ~8s  (6s - 10s)
//	attempt 3: ~16s (12s - 20s)
//	attempt 4: ~32s (24s - 40s)
//	attempt 5: ~64s (48s - 80s)
func RetryWithBackoff(ctx context.Context, maxRetries int, initialDelay time.Duration, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Try the function
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't delay after the last attempt
		if attempt == maxRetries {
			break
		}

		// Calculate exponential backoff delay
		delay := time.Duration(float64(initialDelay) * math.Pow(2, float64(attempt)))

		// Add jitter (±25%)
		var b [8]byte
		_, _ = rand.Read(b[:])
		jitterValue := int64(binary.LittleEndian.Uint64(b[:]))
		if jitterValue < 0 {
			jitterValue = -jitterValue
		}
		jitter := time.Duration(jitterValue % (int64(delay) / 2))
		delay = delay - delay/4 + jitter

		// Wait for delay or context cancellation
		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

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
