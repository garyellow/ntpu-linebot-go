package contact

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
			name:   "search intent with query",
			intent: IntentSearch,
			params: map[string]string{"query": "資工系"},
		},
		{
			name:        "search intent missing query",
			intent:      IntentSearch,
			params:      map[string]string{},
			wantErr:     true,
			errContains: "missing required param: query",
		},
		{
			name:        "search intent empty query",
			intent:      IntentSearch,
			params:      map[string]string{"query": ""},
			wantErr:     true,
			errContains: "missing required param: query",
		},
		{
			name:   "emergency intent (no params)",
			intent: IntentEmergency,
			params: map[string]string{},
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
			// Emergency intent doesn't require handler setup for error validation
			if !tt.wantErr && tt.intent != IntentEmergency {
				t.Skip("Skipping test that requires full handler setup")
			}
			// Emergency intent calls handleEmergencyPhones which uses stickerManager
			if tt.intent == IntentEmergency {
				t.Skip("Skipping test that requires stickerManager setup")
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
