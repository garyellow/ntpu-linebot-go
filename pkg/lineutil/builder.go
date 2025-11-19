// Package lineutil provides utility functions for building LINE messages and actions.
package lineutil

import (
	"fmt"
	"strings"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// CarouselColumn represents a column in a carousel template.
type CarouselColumn struct {
	ThumbnailImageURL    string
	ImageBackgroundColor string
	Title                string
	Text                 string
	Actions              []messaging_api.ActionInterface
}

// QuickReplyItem represents an item in a quick reply.
type QuickReplyItem struct {
	ImageURL string
	Action   messaging_api.ActionInterface
}

// Action is an alias for the LINE SDK action interface for convenience.
type Action = messaging_api.ActionInterface

// NewTextMessage creates a simple text message.
// The text parameter is the message content to send.
func NewTextMessage(text string) messaging_api.MessageInterface {
	return &messaging_api.TextMessage{
		Text: text,
	}
}

// NewCarouselTemplate creates a carousel template message with multiple columns.
// The altText is displayed in push notifications and chat lists.
// The columns parameter contains the carousel columns to display.
func NewCarouselTemplate(altText string, columns []CarouselColumn) messaging_api.MessageInterface {
	templateColumns := make([]messaging_api.CarouselColumn, len(columns))

	for i, col := range columns {
		column := messaging_api.CarouselColumn{
			Text:    col.Text,
			Actions: col.Actions,
		}

		if col.ThumbnailImageURL != "" {
			column.ThumbnailImageUrl = col.ThumbnailImageURL
		}
		if col.ImageBackgroundColor != "" {
			column.ImageBackgroundColor = col.ImageBackgroundColor
		}
		if col.Title != "" {
			column.Title = col.Title
		}

		templateColumns[i] = column
	}

	return &messaging_api.TemplateMessage{
		AltText: altText,
		Template: &messaging_api.CarouselTemplate{
			Columns: templateColumns,
		},
	}
}

// NewButtonsTemplate creates a buttons template message.
// The altText is displayed in push notifications and chat lists.
// The title is the template title, text is the message content, and actions are the buttons.
func NewButtonsTemplate(altText, title, text string, actions []Action) messaging_api.MessageInterface {
	template := &messaging_api.ButtonsTemplate{
		Text:    text,
		Actions: actions,
	}

	if title != "" {
		template.Title = title
	}

	return &messaging_api.TemplateMessage{
		AltText:  altText,
		Template: template,
	}
}

// NewQuickReply creates a quick reply message component.
// The items parameter contains the quick reply buttons to display.
// Returns a QuickReply object that can be attached to text or template messages.
func NewQuickReply(items []QuickReplyItem) *messaging_api.QuickReply {
	quickReplyItems := make([]messaging_api.QuickReplyItem, len(items))

	for i, item := range items {
		qrItem := messaging_api.QuickReplyItem{
			Action: item.Action,
		}

		if item.ImageURL != "" {
			qrItem.ImageUrl = item.ImageURL
		}

		quickReplyItems[i] = qrItem
	}

	return &messaging_api.QuickReply{
		Items: quickReplyItems,
	}
}

// NewConfirmTemplate creates a confirmation template with Yes/No buttons.
// The altText is displayed in push notifications and chat lists.
// The text is the confirmation question, yesAction and noAction are the button actions.
func NewConfirmTemplate(altText, text string, yesAction, noAction Action) messaging_api.MessageInterface {
	return &messaging_api.TemplateMessage{
		AltText: altText,
		Template: &messaging_api.ConfirmTemplate{
			Text:    text,
			Actions: []messaging_api.ActionInterface{yesAction, noAction},
		},
	}
}

// NewMessageAction creates a message action that sends a message when clicked.
// The label is displayed on the button, and text is the message that will be sent.
func NewMessageAction(label, text string) Action {
	return &messaging_api.MessageAction{
		Label: label,
		Text:  text,
	}
}

// NewPostbackAction creates a postback action that sends data to the bot when clicked.
// The label is displayed on the button, and data is sent as postback data.
func NewPostbackAction(label, data string) Action {
	return &messaging_api.PostbackAction{
		Label: label,
		Data:  data,
	}
}

// NewURIAction creates a URI action that opens a URL when clicked.
// The label is displayed on the button, and uri is the URL to open.
func NewURIAction(label, uri string) Action {
	return &messaging_api.UriAction{
		Label: label,
		Uri:   uri,
	}
}

// ErrorMessage creates a generic error message for any error.
// The err parameter is the error to display.
func ErrorMessage(err error) messaging_api.MessageInterface {
	return NewTextMessage(fmt.Sprintf("❌ 發生錯誤：%s\n\n請稍後再試或聯絡管理員。", err.Error()))
}

// ServiceUnavailableMessage creates a message indicating the service is unavailable.
func ServiceUnavailableMessage() messaging_api.MessageInterface {
	return NewTextMessage("⚠️ 服務暫時無法使用\n\n系統正在維護中，請稍後再試。")
}

// NoResultsMessage creates a message indicating no search results were found.
func NoResultsMessage() messaging_api.MessageInterface {
	return NewTextMessage("🔍 查無資料\n\n請檢查輸入的關鍵字是否正確，或嘗試其他搜尋條件。")
}

// DataExpiredWarningMessage creates a warning message for potentially outdated data.
// The year parameter indicates the data year that may be expired.
func DataExpiredWarningMessage(year int) messaging_api.MessageInterface {
	if year >= 2024 {
		return NewTextMessage(fmt.Sprintf(
			"⚠️ 資料更新提醒\n\n%d 年度的資料可能尚未更新或已過期。\n如有疑問，請洽詢相關單位確認最新資訊。",
			year,
		))
	}
	return NewTextMessage(fmt.Sprintf(
		"ℹ️ 歷史資料提醒\n\n您查詢的是 %d 年度的資料，此資料可能已過時。\n建議查詢最新學年度的資訊。",
		year,
	))
}

// TruncateText truncates text to a maximum length and adds "..." if truncated.
// The text parameter is the original text, and maxLen is the maximum allowed length.
func TruncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	if maxLen <= 3 {
		return text[:maxLen]
	}

	return text[:maxLen-3] + "..."
}

// SplitMessages splits a slice of messages into batches of a specified size.
// The messages parameter contains all messages to split, and maxCount is the batch size.
// LINE API has a limit of 5 messages per request, so the default should be 5.
func SplitMessages(messages []messaging_api.MessageInterface, maxCount int) [][]messaging_api.MessageInterface {
	if maxCount <= 0 {
		maxCount = 5 // Default LINE API limit
	}

	if len(messages) == 0 {
		return [][]messaging_api.MessageInterface{}
	}

	var batches [][]messaging_api.MessageInterface

	for i := 0; i < len(messages); i += maxCount {
		end := i + maxCount
		if end > len(messages) {
			end = len(messages)
		}
		batches = append(batches, messages[i:end])
	}

	return batches
}

// FormatList creates a formatted list message from a slice of items.
// The title is the list header, items are the list entries.
func FormatList(title string, items []string) string {
	if len(items) == 0 {
		return title + "\n\n(無項目)"
	}

	var builder strings.Builder
	builder.WriteString(title)
	builder.WriteString("\n\n")

	for i, item := range items {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
	}

	return builder.String()
}

// AddQuickReply adds a quick reply to a text message.
// This is a convenience function for adding quick replies to text messages.
func AddQuickReply(message *messaging_api.TextMessage, items []QuickReplyItem) *messaging_api.TextMessage {
	message.QuickReply = NewQuickReply(items)
	return message
}

// NewFlexMessage creates a flex message with the given alt text and flex container.
// Flex messages allow for rich, customizable layouts.
func NewFlexMessage(altText string, contents *messaging_api.FlexContainer) messaging_api.MessageInterface {
	return &messaging_api.FlexMessage{
		AltText:  altText,
		Contents: contents,
	}
}

// ValidationError represents an input validation error.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface for ValidationError.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NewValidationError creates a validation error message.
func NewValidationError(field, message string) error {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// ValidationErrorMessage creates a user-friendly validation error message.
func ValidationErrorMessage(field, message string) messaging_api.MessageInterface {
	return NewTextMessage(fmt.Sprintf("❌ 輸入錯誤\n\n欄位：%s\n說明：%s", field, message))
}
