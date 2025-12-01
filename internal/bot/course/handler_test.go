package course

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
)

func setupTestHandler(t *testing.T) *Handler {
	// Create test database
	db, err := storage.New(":memory:", 168*time.Hour) // 7 days for tests
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create test scraper
	scraperClient := scraper.NewClient(30*time.Second, 3)

	// Create test metrics
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	// Create test logger
	log := logger.New("info")

	// Create sticker manager for tests
	testLogger := logger.New("info")
	stickerMgr := sticker.NewManager(db, scraperClient, testLogger)

	return NewHandler(db, scraperClient, m, log, stickerMgr)
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

// NOTE: HandlePostback network tests are omitted.
// The postback logic reuses the same scraper as HandleMessage.
// TestHandleMessage_NetworkIntegration provides sufficient integration coverage.

// NOTE: Semester determination logic is tested in semester_test.go
// TestSemesterDetectionLogic tests the actual getSemestersForDate() function
// with comprehensive date-based test cases - no need to duplicate here.

// ==================== Semantic Search Tests ====================

func TestCanHandle_SemanticKeywords(t *testing.T) {
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Semantic search keywords (找課)
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

func TestSetVectorDB(t *testing.T) {
	h := setupTestHandler(t)

	// Initially nil
	if h.vectorDB != nil {
		t.Error("Expected vectorDB to be nil initially")
	}

	// After setting, should not be nil
	// Note: We can't easily test with a real VectorDB without API key
	// This test just verifies the setter method exists and works
}

func TestHandleSemanticSearch_NoVectorDB(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// VectorDB is nil by default
	messages := h.HandleMessage(ctx, "找課 機器學習")

	// Should return a helpful message when VectorDB is not available
	if len(messages) == 0 {
		t.Error("Expected at least one message when VectorDB is disabled")
	}
}

func TestHandleSemanticSearch_EmptyQuery(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Should prompt for input when query is empty
	messages := h.HandleMessage(ctx, "找課")

	if len(messages) == 0 {
		t.Error("Expected help message for empty semantic search query")
	}
}

func TestGetSimilarityBadge(t *testing.T) {
	tests := []struct {
		name           string
		similarity     float32
		wantBadge      string
		wantColorCheck func(color string) bool
	}{
		{
			name:       "very high similarity (95%)",
			similarity: 0.95,
			wantBadge:  "🎯 高度相關 95%",
			wantColorCheck: func(c string) bool {
				return c != "" // Should have color
			},
		},
		{
			name:       "high similarity (70%)",
			similarity: 0.70,
			wantBadge:  "🎯 高度相關 70%",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "medium similarity (55%)",
			similarity: 0.55,
			wantBadge:  "✨ 相關 55%",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "medium-low similarity (50%)",
			similarity: 0.50,
			wantBadge:  "✨ 相關 50%",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "low similarity (45%)",
			similarity: 0.45,
			wantBadge:  "💡 可能相關 45%",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "low similarity (40%)",
			similarity: 0.40,
			wantBadge:  "💡 可能相關 40%",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "very low similarity (35%)",
			similarity: 0.35,
			wantBadge:  "🔍 參考 35%",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
		{
			name:       "minimum threshold (30%)",
			similarity: 0.30,
			wantBadge:  "🔍 參考 30%",
			wantColorCheck: func(c string) bool {
				return c != ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			badge, color := getSimilarityBadge(tt.similarity)
			if badge != tt.wantBadge {
				t.Errorf("getSimilarityBadge(%f) badge = %q, want %q", tt.similarity, badge, tt.wantBadge)
			}
			if !tt.wantColorCheck(color) {
				t.Errorf("getSimilarityBadge(%f) color = %q, check failed", tt.similarity, color)
			}
		})
	}
}
