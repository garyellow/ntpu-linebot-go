// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
package genai

import (
	"context"
	"testing"
)

func TestIntentModuleMap(t *testing.T) {
	t.Parallel()
	// Verify all expected functions are mapped
	expectedFunctions := []string{
		// Course module
		"course_search",
		"course_smart",
		"course_uid",
		"course_extended",
		"course_historical",
		// ID module
		"id_search",
		"id_student_id",
		"id_department",
		"id_year",
		"id_dept_codes",
		// Contact module
		"contact_search",
		"contact_emergency",
		// Program module
		"program_list",
		"program_search",
		"program_courses",
		// Usage module
		"usage_query",
		// Help
		"help",
		// Direct reply
		"direct_reply",
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

func TestParamKeysMap(t *testing.T) {
	t.Parallel()
	// Verify parameter keys for functions that require parameters
	testCases := []struct {
		funcName     string
		expectedKeys []string
		shouldExist  bool
	}{
		// Course module
		{"course_search", []string{"keyword"}, true},
		{"course_smart", []string{"query"}, true},
		{"course_uid", []string{"uid"}, true},
		{"course_extended", []string{"keyword"}, true},
		{"course_historical", []string{"year", "keyword"}, true}, // Multi-param
		// ID module
		{"id_search", []string{"name"}, true},
		{"id_student_id", []string{"student_id"}, true},
		{"id_department", []string{"department"}, true},
		{"id_year", []string{"year"}, true},
		{"id_dept_codes", []string{"degree"}, true},
		// Contact module
		{"contact_search", []string{"query"}, true},
		{"contact_emergency", nil, false}, // No parameters
		// Program module
		{"program_list", nil, false}, // No parameters
		{"program_search", []string{"query"}, true},
		{"program_courses", []string{"programName"}, true},
		// Help
		{"help", nil, false}, // No parameters
		// Direct reply
		{"direct_reply", []string{"message"}, true},
	}

	for _, tc := range testCases {
		keys, exists := ParamKeysMap[tc.funcName]
		if tc.shouldExist {
			if !exists {
				t.Errorf("Function %s should have param keys but doesn't", tc.funcName)
			} else if len(keys) != len(tc.expectedKeys) {
				t.Errorf("Function %s: expected %d keys, got %d", tc.funcName, len(tc.expectedKeys), len(keys))
			} else {
				for i, expectedKey := range tc.expectedKeys {
					if keys[i] != expectedKey {
						t.Errorf("Function %s: expected key[%d]=%s, got %s", tc.funcName, i, expectedKey, keys[i])
					}
				}
			}
		} else {
			if exists && len(keys) > 0 {
				t.Errorf("Function %s should not have param keys but has: %v", tc.funcName, keys)
			}
		}
	}
}

func TestBuildIntentFunctions(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	parser, err := newGeminiIntentParser(context.Background(), "", "")
	if err != nil {
		t.Errorf("Expected nil error for empty key, got: %v", err)
	}
	if parser != nil {
		t.Error("Expected nil parser for empty key")
	}
}

func TestIntentParser_IsEnabled(t *testing.T) {
	t.Parallel()
	// nil parser
	var nilParser *geminiIntentParser
	if nilParser.IsEnabled() {
		t.Error("nil parser should return false for IsEnabled")
	}

	// parser with nil client
	parserWithNilClient := &geminiIntentParser{client: nil}
	if parserWithNilClient.IsEnabled() {
		t.Error("parser with nil client should return false for IsEnabled")
	}
}

func TestIntentParser_ParseNil(t *testing.T) {
	t.Parallel()
	var nilParser *geminiIntentParser
	_, err := nilParser.Parse(context.Background(), "test")
	if err == nil {
		t.Error("Expected error for nil parser")
	}
}
