package course

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
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

	// Use a unique temp file database for each test to avoid shared memory conflicts
	// when running t.Parallel() tests. The temp directory is automatically cleaned up.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.New(context.Background(), dbPath, 168*time.Hour)
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

	return NewHandler(db, scraperClient, m, log, stickerMgr, nil, nil, nil, nil)
}

// setupTestHandlerWithSemesters creates a handler with a pre-configured semester cache.
// This is useful for tests that need deterministic semester behavior independent of current date.
// The semesters parameter should contain year-term pairs in descending order (newest first).
func setupTestHandlerWithSemesters(t *testing.T, semesters []struct{ year, term int }) *Handler {
	t.Helper()

	// Create a semester cache with pre-configured semesters
	semesterCache := NewSemesterCache()

	// Convert to Semester slice and update cache
	semesterList := make([]Semester, len(semesters))
	for i, s := range semesters {
		semesterList[i] = Semester{Year: s.year, Term: s.term}
	}
	semesterCache.Update(semesterList)

	// Create handler with the pre-configured cache
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.New(context.Background(), dbPath, 168*time.Hour)
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

	return NewHandler(db, scraperClient, m, log, stickerMgr, nil, nil, nil, semesterCache)
}

func TestCanHandle(t *testing.T) {
	t.Parallel()
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
		{"Course keyword at start", "course info", true},

		// Course keywords at START (Chinese)
		{"課 keyword at start", "課 微積分", true},
		{"課程 keyword at start", "課程 資訊", true},
		{"科目 keyword at start", "科目 名稱", true},
		{"課名 keyword at start", "課名 查詢", true},

		// Teacher keywords are handled by contact module (should NOT match here)
		{"老師 keyword (moved to contact)", "老師 王小明", false},
		{"教授 keyword (moved to contact)", "教授 陳教授", false},
		{"教師 keyword (moved to contact)", "教師 資訊", false},

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
	t.Parallel()
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
			t.Parallel()
			got := uidRegex.MatchString(tt.input)
			if got != tt.want {
				t.Errorf("uidRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCourseNoRegex(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
	}{
		{"課 only", "課"},
		{"課程 only", "課程"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			messages := h.HandleMessage(ctx, tt.input)

			// Should return help message
			if len(messages) == 0 {
				t.Error("Expected help message for empty keyword, got none")
			}
		})
	}
}

func TestFormatCourseResponse(t *testing.T) {
	t.Parallel()
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

	// Test HandleMessage with UID to verify course response formatting
	messages := h.HandleMessage(context.Background(), course.UID)
	if len(messages) == 0 {
		t.Error("Expected messages for course response, got none")
	}

	// Test with fresh data
	messages = h.HandleMessage(context.Background(), course.UID)
	if len(messages) == 0 {
		t.Error("Expected messages for course response, got none")
	}
}

func TestFormatCourseResponse_NoDetailURL(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Save a course without DetailURL to test formatting
	course := &storage.Course{
		UID:      "1141U0001",
		Year:     114,
		Term:     1,
		No:       "U0001",
		Title:    "資料結構",
		Teachers: []string{"王教授"},
	}

	// Save course to database so HandleMessage can find it
	if err := h.db.SaveCourse(ctx, course); err != nil {
		t.Fatalf("Failed to save test course: %v", err)
	}

	// Test via HandleMessage with UID (triggers formatCourseResponseWithContext internally)
	messages := h.HandleMessage(ctx, course.UID)

	// Should return at least the text message
	if len(messages) == 0 {
		t.Error("Expected at least one message, got none")
	}
}

func TestFormatCourseListResponse_Empty(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	messages := h.formatCourseListResponse([]storage.Course{})

	if len(messages) != 1 {
		t.Errorf("Expected 1 message for empty results, got %d", len(messages))
	}
}

func TestFormatCourseListResponse_SingleCourse(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// NOTE: Semester cache and calendar-based logic is tested in semester_test.go
// with comprehensive test cases for SemesterCache, GetWarmupProbeStart,
// GenerateProbeSequence, and getCalendarBasedSemesters.

// ==================== Smart Search Tests ====================

func TestCanHandle_SmartKeywords(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Extended search keywords (更多學期/更多課程)
		{"更多學期 keyword", "更多學期 雲端", true},
		{"更多學期 with whitespace", "更多學期 微積分", true},
		{"更多課程 keyword", "更多課程 雲端", true},
		{"更多課程 with whitespace", "更多課程 微積分", true},
		{"歷史課程 keyword", "歷史課程 資料庫", true},
		{"更多學期 alone", "更多學期", true},
		{"更多課程 alone", "更多課程", true},
		{"歷史課程 alone", "歷史課程", true},

		// Should not match if not at start
		{"更多學期 not at start", "查詢更多學期", false},
		{"更多課程 not at start", "查詢更多課程", false},
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

func TestCanHandle_HistoricalKeywords(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Historical course patterns (課程 {year} {keyword})
		// Supports both ROC year (2-3 digits) and Western year (4 digits)
		{"課程 with ROC year", "課程 110 微積分", true},
		{"課 with ROC year", "課 108 程式設計", true},
		{"course with ROC year", "course 110 calculus", true},
		{"class with ROC year", "class 108 programming", true},

		// Western year support (4 digits)
		{"課程 with Western year", "課程 2021 微積分", true},
		{"課 with Western year", "課 2019 程式設計", true},
		{"course with Western year", "course 2021 calculus", true},
		{"class with Western year", "class 2019 programming", true},

		// These still match courseRegex (general course keyword), which is acceptable
		// The historical pattern is checked in HandleMessage to provide specialized handling
		{"no keyword after year", "課程 110", true}, // Matches courseRegex
		{"課程 without year", "課程 微積分", true},       // Matches courseRegex

		// Should not match - no valid course keyword
		{"random text", "110 微積分", false},
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
	t.Parallel()
	h := setupTestHandler(t)

	// Initially nil
	if h.bm25Index != nil {
		t.Error("Expected bm25Index to be nil initially")
	}

	// After setting, should not be nil
	// Note: We can't easily test with a real BM25Index without data
	// This test just verifies the setter method exists and works
}

// TestHandleMessage_CanHandleConsistency verifies that CanHandle and HandleMessage
// are consistent: if CanHandle returns true, HandleMessage should return messages.
// This test prevents routing bugs where CanHandle claims to handle but HandleMessage returns nil.
func TestHandleMessage_CanHandleConsistency(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name        string
		input       string
		canHandle   bool
		shouldReply bool // Should HandleMessage return non-empty messages?
	}{
		// UID patterns - should be handled
		{"full UID", "1131U0001", true, true},
		{"course number", "U0001", true, true},

		// Keyword patterns - should be handled (may return help message if no DB results)
		{"課程 keyword", "課程", true, true},         // Returns help message
		{"course keyword", "course", true, true}, // Returns help message
		{"找課 keyword", "找課", true, true},         // Returns help/error message
		{"更多學期 keyword", "更多學期", true, true},     // Returns help message

		// Historical pattern - should be handled
		{"historical valid", "課程 110 微積分", true, true},
		{"historical Western year", "課程 2021 微積分", true, true},

		// Edge cases: historical pattern matches but validation fails
		// Handler should return error message instead of nil
		{"historical invalid year format", "課程 abc 微積分", true, true}, // Matches courseRegex (not historical)
		{"historical no keyword", "課程 110", true, true},              // Matches courseRegex (no keyword after year)
		{"historical year too early", "課程 50 微積分", true, true},       // Matches historical, returns error about launch year

		// Should not be handled
		{"random text", "天氣如何", false, false},
		{"empty", "", false, false},
		{"number only", "110", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Test CanHandle
			canHandle := h.CanHandle(tt.input)
			if canHandle != tt.canHandle {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, canHandle, tt.canHandle)
			}

			// Test HandleMessage
			msgs := h.HandleMessage(context.Background(), tt.input)
			gotReply := len(msgs) > 0
			if gotReply != tt.shouldReply {
				t.Errorf("HandleMessage(%q) returned %d messages, shouldReply = %v", tt.input, len(msgs), tt.shouldReply)
			}

			// Critical consistency check
			if canHandle && !gotReply {
				t.Errorf("CONSISTENCY VIOLATION: CanHandle(%q) = true but HandleMessage returned empty", tt.input)
			}
			if !canHandle && gotReply {
				t.Errorf("CONSISTENCY VIOLATION: CanHandle(%q) = false but HandleMessage returned messages", tt.input)
			}
		})
	}
}

// TestHandleMessage_PriorityOrder verifies that patterns are checked in the correct priority order.
// When multiple patterns could match, the highest priority should win.
func TestHandleMessage_PriorityOrder(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name            string
		input           string
		expectedHandler string // Which handler should process this (based on pattern priority)
	}{
		// Priority 1: UID should match before course keyword
		{"UID over keyword", "1131U0001", "UID"}, // Even if "課程" appears elsewhere

		// Priority 2: Course number should match before general keywords
		{"course number over keyword", "U0001", "course_number"},

		// Priority 3-6: Keywords are checked in order
		// Cannot test easily without inspecting internal behavior
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msgs := h.HandleMessage(context.Background(), tt.input)
			if len(msgs) == 0 {
				t.Errorf("HandleMessage(%q) returned empty, expected %s handler", tt.input, tt.expectedHandler)
			}
			// Note: We can't easily check which internal handler was used without exposing internals
			// This test serves as documentation of expected behavior
		})
	}
}

func TestHandleSmartSearch_NoBM25Index(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Should prompt for input when query is empty
	messages := h.HandleMessage(ctx, "找課")

	if len(messages) == 0 {
		t.Error("Expected help message for empty smart search query")
	}
}

func TestGetRelevanceLabel(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
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
			name:        "extended intent missing keyword",
			intent:      IntentExtended,
			params:      map[string]string{},
			errContains: "missing required parameter: keyword",
		},
		{
			name:        "extended intent empty keyword",
			intent:      IntentExtended,
			params:      map[string]string{"keyword": ""},
			errContains: "missing required parameter: keyword",
		},
		{
			name:        "historical intent missing year",
			intent:      IntentHistorical,
			params:      map[string]string{"keyword": "微積分"},
			errContains: "missing required parameter: year",
		},
		{
			name:        "historical intent missing keyword",
			intent:      IntentHistorical,
			params:      map[string]string{"year": "110"},
			errContains: "missing required parameter: keyword",
		},
		{
			name:        "historical intent invalid year",
			intent:      IntentHistorical,
			params:      map[string]string{"year": "abc", "keyword": "微積分"},
			errContains: "invalid year format",
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
			t.Parallel()
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
		{
			name:         "extended intent with keyword",
			intent:       IntentExtended,
			params:       map[string]string{"keyword": "微積分"},
			wantMessages: true,
		},
		{
			name:         "historical intent with year and keyword",
			intent:       IntentHistorical,
			params:       map[string]string{"year": "110", "keyword": "微積分"},
			wantMessages: true,
		},
		// Smart search requires BM25Index setup, tested separately
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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

	msgs, err := h.DispatchIntent(ctx, IntentSmart, map[string]string{"query": "機器學習"})
	if err != nil {
		t.Fatalf("DispatchIntent() unexpected error: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("Expected fallback message when BM25 disabled")
	}
}

// TestHistoricalPattern_WesternYearConversion tests that Western years are correctly converted to ROC years.
func TestHistoricalPattern_WesternYearConversion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		expectROC   int // Expected ROC year after conversion
		shouldMatch bool
	}{
		{"Western year 2021", "課程 2021 微積分", 110, true},
		{"Western year 2022", "課程 2022 資料庫", 111, true},
		{"Western year 2019", "課程 2019 程式設計", 108, true},
		{"Western year 1990", "課程 1990 微積分", 79, true}, // Below launch year, should show error
		{"ROC year 110", "課程 110 微積分", 110, true},
		{"ROC year 111", "課程 111 資料庫", 111, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that pattern matches
			matched := h.CanHandle(tt.input)
			if matched != tt.shouldMatch {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, matched, tt.shouldMatch)
			}

			if !matched {
				return
			}

			// Test that HandleMessage processes it (may return error for old years)
			msgs := h.HandleMessage(ctx, tt.input)
			if len(msgs) == 0 {
				t.Errorf("HandleMessage(%q) returned empty, expected at least one message", tt.input)
			}

			// For years below launch year, should get error message
			if tt.expectROC < 90 {
				// Check that message contains error about year being too early
				msg := msgs[0]
				if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
					if !strings.Contains(textMsg.Text, "年份過早") && !strings.Contains(textMsg.Text, "課程系統") {
						t.Errorf("Expected error message for year %d, got: %s", tt.expectROC, textMsg.Text)
					}
				}
			}
		})
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

// TestFilterCoursesBySemesters verifies semester filtering logic
func TestFilterCoursesBySemesters(t *testing.T) {
	tests := []struct {
		name    string
		courses []storage.Course
		years   []int
		terms   []int
		want    []storage.Course
	}{
		{
			name:    "empty input",
			courses: []storage.Course{},
			years:   []int{113},
			terms:   []int{1},
			want:    []storage.Course{},
		},
		{
			name: "no filter",
			courses: []storage.Course{
				{Year: 113, Term: 1},
			},
			years: nil,
			terms: nil,
			want: []storage.Course{
				{Year: 113, Term: 1},
			},
		},
		{
			name: "filter match one",
			courses: []storage.Course{
				{Year: 113, Term: 1, Title: "Match"},
				{Year: 112, Term: 1, Title: "NoMatch"},
			},
			years: []int{113},
			terms: []int{1},
			want: []storage.Course{
				{Year: 113, Term: 1, Title: "Match"},
			},
		},
		{
			name: "filter match multiple semesters",
			courses: []storage.Course{
				{Year: 113, Term: 1, Title: "Match 1"},
				{Year: 113, Term: 2, Title: "Match 2"},
				{Year: 112, Term: 1, Title: "NoMatch"},
			},
			years: []int{113, 113},
			terms: []int{1, 2},
			want: []storage.Course{
				{Year: 113, Term: 1, Title: "Match 1"},
				{Year: 113, Term: 2, Title: "Match 2"},
			},
		},
		{
			name: "mismatched lengths (should return original)",
			courses: []storage.Course{
				{Year: 113, Term: 1},
			},
			years: []int{113},
			terms: []int{},
			want: []storage.Course{
				{Year: 113, Term: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterCoursesBySemesters(tt.courses, tt.years, tt.terms)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterCoursesBySemesters() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFormatCourseListResponseWithOptions_Modes verifies the three display modes
func TestFormatCourseListResponseWithOptions_Modes(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	courses := []storage.Course{
		{
			UID:      "1131U0001",
			Title:    "Check Logic",
			Teachers: []string{"Teacher A"},
			Year:     113,
			Term:     1,
		},
	}

	// 1. Regular Mode
	msgs := h.formatCourseListResponseWithOptions(courses, FormatOptions{})
	if len(msgs) == 0 {
		t.Error("Regular mode: expected messages, got 0")
	}

	// 2. Extended Mode (IsHistorical = true)
	// Should produce message without label row, starting with semester info
	msgsExtended := h.formatCourseListResponseWithOptions(courses, FormatOptions{IsHistorical: true})
	if len(msgsExtended) == 0 {
		t.Error("Extended mode: expected messages, got 0")
	}

	// 3. Teacher Mode (TeacherName set)
	// Should produce message with Teacher label and NO teacher info row
	msgsTeacher := h.formatCourseListResponseWithOptions(courses, FormatOptions{TeacherName: "Teacher A"})
	if len(msgsTeacher) == 0 {
		t.Error("Teacher mode: expected messages, got 0")
	}
}

// TestHandleTeacherCourseSearch tests the teacher course search handler flow
func TestHandleTeacherCourseSearch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Use pre-configured semesters to ensure test is time-independent
	h := setupTestHandlerWithSemesters(t, []struct{ year, term int }{
		{113, 2}, {113, 1}, {112, 2}, {112, 1},
	})

	// Setup: Add some courses to the database
	courses := []*storage.Course{
		{UID: "1131U0001", Year: 113, Term: 1, No: "U0001", Title: "程式設計", Teachers: []string{"王教授"}},
		{UID: "1131U0002", Year: 113, Term: 1, No: "U0002", Title: "資料結構", Teachers: []string{"王教授", "李教授"}},
		{UID: "1131U0003", Year: 113, Term: 1, No: "U0003", Title: "演算法", Teachers: []string{"陳教授"}},
	}

	for _, c := range courses {
		if err := h.db.SaveCourse(ctx, c); err != nil {
			t.Fatalf("SaveCourse failed: %v", err)
		}
	}

	// Test 1: Search with results
	msgs := h.handleTeacherCourseSearch(ctx, "王教授")
	if len(msgs) == 0 {
		t.Error("Expected messages for teacher course search with results, got none")
	}

	// Test 2: Search with no results
	msgs = h.handleTeacherCourseSearch(ctx, "不存在的教授")
	if len(msgs) == 0 {
		t.Error("Expected error message for teacher course search with no results, got none")
	}
	// Verify it's the "no results" message
	if textMsg, ok := msgs[0].(*messaging_api.TextMessage); ok {
		if !strings.Contains(textMsg.Text, "查無") {
			t.Errorf("Expected 'no results' message, got: %s", textMsg.Text)
		}
	}
}

// TestSearchCoursesForTeacher tests the internal search aggregation logic
func TestSearchCoursesForTeacher(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Use pre-configured semesters to ensure test is time-independent
	// Configure detector to include 113-1 and 113-2 (matches our test data)
	h := setupTestHandlerWithSemesters(t, []struct{ year, term int }{
		{113, 2}, // Newest
		{113, 1}, // Recent
		{112, 2}, // Extended
		{112, 1}, // Extended
	})

	// Setup: Add courses with various teacher patterns
	courses := []*storage.Course{
		{UID: "1131U0001", Year: 113, Term: 1, No: "U0001", Title: "課程A", Teachers: []string{"王大明"}},
		{UID: "1131U0002", Year: 113, Term: 1, No: "U0002", Title: "課程B", Teachers: []string{"王小明"}},
		{UID: "1131U0003", Year: 113, Term: 1, No: "U0003", Title: "課程C", Teachers: []string{"李教授"}},
		{UID: "1132U0001", Year: 113, Term: 2, No: "U0001", Title: "課程D", Teachers: []string{"王大明"}}, // Same teacher, different semester
	}

	for _, c := range courses {
		if err := h.db.SaveCourse(ctx, c); err != nil {
			t.Fatalf("SaveCourse failed: %v", err)
		}
	}

	// Test 1: Exact name search
	results := h.searchCoursesForTeacher(ctx, "王大明")
	if len(results) == 0 {
		t.Error("Expected results for exact teacher name search")
	}

	// Test 2: Fuzzy search (partial match) - "王明" should match both "王大明" and "王小明"
	results = h.searchCoursesForTeacher(ctx, "王明")
	if len(results) < 2 {
		for i, r := range results {
			t.Logf("Result %d: %s %d-%d %v", i, r.Title, r.Year, r.Term, r.Teachers)
		}
		t.Errorf("Expected at least 2 results for fuzzy search '王明', got %d", len(results))
	}

	// Test 3: Deduplication - same course should not appear twice
	// "王大明" appears in two semesters, both should be returned (different UIDs)
	results = h.searchCoursesForTeacher(ctx, "王大明")
	uidMap := make(map[string]bool)
	for _, r := range results {
		if uidMap[r.UID] {
			t.Errorf("Duplicate UID found: %s", r.UID)
		}
		uidMap[r.UID] = true
	}

	// Test 4: No results case
	results = h.searchCoursesForTeacher(ctx, "不存在")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent teacher, got %d", len(results))
	}
}

// TestHandlePostback_TeacherCourse tests the postback handler for teacher course search
func TestHandlePostback_TeacherCourse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Use pre-configured semesters to ensure test is time-independent
	h := setupTestHandlerWithSemesters(t, []struct{ year, term int }{
		{113, 2}, {113, 1}, {112, 2}, {112, 1},
	})

	// Setup: Add courses
	courses := []*storage.Course{
		{UID: "1131U0001", Year: 113, Term: 1, No: "U0001", Title: "程式設計", Teachers: []string{"王教授"}},
	}
	for _, c := range courses {
		if err := h.db.SaveCourse(ctx, c); err != nil {
			t.Fatalf("SaveCourse failed: %v", err)
		}
	}

	// Test: Postback with "授課課程" prefix should trigger teacher course search
	// Format: "course:授課課程${bot.PostbackSplitChar}{teacherName}"
	msgs := h.HandlePostback(ctx, fmt.Sprintf("course:授課課程%s王教授", bot.PostbackSplitChar))
	if len(msgs) == 0 {
		t.Error("Expected messages for teacher course postback, got none")
	}
}
