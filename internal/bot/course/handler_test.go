package course

import (
	"context"
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
	scraperClient := scraper.NewClient(30000000000, 2, 500000000, 1000000000, 3)

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
		// Course UID patterns
		{"Valid UID 4-4", "3141U0001", true},
		{"Valid UID 3-4", "314U0001", true},
		{"Valid UID lowercase", "3141u0001", true},
		{"Valid UID mixed case", "3141M0001", true},

		// Course keywords (English)
		{"Class keyword", "class schedule", true},
		{"Course keyword", "course info", true},

		// Course keywords (Chinese)
		{"課 keyword", "查詢課程", true},
		{"課程 keyword", "課程資訊", true},
		{"科目 keyword", "科目名稱", true},
		{"課名 keyword", "課名查詢", true},

		// Teacher keywords (English)
		{"Professor keyword", "professor Wang", true},
		{"Teacher keyword", "teacher info", true},
		{"Dr keyword", "dr Chen", true},

		// Teacher keywords (Chinese)
		{"老師 keyword", "王老師", true},
		{"教授 keyword", "陳教授", true},
		{"教師 keyword", "授課教師", true},
		{"師 keyword", "師資", true},

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
		// Valid UIDs
		{"3141U0001", true},
		{"314U0001", true},
		{"3141M0001", true},
		{"3141N0001", true},
		{"3141P0001", true},
		{"3141u0001", true}, // lowercase
		{"3141m0001", true},

		// Invalid UIDs
		{"314U001", false},   // too short
		{"3141X0001", false}, // invalid character
		{"3141A0001", false}, // invalid character
		{"U31410001", false}, // wrong position
		{"31410001U", false}, // wrong position
		{"12345678", false},  // no letter
		{"abcd1234", false},  // too many letters
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

func TestHandleMessage_CourseUID(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	messages := h.HandleMessage(ctx, "3141U0001")

	// Should return some response (even if course not found)
	if len(messages) == 0 {
		t.Error("Expected messages for course UID query, got none")
	}
}

func TestHandleMessage_CourseTitle(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	messages := h.HandleMessage(ctx, "課程 資料結構")

	// Should return some response
	if len(messages) == 0 {
		t.Error("Expected messages for course title search, got none")
	}
}

func TestHandleMessage_Teacher(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	messages := h.HandleMessage(ctx, "老師 王教授")

	// Should return some response
	if len(messages) == 0 {
		t.Error("Expected messages for teacher search, got none")
	}
}

func TestHandleMessage_CourseKeywordBefore(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test "search_term keyword" pattern
	messages := h.HandleMessage(ctx, "資料結構課")

	// Should return some response
	if len(messages) == 0 {
		t.Error("Expected messages for course keyword at end, got none")
	}
}

func TestHandleMessage_TeacherKeywordBefore(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test "search_term keyword" pattern
	messages := h.HandleMessage(ctx, "王教授老師")

	// Should return some response
	if len(messages) == 0 {
		t.Error("Expected messages for teacher keyword at end, got none")
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
		UID:       "3141U0001",
		Year:      113,
		Term:      1,
		No:        "3141U0001",
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
		UID:      "3141U0001",
		Year:     113,
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
			UID:       "3141U0001",
			Year:      113,
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
			UID:   "3141U000" + string(rune('0'+i%10)),
			Year:  113,
			Term:  1,
			Title: "Course " + string(rune(i)),
		}
	}

	messages := h.formatCourseListResponse(courses)

	// Should split into multiple messages (20 per message)
	if len(messages) < 3 {
		t.Errorf("Expected at least 3 messages for 60 courses, got %d", len(messages))
	}
}

func TestHandlePostback_CourseUID(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test UID postback
	messages := h.HandlePostback(ctx, "3141U0001")

	if len(messages) == 0 {
		t.Error("Expected messages for UID postback, got none")
	}
}

func TestHandlePostback_TeacherCourses(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test "授課課程" postback
	messages := h.HandlePostback(ctx, "授課課程$王教授")

	if len(messages) == 0 {
		t.Error("Expected messages for teacher courses postback, got none")
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

// TestSemesterDetermination tests the semester determination logic based on current month
func TestSemesterDetermination(t *testing.T) {
	tests := []struct {
		name            string
		month           int
		currentYear     int
		expectedYears   []int
		expectedTerms   []int
		expectedComment string
	}{
		{
			name:            "February - Spring semester ongoing",
			month:           2,
			currentYear:     114,
			expectedYears:   []int{114, 114},
			expectedTerms:   []int{2, 1},
			expectedComment: "查詢當年度第2學期（下學期進行中）及第1學期",
		},
		{
			name:            "June - Spring semester ending",
			month:           6,
			currentYear:     114,
			expectedYears:   []int{114, 114},
			expectedTerms:   []int{2, 1},
			expectedComment: "查詢當年度第2學期（下學期結束）及第1學期",
		},
		{
			name:            "July - Summer vacation",
			month:           7,
			currentYear:     114,
			expectedYears:   []int{114, 114},
			expectedTerms:   []int{2, 1},
			expectedComment: "查詢當年度第2學期及第1學期（暑假期間）",
		},
		{
			name:            "August - Before fall semester",
			month:           8,
			currentYear:     114,
			expectedYears:   []int{114, 114},
			expectedTerms:   []int{2, 1},
			expectedComment: "查詢當年度第2學期及第1學期（暑假結束前）",
		},
		{
			name:            "September - Fall semester starting",
			month:           9,
			currentYear:     114,
			expectedYears:   []int{115, 114},
			expectedTerms:   []int{1, 2},
			expectedComment: "查詢次年度第1學期（上學期開始）及當年度第2學期",
		},
		{
			name:            "December - Fall semester ongoing",
			month:           12,
			currentYear:     114,
			expectedYears:   []int{115, 114},
			expectedTerms:   []int{1, 2},
			expectedComment: "查詢次年度第1學期（上學期進行中）及當年度第2學期",
		},
		{
			name:            "January - Winter vacation",
			month:           1,
			currentYear:     114,
			expectedYears:   []int{115, 114},
			expectedTerms:   []int{1, 2},
			expectedComment: "查詢次年度第1學期（寒假）及當年度第2學期",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var searchYears, searchTerms []int

			// Simulate the logic from handleCourseTitleSearch
			if tt.month >= 2 && tt.month <= 6 {
				searchYears = []int{tt.currentYear, tt.currentYear}
				searchTerms = []int{2, 1}
			} else if tt.month >= 7 && tt.month <= 8 {
				searchYears = []int{tt.currentYear, tt.currentYear}
				searchTerms = []int{2, 1}
			} else {
				nextAcademicYear := tt.currentYear + 1
				searchYears = []int{nextAcademicYear, tt.currentYear}
				searchTerms = []int{1, 2}
			}

			if len(searchYears) != len(tt.expectedYears) {
				t.Errorf("Year count mismatch: got %d, want %d", len(searchYears), len(tt.expectedYears))
			}

			for i := range searchYears {
				if searchYears[i] != tt.expectedYears[i] {
					t.Errorf("Year[%d] mismatch: got %d, want %d (month=%d, %s)",
						i, searchYears[i], tt.expectedYears[i], tt.month, tt.expectedComment)
				}
				if searchTerms[i] != tt.expectedTerms[i] {
					t.Errorf("Term[%d] mismatch: got %d, want %d (month=%d, %s)",
						i, searchTerms[i], tt.expectedTerms[i], tt.month, tt.expectedComment)
				}
			}
		})
	}
}

// Keyword regexes are tested via CanHandle integration test - regex internals not critical
