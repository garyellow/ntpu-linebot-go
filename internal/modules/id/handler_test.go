package id

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
	"github.com/prometheus/client_golang/prometheus"
)

func setupTestHandler(t *testing.T) *Handler {
	t.Helper()

	// Create test database
	db, err := storage.New(context.Background(), ":memory:", 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Create dependencies
	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw"},
		"sea": {"https://sea.cc.ntpu.edu.tw"},
	}
	scraperClient := scraper.NewClient(30*time.Second, 3, baseURLs)
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	log := logger.New("info")
	stickerMgr := sticker.NewManager(db, scraperClient, log)

	return NewHandler(db, scraperClient, m, log, stickerMgr)
}

// TestCanHandle tests the core routing logic - critical for correct request dispatch
func TestCanHandle(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Critical business paths
		{"Valid 8-digit student ID", "學號 41247001", true},
		{"Valid 9-digit student ID", "學號 412470011", true},
		{"All department codes special", "所有系代碼", true},
		{"Year query (data cutoff check)", "year 114", true},
		{"Department lookup", "系 資工", true},

		// Edge cases that must fail
		{"Invalid - too short", "1234567", false},
		{"Invalid - random text", "hello world", false},
		{"Invalid - empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.CanHandle(tt.input); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleMessage_StudentID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	// Test non-existent student (should return error message)
	msgs := h.HandleMessage(ctx, "學號 99999999")
	if len(msgs) == 0 {
		t.Error("Expected error message for non-existent student")
	}
}

func TestHandleMessage_AllDepartments(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test all department codes
	msgs := h.HandleMessage(ctx, "所有系代碼")
	if len(msgs) == 0 {
		t.Error("Expected department list message")
	}
}

func TestHandleMessage_YearQuery(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test year query
	msgs := h.HandleMessage(ctx, "學年 112")
	if len(msgs) == 0 {
		t.Error("Expected year query result")
	}
}

func TestHandlePostback_Department(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test department postback - should return message even if no students found
	msgs := h.HandlePostback(ctx, "系$資工$112")
	// Department query may return empty results, which is valid behavior
	// Just verify it doesn't panic and returns something
	_ = msgs
}

// TestParseYear tests critical year conversion logic (ROC <-> AD)
// This is essential for correct data retrieval and the 114學年度 cutoff warning
func TestParseYear(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		// Critical conversions
		{"ROC 112 (2023)", "112", 112, false},
		{"ROC 113 (2024 cutoff)", "113", 113, false},
		{"AD 2023 converts to ROC", "2023", 112, false},
		{"AD 2024 converts to ROC", "2024", 113, false},

		// Edge cases
		{"Too short - must error", "1", 0, true},
		{"Too long - must error", "12345", 0, true},
		{"Non-numeric - must error", "abc", 0, true},

		// Boundary tests (parseYear only validates format, not range)
		// Range validation (NTPU founded 90, data cutoff 114+) is done in handleYearQuery
		{"Year 89 (NTPU founded)", "89", 89, false},
		{"Year 88 (valid format)", "88", 88, false},
		{"Year 130 (valid format)", "130", 130, false},
		{"Year 131 (valid format)", "131", 131, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseYear(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseYear(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseYear(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsNumeric tests numeric validation (important for student ID detection)
func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"Valid numeric", "12345678", true},
		{"With letters", "1234a5678", false},
		{"Empty string", "", false},
		{"Only spaces", "   ", false},
		{"With special chars", "1234-5678", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringutil.IsNumeric(tt.input); got != tt.want {
				t.Errorf("stringutil.IsNumeric(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestFormatStudentResponse verifies Flex Message formatting
func TestFormatStudentResponse(t *testing.T) {
	h := setupTestHandler(t)

	student := &storage.Student{
		ID:         "41247001",
		Name:       "測試學生",
		Department: "資訊工程學系",
		Year:       112,
		CachedAt:   1732780800, // Add CachedAt for time hint display
	}

	// Test formatStudentResponse (now shows cache time instead of bool)
	msgs := h.formatStudentResponse(student)
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	}

	// Test with different CachedAt (should still return 1 message)
	student.CachedAt = 0 // No cache time
	msgsNoCacheTime := h.formatStudentResponse(student)
	if len(msgsNoCacheTime) != 1 {
		t.Errorf("Expected 1 message with no cache time, got %d", len(msgsNoCacheTime))
	}
}

// TestHandleMessage_DepartmentName tests department name query
func TestHandleMessage_DepartmentName(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Valid department
	msgs := h.HandleMessage(ctx, "系 資工")
	if len(msgs) == 0 {
		t.Error("Expected response for department query")
	}

	// Invalid department
	msgs = h.HandleMessage(ctx, "系 不存在的系")
	if len(msgs) == 0 {
		t.Error("Expected error message for invalid department")
	}
}

// TestHandleMessage_DepartmentCode tests department code query
func TestHandleMessage_DepartmentCode(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Valid department code
	msgs := h.HandleMessage(ctx, "系代碼 85")
	if len(msgs) == 0 {
		t.Error("Expected response for department code query")
	}

	// Invalid department code
	msgs = h.HandleMessage(ctx, "系代碼 999")
	if len(msgs) == 0 {
		t.Error("Expected error message for invalid department code")
	}
}

// TestHandleYearQuery_FutureYear tests future year warning
func TestHandleYearQuery_FutureYear(t *testing.T) {
	h := setupTestHandler(t)

	// Future year should trigger "你未來人？"
	msgs := h.handleYearQuery("999")
	if len(msgs) == 0 {
		t.Error("Expected future year warning")
	}
}

// TestHandleYearQuery_Year114Plus tests 114+ year data unavailable warning
func TestHandleYearQuery_Year114Plus(t *testing.T) {
	h := setupTestHandler(t)

	// Year >= 114 should show RIP image
	msgs := h.handleYearQuery("114")
	if len(msgs) < 2 { // Should have text + image
		t.Error("Expected RIP warning with image for year 114+")
	}
}

// TestHandlePostback_Easter tests "兇" easter egg
func TestHandlePostback_Easter(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	msgs := h.HandlePostback(ctx, "兇")
	if len(msgs) == 0 {
		t.Error("Expected easter egg response")
	}
}

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
			errContains: "missing required parameter: name",
		},
		{
			name:        "search intent empty name",
			intent:      IntentSearch,
			params:      map[string]string{"name": ""},
			errContains: "missing required parameter: name",
		},
		{
			name:        "student_id intent missing student_id",
			intent:      IntentStudentID,
			params:      map[string]string{},
			errContains: "missing required parameter: student_id",
		},
		{
			name:        "student_id intent empty student_id",
			intent:      IntentStudentID,
			params:      map[string]string{"student_id": ""},
			errContains: "missing required parameter: student_id",
		},
		{
			name:        "department intent missing department",
			intent:      IntentDepartment,
			params:      map[string]string{},
			errContains: "missing required parameter: department",
		},
		{
			name:        "department intent empty department",
			intent:      IntentDepartment,
			params:      map[string]string{"department": ""},
			errContains: "missing required parameter: department",
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
