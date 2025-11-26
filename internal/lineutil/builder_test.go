package lineutil

import (
	"fmt"
	"testing"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

func TestNewMessageAction(t *testing.T) {
	label := "Click me"
	text := "User clicked"

	action := NewMessageAction(label, text)
	msgAction, ok := action.(*messaging_api.MessageAction)
	if !ok {
		t.Fatal("Expected *messaging_api.MessageAction")
	}

	if msgAction.Label != label {
		t.Errorf("Expected label %q, got %q", label, msgAction.Label)
	}

	if msgAction.Text != text {
		t.Errorf("Expected text %q, got %q", text, msgAction.Text)
	}
}

func TestNewPostbackAction(t *testing.T) {
	label := "Confirm"
	data := "action=confirm&id=123"

	action := NewPostbackAction(label, data)
	pbAction, ok := action.(*messaging_api.PostbackAction)
	if !ok {
		t.Fatal("Expected *messaging_api.PostbackAction")
	}

	if pbAction.Label != label {
		t.Errorf("Expected label %q, got %q", label, pbAction.Label)
	}

	if pbAction.Data != data {
		t.Errorf("Expected data %q, got %q", data, pbAction.Data)
	}
}

func TestNewURIAction(t *testing.T) {
	label := "Open Website"
	uri := "https://www.ntpu.edu.tw"

	action := NewURIAction(label, uri)
	uriAction, ok := action.(*messaging_api.UriAction)
	if !ok {
		t.Fatal("Expected *messaging_api.UriAction")
	}

	if uriAction.Label != label {
		t.Errorf("Expected label %q, got %q", label, uriAction.Label)
	}

	if uriAction.Uri != uri {
		t.Errorf("Expected uri %q, got %q", uri, uriAction.Uri)
	}
}

// TestTruncateText tests critical LINE API constraint (5000 char limit)
func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected string
	}{
		{"Within limit", "Short text", 20, "Short text"},
		{"Exceeds limit - must truncate", "This is a very long text that needs truncation", 20, "This is a very lo..."},
		{"Empty string edge case", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateText(tt.text, tt.maxLen)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestContainsAllRunes tests the fuzzy matching function for contact/course search
func TestContainsAllRunes(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		chars    string
		expected bool
	}{
		// Chinese department matching (key use case)
		{"資工系 matches 資訊工程學系", "資訊工程學系", "資工系", true},
		{"電機系 matches 電機工程學系", "電機工程學系", "電機系", true},
		{"通訊系 matches 通訊工程學系", "通訊工程學系", "通訊系", true},
		{"社工系 matches 社會工作學系", "社會工作學系", "社工系", true},
		{"企管系 matches 企業管理學系", "企業管理學系", "企管系", true},
		{"會計系 matches 會計學系", "會計學系", "會計系", true},
		{"統計系 matches 統計學系", "統計學系", "統計系", true},
		{"金融系 matches 金融與合作經營學系", "金融與合作經營學系", "金融系", true},
		{"公行系 matches 公共行政暨政策學系", "公共行政暨政策學系", "公行系", true}, // 公共"行"政 contains 行
		{"財法組 matches 財經法組", "財經法組", "財法組", true},

		// Teacher name matching
		{"王 matches 王小明", "王小明", "王", true},
		{"陳老師 matches 陳大明教授", "陳大明教授", "陳", true},
		{"Full name match", "張三", "張三", true},

		// Edge cases
		{"Empty chars", "任意字串", "", true},
		{"Empty s", "", "abc", false},
		{"Both empty", "", "", true},
		{"Exact match", "資工系", "資工系", true},
		{"Superset chars - no match", "資工", "資工系", false}, // 資工 doesn't have 系

		// Case insensitivity (for ASCII)
		{"Case insensitive English", "Computer Science", "cs", true},
		{"Mixed case", "ABCDEF", "abc", true},

		// Course title matching
		{"程式 in 程式設計", "程式設計", "程式", true},
		{"微積分 matches", "微積分（一）", "微積分", true},
		{"資料結構 matches", "資料結構與演算法", "資料", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsAllRunes(tt.s, tt.chars)
			if result != tt.expected {
				t.Errorf("ContainsAllRunes(%q, %q) = %v, expected %v", tt.s, tt.chars, result, tt.expected)
			}
		})
	}
}

// TestSplitMessages tests critical LINE API constraint (5 messages max per reply)
func TestSplitMessages(t *testing.T) {
	tests := []struct {
		name            string
		messageCount    int
		maxCount        int
		expectedBatches int
	}{
		{"Within limit", 5, 5, 1},
		{"Exceeds limit - must split", 7, 5, 2},
		{"Empty - edge case", 0, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := make([]messaging_api.MessageInterface, tt.messageCount)
			sender := &messaging_api.Sender{Name: "測試", IconUrl: "https://example.com/avatar.png"}
			for i := 0; i < tt.messageCount; i++ {
				messages[i] = NewTextMessageWithConsistentSender("Test", sender)
			}
			batches := SplitMessages(messages, tt.maxCount)
			if len(batches) != tt.expectedBatches {
				t.Errorf("Expected %d batches, got %d", tt.expectedBatches, len(batches))
			}
		})
	}
}

// TestErrorMessage tests that technical errors are NOT exposed to users
func TestErrorMessage(t *testing.T) {
	err := fmt.Errorf("database connection failed")
	sender := &messaging_api.Sender{Name: "系統魔法師", IconUrl: "https://example.com/avatar.png"}
	msg := ErrorMessageWithSender(err, sender)

	textMsg, ok := msg.(*messaging_api.TextMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TextMessage")
	}

	// Critical: Must NOT expose technical details
	if contains(textMsg.Text, "database") || contains(textMsg.Text, "connection failed") {
		t.Error("Error message MUST NOT expose technical details to users")
	}

	// Must provide user-friendly message
	if textMsg.Text == "" {
		t.Error("Error message cannot be empty")
	}
}

func TestNewCarouselTemplate(t *testing.T) {
	columns := []CarouselColumn{
		{
			Title: "Column 1",
			Text:  "Text 1",
			Actions: []Action{
				NewMessageAction("Action 1", "Message 1"),
			},
		},
		{
			ThumbnailImageURL: "https://example.com/image.jpg",
			Title:             "Column 2",
			Text:              "Text 2",
			Actions: []Action{
				NewURIAction("Action 2", "https://example.com"),
			},
		},
	}

	msg := NewCarouselTemplate("Alt text", columns)

	templateMsg, ok := msg.(*messaging_api.TemplateMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TemplateMessage")
	}

	if templateMsg.AltText != "Alt text" {
		t.Errorf("Expected alt text 'Alt text', got %q", templateMsg.AltText)
	}

	carousel, ok := templateMsg.Template.(*messaging_api.CarouselTemplate)
	if !ok {
		t.Fatal("Expected *messaging_api.CarouselTemplate")
	}

	if len(carousel.Columns) != 2 {
		t.Errorf("Expected 2 columns, got %d", len(carousel.Columns))
	}
}

func TestNewButtonsTemplate(t *testing.T) {
	actions := []Action{
		NewMessageAction("Button 1", "Message 1"),
		NewPostbackAction("Button 2", "data=123"),
	}

	msg := NewButtonsTemplate("Alt text", "Title", "Text content", actions)

	templateMsg, ok := msg.(*messaging_api.TemplateMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TemplateMessage")
	}

	buttons, ok := templateMsg.Template.(*messaging_api.ButtonsTemplate)
	if !ok {
		t.Fatal("Expected *messaging_api.ButtonsTemplate")
	}

	if buttons.Title != "Title" {
		t.Errorf("Expected title 'Title', got %q", buttons.Title)
	}

	if buttons.Text != "Text content" {
		t.Errorf("Expected text 'Text content', got %q", buttons.Text)
	}

	if len(buttons.Actions) != 2 {
		t.Errorf("Expected 2 actions, got %d", len(buttons.Actions))
	}
}

func TestNewConfirmTemplate(t *testing.T) {
	yesAction := NewPostbackAction("Yes", "confirm=yes")
	noAction := NewPostbackAction("No", "confirm=no")

	msg := NewConfirmTemplate("Confirm", "Are you sure?", yesAction, noAction)

	templateMsg, ok := msg.(*messaging_api.TemplateMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TemplateMessage")
	}

	confirm, ok := templateMsg.Template.(*messaging_api.ConfirmTemplate)
	if !ok {
		t.Fatal("Expected *messaging_api.ConfirmTemplate")
	}

	if confirm.Text != "Are you sure?" {
		t.Errorf("Expected text 'Are you sure?', got %q", confirm.Text)
	}

	if len(confirm.Actions) != 2 {
		t.Errorf("Expected 2 actions, got %d", len(confirm.Actions))
	}
}

func TestNewQuickReply(t *testing.T) {
	items := []QuickReplyItem{
		{
			Action: NewMessageAction("Option 1", "Message 1"),
		},
		{
			ImageURL: "https://example.com/icon.png",
			Action:   NewMessageAction("Option 2", "Message 2"),
		},
	}

	quickReply := NewQuickReply(items)

	if len(quickReply.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(quickReply.Items))
	}
}

func TestNewClipboardAction(t *testing.T) {
	tests := []struct {
		name          string
		label         string
		clipboardText string
	}{
		{
			name:          "Emergency phone",
			label:         "複製三峽24H緊急行政",
			clipboardText: "02-2673-2123",
		},
		{
			name:          "Normal phone",
			label:         "複製電話",
			clipboardText: "02-1234-5678",
		},
		{
			name:          "Email address",
			label:         "複製信箱",
			clipboardText: "test@gm.ntpu.edu.tw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewClipboardAction(tt.label, tt.clipboardText)

			clipAction, ok := action.(*messaging_api.ClipboardAction)
			if !ok {
				t.Fatal("Expected *messaging_api.ClipboardAction")
			}

			if clipAction.Label != tt.label {
				t.Errorf("Expected label %q, got %q", tt.label, clipAction.Label)
			}

			if clipAction.ClipboardText != tt.clipboardText {
				t.Errorf("Expected clipboardText %q, got %q", tt.clipboardText, clipAction.ClipboardText)
			}
		})
	}
}

func TestFormatDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		nameCN   string
		nameEN   string
		expected string
	}{
		{
			name:     "Both names different",
			nameCN:   "王小明",
			nameEN:   "Wang Xiao Ming",
			expected: "王小明 Wang Xiao Ming",
		},
		{
			name:     "Names identical - show only Chinese",
			nameCN:   "資訊中心",
			nameEN:   "資訊中心",
			expected: "資訊中心",
		},
		{
			name:     "English name empty",
			nameCN:   "陳大文",
			nameEN:   "",
			expected: "陳大文",
		},
		{
			name:     "Chinese name empty",
			nameCN:   "",
			nameEN:   "John Doe",
			expected: " John Doe",
		},
		{
			name:     "Both names empty",
			nameCN:   "",
			nameEN:   "",
			expected: "",
		},
		{
			name:     "Case insensitive - still show both",
			nameCN:   "ABC",
			nameEN:   "abc",
			expected: "ABC abc",
		},
		{
			name:     "Whitespace trimmed - identical after trim",
			nameCN:   "測試",
			nameEN:   "測試 ",
			expected: "測試",
		},
		{
			name:     "Different after trim",
			nameCN:   "測試 ",
			nameEN:   "Test",
			expected: "測試 Test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDisplayName(tt.nameCN, tt.nameEN)
			if result != tt.expected {
				t.Errorf("FormatDisplayName(%q, %q) = %q, want %q",
					tt.nameCN, tt.nameEN, result, tt.expected)
			}
		})
	}
}

func TestBuildTelURI(t *testing.T) {
	tests := []struct {
		name      string
		mainPhone string
		extension string
		expected  string
	}{
		{
			name:      "Phone with extension",
			mainPhone: "0286741111",
			extension: "67114",
			expected:  "tel:+886286741111,67114",
		},
		{
			name:      "Phone without extension",
			mainPhone: "0286741111",
			extension: "",
			expected:  "tel:+886286741111",
		},
		{
			name:      "Phone with dashes removed",
			mainPhone: "02-8674-1111",
			extension: "67114",
			expected:  "tel:+886286741111,67114",
		},
		{
			name:      "Short extension",
			mainPhone: "0286741111",
			extension: "123",
			expected:  "tel:+886286741111,123",
		},
		{
			name:      "Empty phone",
			mainPhone: "",
			extension: "67114",
			expected:  "tel:+886,67114",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildTelURI(tt.mainPhone, tt.extension)
			if result != tt.expected {
				t.Errorf("BuildTelURI(%q, %q) = %q, want %q",
					tt.mainPhone, tt.extension, result, tt.expected)
			}
		})
	}
}

func TestBuildFullPhone(t *testing.T) {
	tests := []struct {
		name      string
		mainPhone string
		extension string
		expected  string
	}{
		{
			name:      "Phone with 5-digit extension",
			mainPhone: "0286741111",
			extension: "67114",
			expected:  "0286741111,67114",
		},
		{
			name:      "Phone with 6-digit extension - truncate to 5",
			mainPhone: "0286741111",
			extension: "671145",
			expected:  "0286741111,67114",
		},
		{
			name:      "Phone without extension - returns empty",
			mainPhone: "0286741111",
			extension: "",
			expected:  "",
		},
		{
			name:      "Phone with short extension (4 digits) - returns empty",
			mainPhone: "0286741111",
			extension: "1234",
			expected:  "",
		},
		{
			name:      "Both empty - returns empty",
			mainPhone: "",
			extension: "",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildFullPhone(tt.mainPhone, tt.extension)
			if result != tt.expected {
				t.Errorf("BuildFullPhone(%q, %q) = %q, want %q",
					tt.mainPhone, tt.extension, result, tt.expected)
			}
		})
	}
}

func TestNewTextMessageWithConsistentSender(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		senderName     string
		stickerIconURL string
	}{
		{
			name:           "With sticker icon",
			text:           "Hello, World!",
			senderName:     "學號魔法師",
			stickerIconURL: "https://stickershop.line-scdn.net/stickershop/v1/sticker/52002734/android/sticker.png",
		},
		{
			name:           "With UI avatar",
			text:           "查詢結果",
			senderName:     "聯繫魔法師",
			stickerIconURL: "https://ui-avatars.com/api/?name=A&size=256",
		},
		{
			name:           "Empty sticker URL",
			text:           "測試訊息",
			senderName:     "課程魔法師",
			stickerIconURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &messaging_api.Sender{Name: tt.senderName, IconUrl: tt.stickerIconURL}
			msg := NewTextMessageWithConsistentSender(tt.text, sender)

			if msg.Text != tt.text {
				t.Errorf("Expected text %q, got %q", tt.text, msg.Text)
			}

			if msg.Sender == nil {
				t.Fatal("Expected non-nil Sender")
			}

			if msg.Sender.Name != tt.senderName {
				t.Errorf("Expected sender name %q, got %q", tt.senderName, msg.Sender.Name)
			}

			if tt.stickerIconURL != "" && msg.Sender.IconUrl != tt.stickerIconURL {
				t.Errorf("Expected sender icon URL %q, got %q", tt.stickerIconURL, msg.Sender.IconUrl)
			}
		})
	}
}

func TestFormatTeachers(t *testing.T) {
	tests := []struct {
		name     string
		teachers []string
		max      int
		expected string
	}{
		{
			name:     "Empty list",
			teachers: []string{},
			max:      5,
			expected: "",
		},
		{
			name:     "Single teacher",
			teachers: []string{"王教授"},
			max:      5,
			expected: "王教授",
		},
		{
			name:     "Under limit",
			teachers: []string{"王教授", "李教授", "陳教授"},
			max:      5,
			expected: "王教授、李教授、陳教授",
		},
		{
			name:     "Exactly at limit",
			teachers: []string{"王教授", "李教授", "陳教授", "林教授", "張教授"},
			max:      5,
			expected: "王教授、李教授、陳教授、林教授、張教授",
		},
		{
			name:     "Over limit - truncate",
			teachers: []string{"王教授", "李教授", "陳教授", "林教授", "張教授", "劉教授", "黃教授"},
			max:      5,
			expected: "王教授、李教授、陳教授、林教授、張教授 等 2 人",
		},
		{
			name:     "Over limit by 1",
			teachers: []string{"王教授", "李教授", "陳教授", "林教授", "張教授", "劉教授"},
			max:      5,
			expected: "王教授、李教授、陳教授、林教授、張教授 等 1 人",
		},
		{
			name:     "Max 0 - no limit",
			teachers: []string{"王教授", "李教授", "陳教授", "林教授", "張教授", "劉教授"},
			max:      0,
			expected: "王教授、李教授、陳教授、林教授、張教授、劉教授",
		},
		{
			name:     "Negative max - no limit",
			teachers: []string{"王教授", "李教授", "陳教授"},
			max:      -1,
			expected: "王教授、李教授、陳教授",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTeachers(tt.teachers, tt.max)
			if result != tt.expected {
				t.Errorf("FormatTeachers(%v, %d) = %q, want %q",
					tt.teachers, tt.max, result, tt.expected)
			}
		})
	}
}

func TestFormatTimes(t *testing.T) {
	tests := []struct {
		name     string
		times    []string
		max      int
		expected string
	}{
		{
			name:     "Empty list",
			times:    []string{},
			max:      4,
			expected: "",
		},
		{
			name:     "Single time slot",
			times:    []string{"週一1-2"},
			max:      4,
			expected: "週一1-2",
		},
		{
			name:     "Under limit",
			times:    []string{"週一1-2", "週二3-4", "週三5-6"},
			max:      4,
			expected: "週一1-2、週二3-4、週三5-6",
		},
		{
			name:     "Exactly at limit",
			times:    []string{"週一1-2", "週二3-4", "週三5-6", "週四7-8"},
			max:      4,
			expected: "週一1-2、週二3-4、週三5-6、週四7-8",
		},
		{
			name:     "Over limit - truncate",
			times:    []string{"週一1-2", "週二3-4", "週三5-6", "週四7-8", "週五1-2", "週五3-4"},
			max:      4,
			expected: "週一1-2、週二3-4、週三5-6、週四7-8 等 2 節",
		},
		{
			name:     "Over limit by 1",
			times:    []string{"週一1-2", "週二3-4", "週三5-6", "週四7-8", "週五1-2"},
			max:      4,
			expected: "週一1-2、週二3-4、週三5-6、週四7-8 等 1 節",
		},
		{
			name:     "Max 0 - no limit",
			times:    []string{"週一1-2", "週二3-4", "週三5-6", "週四7-8", "週五1-2"},
			max:      0,
			expected: "週一1-2、週二3-4、週三5-6、週四7-8、週五1-2",
		},
		{
			name:     "Negative max - no limit",
			times:    []string{"週一1-2", "週二3-4"},
			max:      -1,
			expected: "週一1-2、週二3-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTimes(tt.times, tt.max)
			if result != tt.expected {
				t.Errorf("FormatTimes(%v, %d) = %q, want %q",
					tt.times, tt.max, result, tt.expected)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
