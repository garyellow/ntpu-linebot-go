package id

import (
	"context"
	"strings"
	"testing"
)

// TestDispatchIntent_ParamValidation tests parameter validation logic
// without requiring full handler setup. Uses nil dependencies (acceptable for error paths).
func TestDispatchIntent_ParamValidation(t *testing.T) {
	tests := []struct {
		name        string
		intent      string
		params      map[string]string
		errContains string
	}{
		{
			name:        "search intent missing name",
			intent:      IntentSearch,
			params:      map[string]string{},
			errContains: "missing required param: name",
		},
		{
			name:        "search intent empty name",
			intent:      IntentSearch,
			params:      map[string]string{"name": ""},
			errContains: "missing required param: name",
		},
		{
			name:        "student_id intent missing student_id",
			intent:      IntentStudentID,
			params:      map[string]string{},
			errContains: "missing required param: student_id",
		},
		{
			name:        "student_id intent empty student_id",
			intent:      IntentStudentID,
			params:      map[string]string{"student_id": ""},
			errContains: "missing required param: student_id",
		},
		{
			name:        "department intent missing department",
			intent:      IntentDepartment,
			params:      map[string]string{},
			errContains: "missing required param: department",
		},
		{
			name:        "department intent empty department",
			intent:      IntentDepartment,
			params:      map[string]string{"department": ""},
			errContains: "missing required param: department",
		},
		{
			name:        "unknown intent",
			intent:      "unknown",
			params:      map[string]string{},
			errContains: "unknown intent",
		},
	}

	// Minimal handler for param validation tests (nil dependencies are acceptable)
	h := &Handler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.DispatchIntent(context.Background(), tt.intent, tt.params)
			if err == nil {
				t.Error("DispatchIntent() expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("DispatchIntent() error = %v, should contain %q", err, tt.errContains)
			}
		})
	}
}

// TestDispatchIntent_Integration tests the full dispatch flow with real dependencies.
// These tests verify that valid parameters correctly route to handler methods.
func TestDispatchIntent_Integration(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name         string
		intent       string
		params       map[string]string
		wantMessages bool // expect at least one message (success or error message)
	}{
		{
			name:         "search intent with name",
			intent:       IntentSearch,
			params:       map[string]string{"name": "王小明"},
			wantMessages: true,
		},
		{
			name:         "student_id intent with valid id",
			intent:       IntentStudentID,
			params:       map[string]string{"student_id": "412345678"},
			wantMessages: true,
		},
		{
			name:         "department intent with department name",
			intent:       IntentDepartment,
			params:       map[string]string{"department": "資工系"},
			wantMessages: true,
		},
		{
			name:         "department intent with department code",
			intent:       IntentDepartment,
			params:       map[string]string{"department": "85"},
			wantMessages: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs, err := h.DispatchIntent(ctx, tt.intent, tt.params)
			if err != nil {
				t.Errorf("DispatchIntent() unexpected error: %v", err)
				return
			}
			if tt.wantMessages && len(msgs) == 0 {
				t.Error("DispatchIntent() expected messages, got none")
			}
		})
	}
}
