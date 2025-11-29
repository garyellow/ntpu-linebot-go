package course

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
			name:   "search intent with keyword",
			intent: IntentSearch,
			params: map[string]string{"keyword": "微積分"},
		},
		{
			name:        "search intent missing keyword",
			intent:      IntentSearch,
			params:      map[string]string{},
			wantErr:     true,
			errContains: "missing required param: keyword",
		},
		{
			name:        "search intent empty keyword",
			intent:      IntentSearch,
			params:      map[string]string{"keyword": ""},
			wantErr:     true,
			errContains: "missing required param: keyword",
		},
		{
			name:   "semantic intent with query",
			intent: IntentSemantic,
			params: map[string]string{"query": "想學程式設計"},
		},
		{
			name:        "semantic intent missing query",
			intent:      IntentSemantic,
			params:      map[string]string{},
			wantErr:     true,
			errContains: "missing required param: query",
		},
		{
			name:   "uid intent with uid",
			intent: IntentUID,
			params: map[string]string{"uid": "1131U0001"},
		},
		{
			name:        "uid intent missing uid",
			intent:      IntentUID,
			params:      map[string]string{},
			wantErr:     true,
			errContains: "missing required param: uid",
		},
		{
			name:        "unknown intent",
			intent:      "unknown",
			params:      map[string]string{},
			wantErr:     true,
			errContains: "unknown intent",
		},
	}

	// Create a minimal handler for testing (nil dependencies will panic on actual calls)
	// This is acceptable for parameter validation tests
	h := &Handler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests that would actually call handler methods (they need full setup)
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
