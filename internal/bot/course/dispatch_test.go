package course

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
			name:        "search intent missing keyword",
			intent:      IntentSearch,
			params:      map[string]string{},
			errContains: "missing required param: keyword",
		},
		{
			name:        "search intent empty keyword",
			intent:      IntentSearch,
			params:      map[string]string{"keyword": ""},
			errContains: "missing required param: keyword",
		},
		{
			name:        "semantic intent missing query",
			intent:      IntentSemantic,
			params:      map[string]string{},
			errContains: "missing required param: query",
		},
		{
			name:        "semantic intent empty query",
			intent:      IntentSemantic,
			params:      map[string]string{"query": ""},
			errContains: "missing required param: query",
		},
		{
			name:        "uid intent missing uid",
			intent:      IntentUID,
			params:      map[string]string{},
			errContains: "missing required param: uid",
		},
		{
			name:        "uid intent empty uid",
			intent:      IntentUID,
			params:      map[string]string{"uid": ""},
			errContains: "missing required param: uid",
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
			name:         "search intent with keyword",
			intent:       IntentSearch,
			params:       map[string]string{"keyword": "微積分"},
			wantMessages: true,
		},
		{
			name:         "search intent with teacher name",
			intent:       IntentSearch,
			params:       map[string]string{"keyword": "王教授"},
			wantMessages: true,
		},
		{
			name:         "uid intent with valid uid",
			intent:       IntentUID,
			params:       map[string]string{"uid": "1141U0001"},
			wantMessages: true,
		},
		// Semantic search requires VectorDB setup, tested separately
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

// TestDispatchIntent_SemanticNoVectorDB tests semantic search fallback when VectorDB is not configured.
func TestDispatchIntent_SemanticNoVectorDB(t *testing.T) {
	h := setupTestHandler(t)
	// VectorDB is nil by default in setupTestHandler
	ctx := context.Background()

	msgs, err := h.DispatchIntent(ctx, IntentSemantic, map[string]string{"query": "想學程式設計"})
	if err != nil {
		t.Errorf("DispatchIntent() unexpected error: %v", err)
		return
	}
	// Should return a message indicating semantic search is not available
	if len(msgs) == 0 {
		t.Error("DispatchIntent() expected fallback message, got none")
	}
}
