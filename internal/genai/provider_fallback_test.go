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

	parser := NewFallbackIntentParser(DefaultRetryConfig(), primary, nil)

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

	parser := NewFallbackIntentParser(cfg, primary, fallback)

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
			return nil, errors.New("bad request") // permanent: won't improve on any provider
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

	parser := NewFallbackIntentParser(DefaultRetryConfig(), primary, fallback)

	_, err := parser.Parse(context.Background(), "test query")
	if err == nil {
		t.Error("Parse() should return error for permanent failure")
	}
	if fallbackCalled {
		t.Error("fallback should not be called for permanent errors")
	}
}

func TestFallbackIntentParser_Parse_AuthErrorFallback(t *testing.T) {
	t.Parallel()
	primary := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			return nil, errors.New("invalid api key") // auth failure: try next provider
		},
		provider: ProviderGemini,
		enabled:  true,
	}

	fallbackCalled := false
	fallback := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			fallbackCalled = true
			return &ParseResult{Module: "course", Intent: "search"}, nil
		},
		provider: ProviderGroq,
		enabled:  true,
	}

	parser := NewFallbackIntentParser(DefaultRetryConfig(), primary, fallback)

	result, err := parser.Parse(context.Background(), "test query")
	if err != nil {
		t.Errorf("Parse() should succeed via fallback, got error: %v", err)
	}
	if !fallbackCalled {
		t.Error("fallback should be called when primary has auth error")
	}
	if result == nil || result.Module != "course" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestFallbackIntentParser_Parse_404Fallback(t *testing.T) {
	t.Parallel()
	// Simulate the real production bug: Cerebras returns 404 for a deprecated model.
	primary := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			return nil, errors.New(`chat completion failed: POST "https://api.cerebras.ai/v1/chat/completions": 404 Not Found `)
		},
		provider: ProviderCerebras,
		enabled:  true,
	}

	fallbackCalled := false
	fallback := &mockIntentParser{
		parseFunc: func(_ context.Context, _ string) (*ParseResult, error) {
			fallbackCalled = true
			return &ParseResult{Module: "course", Intent: "smart"}, nil
		},
		provider: ProviderGroq,
		enabled:  true,
	}

	parser := NewFallbackIntentParser(DefaultRetryConfig(), primary, fallback)

	result, err := parser.Parse(context.Background(), "我是資工系的想學點金融知識，推薦修哪些課")
	if err != nil {
		t.Errorf("Parse() should succeed via fallback, got error: %v", err)
	}
	if !fallbackCalled {
		t.Error("fallback should be called when primary returns 404 (model not found)")
	}
	if result == nil || result.Module != "course" {
		t.Errorf("unexpected result: %v", result)
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
		t.Error("Parse() should return error when parsers is empty")
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
				DefaultRetryConfig(),
				&mockIntentParser{enabled: true, provider: ProviderGemini},
			),
			expected: true,
		},
		{
			name: "only fallback enabled",
			parser: NewFallbackIntentParser(
				DefaultRetryConfig(),
				&mockIntentParser{enabled: false, provider: ProviderGemini},
				&mockIntentParser{enabled: true, provider: ProviderGroq},
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

	parser := NewFallbackIntentParser(DefaultRetryConfig(), primary, fallback)
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
	parser := NewFallbackIntentParser(DefaultRetryConfig(), primary, nil)

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

	expander := NewFallbackQueryExpander(DefaultRetryConfig(), primary, nil)

	result, err := expander.Expand(context.Background(), "test")
	if err != nil {
		t.Errorf("Expand() error = %v, want nil", err)
	}
	if result != "test expanded" {
		t.Errorf("Expand() = %q, want %q", result, "test expanded")
	}
}

func TestFallbackQueryExpander_Expand_AllProvidersFailed(t *testing.T) {
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

	expander := NewFallbackQueryExpander(cfg, primary, fallback)

	// Should return original query AND an error when all providers fail
	result, err := expander.Expand(context.Background(), "original")
	if err == nil {
		t.Error("Expand() should return error when all expanders fail")
	}
	if result != "original" {
		t.Errorf("Expand() = %q, want %q (original query preserved on failure)", result, "original")
	}
}

func TestFallbackQueryExpander_Expand_NilExpander(t *testing.T) {
	t.Parallel()
	var expander *FallbackQueryExpander
	result, err := expander.Expand(context.Background(), "test")
	if err != nil {
		t.Errorf("Expand() on nil expander (feature disabled) should return no error, got: %v", err)
	}
	if result != "test" {
		t.Errorf("Expand() = %q, want %q (original)", result, "test")
	}
}

func TestFallbackQueryExpander_Close(t *testing.T) {
	t.Parallel()
	primary := &mockQueryExpander{provider: ProviderGemini}
	fallback := &mockQueryExpander{provider: ProviderGroq}

	expander := NewFallbackQueryExpander(DefaultRetryConfig(), primary, fallback)
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

func TestClassifyErrorType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, "success"},
		{"deadline exceeded", context.DeadlineExceeded, "timeout"},
		{"context canceled", context.Canceled, "canceled"},
		// Auth errors must be labeled "auth_error", not "quota_exhausted"
		{"401 in string", errors.New(`POST "https://api.example.com": 401 Unauthorized`), "auth_error"},
		{"unauthorized", errors.New("unauthorized access denied"), "auth_error"},
		{"invalid api key", errors.New("invalid api key provided"), "auth_error"},
		{"invalid_api_key code", errors.New("code: invalid_api_key"), "auth_error"},
		{"403 forbidden", errors.New("403 forbidden"), "auth_error"},
		{"permission denied", errors.New("permission denied"), "auth_error"},
		// 404 means model/endpoint not found on this provider → fallback, labeled "model_not_found"
		{"404 not found", errors.New(`POST "https://api.cerebras.ai/v1/chat/completions": 404 Not Found`), "model_not_found"},
		{"model not found", errors.New("the model 'zai-glm-4.7' was not found"), "model_not_found"},
		// Quota exhaustion must NOT be mistaken for auth or model_not_found
		{"quota exceeded", errors.New("quota exceeded for today"), "quota_exhausted"},
		// Rate limit is transient (retry), not quota
		{"rate limit 429", errors.New("429 too many requests"), "transient_error"},
		// Transient / permanent
		{"server error 503", errors.New("503 service unavailable"), "transient_error"},
		{"bad request 400", errors.New("400 bad request"), "error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyErrorType(tc.err)
			if got != tc.want {
				t.Errorf("classifyErrorType(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
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

	expander := NewFallbackQueryExpander(cfg, primary, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := expander.Expand(ctx, "test")
	// Should return original query on cancellation
	if result != "test" {
		t.Errorf("Expand() = %q, want %q on cancellation", result, "test")
	}
	// Error should be returned (context cancellation is not graceful degradation)
	if err == nil {
		t.Error("Expand() should return error on context cancellation")
	}
}
