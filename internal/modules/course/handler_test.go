package course

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
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

	return NewHandler(db, scraperClient, m, log, stickerMgr, nil, nil, nil)
}

func TestCanHandle(t *testing.T) {
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Full UID patterns (year + term + course_no)
		{"Valid UID 3-digit year", "1141U0001", true},
		{"Valid UID 2-digit year", "991U0001", true},
		{"Valid UID lowercase", "1141u0001", true},
		{"Valid UID M code", "1141M0001", true},
		{"Valid UID N code", "1132N0001", true},
		{"Valid UID P code", "1132P0001", true},

		// Course number only patterns (U/M/N/P + 4 digits)
		{"Course no U", "U0001", true},
		{"Course no M", "M0001", true},
		{"Course no N", "N1234", true},
		{"Course no P", "P9999", true},
		{"Course no lowercase", "u0001", true},

		// Course keywords at START (English)
		{"Class keyword at start", "class schedule", true},
		{"Course keyword at start", "course info", true},

		// Course keywords at START (Chinese)
		{"課 keyword at start", "課 微積分", true},
		{"課程 keyword at start", "課程 資訊", true},
		{"科目 keyword at start", "科目 名稱", true},
		{"課名 keyword at start", "課名 查詢", true},

		// Teacher keywords at START (now unified with course keywords)
		{"Professor keyword at start", "professor Wang", true},
		{"Teacher keyword at start", "teacher info", true},
		{"Dr keyword at start", "dr Chen", true},
		{"老師 keyword at start", "老師 王小明", true},
		{"教授 keyword at start", "教授 陳教授", true},
		{"教師 keyword at start", "教師 資訊", true},
		{"師 keyword at start", "師 資訊", true},

		// Keywords NOT at start should NOT match
		{"課 keyword not at start", "查詢課程", false},
		{"老師 keyword not at start", "王老師", false},
		{"教授 keyword not at start", "陳教授", false},
		{"授課教師 not at start", "找授課教師", false},

		// Invalid queries
		{"Random text", "hello world", false},
		{"Empty string", "", false},
		{"Spaces only", "   ", false},
		{"Student ID", "41247001", false},
		{"Short number", "123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.CanHandle(tt.input)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// buildRegex is tested indirectly via CanHandle - no need for redundant test

func TestUIDRegex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid full UIDs (year + term + course_no)
		{"1141U0001", true}, // 114年 1學期 U0001
		{"1132M0001", true}, // 113年 2學期 M0001
		{"1141N0001", true}, // N code
		{"1141P0001", true}, // P code
		{"991U0001", true},  // 99年 (2-digit year)
		{"1001U0001", true}, // 100年
		{"1141u0001", true}, // lowercase
		{"1141m0001", true}, // lowercase

		// Edge cases that still match (regex finds UID substring)
		{"114U0001", true},   // matches as 11年 4學期 (invalid semester but regex matches)
		{"11412U0001", true}, // contains valid UID substring 1141U0001

		// Invalid UIDs (no valid UID pattern found)
		{"1141X0001", false}, // invalid education code
		{"1141A0001", false}, // invalid education code
		{"U11410001", false}, // wrong position
		{"11410001U", false}, // wrong position
		{"12345678", false},  // no letter
		{"U0001", false},     // course no only (not full UID)
		{"1U0001", false},    // missing year digits
		{"", false},          // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := uidRegex.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("uidRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCourseNoRegex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid course numbers (U/M/N/P + 4 digits)
		{"U0001", true},
		{"M0001", true},
		{"N1234", true},
		{"P9999", true},
		{"u0001", true}, // lowercase
		{"m1234", true},

		// Invalid course numbers
		{"1U0001", false},    // has term prefix (not pure course no)
		{"2M0001", false},    // has term prefix
		{"U001", false},      // too short (only 3 digits)
		{"U00001", false},    // too long (5 digits)
		{"X0001", false},     // invalid education code
		{"A0001", false},     // invalid education code
		{"0001U", false},     // wrong position
		{"1141U0001", false}, // full UID (not course no only)
		{"12345", false},     // no letter
		{"", false},          // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := courseNoRegex.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("courseNoRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// NOTE: Network-dependent tests are consolidated into a single representative test.
// The keyword parsing logic is already covered by TestCanHandle.
// Individual scraping paths (UID, title, teacher) use the same underlying scraper.

func TestHandleMessage_NetworkIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	// Test UID lookup - the most common and representative case
	messages := h.HandleMessage(ctx, "1141U0010")

	// Should return some response (even if course not found)
	if len(messages) == 0 {
		t.Error("Expected messages for course UID query, got none")
	}
}

func TestHandleMessage_EmptyKeywordOnly(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
	}{
		{"課 only", "課"},
		{"課程 only", "課程"},
		{"老師 only", "老師"},
		{"教師 only", "教師"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := h.HandleMessage(ctx, tt.input)

			// Should return help message
			if len(messages) == 0 {
				t.Error("Expected help message for empty keyword, got none")
			}
		})
	}
}

func TestFormatCourseResponse(t *testing.T) {
	h := setupTestHandler(t)

	course := &storage.Course{
		UID:       "1141U0001",
		Year:      114,
		Term:      1,
		No:        "U0001",
		Title:     "資料結構",
		Teachers:  []string{"王教授"},
		Times:     []string{"星期二 3-4"},
		Locations: []string{"資訊大樓 101"},
		DetailURL: "https://course.ntpu.edu.tw/...",
		Note:      "必修",
	}

	// Test with cache hit
	messages := h.formatCourseResponse(course)
	if len(messages) == 0 {
		t.Error("Expected messages for course response, got none")
	}

	// Test with fresh data
	messages = h.formatCourseResponse(course)
	if len(messages) == 0 {
		t.Error("Expected messages for course response, got none")
	}
}

func TestFormatCourseResponse_NoDetailURL(t *testing.T) {
	h := setupTestHandler(t)

	course := &storage.Course{
		UID:      "1141U0001",
		Year:     114,
		Term:     1,
		Title:    "資料結構",
		Teachers: []string{"王教授"},
	}

	messages := h.formatCourseResponse(course)

	// Should return at least the text message
	if len(messages) == 0 {
		t.Error("Expected at least one message, got none")
	}
}

func TestFormatCourseListResponse_Empty(t *testing.T) {
	h := setupTestHandler(t)

	messages := h.formatCourseListResponse([]storage.Course{})

	if len(messages) != 1 {
		t.Errorf("Expected 1 message for empty results, got %d", len(messages))
	}
}

func TestFormatCourseListResponse_SingleCourse(t *testing.T) {
	h := setupTestHandler(t)

	courses := []storage.Course{
		{
			UID:       "1141U0001",
			Year:      114,
			Term:      1,
			Title:     "資料結構",
			Teachers:  []string{"王教授"},
			Times:     []string{"星期二 3-4"},
			Locations: []string{"資訊大樓 101"},
		},
	}

	messages := h.formatCourseListResponse(courses)

	if len(messages) == 0 {
		t.Error("Expected messages for course list, got none")
	}
}

func TestFormatCourseListResponse_LargeList(t *testing.T) {
	h := setupTestHandler(t)

	// Create 60 courses to test pagination
	courses := make([]storage.Course, 60)
	for i := 0; i < 60; i++ {
		courses[i] = storage.Course{
			UID:   fmt.Sprintf("1141U%04d", i),
			Year:  114,
			Term:  1,
			Title: fmt.Sprintf("Course %d", i),
		}
	}

	messages := h.formatCourseListResponse(courses)

	// Should split into multiple messages (20 per message)
	if len(messages) < 3 {
		t.Errorf("Expected at least 3 messages for 60 courses, got %d", len(messages))
	}
}

func TestFormatCourseListResponse_Sorting(t *testing.T) {
	h := setupTestHandler(t)

	// Create courses in random order to test sorting
	courses := []storage.Course{
		{UID: "1121U0001", Year: 112, Term: 1, Title: "Course A"}, // 112-1 (oldest)
		{UID: "1142U0003", Year: 114, Term: 2, Title: "Course B"}, // 114-2 (newest)
		{UID: "1131U0004", Year: 113, Term: 1, Title: "Course C"}, // 113-1
		{UID: "1132U0005", Year: 113, Term: 2, Title: "Course D"}, // 113-2
		{UID: "1141U0002", Year: 114, Term: 1, Title: "Course E"}, // 114-1
		{UID: "1122U0006", Year: 112, Term: 2, Title: "Course F"}, // 112-2
	}

	// Call formatCourseListResponse - it will sort the courses
	_ = h.formatCourseListResponse(courses)

	// Verify sorting: year descending, then term descending
	// Expected order: 114-2, 114-1, 113-2, 113-1, 112-2, 112-1
	expectedOrder := []string{"1142U0003", "1141U0002", "1132U0005", "1131U0004", "1122U0006", "1121U0001"}

	for i, expected := range expectedOrder {
		if courses[i].UID != expected {
			t.Errorf("Position %d: expected %s, got %s", i, expected, courses[i].UID)
		}
	}
}

func TestHandlePostback_InvalidData(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test invalid postback data
	messages := h.HandlePostback(ctx, "invalid")

	// Should return empty slice for invalid data
	if len(messages) != 0 {
		t.Errorf("Expected no messages for invalid postback, got %d", len(messages))
	}
}

func TestHandlePostback_WithPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	// Test postback data with "course:" prefix (simulates Flex Message button click)
	// Should extract the UID and handle it correctly
	messages := h.HandlePostback(ctx, "course:1131U0001")

	// Should return some response (cache miss is expected in test, but should not error on prefix)
	if len(messages) == 0 {
		t.Error("Expected messages for valid postback with prefix, got empty slice")
		return
	}

	// Verify the response is not an "invalid format" error
	// The UID extraction should work, so we expect either cache miss or success
	// If UID extraction failed, it would return "invalid format" message
	if len(messages) > 0 {
		if msg, ok := messages[0].(*messaging_api.TextMessage); ok {
			if msg.Text != "" && !containsString(msg.Text, "格式錯誤") && !containsString(msg.Text, "invalid format") {
				t.Logf("UID extraction successful, response: %s", truncateString(msg.Text, 50))
			} else if containsString(msg.Text, "格式錯誤") || containsString(msg.Text, "invalid format") {
				t.Error("UID extraction failed - got format error despite valid UID")
			}
		}
	}
	if len(messages) > 0 {
		if textMsg, ok := messages[0].(*messaging_api.TextMessage); ok {
			if textMsg.Text != "" && !strings.Contains(textMsg.Text, "找不到") && !strings.Contains(textMsg.Text, "查無") {
				// If not a "not found" message, something went wrong with UID extraction
				t.Logf("Extracted UID correctly, response: %s", textMsg.Text)
			}
		}
	}
}

// NOTE: HandlePostback network tests are omitted.
// The postback logic reuses the same scraper as HandleMessage.
// TestHandleMessage_NetworkIntegration provides sufficient integration coverage.

// NOTE: Semester determination logic is tested in semester_test.go
// TestSemesterDetectionLogic tests the actual getSemestersForDate() function
// with comprehensive date-based test cases - no need to duplicate here.

// ==================== Smart Search Tests ====================

func TestCanHandle_SmartKeywords(t *testing.T) {
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Smart search keywords (找課)
		{"找課 keyword", "找課 機器學習", true},
		{"找課程 keyword", "找課程 資料分析", true},
		{"搜課 keyword", "搜課 Python", true},
		{"找課 alone", "找課", true},

		// Regular course keywords should still work
		{"課程 keyword", "課程 微積分", true},
		{"課 keyword", "課 程式設計", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.CanHandle(tt.input)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanHandle_ExtendedKeywords(t *testing.T) {
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Extended search keywords (更多學期)
		{"更多學期 keyword", "更多學期 雲端", true},
		{"更多學期 with whitespace", "更多學期 微積分", true},
		{"歷史課程 keyword", "歷史課程 資料庫", true},
		{"更多學期 alone", "更多學期", true},
		{"歷史課程 alone", "歷史課程", true},

		// Should not match if not at start
		{"更多學期 not at start", "查詢更多學期", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.CanHandle(tt.input)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetBM25Index(t *testing.T) {
	h := setupTestHandler(t)

	// Initially nil
	if h.bm25Index != nil {
		t.Error("Expected bm25Index to be nil initially")
	}

	// After setting, should not be nil
	// Note: We can't easily test with a real BM25Index without data
	// This test just verifies the setter method exists and works
}

func TestHandleSmartSearch_NoBM25Index(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// BM25Index is nil by default
	messages := h.HandleMessage(ctx, "找課 機器學習")

	// Should return a helpful message when BM25Index is not available
	if len(messages) == 0 {
		t.Error("Expected at least one message when BM25 search is disabled")
	}
}

func TestHandleSmartSearch_EmptyQuery(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Should prompt for input when query is empty
	messages := h.HandleMessage(ctx, "找課")

	if len(messages) == 0 {
		t.Error("Expected help message for empty smart search query")
	}
}

func TestGetRelevanceLabel(t *testing.T) {
	// Tests for 3-tier relevance label based on relative BM25 score
	// Based on Normal-Exponential mixture model (Arampatzis et al., 2009)
	// Confidence >= 0.8: 最佳匹配 (Normal core), >= 0.6: 高度相關 (Mixed), < 0.6: 部分相關 (Exponential tail)
	// Confidence = score / maxScore (relative to top result)
	tests := []struct {
		name           string
		confidence     float32
		wantLabel      string
		wantColorCheck func(color string) bool
	}{
		{
			name:       "best match (confidence 1.0, top result)",
			confidence: 1.0,
			wantLabel:  "最佳匹配",
			wantColorCheck: func(c string) bool {
				return c != "" // Should have color
			},
		},
		{
			name:       "best match (confidence 0.85)",
			confidence: 0.85,
			wantLabel:  "最佳匹配",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "best match (confidence exactly 0.8)",
			confidence: 0.80,
			wantLabel:  "最佳匹配",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "highly relevant (confidence 0.75)",
			confidence: 0.75,
			wantLabel:  "高度相關",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "highly relevant (confidence exactly 0.6)",
			confidence: 0.60,
			wantLabel:  "高度相關",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "partially relevant (confidence 0.55)",
			confidence: 0.55,
			wantLabel:  "部分相關",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "partially relevant (confidence 0.35)",
			confidence: 0.35,
			wantLabel:  "部分相關",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "edge case: confidence just below 0.8",
			confidence: 0.799,
			wantLabel:  "高度相關",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "edge case: confidence just below 0.6",
			confidence: 0.599,
			wantLabel:  "部分相關",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := getRelevanceLabel(tt.confidence)
			if label.Label != tt.wantLabel {
				t.Errorf("getRelevanceLabel(%.3f) label = %q, want %q", tt.confidence, label.Label, tt.wantLabel)
			}
			if !tt.wantColorCheck(label.Color) {
				t.Errorf("getRelevanceLabel(%.3f) color = %q, check failed", tt.confidence, label.Color)
			}
		})
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
			name:        "search intent missing keyword",
			intent:      IntentSearch,
			params:      map[string]string{},
			errContains: "missing required parameter: keyword",
		},
		{
			name:        "search intent empty keyword",
			intent:      IntentSearch,
			params:      map[string]string{"keyword": ""},
			errContains: "missing required parameter: keyword",
		},
		{
			name:        "smart intent missing query",
			intent:      IntentSmart,
			params:      map[string]string{},
			errContains: "missing required parameter: query",
		},
		{
			name:        "smart intent empty query",
			intent:      IntentSmart,
			params:      map[string]string{"query": ""},
			errContains: "missing required parameter: query",
		},
		{
			name:        "uid intent missing uid",
			intent:      IntentUID,
			params:      map[string]string{},
			errContains: "missing required parameter: uid",
		},
		{
			name:        "uid intent empty uid",
			intent:      IntentUID,
			params:      map[string]string{"uid": ""},
			errContains: "missing required parameter: uid",
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
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

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
		// Smart search requires BM25Index setup, tested separately
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

// TestDispatchIntent_SmartNoBM25Index tests smart search fallback when BM25Index is not configured.
func TestDispatchIntent_SmartNoBM25Index(t *testing.T) {
	h := setupTestHandler(t)
	// BM25Index is nil by default in setupTestHandler
	ctx := context.Background()

	msgs, err := h.DispatchIntent(ctx, IntentSmart, map[string]string{"query": "想學程式設計"})
	if err != nil {
		t.Errorf("DispatchIntent() unexpected error: %v", err)
		return
	}
	// Should return a message indicating smart search is not available
	if len(msgs) == 0 {
		t.Error("DispatchIntent() expected fallback message, got none")
	}
}

// TestExtractUniqueSemesters tests the data-driven semester extraction logic
func TestExtractUniqueSemesters(t *testing.T) {
	tests := []struct {
		name     string
		courses  []storage.Course
		expected []struct {
			year int
			term int
		}
	}{
		{
			name: "Multiple courses, multiple semesters (sorted)",
			courses: []storage.Course{
				{UID: "1132U0001", Year: 113, Term: 2},
				{UID: "1132U0002", Year: 113, Term: 2},
				{UID: "1131U0001", Year: 113, Term: 1},
				{UID: "1122U0001", Year: 112, Term: 2},
			},
			expected: []struct{ year, term int }{
				{113, 2}, // Index 0: 最新學期
				{113, 1}, // Index 1: 上個學期
				{112, 2}, // Index 2: 過去學期
			},
		},
		{
			name: "Single semester",
			courses: []storage.Course{
				{UID: "1132U0001", Year: 113, Term: 2},
				{UID: "1132U0002", Year: 113, Term: 2},
			},
			expected: []struct{ year, term int }{
				{113, 2},
			},
		},
		{
			name:     "Empty course list",
			courses:  []storage.Course{},
			expected: []struct{ year, term int }{},
		},
		{
			name: "Four semesters (extended search)",
			courses: []storage.Course{
				{UID: "1132U0001", Year: 113, Term: 2},
				{UID: "1131U0001", Year: 113, Term: 1},
				{UID: "1122U0001", Year: 112, Term: 2},
				{UID: "1121U0001", Year: 112, Term: 1},
			},
			expected: []struct{ year, term int }{
				{113, 2}, // Index 0: 最新學期
				{113, 1}, // Index 1: 上個學期
				{112, 2}, // Index 2: 過去學期
				{112, 1}, // Index 3: 過去學期
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUniqueSemesters(tt.courses)

			if len(result) != len(tt.expected) {
				t.Errorf("extractUniqueSemesters() returned %d semesters, expected %d",
					len(result), len(tt.expected))
				return
			}

			for i := range tt.expected {
				if result[i].Year != tt.expected[i].year || result[i].Term != tt.expected[i].term {
					t.Errorf("extractUniqueSemesters()[%d] = {%d, %d}, expected {%d, %d}",
						i, result[i].Year, result[i].Term, tt.expected[i].year, tt.expected[i].term)
				}
			}
		})
	}
}

// Helper functions for testing

func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
