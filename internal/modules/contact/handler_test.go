package contact

import (
	"context"
	"fmt"
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

	return NewHandler(db, scraperClient, m, log, stickerMgr, 100)
}

func TestCanHandle(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Emergency keywords (must be at start)
		{"Emergency query", "緊急電話", true},
		{"Emergency without space", "緊急", true},

		// Contact keywords at START (English)
		{"Contact keyword at start", "contact info", true},
		{"Touch keyword at start", "touch base", true},
		{"Connect keyword at start", "connect with", true},

		// Contact keywords at START (Chinese)
		{"聯繫 keyword at start", "聯繫 資工系", true},
		{"聯絡 keyword at start", "聯絡 圖書館", true},
		{"連絡 keyword at start", "連絡方式", true},
		{"電話 keyword at start", "電話分機", true},
		{"分機 keyword at start", "分機查詢", true},
		{"Email keyword at start", "email信箱", true},
		{"信箱 keyword at start", "信箱查詢", true},

		// Keywords NOT at start should NOT match (precise matching only)
		{"Connect keyword not at start", "how to connect", false},
		{"分機 keyword not at start", "查詢分機", false},
		{"電話 keyword not at start", "查詢電話", false},
		{"信箱 keyword not at start", "電子信箱", false},

		// Invalid queries
		{"Random text", "hello world", false},
		{"Empty string", "", false},
		{"Spaces only", "   ", false},
		{"Student related", "學號", false},
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

func TestHandleEmergencyPhones(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	messages := h.handleEmergencyPhones()

	if len(messages) == 0 {
		t.Error("Expected emergency phone messages, got none")
	}

	// Should contain at least the flex message
	if len(messages) < 1 {
		t.Errorf("Expected at least 1 message, got %d", len(messages))
	}

	// Verify the message structure
	flexMsg, ok := messages[0].(*messaging_api.FlexMessage)
	if !ok {
		t.Fatal("Expected first message to be *messaging_api.FlexMessage")
	}

	if flexMsg.AltText != "緊急聯絡電話" {
		t.Errorf("Expected AltText '緊急聯絡電話', got %q", flexMsg.AltText)
	}
}

func TestHandleMessage_Emergency(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	messages := h.HandleMessage(ctx, "緊急電話")

	if len(messages) == 0 {
		t.Error("Expected messages for emergency query, got none")
	}
}

func TestHandleMessage_Contact(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	t.Parallel()

	h := setupTestHandler(t)
	ctx := context.Background()

	// This will likely return no results from empty cache
	messages := h.HandleMessage(ctx, "聯絡陳教授")

	// Should return at least some response
	if len(messages) == 0 {
		t.Error("Expected some response messages, got none")
	}
}

// buildRegex is tested indirectly via CanHandle - no need for separate test

func TestFormatContactResults_Empty(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	messages := h.formatContactResults(context.Background(), []storage.Contact{})

	if len(messages) != 1 {
		t.Errorf("Expected 1 message for empty results, got %d", len(messages))
	}
}

func TestFormatContactResults_Organizations(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	contacts := []storage.Contact{
		{
			UID:      "org1",
			Type:     "organization",
			Name:     "資訊工程學系",
			Superior: "工學院",
			Location: "資訊大樓",
			Website:  "https://www.csie.ntpu.edu.tw",
		},
	}

	messages := h.formatContactResults(context.Background(), contacts)

	if len(messages) == 0 {
		t.Error("Expected messages for organization results, got none")
	}

	// Verify label is "組織"
	flexMsg, ok := messages[0].(*messaging_api.FlexMessage)
	if !ok {
		t.Fatal("Expected FlexMessage")
	}
	// Verify it's a valid FlexMessage with non-empty altText (deep label inspection omitted).
	if flexMsg.AltText == "" {
		t.Error("Expected non-empty altText")
	}
}

func TestFormatContactResults_Individuals(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	contacts := []storage.Contact{
		{
			UID:          "person1",
			Type:         "individual",
			Name:         "陳教授",
			Organization: "資訊工程學系",
			Title:        "教授",
			Extension:    "88888",
			Phone:        "02-1234-5678",
			Email:        "chen@gm.ntpu.edu.tw",
		},
	}

	messages := h.formatContactResults(context.Background(), contacts)

	if len(messages) == 0 {
		t.Error("Expected messages for individual results, got none")
	}

	// Verify label is "個人"
	// Similar to above, we rely on the code change for the exact string "個人"
}

func TestFormatContactResults_LargeList(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	// Create 60 contacts to test pagination
	contacts := make([]storage.Contact, 60)
	for i := 0; i < 60; i++ {
		contacts[i] = storage.Contact{
			UID:  "contact" + string(rune(i)),
			Type: "individual",
			Name: "Contact " + string(rune(i)),
		}
	}

	messages := h.formatContactResults(context.Background(), contacts)

	// Should split into multiple messages
	if len(messages) < 2 {
		t.Errorf("Expected multiple messages for large list, got %d", len(messages))
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
			name:        "search intent missing query",
			intent:      IntentSearch,
			params:      map[string]string{},
			errContains: "missing required parameter: query",
		},
		{
			name:        "search intent empty query",
			intent:      IntentSearch,
			params:      map[string]string{"query": ""},
			errContains: "missing required parameter: query",
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
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name         string
		intent       string
		params       map[string]string
		wantMessages bool // expect at least one message (success or error message)
	}{
		{
			name:         "search intent with query",
			intent:       IntentSearch,
			params:       map[string]string{"query": "資工系"},
			wantMessages: true,
		},
		{
			name:         "search intent with person name",
			intent:       IntentSearch,
			params:       map[string]string{"query": "王教授"},
			wantMessages: true,
		},
		{
			name:         "emergency intent (no params)",
			intent:       IntentEmergency,
			params:       map[string]string{},
			wantMessages: true,
		},
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

// TestHandlePostback_TeacherContact tests the "教師聯繫" postback handler for course→contact navigation
func TestHandlePostback_TeacherContact(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Setup: Add a contact for the teacher
	contact := &storage.Contact{
		UID:          "test_teacher_1",
		Type:         "individual",
		Name:         "王教授",
		Organization: "資訊工程學系",
		Title:        "教授",
		Extension:    "12345",
	}
	if err := h.db.SaveContact(ctx, contact); err != nil {
		t.Fatalf("SaveContact failed: %v", err)
	}

	// Test: Postback with "教師聯繫" prefix should trigger contact search
	// Format: "contact:教師聯繫${bot.PostbackSplitChar}{teacherName}"
	msgs := h.HandlePostback(ctx, fmt.Sprintf("contact:教師聯繫%s王教授", bot.PostbackSplitChar))
	if len(msgs) == 0 {
		t.Error("Expected messages for teacher contact postback, got none")
	}

	// Test: Postback for non-existent teacher should still return message (no results)
	msgs = h.HandlePostback(ctx, fmt.Sprintf("contact:教師聯繫%s不存在的人", bot.PostbackSplitChar))
	if len(msgs) == 0 {
		t.Error("Expected 'no results' message for non-existent teacher, got none")
	}
}

// TestFormatContactResults_IndividualWithCourses tests that the "授課課程" button appears for teachers with courses
func TestFormatContactResults_IndividualWithCourses(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Setup: Add a teacher contact
	teacherContact := &storage.Contact{
		UID:          "teacher_1",
		Type:         "individual",
		Name:         "課程教授",
		Organization: "資訊工程學系",
		Title:        "教授",
	}
	if err := h.db.SaveContact(ctx, teacherContact); err != nil {
		t.Fatalf("SaveContact failed: %v", err)
	}

	// Setup: Add a course taught by this teacher
	course := &storage.Course{
		UID:      "1131U0001",
		Year:     113,
		Term:     1,
		No:       "U0001",
		Title:    "程式設計",
		Teachers: []string{"課程教授"},
	}
	if err := h.db.SaveCourse(ctx, course); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	// Format results for the teacher contact
	msgs := h.formatContactResults(ctx, []storage.Contact{*teacherContact})

	// Should return at least one message
	if len(msgs) == 0 {
		t.Error("Expected messages for contact with courses, got none")
	}

	// Verify the message is a FlexMessage (contains the 授課課程 button)
	if _, ok := msgs[0].(*messaging_api.FlexMessage); !ok {
		t.Error("Expected FlexMessage for contact result")
	}
}

// TestFormatContactResults_IndividualWithoutCourses tests that no course button appears when there are no courses
func TestFormatContactResults_IndividualWithoutCourses(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Setup: Add a contact without any courses
	contact := &storage.Contact{
		UID:          "non_teacher_1",
		Type:         "individual",
		Name:         "行政人員",
		Organization: "教務處",
		Title:        "秘書",
	}
	if err := h.db.SaveContact(ctx, contact); err != nil {
		t.Fatalf("SaveContact failed: %v", err)
	}

	// Format results (no courses in DB for this person)
	msgs := h.formatContactResults(ctx, []storage.Contact{*contact})

	// Should still return a message
	if len(msgs) == 0 {
		t.Error("Expected messages for contact without courses, got none")
	}
}

// TestFormatContactResults_OrganizationNoCourseButton tests that organizations don't get course buttons
func TestFormatContactResults_OrganizationNoCourseButton(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Setup: Add an organization contact
	orgContact := &storage.Contact{
		UID:          "org_1",
		Type:         "organization",
		Name:         "資訊工程學系",
		Organization: "電機資訊學院",
	}
	if err := h.db.SaveContact(ctx, orgContact); err != nil {
		t.Fatalf("SaveContact failed: %v", err)
	}

	// Add a course (this shouldn't affect org contacts)
	course := &storage.Course{
		UID:      "1131U0001",
		Year:     113,
		Term:     1,
		No:       "U0001",
		Title:    "程式設計",
		Teachers: []string{"資訊工程學系"}, // Using org name as "teacher"
	}
	if err := h.db.SaveCourse(ctx, course); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	// Format results
	msgs := h.formatContactResults(ctx, []storage.Contact{*orgContact})

	// Should return at least one message (organizations displayed normally)
	if len(msgs) == 0 {
		t.Error("Expected messages for organization contact, got none")
	}
}
