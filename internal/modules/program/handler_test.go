package program

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/course"
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
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	log := logger.New("info")
	stickerMgr := sticker.NewManager(db, nil, log)

	// Create a mock semester detector (nil is acceptable - handler will return all courses)
	// In production, this comes from course.Handler.GetSemesterDetector()
	var semesterDetector *course.SemesterDetector

	return NewHandler(db, m, log, stickerMgr, semesterDetector)
}

// TestCanHandle verifies keyword pattern matching for program queries
func TestCanHandle(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// List program keywords (at start, case-insensitive)
		{"List - 學程列表", "學程列表", true},
		{"List - 所有學程", "所有學程", true},
		{"List - English program list", "program list", true},
		{"List - English programs", "programs", true},

		// Search program keywords (at start)
		{"Search - 學程 keyword", "學程 資訊", true},
		{"Search - program keyword", "program", true},
		{"Search - 學程 only", "學程", true},

		// Keywords NOT at start should NOT match
		{"Not at start - list", "查詢學程列表", false},
		{"Not at start - search", "有什麼學程", false},

		// Invalid queries
		{"Random text", "hello world", false},
		{"Empty string", "", false},
		{"Spaces only", "   ", false},
		{"Course query", "課程 微積分", false},
		{"Student query", "學號 412345678", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := h.CanHandle(tt.input)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestHandleMessage_List verifies listing all programs
func TestHandleMessage_List(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
	}{
		{"學程列表", "學程列表"},
		{"所有學程", "所有學程"},
		{"program list", "program list"},
		{"programs", "programs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := h.HandleMessage(ctx, tt.input)
			if len(msgs) == 0 {
				t.Error("Expected messages for program list query")
			}
		})
	}
}

// TestHandleMessage_ListSplit verifies listing splits into multiple messages when exceeding batch size
func TestHandleMessage_ListSplit(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// 1. Seed database with 55 programs
	// Per design change (Refactor Program List Display), all programs are consolidated
	// into a single message if they fit within LINE's 5000 char limit
	programs := make([]struct{ Name, Category, URL string }, 55)
	for i := 0; i < 55; i++ {
		programs[i] = struct{ Name, Category, URL string }{
			Name:     fmt.Sprintf("Program %02d", i+1),
			Category: "Bachelor",
			URL:      "http://example.com",
		}
	}

	if err := h.db.SyncPrograms(ctx, programs); err != nil {
		t.Fatalf("Failed to sync programs: %v", err)
	}

	// 2. Call handler
	msgs := h.HandleMessage(ctx, "學程列表")

	// 3. Verify multiple messages (split per design)
	// 55 programs > TextListBatchSize (50), should split into 2 messages (50 + 5)
	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages for 55 programs (split), got %d", len(msgs))
	}

	// Verify it's a text message and not too long
	if len(msgs) > 0 {
		txtMsg, ok := msgs[0].(*messaging_api.TextMessage)
		if !ok {
			t.Fatalf("Message is not a TextMessage, got %T", msgs[0])
		}
		if utf8.RuneCountInString(txtMsg.Text) > 5000 {
			t.Errorf("Message too long: %d runes (LINE limit is 5000)", utf8.RuneCountInString(txtMsg.Text))
		}
	}
}

// TestHandleMessage_Search verifies program search
func TestHandleMessage_Search(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
	}{
		{"學程 with term", "學程 資訊"},
		{"學程 with term 2", "學程 管理"},
		{"program with term", "program information"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := h.HandleMessage(ctx, tt.input)
			if len(msgs) == 0 {
				t.Error("Expected messages for program search query")
			}
		})
	}
}

// TestHandleMessage_SearchEmptyTerm verifies search with empty term returns help
func TestHandleMessage_SearchEmptyTerm(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	msgs := h.HandleMessage(ctx, "學程")
	if len(msgs) == 0 {
		t.Error("Expected help message for empty search term")
	}
}

// TestHandleMessage_NoMatch verifies unmatched queries return empty
func TestHandleMessage_NoMatch(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	msgs := h.HandleMessage(ctx, "random text")
	if len(msgs) != 0 {
		t.Error("Expected empty response for unmatched query")
	}
}

// TestHandlePostback_ViewCourses verifies program courses postback
func TestHandlePostback_ViewCourses(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test valid postback
	data := "program:courses" + bot.PostbackSplitChar + "智慧財產權學士學分學程"
	msgs := h.HandlePostback(ctx, data)
	if len(msgs) == 0 {
		t.Error("Expected messages for view courses postback")
	}
}

// TestHandlePostback_CourseProgramsList verifies course programs list postback
func TestHandlePostback_CourseProgramsList(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test valid postback
	data := "program:course_programs" + bot.PostbackSplitChar + "1131U0001"
	msgs := h.HandlePostback(ctx, data)
	if len(msgs) == 0 {
		t.Error("Expected messages for course programs list postback")
	}
}

// TestHandlePostback_InvalidAction verifies unknown postback action
func TestHandlePostback_InvalidAction(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test invalid action
	data := "program:invalid_action" + bot.PostbackSplitChar + "data"
	msgs := h.HandlePostback(ctx, data)
	if msgs != nil {
		t.Error("Expected nil for invalid postback action")
	}
}

// TestHandlePostback_InvalidFormat verifies malformed postback data
func TestHandlePostback_InvalidFormat(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test malformed postback (missing split char)
	data := "program:courses_no_split"
	msgs := h.HandlePostback(ctx, data)
	if msgs != nil {
		t.Error("Expected nil for malformed postback data")
	}
}

// TestHandlePostback_WrongModule verifies postback from different module
func TestHandlePostback_WrongModule(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test postback from different module
	data := "course:1131U0001"
	msgs := h.HandlePostback(ctx, data)
	if msgs != nil {
		t.Error("Expected nil for postback from different module")
	}
}

// TestCanHandlePostback verifies postback module prefix check
func TestCanHandlePostback(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name string
		data string
		want bool
	}{
		{"Valid - courses", "program:courses" + bot.PostbackSplitChar + "test", true},
		{"Valid - course_programs", "program:course_programs" + bot.PostbackSplitChar + "test", true},
		{"Invalid - different module", "course:1131U0001", false},
		{"Invalid - empty", "", false},
		{"Invalid - no prefix", "random_data", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := h.CanHandlePostback(tt.data)
			if got != tt.want {
				t.Errorf("CanHandlePostback(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

// TestDispatchIntent_List verifies NLU intent dispatching for list
func TestDispatchIntent_List(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	msgs, err := h.DispatchIntent(ctx, IntentList, nil)
	if err != nil {
		t.Errorf("Expected no error for list intent, got: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("Expected messages for list intent")
	}
}

// TestDispatchIntent_Search verifies NLU intent dispatching for search
func TestDispatchIntent_Search(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	params := map[string]string{"query": "資訊"}
	msgs, err := h.DispatchIntent(ctx, IntentSearch, params)
	if err != nil {
		t.Errorf("Expected no error for search intent, got: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("Expected messages for search intent")
	}
}

// TestDispatchIntent_SearchMissingParam verifies search without query param
func TestDispatchIntent_SearchMissingParam(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Missing query param
	_, err := h.DispatchIntent(ctx, IntentSearch, nil)
	if err == nil {
		t.Error("Expected error for search intent without query param")
	}

	// Empty query param
	params := map[string]string{"query": ""}
	_, err = h.DispatchIntent(ctx, IntentSearch, params)
	if err == nil {
		t.Error("Expected error for search intent with empty query param")
	}
}

// TestDispatchIntent_Courses verifies NLU intent dispatching for courses
func TestDispatchIntent_Courses(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	params := map[string]string{"programName": "智慧財產權學士學分學程"}
	msgs, err := h.DispatchIntent(ctx, IntentCourses, params)
	if err != nil {
		t.Errorf("Expected no error for courses intent, got: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("Expected messages for courses intent")
	}
}

// TestDispatchIntent_CoursesMissingParam verifies courses without programName param
func TestDispatchIntent_CoursesMissingParam(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Missing programName param
	_, err := h.DispatchIntent(ctx, IntentCourses, nil)
	if err == nil {
		t.Error("Expected error for courses intent without programName param")
	}

	// Empty programName param
	params := map[string]string{"programName": ""}
	_, err = h.DispatchIntent(ctx, IntentCourses, params)
	if err == nil {
		t.Error("Expected error for courses intent with empty programName param")
	}
}

// TestDispatchIntent_UnknownIntent verifies unknown intent handling
func TestDispatchIntent_UnknownIntent(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	_, err := h.DispatchIntent(ctx, "unknown", nil)
	if err == nil {
		t.Error("Expected error for unknown intent")
	}
	if err != nil && err != domerrors.ErrUnknownIntent {
		// Check if error wraps ErrUnknownIntent
		found := false
		for e := err; e != nil; {
			if e == domerrors.ErrUnknownIntent {
				found = true
				break
			}
			// Try to unwrap
			type unwrapper interface{ Unwrap() error }
			if u, ok := e.(unwrapper); ok {
				e = u.Unwrap()
			} else {
				break
			}
		}
		if !found {
			t.Errorf("Expected ErrUnknownIntent, got: %v", err)
		}
	}
}

// TestName verifies module name
func TestName(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	if h.Name() != ModuleName {
		t.Errorf("Expected name %q, got %q", ModuleName, h.Name())
	}
}

// TestPatternMatcherPriority verifies matchers are sorted by priority
func TestPatternMatcherPriority(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	if len(h.matchers) < 2 {
		t.Fatal("Expected at least 2 matchers")
	}

	// Verify matchers are sorted by priority (lower number = higher priority)
	for i := 1; i < len(h.matchers); i++ {
		if h.matchers[i-1].priority > h.matchers[i].priority {
			t.Errorf("Matchers not sorted: priority[%d]=%d > priority[%d]=%d",
				i-1, h.matchers[i-1].priority, i, h.matchers[i].priority)
		}
	}
}

// TestFindMatcher verifies pattern matching logic
func TestFindMatcher(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name    string
		text    string
		wantNil bool
	}{
		{"Match list", "學程列表", false},
		{"Match search", "學程 資訊", false},
		{"No match", "random text", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			matcher := h.findMatcher(tt.text)
			if (matcher == nil) != tt.wantNil {
				t.Errorf("findMatcher(%q) nil=%v, want nil=%v", tt.text, matcher == nil, tt.wantNil)
			}
		})
	}
}

// TestQuickReplyActions verifies Quick Reply helper functions
func TestQuickReplyActions(t *testing.T) {
	t.Parallel()
	// Test QuickReplyProgramListAction
	listAction := lineutil.QuickReplyProgramListAction()
	if listAction.Action == nil {
		t.Error("Expected non-nil action for QuickReplyProgramListAction")
	}

	// Test QuickReplyProgramSearchAction
	searchAction := lineutil.QuickReplyProgramAction()
	if searchAction.Action == nil {
		t.Error("Expected non-nil action for QuickReplyProgramSearchAction")
	}

	// Test QuickReplyProgramNav
	navItems := lineutil.QuickReplyProgramNav()
	if len(navItems) < 2 {
		t.Errorf("Expected at least 2 nav items, got %d", len(navItems))
	}
}

// TestDispatchIntent_ParameterValidation verifies parameter validation before execution
func TestDispatchIntent_ParameterValidation(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test list intent (no params required)
	msgs, err := h.DispatchIntent(ctx, IntentList, nil)
	if err != nil {
		t.Errorf("Expected no error for list intent, got: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("Expected messages for list intent")
	}

	// Test search intent with valid params
	params := map[string]string{"query": "test"}
	msgs, err = h.DispatchIntent(ctx, IntentSearch, params)
	if err != nil {
		t.Errorf("Expected no error for search intent, got: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("Expected messages for search intent")
	}

	// Test search intent with missing params
	_, err = h.DispatchIntent(ctx, IntentSearch, nil)
	if err == nil {
		t.Error("Expected error for search intent without params")
	}

	// Test courses intent with valid params
	courseParams := map[string]string{"programName": "test"}
	msgs, err = h.DispatchIntent(ctx, IntentCourses, courseParams)
	if err != nil {
		t.Errorf("Expected no error for courses intent, got: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("Expected messages for courses intent")
	}

	// Test courses intent with missing params
	_, err = h.DispatchIntent(ctx, IntentCourses, nil)
	if err == nil {
		t.Error("Expected error for courses intent without params")
	}
}

// TestNewHandler_NilSemesterDetector verifies handler works without semester detector
func TestNewHandler_NilSemesterDetector(t *testing.T) {
	t.Parallel()
	// Use a unique temp file database for each test to avoid shared memory conflicts
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.New(context.Background(), dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Create dependencies
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	log := logger.New("info")
	stickerMgr := sticker.NewManager(db, nil, log)

	// Create handler with nil semester detector (should not panic)
	h := NewHandler(db, m, log, stickerMgr, nil)
	if h == nil {
		t.Fatal("Expected non-nil handler")
	}

	// Verify handler can still process queries (without semester filtering)
	msgs := h.HandleMessage(context.Background(), "學程列表")
	if len(msgs) == 0 {
		t.Error("Expected messages for program list query")
	}
}
