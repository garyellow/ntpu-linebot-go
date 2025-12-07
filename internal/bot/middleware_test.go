package bot

import (
	"context"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
	"github.com/prometheus/client_golang/prometheus"
)

// mockHandler implements Handler for testing
type mockHandler struct {
	name           string
	postbackPrefix string
	canHandle      bool
	panicOnHandle  bool
}

func (m *mockHandler) Name() string               { return m.name }
func (m *mockHandler) PostbackPrefix() string     { return m.postbackPrefix }
func (m *mockHandler) CanHandle(text string) bool { return m.canHandle }
func (m *mockHandler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	if m.panicOnHandle {
		panic("test panic")
	}
	return []messaging_api.MessageInterface{&messaging_api.TextMessage{Text: "test"}}
}
func (m *mockHandler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	return nil
}

func TestLoggingMiddleware(t *testing.T) {
	log := logger.New("debug")
	mw := LoggingMiddleware(log)
	handler := &mockHandler{name: "test", canHandle: true}

	called := false
	next := func(ctx context.Context, h Handler, text string, _ HandlerFunc) []messaging_api.MessageInterface {
		called = true
		return handler.HandleMessage(ctx, text)
	}

	msgs := mw(context.Background(), handler, "test input", next)

	if !called {
		t.Error("Expected next to be called")
	}
	if len(msgs) == 0 {
		t.Error("Expected messages from handler")
	}
}

func TestMetricsMiddleware(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	mw := MetricsMiddleware(m)
	handler := &mockHandler{name: "test", canHandle: true}

	called := false
	next := func(ctx context.Context, h Handler, text string, _ HandlerFunc) []messaging_api.MessageInterface {
		called = true
		time.Sleep(10 * time.Millisecond) // Simulate work
		return handler.HandleMessage(ctx, text)
	}

	msgs := mw(context.Background(), handler, "test input", next)

	if !called {
		t.Error("Expected next to be called")
	}
	if len(msgs) == 0 {
		t.Error("Expected messages from handler")
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	log := logger.New("info")
	mw := RecoveryMiddleware(log, nil)
	handler := &mockHandler{name: "test", canHandle: true, panicOnHandle: true}

	// Test that panic is recovered
	defer func() {
		if r := recover(); r != nil {
			t.Error("Panic should have been recovered by middleware")
		}
	}()

	next := func(ctx context.Context, h Handler, text string, _ HandlerFunc) []messaging_api.MessageInterface {
		return handler.HandleMessage(ctx, text)
	}

	// Should not panic
	mw(context.Background(), handler, "test input", next)
}

func TestMiddlewareChaining(t *testing.T) {
	handler := &mockHandler{name: "test", canHandle: true}

	// Chain multiple middlewares
	var executionOrder []string

	mw1 := func(ctx context.Context, h Handler, text string, next HandlerFunc) []messaging_api.MessageInterface {
		executionOrder = append(executionOrder, "mw1_before")
		msgs := next(ctx, h, text, next)
		executionOrder = append(executionOrder, "mw1_after")
		return msgs
	}

	mw2 := func(ctx context.Context, h Handler, text string, next HandlerFunc) []messaging_api.MessageInterface {
		executionOrder = append(executionOrder, "mw2_before")
		msgs := next(ctx, h, text, next)
		executionOrder = append(executionOrder, "mw2_after")
		return msgs
	}

	// Build chain: mw1 -> mw2 -> handler
	botRegistry := NewRegistry()
	botRegistry.Use(mw1)
	botRegistry.Use(mw2)
	botRegistry.Register(handler)

	msgs := botRegistry.DispatchMessage(context.Background(), "test")

	if len(msgs) == 0 {
		t.Error("Expected messages from handler")
	}

	// Verify execution order
	expected := []string{"mw1_before", "mw2_before", "mw2_after", "mw1_after"}
	if len(executionOrder) != len(expected) {
		t.Errorf("Expected %d middleware calls, got %d", len(expected), len(executionOrder))
	}
	for i, exp := range expected {
		if i >= len(executionOrder) || executionOrder[i] != exp {
			t.Errorf("Expected execution order[%d] = %s, got %v", i, exp, executionOrder)
		}
	}
}
