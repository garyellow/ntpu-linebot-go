package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		level      string
		checkLevel slog.Level
	}{
		{
			name:       "Valid debug level",
			level:      "debug",
			checkLevel: slog.LevelDebug,
		},
		{
			name:       "Valid info level",
			level:      "info",
			checkLevel: slog.LevelInfo,
		},
		{
			name:       "Valid warn level",
			level:      "warn",
			checkLevel: slog.LevelWarn,
		},
		{
			name:       "Valid error level",
			level:      "error",
			checkLevel: slog.LevelError,
		},
		{
			name:       "Invalid level defaults to info",
			level:      "invalid",
			checkLevel: slog.LevelInfo,
		},
		{
			name:       "Empty level defaults to info",
			level:      "",
			checkLevel: slog.LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			log := New(tt.level)
			if log == nil {
				t.Fatal("New() returned nil")
				return
			}

			if !log.Enabled(context.Background(), tt.checkLevel) {
				t.Errorf("Logger should be enabled for level %v", tt.checkLevel)
			}
		})
	}
}

func TestLogger_WithModule(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := NewWithWriter("info", &buf)

	log.WithModule("test_module").Info("test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if module, ok := logEntry["module"].(string); !ok || module != "test_module" {
		t.Errorf("WithModule() module = %v, want %q", logEntry["module"], "test_module")
	}
}

func TestLogger_WithRequestID(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := NewWithWriter("info", &buf)

	log.WithRequestID("req-123").Info("test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if requestID, ok := logEntry["request_id"].(string); !ok || requestID != "req-123" {
		t.Errorf("WithRequestID() request_id = %v, want %q", logEntry["request_id"], "req-123")
	}
}

func TestLogger_WithError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := NewWithWriter("info", &buf)

	testErr := &testError{msg: "test error message"}
	log.WithError(testErr).Error("operation failed")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if errField, ok := logEntry["error"].(string); !ok || errField != "test error message" {
		t.Errorf("WithError() error = %v, want %q", logEntry["error"], "test error message")
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := NewWithWriter("info", &buf)

	log.Info("test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	// Check required fields
	requiredFields := []string{"timestamp", "level", "message"}
	for _, field := range requiredFields {
		if _, ok := logEntry[field]; !ok {
			t.Errorf("JSON log missing required field %q", field)
		}
	}

	if logEntry["message"] != "test message" {
		t.Errorf("message = %v, want %q", logEntry["message"], "test message")
	}
	if logEntry["level"] != "info" {
		t.Errorf("level = %v, want %q", logEntry["level"], "info")
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
