package lineutil

import (
	"errors"
	"testing"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

func TestNewTextMessage(t *testing.T) {
	text := "Hello, World!"
	msg := NewTextMessage(text)

	textMsg, ok := msg.(*messaging_api.TextMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TextMessage")
	}

	if textMsg.Text != text {
		t.Errorf("Expected text %q, got %q", text, textMsg.Text)
	}

	if textMsg.Type != "text" {
		t.Errorf("Expected type 'text', got %q", textMsg.Type)
	}
}

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
	uriAction, ok := action.(*messaging_api.URIAction)
	if !ok {
		t.Fatal("Expected *messaging_api.URIAction")
	}

	if uriAction.Label != label {
		t.Errorf("Expected label %q, got %q", label, uriAction.Label)
	}

	if uriAction.Uri != uri {
		t.Errorf("Expected uri %q, got %q", uri, uriAction.Uri)
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected string
	}{
		{
			name:     "No truncation needed",
			text:     "Short text",
			maxLen:   20,
			expected: "Short text",
		},
		{
			name:     "Truncate with ellipsis",
			text:     "This is a very long text that needs truncation",
			maxLen:   20,
			expected: "This is a very lo...",
		},
		{
			name:     "Exact length",
			text:     "Exactly20Characters!",
			maxLen:   20,
			expected: "Exactly20Characters!",
		},
		{
			name:     "Very short maxLen",
			text:     "Hello",
			maxLen:   3,
			expected: "Hel",
		},
		{
			name:     "Empty string",
			text:     "",
			maxLen:   10,
			expected: "",
		},
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

func TestSplitMessages(t *testing.T) {
	tests := []struct {
		name            string
		messageCount    int
		maxCount        int
		expectedBatches int
		lastBatchSize   int
	}{
		{
			name:            "Exactly one batch",
			messageCount:    5,
			maxCount:        5,
			expectedBatches: 1,
			lastBatchSize:   5,
		},
		{
			name:            "Two batches",
			messageCount:    7,
			maxCount:        5,
			expectedBatches: 2,
			lastBatchSize:   2,
		},
		{
			name:            "Empty messages",
			messageCount:    0,
			maxCount:        5,
			expectedBatches: 0,
			lastBatchSize:   0,
		},
		{
			name:            "Single message",
			messageCount:    1,
			maxCount:        5,
			expectedBatches: 1,
			lastBatchSize:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := make([]messaging_api.MessageInterface, tt.messageCount)
			for i := 0; i < tt.messageCount; i++ {
				messages[i] = NewTextMessage("Test message")
			}

			batches := SplitMessages(messages, tt.maxCount)

			if len(batches) != tt.expectedBatches {
				t.Errorf("Expected %d batches, got %d", tt.expectedBatches, len(batches))
			}

			if tt.expectedBatches > 0 {
				lastBatch := batches[len(batches)-1]
				if len(lastBatch) != tt.lastBatchSize {
					t.Errorf("Expected last batch size %d, got %d", tt.lastBatchSize, len(lastBatch))
				}
			}
		})
	}
}

func TestErrorMessage(t *testing.T) {
	err := errors.New("test error")
	msg := ErrorMessage(err)

	textMsg, ok := msg.(*messaging_api.TextMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TextMessage")
	}

	if textMsg.Text == "" {
		t.Error("Error message text should not be empty")
	}

	// Should contain the error text
	if !contains(textMsg.Text, "test error") {
		t.Errorf("Expected message to contain 'test error', got %q", textMsg.Text)
	}
}

func TestServiceUnavailableMessage(t *testing.T) {
	msg := ServiceUnavailableMessage()

	textMsg, ok := msg.(*messaging_api.TextMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TextMessage")
	}

	if textMsg.Text == "" {
		t.Error("Service unavailable message should not be empty")
	}
}

func TestNoResultsMessage(t *testing.T) {
	msg := NoResultsMessage()

	textMsg, ok := msg.(*messaging_api.TextMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TextMessage")
	}

	if textMsg.Text == "" {
		t.Error("No results message should not be empty")
	}
}

func TestDataExpiredWarningMessage(t *testing.T) {
	tests := []struct {
		name string
		year int
	}{
		{"Recent year", 2024},
		{"Future year", 2025},
		{"Past year", 2020},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := DataExpiredWarningMessage(tt.year)

			textMsg, ok := msg.(*messaging_api.TextMessage)
			if !ok {
				t.Fatal("Expected *messaging_api.TextMessage")
			}

			if textMsg.Text == "" {
				t.Error("Data expired warning message should not be empty")
			}
		})
	}
}

func TestFormatList(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		items    []string
		contains []string
	}{
		{
			name:     "Normal list",
			title:    "Test List",
			items:    []string{"Item 1", "Item 2", "Item 3"},
			contains: []string{"Test List", "1. Item 1", "2. Item 2", "3. Item 3"},
		},
		{
			name:     "Empty list",
			title:    "Empty",
			items:    []string{},
			contains: []string{"Empty", "(無項目)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatList(tt.title, tt.items)

			for _, expected := range tt.contains {
				if !contains(result, expected) {
					t.Errorf("Expected result to contain %q, got %q", expected, result)
				}
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	field := "學號"
	message := "格式不正確"

	err := NewValidationError(field, message)

	valErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatal("Expected *ValidationError")
	}

	if valErr.Field != field {
		t.Errorf("Expected field %q, got %q", field, valErr.Field)
	}

	if valErr.Message != message {
		t.Errorf("Expected message %q, got %q", message, valErr.Message)
	}

	errStr := err.Error()
	if !contains(errStr, field) || !contains(errStr, message) {
		t.Errorf("Error string should contain field and message, got %q", errStr)
	}
}

func TestValidationErrorMessage(t *testing.T) {
	field := "email"
	message := "Invalid format"

	msg := ValidationErrorMessage(field, message)

	textMsg, ok := msg.(*messaging_api.TextMessage)
	if !ok {
		t.Fatal("Expected *messaging_api.TextMessage")
	}

	if !contains(textMsg.Text, field) {
		t.Errorf("Expected message to contain field %q", field)
	}

	if !contains(textMsg.Text, message) {
		t.Errorf("Expected message to contain message %q", message)
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
