package lineutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// TestFormatError tests the FormatError function
func TestFormatError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		title  string
		detail string
		want   string
	}{
		{
			name:   "Standard error",
			title:  "操作失敗",
			detail: "請稍後再試",
			want:   "❌ 操作失敗\n\n請稍後再試",
		},
		{
			name:   "Empty detail",
			title:  "錯誤",
			detail: "",
			want:   "❌ 錯誤\n\n",
		},
		{
			name:   "Multi-line detail",
			title:  "驗證失敗",
			detail: "原因一\n原因二",
			want:   "❌ 驗證失敗\n\n原因一\n原因二",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatError(tt.title, tt.detail)
			if got != tt.want {
				t.Errorf("FormatError() = %q, want %q", got, tt.want)
			}
			// Verify emoji is present
			if !strings.HasPrefix(got, "❌") {
				t.Error("FormatError() should start with ❌ emoji")
			}
		})
	}
}

// TestFormatInfo tests the FormatInfo function
func TestFormatInfo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		title  string
		detail string
		want   string
	}{
		{
			name:   "Standard info",
			title:  "系統通知",
			detail: "伺服器維護中",
			want:   "ℹ️ 系統通知\n\n伺服器維護中",
		},
		{
			name:   "Empty title",
			title:  "",
			detail: "詳細資訊",
			want:   "ℹ️ \n\n詳細資訊",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatInfo(tt.title, tt.detail)
			if got != tt.want {
				t.Errorf("FormatInfo() = %q, want %q", got, tt.want)
			}
			// Verify emoji is present
			if !strings.HasPrefix(got, "ℹ️") {
				t.Error("FormatInfo() should start with ℹ️ emoji")
			}
		})
	}
}

// TestFormatWarning tests the FormatWarning function
func TestFormatWarning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		title  string
		detail string
		want   string
	}{
		{
			name:   "Standard warning",
			title:  "配額警告",
			detail: "即將達到上限",
			want:   "⚠️ 配額警告\n\n即將達到上限",
		},
		{
			name:   "Long detail",
			title:  "注意",
			detail: "這是一個很長的警告訊息，包含許多細節和說明",
			want:   "⚠️ 注意\n\n這是一個很長的警告訊息，包含許多細節和說明",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatWarning(tt.title, tt.detail)
			if got != tt.want {
				t.Errorf("FormatWarning() = %q, want %q", got, tt.want)
			}
			// Verify emoji is present
			if !strings.HasPrefix(got, "⚠️") {
				t.Error("FormatWarning() should start with ⚠️ emoji")
			}
		})
	}
}

// TestFormatSuccess tests the FormatSuccess function
func TestFormatSuccess(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		title  string
		detail string
		want   string
	}{
		{
			name:   "Standard success",
			title:  "操作完成",
			detail: "資料已成功儲存",
			want:   "✅ 操作完成\n\n資料已成功儲存",
		},
		{
			name:   "Simple success",
			title:  "成功",
			detail: "完成",
			want:   "✅ 成功\n\n完成",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatSuccess(tt.title, tt.detail)
			if got != tt.want {
				t.Errorf("FormatSuccess() = %q, want %q", got, tt.want)
			}
			// Verify emoji is present
			if !strings.HasPrefix(got, "✅") {
				t.Error("FormatSuccess() should start with ✅ emoji")
			}
		})
	}
}

// TestFormatFunctionsConsistency tests that all format functions have consistent structure
func TestFormatFunctionsConsistency(t *testing.T) {
	t.Parallel()
	title := "標題"
	detail := "詳細內容"

	formats := []struct {
		name  string
		fn    func(string, string) string
		emoji string
	}{
		{"FormatError", FormatError, "❌"},
		{"FormatInfo", FormatInfo, "ℹ️"},
		{"FormatWarning", FormatWarning, "⚠️"},
		{"FormatSuccess", FormatSuccess, "✅"},
	}

	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			result := f.fn(title, detail)

			// Check structure: emoji + space + title + double newline + detail
			expectedPattern := f.emoji + " " + title + "\n\n" + detail
			if result != expectedPattern {
				t.Errorf("%s() = %q, want pattern %q", f.name, result, expectedPattern)
			}

			// Check that result contains title
			if !strings.Contains(result, title) {
				t.Errorf("%s() result should contain title %q", f.name, title)
			}

			// Check that result contains detail
			if !strings.Contains(result, detail) {
				t.Errorf("%s() result should contain detail %q", f.name, detail)
			}

			// Check double newline separator
			if !strings.Contains(result, "\n\n") {
				t.Errorf("%s() result should contain double newline separator", f.name)
			}
		})
	}
}

func TestNewMessageAction(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			result := TruncateRunes(tt.text, tt.maxLen)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestErrorMessage tests that technical errors are NOT exposed to users
func TestErrorMessage(t *testing.T) {
	t.Parallel()
	err := errors.New("database connection failed")
	sender := &messaging_api.Sender{Name: "系統小幫手", IconUrl: "https://example.com/avatar.png"}
	msg := ErrorMessageWithSender(err, sender)

	textMsg, ok := msg.(*messaging_api.TextMessageV2)
	if !ok {
		t.Fatal("Expected *messaging_api.TextMessageV2")
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			result := FormatDisplayName(tt.nameCN, tt.nameEN)
			if result != tt.expected {
				t.Errorf("FormatDisplayName(%q, %q) = %q, want %q",
					tt.nameCN, tt.nameEN, result, tt.expected)
			}
		})
	}
}

func TestBuildTelURI(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			result := BuildTelURI(tt.mainPhone, tt.extension)
			if result != tt.expected {
				t.Errorf("BuildTelURI(%q, %q) = %q, want %q",
					tt.mainPhone, tt.extension, result, tt.expected)
			}
		})
	}
}

func TestNewTextMessageWithConsistentSender(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		text           string
		senderName     string
		stickerIconURL string
	}{
		{
			name:           "With sticker icon",
			text:           "Hello, World!",
			senderName:     "學號小幫手",
			stickerIconURL: "https://stickershop.line-scdn.net/stickershop/v1/sticker/52002734/android/sticker.png",
		},
		{
			name:           "With UI avatar",
			text:           "查詢結果",
			senderName:     "聯繫小幫手",
			stickerIconURL: "https://ui-avatars.com/api/?name=A&size=256",
		},
		{
			name:           "Empty sticker URL",
			text:           "測試訊息",
			senderName:     "課程小幫手",
			stickerIconURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			result := FormatTeachers(tt.teachers, tt.max)
			if result != tt.expected {
				t.Errorf("FormatTeachers(%v, %d) = %q, want %q",
					tt.teachers, tt.max, result, tt.expected)
			}
		})
	}
}

func TestFormatTimes(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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

// TestExtractCourseCode tests the course code extraction from UID strings
func TestExtractCourseCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		uid      string
		expected string
	}{
		{
			name:     "Valid UID with U code",
			uid:      "11312U0001",
			expected: "U0001",
		},
		{
			name:     "Valid UID with M code",
			uid:      "1131M0002",
			expected: "M0002",
		},
		{
			name:     "Valid UID with N code",
			uid:      "11321N1234",
			expected: "N1234",
		},
		{
			name:     "Valid UID with P code",
			uid:      "11312P9999",
			expected: "P9999",
		},
		{
			name:     "Lowercase code - returns uppercase",
			uid:      "11312u0001",
			expected: "U0001",
		},
		{
			name:     "Mixed case code",
			uid:      "11312m0002",
			expected: "M0002",
		},
		{
			name:     "Short year (2 digits)",
			uid:      "121U0001",
			expected: "U0001",
		},
		{
			name:     "Empty string",
			uid:      "",
			expected: "",
		},
		{
			name:     "No valid code pattern",
			uid:      "11312X0001",
			expected: "",
		},
		{
			name:     "Incomplete code pattern",
			uid:      "11312U001",
			expected: "",
		},
		{
			name:     "Only numbers",
			uid:      "1131200001",
			expected: "",
		},
		{
			name:     "Code without year prefix",
			uid:      "U0001",
			expected: "U0001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCourseCode(tt.uid)
			if result != tt.expected {
				t.Errorf("ExtractCourseCode(%q) = %q, want %q",
					tt.uid, result, tt.expected)
			}
		})
	}
}

// TestFormatSemester tests the semester formatting function
func TestFormatSemester(t *testing.T) {
	tests := []struct {
		name     string
		year     int
		term     int
		expected string
	}{
		{
			name:     "First semester (上學期)",
			year:     113,
			term:     1,
			expected: "113 學年度 上學期",
		},
		{
			name:     "Second semester (下學期)",
			year:     113,
			term:     2,
			expected: "113 學年度 下學期",
		},
		{
			name:     "Older year - first semester",
			year:     100,
			term:     1,
			expected: "100 學年度 上學期",
		},
		{
			name:     "Older year - second semester",
			year:     100,
			term:     2,
			expected: "100 學年度 下學期",
		},
		{
			name:     "Invalid term value (defaults to 上學期)",
			year:     113,
			term:     0,
			expected: "113 學年度 上學期",
		},
		{
			name:     "Invalid term value 3 (defaults to 上學期)",
			year:     113,
			term:     3,
			expected: "113 學年度 上學期",
		},
		{
			name:     "Negative term (defaults to 上學期)",
			year:     113,
			term:     -1,
			expected: "113 學年度 上學期",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSemester(tt.year, tt.term)
			if result != tt.expected {
				t.Errorf("FormatSemester(%d, %d) = %q, want %q",
					tt.year, tt.term, result, tt.expected)
			}
		})
	}
}

// TestGetSemesterLabel tests the data-driven semester label logic
func TestGetSemesterLabel(t *testing.T) {
	// Test data: 4 semesters sorted newest first
	// This simulates actual course data with 113-2, 113-1, 112-2, 112-1
	dataSemesters := []SemesterPair{
		{Year: 113, Term: 2}, // Index 0: 最新學期
		{Year: 113, Term: 1}, // Index 1: 上個學期
		{Year: 112, Term: 2}, // Index 2: 過去學期
		{Year: 112, Term: 1}, // Index 3: 過去學期
	}

	tests := []struct {
		name          string
		year          int
		term          int
		dataSemesters []SemesterPair
		wantEmoji     string
		wantLabel     string
		wantColor     string
	}{
		{
			name:          "Newest semester in data (最新學期)",
			year:          113,
			term:          2,
			dataSemesters: dataSemesters,
			wantEmoji:     "🆕",
			wantLabel:     "最新學期",
			wantColor:     ColorHeaderRecent,
		},
		{
			name:          "Second semester in data (上個學期)",
			year:          113,
			term:          1,
			dataSemesters: dataSemesters,
			wantEmoji:     "📅",
			wantLabel:     "上個學期",
			wantColor:     ColorHeaderPrevious,
		},
		{
			name:          "Third semester in data (過去學期)",
			year:          112,
			term:          2,
			dataSemesters: dataSemesters,
			wantEmoji:     "📦",
			wantLabel:     "過去學期",
			wantColor:     ColorHeaderHistorical,
		},
		{
			name:          "Fourth semester in data (過去學期)",
			year:          112,
			term:          1,
			dataSemesters: dataSemesters,
			wantEmoji:     "📦",
			wantLabel:     "過去學期",
			wantColor:     ColorHeaderHistorical,
		},
		{
			name:          "Semester not in data list (過去學期)",
			year:          111,
			term:          2,
			dataSemesters: dataSemesters,
			wantEmoji:     "📦",
			wantLabel:     "過去學期",
			wantColor:     ColorHeaderHistorical,
		},
		{
			name:          "Single semester data (最新學期)",
			year:          114,
			term:          1,
			dataSemesters: []SemesterPair{{Year: 114, Term: 1}},
			wantEmoji:     "🆕",
			wantLabel:     "最新學期",
			wantColor:     ColorHeaderRecent,
		},
		{
			name:          "Empty data list (過去學期)",
			year:          113,
			term:          2,
			dataSemesters: []SemesterPair{},
			wantEmoji:     "📦",
			wantLabel:     "過去學期",
			wantColor:     ColorHeaderHistorical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := GetSemesterLabel(tt.year, tt.term, tt.dataSemesters)
			if label.Emoji != tt.wantEmoji {
				t.Errorf("GetSemesterLabel().Emoji = %q, want %q", label.Emoji, tt.wantEmoji)
			}
			if label.Label != tt.wantLabel {
				t.Errorf("GetSemesterLabel().Label = %q, want %q", label.Label, tt.wantLabel)
			}
			if label.Color != tt.wantColor {
				t.Errorf("GetSemesterLabel().Color = %q, want %q", label.Color, tt.wantColor)
			}
		})
	}
}

func TestGetExtendedSemesterLabel(t *testing.T) {
	dataSemesters := []SemesterPair{
		{Year: 113, Term: 2},
		{Year: 113, Term: 1},
	}

	tests := []struct {
		name          string
		year          int
		term          int
		dataSemesters []SemesterPair
		wantEmoji     string
		wantLabel     string
		wantColor     string
	}{
		{
			name:          "Newest extended semester (上上學期)",
			year:          113,
			term:          2,
			dataSemesters: dataSemesters,
			wantEmoji:     "📅",
			wantLabel:     "上上學期",
			wantColor:     ColorHeaderPrevious,
		},
		{
			name:          "Older extended semester (上上上學期)",
			year:          113,
			term:          1,
			dataSemesters: dataSemesters,
			wantEmoji:     "📦",
			wantLabel:     "上上上學期",
			wantColor:     ColorHeaderHistorical,
		},
		{
			name:          "Semester not in data list (上上上學期)",
			year:          112,
			term:          2,
			dataSemesters: dataSemesters,
			wantEmoji:     "📦",
			wantLabel:     "上上上學期",
			wantColor:     ColorHeaderHistorical,
		},
		{
			name:          "Empty data list (上上上學期)",
			year:          113,
			term:          2,
			dataSemesters: []SemesterPair{},
			wantEmoji:     "📦",
			wantLabel:     "上上上學期",
			wantColor:     ColorHeaderHistorical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := GetExtendedSemesterLabel(tt.year, tt.term, tt.dataSemesters)
			if label.Emoji != tt.wantEmoji {
				t.Errorf("GetExtendedSemesterLabel().Emoji = %q, want %q", label.Emoji, tt.wantEmoji)
			}
			if label.Label != tt.wantLabel {
				t.Errorf("GetExtendedSemesterLabel().Label = %q, want %q", label.Label, tt.wantLabel)
			}
			if label.Color != tt.wantColor {
				t.Errorf("GetExtendedSemesterLabel().Color = %q, want %q", label.Color, tt.wantColor)
			}
		})
	}
}

func TestSetQuoteToken(t *testing.T) {
	t.Parallel()

	t.Run("TextMessage - sets token", func(t *testing.T) {
		t.Parallel()
		msg := NewTextMessage("Hello")
		SetQuoteToken(msg, "quote-token-123")

		if msg.QuoteToken != "quote-token-123" {
			t.Errorf("Expected QuoteToken 'quote-token-123', got %q", msg.QuoteToken)
		}
	})

	t.Run("TextMessage - empty token is no-op", func(t *testing.T) {
		t.Parallel()
		msg := NewTextMessage("Hello")
		msg.QuoteToken = "existing"
		SetQuoteToken(msg, "")

		if msg.QuoteToken != "existing" {
			t.Errorf("Expected QuoteToken to remain 'existing', got %q", msg.QuoteToken)
		}
	})

	t.Run("FlexMessage - no-op (not supported)", func(t *testing.T) {
		t.Parallel()
		bubble := NewFlexBubble(nil, nil, nil, nil)
		msg := NewFlexMessage("Alt text", bubble.FlexBubble)
		result := SetQuoteToken(msg, "quote-token-456")

		// Should return the same message (for chaining)
		if result != msg {
			t.Error("SetQuoteToken should return the same message")
		}
		// FlexMessage doesn't have QuoteToken field - just verify no panic
	})

	t.Run("nil message - no panic", func(t *testing.T) {
		t.Parallel()
		// Should not panic
		result := SetQuoteToken(nil, "token")
		if result != nil {
			t.Error("SetQuoteToken(nil) should return nil")
		}
	})

	t.Run("returns message for chaining", func(t *testing.T) {
		t.Parallel()
		msg := NewTextMessage("Hello")
		result := SetQuoteToken(msg, "token")
		if result != msg {
			t.Error("SetQuoteToken should return the same message for chaining")
		}
	})
}

func TestSetQuoteTokenToFirst(t *testing.T) {
	t.Parallel()

	t.Run("single message - sets token", func(t *testing.T) {
		t.Parallel()
		msg := NewTextMessage("Hello")
		messages := []messaging_api.MessageInterface{msg}

		SetQuoteTokenToFirst(messages, "quote-token-first")

		if msg.QuoteToken != "quote-token-first" {
			t.Errorf("Expected QuoteToken 'quote-token-first', got %q", msg.QuoteToken)
		}
	})

	t.Run("multiple messages - no-op (reduced clutter)", func(t *testing.T) {
		t.Parallel()
		msg1 := NewTextMessage("First")
		msg2 := NewTextMessage("Second")
		messages := []messaging_api.MessageInterface{msg1, msg2}

		SetQuoteTokenToFirst(messages, "quote-token-first")

		// With multiple messages, quote token should not be set for better UX
		if msg1.QuoteToken != "" {
			t.Errorf("Expected first message QuoteToken to be empty, got %q", msg1.QuoteToken)
		}
		if msg2.QuoteToken != "" {
			t.Errorf("Expected second message QuoteToken to be empty, got %q", msg2.QuoteToken)
		}
	})

	t.Run("empty slice - no panic", func(t *testing.T) {
		t.Parallel()
		// Should not panic
		SetQuoteTokenToFirst([]messaging_api.MessageInterface{}, "token")
	})

	t.Run("nil slice - no panic", func(t *testing.T) {
		t.Parallel()
		// Should not panic
		SetQuoteTokenToFirst(nil, "token")
	})

	t.Run("empty token - no-op", func(t *testing.T) {
		t.Parallel()
		msg := NewTextMessage("Hello")
		msg.QuoteToken = "existing"
		SetQuoteTokenToFirst([]messaging_api.MessageInterface{msg}, "")

		if msg.QuoteToken != "existing" {
			t.Errorf("Expected QuoteToken to remain 'existing', got %q", msg.QuoteToken)
		}
	})

	t.Run("single FlexMessage - no-op (not supported)", func(t *testing.T) {
		t.Parallel()
		bubble := NewFlexBubble(nil, nil, nil, nil)
		flexMsg := NewFlexMessage("Alt", bubble.FlexBubble)
		messages := []messaging_api.MessageInterface{flexMsg}

		// Should not panic, just be a no-op for FlexMessage
		SetQuoteTokenToFirst(messages, "token")
		// FlexMessage doesn't have QuoteToken field - just verify no panic
	})
}
