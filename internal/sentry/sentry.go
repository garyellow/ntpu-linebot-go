// Package sentry provides Sentry SDK initialization for error tracking integration.
// It wraps the Sentry Go SDK to simplify configuration and enforce best practices.
package sentry

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// Config holds Sentry configuration.
type Config struct {
	// DSN is the Sentry DSN (https://TOKEN@HOST/1).
	DSN string

	// Environment identifies the deployment environment (e.g., "production", "staging").
	Environment string

	// Release identifies the application release version.
	Release string

	// ServerName identifies the host instance for this process.
	ServerName string

	// SampleRate controls error sampling (0.0-1.0, default 1.0 = 100%).
	SampleRate float64

	// TracesSampleRate controls tracing sampling (0.0-1.0, default 0.0 = disabled).
	TracesSampleRate float64

	// Debug enables Sentry SDK debug logging.
	Debug bool

	// HTTPTimeout controls the outbound timeout for Sentry transport.
	HTTPTimeout time.Duration

	// ServiceName is a stable service identifier tag.
	ServiceName string
}

// Initialize sets up the Sentry SDK.
// If DSN is empty, Sentry is disabled and nil is returned.
func Initialize(cfg Config) error {
	if cfg.DSN == "" {
		return nil // Sentry disabled
	}

	if cfg.SampleRate < 0 || cfg.SampleRate > 1 {
		return fmt.Errorf("sentry sample rate must be between 0 and 1, got %v", cfg.SampleRate)
	}
	if cfg.TracesSampleRate < 0 || cfg.TracesSampleRate > 1 {
		return fmt.Errorf("sentry traces sample rate must be between 0 and 1, got %v", cfg.TracesSampleRate)
	}

	serverName := cfg.ServerName
	if serverName == "" {
		serverName = hostname()
	}

	options := sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          ResolveRelease(cfg.Release),
		ServerName:       serverName,
		SampleRate:       cfg.SampleRate,
		TracesSampleRate: cfg.TracesSampleRate,
		EnableTracing:    cfg.TracesSampleRate > 0,
		Debug:            cfg.Debug,
		AttachStacktrace: true,
		SendDefaultPII:   false,
		IgnoreErrors:     defaultIgnoreErrors(),
		BeforeSend:       scrubEvent,
	}

	if cfg.HTTPTimeout > 0 {
		options.HTTPClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}

	if err := sentry.Init(options); err != nil {
		return err
	}

	if cfg.ServiceName != "" {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTag("service", cfg.ServiceName)
		})
	}

	return nil
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

// CaptureExceptionWithContext captures an error with context information.
func CaptureExceptionWithContext(ctx context.Context, err error) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.CaptureException(err)
}

func defaultIgnoreErrors() []string {
	return []string{
		"^context canceled$",
		"^context deadline exceeded$",
	}
}

func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	if event.Request != nil {
		event.Request.Headers = nil
		event.Request.Cookies = ""
		event.Request.Data = ""
	}
	return event
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// ResolveRelease returns the explicit release or derives one from build info.
func ResolveRelease(explicit string) string {
	if explicit != "" {
		return explicit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return shortRevision(setting.Value)
		}
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return ""
}

func shortRevision(revision string) string {
	if len(revision) <= 12 {
		return revision
	}
	return strings.TrimSpace(revision[:12])
}
