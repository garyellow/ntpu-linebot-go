// Package genai provides integration with Google's Generative AI APIs.
package genai

import (
	"context"
	"testing"
)

func TestIntentModuleMap(t *testing.T) {
	// Verify all expected functions are mapped
	expectedFunctions := []string{
		"course_search",
		"course_smart",
		"course_uid",
		"id_search",
		"id_student_id",
		"id_department",
		"contact_search",
		"contact_emergency",
		"help",
	}

	for _, funcName := range expectedFunctions {
		moduleIntent, ok := IntentModuleMap[funcName]
		if !ok {
			t.Errorf("Function %s not found in IntentModuleMap", funcName)
			continue
		}
		if moduleIntent[0] == "" {
			t.Errorf("Function %s has empty module", funcName)
		}
	}
}

func TestParamKeyMap(t *testing.T) {
	// Verify parameter keys for functions that require parameters
	testCases := []struct {
		funcName    string
		expectedKey string
		shouldExist bool
	}{
		{"course_search", "keyword", true},
		{"course_smart", "query", true},
		{"course_uid", "uid", true},
		{"id_search", "name", true},
		{"id_student_id", "student_id", true},
		{"id_department", "department", true},
		{"contact_search", "query", true},
		{"contact_emergency", "", false}, // No parameters
		{"help", "", false},              // No parameters
	}

	for _, tc := range testCases {
		key, exists := ParamKeyMap[tc.funcName]
		if tc.shouldExist {
			if !exists {
				t.Errorf("Function %s should have param key but doesn't", tc.funcName)
			} else if key != tc.expectedKey {
				t.Errorf("Function %s: expected key %s, got %s", tc.funcName, tc.expectedKey, key)
			}
		} else {
			if exists && key != "" {
				t.Errorf("Function %s should not have param key but has: %s", tc.funcName, key)
			}
		}
	}
}

func TestBuildIntentFunctions(t *testing.T) {
	funcs := BuildIntentFunctions()

	if len(funcs) == 0 {
		t.Fatal("BuildIntentFunctions returned empty slice")
	}

	// Check that all functions have required fields
	for _, f := range funcs {
		if f.Name == "" {
			t.Error("Function declaration has empty name")
		}
		if f.Description == "" {
			t.Errorf("Function %s has empty description", f.Name)
		}
		// Parameters is optional (e.g., help, contact_emergency)
	}

	// Check that function count matches expected
	expectedCount := len(IntentModuleMap)
	if len(funcs) != expectedCount {
		t.Errorf("Expected %d functions, got %d", expectedCount, len(funcs))
	}
}

func TestNewIntentParser_NilWithEmptyKey(t *testing.T) {
	parser, err := NewIntentParser(context.Background(), "")
	if err != nil {
		t.Errorf("Expected nil error for empty key, got: %v", err)
	}
	if parser != nil {
		t.Error("Expected nil parser for empty key")
	}
}

func TestIntentParser_IsEnabled(t *testing.T) {
	// nil parser
	var nilParser *GeminiIntentParser
	if nilParser.IsEnabled() {
		t.Error("nil parser should return false for IsEnabled")
	}

	// parser with nil client
	parserWithNilClient := &GeminiIntentParser{client: nil}
	if parserWithNilClient.IsEnabled() {
		t.Error("parser with nil client should return false for IsEnabled")
	}
}

func TestIntentParser_ParseNil(t *testing.T) {
	var nilParser *GeminiIntentParser
	_, err := nilParser.Parse(context.Background(), "test")
	if err == nil {
		t.Error("Expected error for nil parser")
	}
}
