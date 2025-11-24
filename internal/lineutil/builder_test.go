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
