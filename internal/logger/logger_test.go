package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		wantErr  bool
		logLevel string
	}{
		{
			name:     "Valid debug level",
			level:    "debug",
			logLevel: "debug",
		},
		{
			name:     "Valid info level",
			level:    "info",
			logLevel: "info",
		},
		{
			name:     "Valid warn level",
			level:    "warn",
			logLevel: "warning",
		},
		{
			name:     "Valid error level",
			level:    "error",
			logLevel: "error",
		},
		{
			name:     "Invalid level defaults to info",
			level:    "invalid",
			logLevel: "info",
		},
		{
			name:     "Empty level defaults to info",
			level:    "",
			logLevel: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := New(tt.level)
			if log == nil {
				t.Fatal("New() returned nil")
			}

			actualLevel := log.GetLevel().String()
			if actualLevel != tt.logLevel {
				t.Errorf("New(%q) log level = %q, want %q", tt.level, actualLevel, tt.logLevel)
			}
		})
	}
}

func TestLogger_WithModule(t *testing.T) {
	var buf bytes.Buffer
	log := New("info")
	log.SetOutput(&buf)

	log.WithModule("test_module").Info("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if module, ok := logEntry["module"].(string); !ok || module != "test_module" {
		t.Errorf("WithModule() module = %v, want %q", logEntry["module"], "test_module")
	}
}

func TestLogger_WithRequestID(t *testing.T) {
	var buf bytes.Buffer
	log := New("info")
	log.SetOutput(&buf)

	log.WithRequestID("req-123").Info("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if requestID, ok := logEntry["request_id"].(string); !ok || requestID != "req-123" {
		t.Errorf("WithRequestID() request_id = %v, want %q", logEntry["request_id"], "req-123")
	}
}

func TestLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer
	log := New("info")
	log.SetOutput(&buf)

	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestIDKey, "ctx-req-456")
	ctx = context.WithValue(ctx, ModuleKey, "ctx_module")

	log.WithContext(ctx).Info("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if requestID, ok := logEntry["request_id"].(string); !ok || requestID != "ctx-req-456" {
		t.Errorf("WithContext() request_id = %v, want %q", logEntry["request_id"], "ctx-req-456")
	}
	if module, ok := logEntry["module"].(string); !ok || module != "ctx_module" {
		t.Errorf("WithContext() module = %v, want %q", logEntry["module"], "ctx_module")
	}
}

func TestLogger_WithError(t *testing.T) {
	var buf bytes.Buffer
	log := New("info")
	log.SetOutput(&buf)

	testErr := &testError{msg: "test error message"}
	log.WithError(testErr).Error("operation failed")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if errField, ok := logEntry["error"].(string); !ok || errField != "test error message" {
		t.Errorf("WithError() error = %v, want %q", logEntry["error"], "test error message")
	}
}

func TestLogger_SetLevel(t *testing.T) {
	log := New("info")

	// Test valid level change
	if err := log.SetLevel("debug"); err != nil {
		t.Errorf("SetLevel(debug) error = %v, want nil", err)
	}
	if log.GetLevel().String() != "debug" {
		t.Errorf("SetLevel(debug) level = %q, want %q", log.GetLevel().String(), "debug")
	}

	// Test invalid level
	if err := log.SetLevel("invalid"); err == nil {
		t.Error("SetLevel(invalid) error = nil, want error")
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := New("info")
	log.SetOutput(&buf)

	log.Info("test message")

	var logEntry map[string]interface{}
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
