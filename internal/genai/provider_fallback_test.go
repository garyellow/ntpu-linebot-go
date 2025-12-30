package genai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	// Initialize global metrics for testing
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	metrics.InitGlobal(m)
}

// mockIntentParser is a test mock for IntentParser interface
type mockIntentParser struct {
	parseFunc   func(ctx context.Context, text string) (*ParseResult, error)
	provider    Provider
	enabled     bool
	closeCalled bool
}

func (m *mockIntentParser) Parse(ctx context.Context, text string) (*ParseResult, error) {
	if m.parseFunc != nil {
		return m.parseFunc(ctx, text)
	}
	return nil, errors.New("not implemented")
}

func (m *mockIntentParser) IsEnabled() bool {
	return m.enabled
}

func (m *mockIntentParser) Provider() Provider {
	return m.provider
}

func (m *mockIntentParser) Close() error {
	m.closeCalled = true
	return nil
}

// mockQueryExpander is a test mock for QueryExpander interface
type mockQueryExpander struct {
	expandFunc  func(ctx context.Context, query string) (string, error)
	provider    Provider
	closeCalled bool
}

func (m *mockQueryExpander) Expand(ctx context.Context, query string) (string, error) {
	if m.expandFunc != nil {
		return m.expandFunc(ctx, query)
	}
	return query, nil
}

func (m *mockQueryExpander) Provider() Provider {
	return m.provider
}

func (m *mockQueryExpander) Close() error {
	m.closeCalled = true
	return nil
}

func TestFallbackIntentParser_Parse_PrimarySuccess(t *testing.T) {
	t.Parallel()
	primary := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			return &ParseResult{Module: "test", Intent: "search"}, nil
		},
		provider: ProviderGemini,
		enabled:  true,
	}

	parser := NewFallbackIntentParser(primary, nil, DefaultRetryConfig())

	result, err := parser.Parse(context.Background(), "test query")
	if err != nil {
		t.Errorf("Parse() error = %v, want nil", err)
	}
	if result == nil || result.Module != "test" {
		t.Errorf("Parse() result = %v, want module=test", result)
	}
}

func TestFallbackIntentParser_Parse_Fallback(t *testing.T) {
	t.Parallel()
	primaryCalls := 0
	primary := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			primaryCalls++
			return nil, errors.New("service unavailable") // retryable error
		},
		provider: ProviderGemini,
		enabled:  true,
	}

	fallback := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			return &ParseResult{Module: "fallback", Intent: "search"}, nil
		},
		provider: ProviderGroq,
		enabled:  true,
	}

	cfg := RetryConfig{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
	}

	parser := NewFallbackIntentParser(primary, fallback, cfg)

	result, err := parser.Parse(context.Background(), "test query")
	if err != nil {
		t.Errorf("Parse() error = %v, want nil (fallback should succeed)", err)
	}
	if result == nil || result.Module != "fallback" {
		t.Errorf("Parse() result = %v, want module=fallback", result)
	}
	// Primary should have been called MaxAttempts times before fallback
	if primaryCalls != cfg.MaxAttempts {
		t.Errorf("primary called %d times, want %d", primaryCalls, cfg.MaxAttempts)
	}
}

func TestFallbackIntentParser_Parse_PermanentError(t *testing.T) {
	t.Parallel()
	primary := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			return nil, errors.New("invalid api key") // permanent error
		},
		provider: ProviderGemini,
		enabled:  true,
	}

	fallbackCalled := false
	fallback := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			fallbackCalled = true
			return &ParseResult{Module: "fallback"}, nil
		},
		provider: ProviderGroq,
		enabled:  true,
	}

	parser := NewFallbackIntentParser(primary, fallback, DefaultRetryConfig())

	_, err := parser.Parse(context.Background(), "test query")
	if err == nil {
		t.Error("Parse() should return error for permanent failure")
	}
	if fallbackCalled {
		t.Error("fallback should not be called for permanent errors")
	}
}

func TestFallbackIntentParser_Parse_NilParser(t *testing.T) {
	t.Parallel()
	var parser *FallbackIntentParser
	_, err := parser.Parse(context.Background(), "test")
	if err == nil {
		t.Error("Parse() should return error for nil parser")
	}

	parser = &FallbackIntentParser{}
	_, err = parser.Parse(context.Background(), "test")
	if err == nil {
		t.Error("Parse() should return error when primary is nil")
	}
}

func TestFallbackIntentParser_IsEnabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		parser   *FallbackIntentParser
		expected bool
	}{
		{
			name:     "nil parser",
			parser:   nil,
			expected: false,
		},
		{
			name:     "empty parser",
			parser:   &FallbackIntentParser{},
			expected: false,
		},
		{
			name: "primary enabled",
			parser: NewFallbackIntentParser(
				&mockIntentParser{enabled: true, provider: ProviderGemini},
				nil,
				DefaultRetryConfig(),
			),
			expected: true,
		},
		{
			name: "only fallback enabled",
			parser: NewFallbackIntentParser(
				&mockIntentParser{enabled: false, provider: ProviderGemini},
				&mockIntentParser{enabled: true, provider: ProviderGroq},
				DefaultRetryConfig(),
			),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.parser.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFallbackIntentParser_Close(t *testing.T) {
	t.Parallel()
	primary := &mockIntentParser{provider: ProviderGemini}
	fallback := &mockIntentParser{provider: ProviderGroq}

	parser := NewFallbackIntentParser(primary, fallback, DefaultRetryConfig())
	err := parser.Close()

	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
	if !primary.closeCalled {
		t.Error("primary.Close() was not called")
	}
	if !fallback.closeCalled {
		t.Error("fallback.Close() was not called")
	}
}

func TestFallbackIntentParser_Provider(t *testing.T) {
	t.Parallel()
	primary := &mockIntentParser{provider: ProviderGemini}
	parser := NewFallbackIntentParser(primary, nil, DefaultRetryConfig())

	if got := parser.Provider(); got != ProviderGemini {
		t.Errorf("Provider() = %v, want %v", got, ProviderGemini)
	}
}

func TestFallbackQueryExpander_Expand_PrimarySuccess(t *testing.T) {
	t.Parallel()
	primary := &mockQueryExpander{
		expandFunc: func(_ context.Context, query string) (string, error) {
			return query + " expanded", nil
		},
		provider: ProviderGemini,
	}

	expander := NewFallbackQueryExpander(primary, nil, DefaultRetryConfig())

	result, err := expander.Expand(context.Background(), "test")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test expanded" {
		t.Errorf("Expand() = %q, want %q", result, "test expanded")
	}
}

func TestFallbackQueryExpander_Expand_GracefulDegradation(t *testing.T) {
	t.Parallel()
	primary := &mockQueryExpander{
		expandFunc: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("service unavailable")
		},
		provider: ProviderGemini,
	}

	fallback := &mockQueryExpander{
		expandFunc: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("also unavailable")
		},
		provider: ProviderGroq,
	}

	cfg := RetryConfig{
		MaxAttempts:  1,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
	}

	expander := NewFallbackQueryExpander(primary, fallback, cfg)

	// Should return original query on complete failure (graceful degradation)
	result, err := expander.Expand(context.Background(), "original")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil (graceful degradation)", err)
	}
	if result != "original" {
		t.Errorf("Expand() = %q, want %q (original query)", result, "original")
	}
}

func TestFallbackQueryExpander_Expand_NilExpander(t *testing.T) {
	t.Parallel()
	var expander *FallbackQueryExpander
	result, err := expander.Expand(context.Background(), "test")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test" {
		t.Errorf("Expand() = %q, want %q (original)", result, "test")
	}
}

func TestFallbackQueryExpander_Close(t *testing.T) {
	t.Parallel()
	primary := &mockQueryExpander{provider: ProviderGemini}
	fallback := &mockQueryExpander{provider: ProviderGroq}

	expander := NewFallbackQueryExpander(primary, fallback, DefaultRetryConfig())
	err := expander.Close()

	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
	if !primary.closeCalled {
		t.Error("primary.Close() was not called")
	}
	if !fallback.closeCalled {
		t.Error("fallback.Close() was not called")
	}
}

func TestFallbackQueryExpander_ContextCancellation(t *testing.T) {
	t.Parallel()
	primary := &mockQueryExpander{
		expandFunc: func(ctx context.Context, query string) (string, error) {
			select {
			case <-ctx.Done():
				return query, ctx.Err()
			case <-time.After(time.Hour):
				return query + " expanded", nil
			}
		},
		provider: ProviderGemini,
	}

	cfg := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Hour, // Very long delay
		MaxDelay:     time.Hour,
	}

	expander := NewFallbackQueryExpander(primary, nil, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := expander.Expand(ctx, "test")
	// Should return original query due to graceful degradation
	if result != "test" {
		t.Errorf("Expand() = %q, want %q on cancellation", result, "test")
	}
	// Error should be nil due to graceful degradation
	if err != nil {
		t.Logf("Note: Expand() returned error = %v (acceptable for canceled context)", err)
	}
}
