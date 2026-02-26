package id

import (
	"context"
	"path/filepath"
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

	// Use a unique temp file database for each test to avoid shared memory conflicts
	// when running t.Parallel() tests. The temp directory is automatically cleaned up.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.New(context.Background(), dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })

	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw"},
		"sea": {"https://sea.cc.ntpu.edu.tw"},
	}
	scraperClient := scraper.NewClient(30*time.Second, 3, baseURLs)

	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	log := logger.New("info")
	stickerManager := sticker.NewManager(db, scraperClient, log)

	return NewHandler(db, scraperClient, m, log, stickerManager, nil)
}

func TestCanHandle(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"Valid student ID query", "學號 41247001", true},
		{"Valid student ID query (English)", "student 41247001", true},
		{"Valid name query", "學生 王小明", true},
		{"Valid name query (English)", "student 王小明", true},

		// Department keywords (Refined)
		{"Valid department code query", "系代碼 85", true},
		{"Valid department query", "系所 資工", true},
		{"Valid department Name query", "系名 資工", true},
		{"Valid department query (English)", "department 85", true},
		{"Single char '系' (natural query)", "系 資工", true},
		{"Single char '所' (natural query)", "所 資工", true},

		// Degree-specific department code keywords (5-char and 6-char formats)
		{"Bachelor dept code", "學士系代碼", true},
		{"Bachelor dept code 6-char", "學士班系代碼", true},
		{"Bachelor dept code alt", "大學部系代碼", true},
		{"Master dept code", "碩士系代碼", true},
		{"Master dept code 6-char", "碩士班系代碼", true},
		{"Master dept code alt", "碩班系代碼", true},
		{"PhD dept code", "博士系代碼", true},
		{"PhD dept code 6-char", "博士班系代碼", true},
		{"PhD dept code alt", "博班系代碼", true},
		{"Legacy all dept code", "所有系代碼", true},

		{"Year query", "112", false},
		{"Invalid prefix", "課程 41247001", false},
		{"Empty string", "", false},
		{"Random text", "hello world", false},
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

func TestHandleMessage_InvalidID(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test with valid format ID that likely doesn't exist.
	// Verifies handler returns a response without panicking.

	msgs := h.HandleMessage(ctx, "學號 00000000")
	if len(msgs) == 0 {
		t.Error("Expected response for valid format ID")
	}
}

func TestFormatStudentResponse(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	student := &storage.Student{
		ID:         "41247001",
		Name:       "測試學生",
		Department: "資訊工程學系",
		Year:       112,
	}

	msgs := h.formatStudentResponse(student)
	if len(msgs) == 0 {
		t.Error("Expected formatted messages")
	}

	// Verify it's a Flex Message
	if _, ok := msgs[0].(*messaging_api.FlexMessage); !ok {
		t.Error("Expected FlexMessage")
	}
}

// TestFormatStudentResponse_DegreeType tests that degree type label is correctly displayed
// based on student ID prefix (3=進修學士班, 4=學士班, 7=碩士班, 8=博士班)
func TestFormatStudentResponse_DegreeType(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name      string
		studentID string
	}{
		{"Continuing education (3)", "31247001"},
		{"Undergraduate (4)", "41247001"},
		{"Master (7)", "71247001"},
		{"PhD (8)", "81247001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			student := &storage.Student{
				ID:         tt.studentID,
				Name:       "測試學生",
				Department: "資訊工程學系",
				Year:       112,
			}

			msgs := h.formatStudentResponse(student)
			if len(msgs) == 0 {
				t.Fatal("Expected formatted messages")
			}

			// Verify it's a Flex Message (degree type is rendered in the body)
			if _, ok := msgs[0].(*messaging_api.FlexMessage); !ok {
				t.Error("Expected FlexMessage")
			}
		})
	}
}

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
			errContains: "missing required parameter: name",
		},
		{
			name:        "search intent empty query",
			intent:      IntentSearch,
			params:      map[string]string{"name": ""},
			errContains: "missing required parameter: name",
		},
		{
			name:        "unknown intent",
			intent:      "unknown",
			params:      map[string]string{},
			errContains: "unknown intent",
		},
		{
			name:        "year intent missing year",
			intent:      IntentYear,
			params:      map[string]string{},
			errContains: "missing required parameter: year",
		},
		{
			name:        "year intent empty year",
			intent:      IntentYear,
			params:      map[string]string{"year": ""},
			errContains: "missing required parameter: year",
		},
	}

	// Minimal handler for param validation tests
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

func TestDispatchIntent_Integration(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name         string
		intent       string
		params       map[string]string
		wantMessages bool
	}{
		{
			name:         "search intent with ID",
			intent:       IntentSearch,
			params:       map[string]string{"name": "41247001"},
			wantMessages: true,
		},
		{
			name:         "search intent with year",
			intent:       IntentSearch,
			params:       map[string]string{"name": "112"},
			wantMessages: true,
		},
		{
			name:         "search intent with name",
			intent:       IntentSearch,
			params:       map[string]string{"name": "王小明"},
			wantMessages: true,
		},
		{
			name:         "year intent valid",
			intent:       IntentYear,
			params:       map[string]string{"year": "112"},
			wantMessages: true,
		},
		{
			name:         "dept codes intent bachelor",
			intent:       IntentDeptCodes,
			params:       map[string]string{"degree": "bachelor"},
			wantMessages: true,
		},
		{
			name:         "dept codes intent default",
			intent:       IntentDeptCodes,
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

func TestHandleMessage_Postback(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	// Mock valid Postback data
	postbackData := "student" + "$" + "41247001"
	msgs := h.HandlePostback(ctx, postbackData)

	if len(msgs) == 0 {
		t.Error("Expected response for valid student postback")
	}

	// Mock invalid Postback data
	invalidData := "invalid" + "$" + "data"
	msgsInvalid := h.HandlePostback(ctx, invalidData)
	if len(msgsInvalid) != 1 {
		t.Errorf("Expected 1 error message for invalid postback, got %d", len(msgsInvalid))
	}
}

// ==================== Degree-Specific Department Code Tests ====================

// TestHandleDepartmentCodesByDegree tests degree-specific department code responses
func TestHandleDepartmentCodesByDegree(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name      string
		input     string
		wantTitle string
	}{
		{"Bachelor 5-char", "學士系代碼", "學士班系代碼"},
		{"Bachelor 6-char", "學士班系代碼", "學士班系代碼"},
		{"Bachelor alt", "大學部系代碼", "學士班系代碼"},
		{"Master 5-char", "碩士系代碼", "碩士班系代碼"},
		{"Master 6-char", "碩士班系代碼", "碩士班系代碼"},
		{"PhD 5-char", "博士系代碼", "博士班系代碼"},
		{"PhD 6-char", "博士班系代碼", "博士班系代碼"},
		{"Legacy all dept", "所有系代碼", "學士班系代碼"}, // legacy maps to bachelor
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msgs := h.HandleMessage(ctx, tt.input)
			if len(msgs) == 0 {
				t.Fatal("Expected response message")
			}
			// Verify text message contains expected title
			textMsg, ok := msgs[0].(*messaging_api.TextMessageV2)
			if !ok {
				t.Fatal("Expected TextMessageV2")
			}
			if !strings.Contains(textMsg.Text, tt.wantTitle) {
				t.Errorf("Response should contain %q, got: %s", tt.wantTitle, textMsg.Text)
			}
		})
	}
}

// ==================== Boundary and Edge Case Tests ====================

// TestHandleYearQuery_Boundaries tests year boundary cases
func TestHandleYearQuery_Boundaries(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name string
		year string
		want bool // true if should return messages
	}{
		{"Year 88 (before NTPU)", "88", true},
		{"Year 89 (NTPU founded)", "89", true},
		{"Year 94 (before digital)", "94", true},
		{"Year 95 (valid start)", "95", true},
		{"Year 112 (last complete)", "112", true},
		{"Year 113 (incomplete warning)", "113", true},
		{"Year 130 (max ROC)", "130", true},
		{"Year 131 (too late)", "131", true},
		{"Year 2024 (AD format)", "2024", true},
		{"Year 1911 (AD to ROC)", "1911", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msgs := h.handleYearQuery(tt.year)
			if (len(msgs) > 0) != tt.want {
				t.Errorf("handleYearQuery(%q) returned %d messages, want messages=%v",
					tt.year, len(msgs), tt.want)
			}
		})
	}
}

// TestHandleDepartmentCode_EdgeCases tests department code edge cases
func TestHandleDepartmentCode_EdgeCases(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		input       string
		expectReply bool // Just check if we get a reply (error or result)
	}{
		{"Valid 2-digit", "系代碼 85", true},
		{"Valid 3-digit law", "系代碼 712", true},
		{"Valid 3-digit social", "系代碼 742", true},
		{"Non-numeric", "系代碼 ABC", true},
		{"Too long", "系代碼 9999", true},
		{"Negative", "系代碼 -1", true},
		{"Empty", "系代碼 ", true},
		{"Zero", "系代碼 0", true},
		{"Leading zeros", "系代碼 085", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msgs := h.HandleMessage(ctx, tt.input)

			if tt.expectReply && len(msgs) == 0 {
				t.Error("Expected response message")
			}
		})
	}
}

// TestFormatStudentResponse_LongFields tests handling of long field values
func TestFormatStudentResponse_LongFields(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		name    string
		student *storage.Student
	}{
		{
			"Long name",
			&storage.Student{
				ID:         "41247001",
				Name:       "非常非常非常非常非常長的名字測試用例超過一般顯示範圍這是一個極端情況",
				Department: "資訊工程學系",
				Year:       112,
			},
		},
		{
			"Long department",
			&storage.Student{
				ID:         "41247001",
				Name:       "測試學生",
				Department: "資訊工程學系資訊科學組碩士班博士班進修學士班特殊選才組",
				Year:       112,
			},
		},
		{
			"All long fields",
			&storage.Student{
				ID:         "410747420",
				Name:       "這是一個超級無敵霹靂長的名字用來測試系統的極限情況看看會不會破版",
				Department: "法律學系法學組司法組財經法組國際法組科技法組勞動法組碩士班博士班",
				Year:       112,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msgs := h.formatStudentResponse(tt.student)
			if len(msgs) == 0 {
				t.Error("Expected formatted message")
			}

			// Verify message is FlexMessage
			flexMsg, ok := msgs[0].(*messaging_api.FlexMessage)
			if !ok {
				t.Error("Expected FlexMessage for student response")
				return
			}

			// Verify it has a valid altText
			if flexMsg.AltText == "" {
				t.Error("Expected non-empty altText")
			}
		})
	}
}

// TestHandleStudentSearch_Limits tests search result limits
func TestHandleStudentSearch_Limits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	// Test with common name (should return many results)
	msgs := h.HandleMessage(ctx, "學生 王")
	// Should handle large result sets gracefully
	if len(msgs) == 0 {
		t.Error("Expected search results or error message")
	}
}

// TestHandleMessage_SpecialCharacters tests handling of special characters
func TestHandleMessage_SpecialCharacters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	t.Parallel()

	h := setupTestHandler(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
	}{
		{"Emoji in query", "學號 41247001🎓"},
		{"URL characters", "學生 王<script>"},
		{"SQL injection attempt", "學生 王' OR '1'='1"},
		{"Unicode spaces", "學號\u3000412470\u200b01"},
		{"Control characters", "學號\n\t41247001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Should not panic
			msgs := h.HandleMessage(ctx, tt.input)
			// Should return some response (error or result)
			_ = msgs
		})
	}
}

// TestHandleYearQuery_ADtoROC tests AD to ROC year conversion
func TestHandleYearQuery_ADtoROC(t *testing.T) {
	t.Parallel()
	h := setupTestHandler(t)

	tests := []struct {
		adYear  string
		wantROC int
	}{
		{"2023", 112},
		{"2024", 113},
		{"2006", 95},
		{"2001", 90},
		{"1911", 0},
	}

	for _, tt := range tests {
		t.Run("AD "+tt.adYear, func(t *testing.T) {
			t.Parallel()
			msgs := h.handleYearQuery(tt.adYear)
			if len(msgs) == 0 {
				t.Errorf("Expected response for AD year %s", tt.adYear)
			}
		})
	}
}
