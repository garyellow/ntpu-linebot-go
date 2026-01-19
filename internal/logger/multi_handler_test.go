package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
)

func TestNewMultiHandler_NilFiltering(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, nil)

	// Should filter out nil handlers
	mh := NewMultiHandler(nil, jsonHandler, nil)
	if mh == nil {
		t.Fatal("NewMultiHandler returned nil")
	}
	if len(mh.handlers) != 1 {
		t.Errorf("Expected 1 handler after filtering nils, got %d", len(mh.handlers))
	}
}

func TestMultiHandler_Enabled(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	debugHandler := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelDebug})
	errorHandler := slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelError})

	mh := NewMultiHandler(debugHandler, errorHandler)

	tests := []struct {
		level    slog.Level
		expected bool
	}{
		{slog.LevelDebug, true},
		{slog.LevelInfo, true},
		{slog.LevelWarn, true},
		{slog.LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := mh.Enabled(context.Background(), tt.level); got != tt.expected {
				t.Errorf("Enabled(%v) = %v, want %v", tt.level, got, tt.expected)
			}
		})
	}
}

func TestMultiHandler_Handle(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	handler1 := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler2 := slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo})

	mh := NewMultiHandler(handler1, handler2)
	logger := slog.New(mh)

	logger.Info("test message", "key", "value")

	// Both handlers should receive the log
	var entry1, entry2 map[string]any
	if err := json.Unmarshal(buf1.Bytes(), &entry1); err != nil {
		t.Fatalf("Failed to parse JSON from handler1: %v", err)
	}
	if err := json.Unmarshal(buf2.Bytes(), &entry2); err != nil {
		t.Fatalf("Failed to parse JSON from handler2: %v", err)
	}

	if entry1["msg"] != "test message" {
		t.Errorf("Handler1 msg = %v, want 'test message'", entry1["msg"])
	}
	if entry2["msg"] != "test message" {
		t.Errorf("Handler2 msg = %v, want 'test message'", entry2["msg"])
	}
	if entry1["key"] != "value" {
		t.Errorf("Handler1 key = %v, want 'value'", entry1["key"])
	}
	if entry2["key"] != "value" {
		t.Errorf("Handler2 key = %v, want 'value'", entry2["key"])
	}
}

func TestMultiHandler_Handle_LevelFiltering(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	debugHandler := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelDebug})
	errorHandler := slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelError})

	mh := NewMultiHandler(debugHandler, errorHandler)
	logger := slog.New(mh)

	logger.Info("info message")

	// Debug handler should receive the log
	if buf1.Len() == 0 {
		t.Error("Debug handler should have received info message")
	}
	// Error handler should NOT receive the log
	if buf2.Len() != 0 {
		t.Error("Error handler should NOT have received info message")
	}
}

func TestMultiHandler_WithAttrs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	mh := NewMultiHandler(handler)

	newHandler := mh.WithAttrs([]slog.Attr{slog.String("module", "test")})
	logger := slog.New(newHandler)

	logger.Info("test message")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry["module"] != "test" {
		t.Errorf("Expected module='test', got %v", entry["module"])
	}
}

func TestMultiHandler_WithGroup(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	mh := NewMultiHandler(handler)

	newHandler := mh.WithGroup("request")
	newHandler = newHandler.WithAttrs([]slog.Attr{slog.String("id", "123")})
	logger := slog.New(newHandler)

	logger.Info("test message")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	request, ok := entry["request"].(map[string]any)
	if !ok {
		t.Fatalf("Expected 'request' group, got %v", entry)
	}
	if request["id"] != "123" {
		t.Errorf("Expected request.id='123', got %v", request["id"])
	}
}

// errorHandler is a test handler that always returns an error
type errorHandler struct {
	slog.Handler
}

func (h *errorHandler) Handle(_ context.Context, _ slog.Record) error {
	return errors.New("handler error")
}

func (h *errorHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func TestMultiHandler_Handle_ErrorCollection(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	goodHandler := slog.NewJSONHandler(&buf, nil)
	badHandler := &errorHandler{}

	mh := NewMultiHandler(goodHandler, badHandler)

	record := slog.Record{}
	record.Message = "test"

	err := mh.Handle(context.Background(), record)

	// Good handler should still write
	if buf.Len() == 0 {
		t.Error("Good handler should have written the log")
	}

	// Error should be returned from bad handler
	if err == nil {
		t.Error("Expected error from bad handler")
	}
	if !errors.Is(err, errors.Unwrap(err)) && err.Error() != "handler error" {
		t.Errorf("Expected 'handler error', got %v", err)
	}
}

func TestMultiHandler_Concurrent(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	var mu1, mu2 sync.Mutex

	// Use locked writers to avoid race conditions in test
	handler1 := slog.NewJSONHandler(&lockedWriter{w: &buf1, mu: &mu1}, nil)
	handler2 := slog.NewJSONHandler(&lockedWriter{w: &buf2, mu: &mu2}, nil)

	mh := NewMultiHandler(handler1, handler2)
	logger := slog.New(mh)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logger.Info("concurrent log", "iteration", i)
		}(i)
	}
	wg.Wait()

	mu1.Lock()
	count1 := bytes.Count(buf1.Bytes(), []byte("concurrent log"))
	mu1.Unlock()

	mu2.Lock()
	count2 := bytes.Count(buf2.Bytes(), []byte("concurrent log"))
	mu2.Unlock()

	if count1 != 100 {
		t.Errorf("Handler1 should have 100 logs, got %d", count1)
	}
	if count2 != 100 {
		t.Errorf("Handler2 should have 100 logs, got %d", count2)
	}
}

// lockedWriter wraps a writer with a mutex for concurrent test safety
type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (lw *lockedWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}
