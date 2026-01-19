// Package sentry provides Sentry SDK initialization for Better Stack error tracking integration.
// It wraps the Sentry Go SDK to simplify configuration and integration with Better Stack's
// error collection backend.
package sentry

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
)

// Config holds Sentry configuration for Better Stack integration.
type Config struct {
	// Token is the Better Stack Errors application token.
	Token string

	// Host is the Better Stack Errors ingesting host (e.g., "errors.betterstack.com").
	Host string

	// Environment identifies the deployment environment (e.g., "production", "staging").
	Environment string

	// Release identifies the application release version.
	Release string

	// SampleRate controls error sampling (0.0-1.0, default 1.0 = 100%).
	SampleRate float64

	// Debug enables Sentry SDK debug logging.
	Debug bool
}

// Initialize sets up the Sentry SDK with Better Stack configuration.
// If Token is empty, Sentry is disabled and nil is returned.
// The DSN is constructed as: https://$TOKEN@$HOST/1
func Initialize(cfg Config) error {
	if cfg.Token == "" {
		return nil // Sentry disabled
	}

	if cfg.Host == "" {
		return fmt.Errorf("sentry host is required when token is provided")
	}

	// Build DSN for Better Stack: https://$TOKEN@$HOST/1
	// The project ID (/1) is required by Sentry SDK but ignored by Better Stack.
	dsn := fmt.Sprintf("https://%s@%s/1", cfg.Token, cfg.Host)

	sampleRate := cfg.SampleRate
	if sampleRate <= 0 {
		sampleRate = 1.0 // Default to 100% sampling
	}

	return sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		SampleRate:       sampleRate,
		Debug:            cfg.Debug,
		AttachStacktrace: true,
	})
}

// Flush waits for buffered events to be sent to the server.
// Returns true if all events were sent within the timeout.
func Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}

// IsEnabled returns true if Sentry is initialized and active.
func IsEnabled() bool {
	return sentry.CurrentHub().Client() != nil
}

// CaptureException captures an error and sends it to Sentry.
func CaptureException(err error) {
	sentry.CaptureException(err)
}

// CaptureExceptionWithContext captures an error with context information.
func CaptureExceptionWithContext(ctx context.Context, err error) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.CaptureException(err)
}

// CaptureMessage captures a message and sends it to Sentry.
func CaptureMessage(message string) {
	sentry.CaptureMessage(message)
}
