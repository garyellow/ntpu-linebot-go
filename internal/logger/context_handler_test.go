package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
)

func TestContextHandler_Handle(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func(context.Context) context.Context
		expectedFields map[string]string
	}{
		{
			name: "extracts all context values",
			setupContext: func(ctx context.Context) context.Context {
				ctx = ctxutil.WithUserID(ctx, "U12345")
				ctx = ctxutil.WithChatID(ctx, "C67890")
				ctx = ctxutil.WithRequestID(ctx, "req-abc-123")
				return ctx
			},
			expectedFields: map[string]string{
				"user_id":    "U12345",
				"chat_id":    "C67890",
				"request_id": "req-abc-123",
			},
		},
		{
			name: "extracts partial context values",
			setupContext: func(ctx context.Context) context.Context {
				ctx = ctxutil.WithUserID(ctx, "U99999")
				return ctx
			},
			expectedFields: map[string]string{
				"user_id": "U99999",
			},
		},
		{
			name: "handles empty context",
			setupContext: func(ctx context.Context) context.Context {
				return ctx
			},
			expectedFields: map[string]string{},
		},
		{
			name: "skips empty string values",
			setupContext: func(ctx context.Context) context.Context {
				ctx = ctxutil.WithUserID(ctx, "")
				ctx = ctxutil.WithChatID(ctx, "C12345")
				return ctx
			},
			expectedFields: map[string]string{
				"chat_id": "C12345",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup buffer to capture log output
			var buf bytes.Buffer
			baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})
			handler := NewContextHandler(baseHandler)

			// Create logger with context handler
			logger := slog.New(handler)

			// Setup context
			ctx := tt.setupContext(context.Background())

			// Log a test message
			logger.InfoContext(ctx, "test message")

			// Parse output
			output := buf.String()

			// Verify expected fields are present
			for key, value := range tt.expectedFields {
				expectedJSON := `"` + key + `":"` + value + `"`
				if !strings.Contains(output, expectedJSON) {
					t.Errorf("Expected field %s=%s not found in output: %s", key, value, output)
				}
			}

			// Verify unexpected fields are not present
			if len(tt.expectedFields) == 0 {
				// If no fields expected, ensure none of the context fields appear
				unexpectedFields := []string{"user_id", "chat_id", "request_id"}
				for _, field := range unexpectedFields {
					if strings.Contains(output, `"`+field+`"`) {
						t.Errorf("Unexpected field %s found in output: %s", field, output)
					}
				}
			}
		})
	}
}

func TestContextHandler_Enabled(t *testing.T) {
	baseHandler := slog.NewJSONHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	handler := NewContextHandler(baseHandler)

	ctx := context.Background()

	tests := []struct {
		name     string
		level    slog.Level
		expected bool
	}{
		{"debug below threshold", slog.LevelDebug, false},
		{"info at threshold", slog.LevelInfo, true},
		{"warn above threshold", slog.LevelWarn, true},
		{"error above threshold", slog.LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := handler.Enabled(ctx, tt.level)
			if enabled != tt.expected {
				t.Errorf("Enabled(%v) = %v, want %v", tt.level, enabled, tt.expected)
			}
		})
	}
}

func TestContextHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, nil)
	handler := NewContextHandler(baseHandler)

	// Add attributes using WithAttrs
	attrs := []slog.Attr{
		slog.String("service", "test-service"),
		slog.Int("version", 1),
	}
	handlerWithAttrs := handler.WithAttrs(attrs)

	logger := slog.New(handlerWithAttrs)
	logger.Info("test message")

	output := buf.String()

	// Verify attributes are present
	if !strings.Contains(output, `"service":"test-service"`) {
		t.Errorf("Expected service attribute not found in output: %s", output)
	}
	if !strings.Contains(output, `"version":1`) {
		t.Errorf("Expected version attribute not found in output: %s", output)
	}
}

func TestContextHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, nil)
	handler := NewContextHandler(baseHandler)

	// Create group
	handlerWithGroup := handler.WithGroup("metrics")
	logger := slog.New(handlerWithGroup)

	logger.Info("test message", "count", 42)

	output := buf.String()

	// Verify group structure
	if !strings.Contains(output, `"metrics":{`) {
		t.Errorf("Expected metrics group not found in output: %s", output)
	}
	if !strings.Contains(output, `"count":42`) {
		t.Errorf("Expected count in group not found in output: %s", output)
	}
}

func TestContextHandler_Integration(t *testing.T) {
	// Test that ContextHandler works with both context values and explicit attributes
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	handler := NewContextHandler(baseHandler)
	logger := slog.New(handler)

	// Setup context with tracing values
	ctx := context.Background()
	ctx = ctxutil.WithUserID(ctx, "U11111")
	ctx = ctxutil.WithRequestID(ctx, "req-test-123")

	// Log with both context values and explicit attributes
	logger.InfoContext(ctx, "processing request",
		slog.String("action", "login"),
		slog.Int("attempt", 1),
	)

	output := buf.String()

	// Verify context values
	if !strings.Contains(output, `"user_id":"U11111"`) {
		t.Errorf("Expected user_id from context not found in output: %s", output)
	}
	if !strings.Contains(output, `"request_id":"req-test-123"`) {
		t.Errorf("Expected request_id from context not found in output: %s", output)
	}

	// Verify explicit attributes
	if !strings.Contains(output, `"action":"login"`) {
		t.Errorf("Expected action attribute not found in output: %s", output)
	}
	if !strings.Contains(output, `"attempt":1`) {
		t.Errorf("Expected attempt attribute not found in output: %s", output)
	}

	// Verify message
	if !strings.Contains(output, `"msg":"processing request"`) {
		t.Errorf("Expected message not found in output: %s", output)
	}
}
