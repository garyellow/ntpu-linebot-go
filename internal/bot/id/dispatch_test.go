package id

import (
	"context"
	"strings"
	"testing"
)

func TestDispatchIntent(t *testing.T) {
	// Note: Full integration tests require database and scraper setup.
	// These tests focus on parameter validation logic.

	tests := []struct {
		name        string
		intent      string
		params      map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name:   "search intent with name",
			intent: IntentSearch,
			params: map[string]string{"name": "王小明"},
		},
		{
			name:        "search intent missing name",
			intent:      IntentSearch,
			params:      map[string]string{},
			wantErr:     true,
			errContains: "missing required param: name",
		},
		{
			name:        "search intent empty name",
			intent:      IntentSearch,
			params:      map[string]string{"name": ""},
			wantErr:     true,
			errContains: "missing required param: name",
		},
		{
			name:   "student_id intent with student_id",
			intent: IntentStudentID,
			params: map[string]string{"student_id": "412345678"},
		},
		{
			name:        "student_id intent missing student_id",
			intent:      IntentStudentID,
			params:      map[string]string{},
			wantErr:     true,
			errContains: "missing required param: student_id",
		},
		{
			name:   "department intent with department",
			intent: IntentDepartment,
			params: map[string]string{"department": "資工系"},
		},
		{
			name:        "department intent missing department",
			intent:      IntentDepartment,
			params:      map[string]string{},
			wantErr:     true,
			errContains: "missing required param: department",
		},
		{
			name:        "unknown intent",
			intent:      "unknown",
			params:      map[string]string{},
			wantErr:     true,
			errContains: "unknown intent",
		},
	}

	// Create a minimal handler for testing
	h := &Handler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that would actually call handler methods
			if !tt.wantErr {
				t.Skip("Skipping test that requires full handler setup")
			}

			_, err := h.DispatchIntent(context.Background(), tt.intent, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("DispatchIntent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("DispatchIntent() error = %v, should contain %q", err, tt.errContains)
				}
			}
		})
	}
}
