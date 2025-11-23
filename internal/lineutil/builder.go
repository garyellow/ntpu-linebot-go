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

// NewImageMessage creates an image message with the given URLs.
// The originalContentURL is the full-size image URL, and previewImageURL is the thumbnail.
// LINE API requires both URLs to be HTTPS.
func NewImageMessage(originalContentURL, previewImageURL string) messaging_api.MessageInterface {
	return &messaging_api.ImageMessage{
		OriginalContentUrl: originalContentURL,
		PreviewImageUrl:    previewImageURL,
	}
}

// NewTextMessage creates a simple text message without sender information.
// The text parameter is the message content.
// LINE API limits: max 5000 characters per text message
func NewTextMessage(text string) *messaging_api.TextMessage {
	// Validate and truncate if necessary
	if len(text) > 5000 {
		text = TruncateText(text, 4997) + "..."
	}

	return &messaging_api.TextMessage{
		Text: text,
	}
}

// NewTextMessageWithSender creates a text message with sender information (name and icon).
// The text parameter is the message content, senderName is the displayed name,
// and stickerIconURL is the icon image URL (e.g., random sticker).
// LINE API limits: max 5000 characters per text message
func NewTextMessageWithSender(text, senderName, stickerIconURL string) *messaging_api.TextMessage {
	// Validate and truncate if necessary
	if len(text) > 5000 {
		text = TruncateText(text, 4997) + "..."
	}

	msg := &messaging_api.TextMessage{
		Text: text,
	}

	// Add sender information if provided
	if senderName != "" || stickerIconURL != "" {
		msg.Sender = &messaging_api.Sender{}
		if senderName != "" {
			msg.Sender.Name = senderName
		}
		if stickerIconURL != "" {
			msg.Sender.IconUrl = stickerIconURL
		}
	}

	return msg
}

// NewCarouselTemplate creates a carousel template message with multiple columns.
// The altText is displayed in push notifications and chat lists.
// The columns parameter contains the carousel columns to display.
// LINE API limits: max 10 columns, each with max 4 actions
func NewCarouselTemplate(altText string, columns []CarouselColumn) messaging_api.MessageInterface {
	// Validate column count (LINE API limit: max 10 columns)
	if len(columns) > 10 {
		columns = columns[:10]
	}

	// Validate altText length (max 400 characters)
	if len(altText) > 400 {
		altText = TruncateText(altText, 397) + "..."
	}

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
// LINE API limits: max 4 actions, text max 160 chars (no image) or 60 chars (with image)
func NewButtonsTemplate(altText, title, text string, actions []Action) messaging_api.MessageInterface {
	return NewButtonsTemplateWithImage(altText, title, text, "", actions)
}

// NewButtonsTemplateWithImage creates a buttons template message with an optional thumbnail image.
// The altText is displayed in push notifications and chat lists.
// The title is the template title, text is the message content, thumbnailImageURL is optional image.
// LINE API limits: max 4 actions, text max 60 chars (with image) or 160 chars (no image)
func NewButtonsTemplateWithImage(altText, title, text, thumbnailImageURL string, actions []Action) messaging_api.MessageInterface {
	// Validate action count (LINE API limit: max 4 actions)
	if len(actions) > 4 {
		actions = actions[:4]
	}

	// Validate text length based on whether image is present
	maxTextLen := 160
	if thumbnailImageURL != "" {
		maxTextLen = 60
	}
	if len(text) > maxTextLen {
		text = TruncateText(text, maxTextLen-3) + "..."
	}

	// Validate title length (max 40 characters)
	if len(title) > 40 {
		title = TruncateText(title, 37) + "..."
	}

	// Validate altText length (max 400 characters)
	if len(altText) > 400 {
		altText = TruncateText(altText, 397) + "..."
	}

	template := &messaging_api.ButtonsTemplate{
		Text:    text,
		Actions: actions,
	}

	if title != "" {
		template.Title = title
	}

	if thumbnailImageURL != "" {
		template.ThumbnailImageUrl = thumbnailImageURL
	}

	return &messaging_api.TemplateMessage{
		AltText:  altText,
		Template: template,
	}
}

// NewQuickReply creates a quick reply message component.
// The items parameter contains the quick reply buttons to display.
// Returns a QuickReply object that can be attached to text or template messages.
// LINE API limits: max 13 items
func NewQuickReply(items []QuickReplyItem) *messaging_api.QuickReply {
	// Validate item count (LINE API limit: max 13 items)
	if len(items) > 13 {
		items = items[:13]
	}

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

// NewPostbackActionWithDisplayText creates a postback action with custom display text.
// The label is displayed on the button, displayText is shown when clicked, data is sent as postback.
func NewPostbackActionWithDisplayText(label, displayText, data string) Action {
	return &messaging_api.PostbackAction{
		Label:       label,
		DisplayText: displayText,
		Data:        data,
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

// NewClipboardAction creates a clipboard action that copies text when clicked.
// The label is displayed on the button, and clipboardText is the text to copy.
func NewClipboardAction(label, clipboardText string) Action {
	return &messaging_api.ClipboardAction{
		Label:         label,
		ClipboardText: clipboardText,
	}
}

// ErrorMessage creates a user-friendly error message with sender information.
// The err parameter is the internal error (technical details are hidden from users).
// The senderName is displayed as the message sender.
// The stickerURL is the avatar icon URL.
// This provides a consistent error experience without exposing implementation details.
func ErrorMessage(err error, senderName, stickerURL string) messaging_api.MessageInterface {
	// Log the actual error internally (caller should log)
	// But show user-friendly message to end users
	return NewTextMessageWithSender("❌ 系統暫時無法處理您的請求\n\n請稍後再試，或聯絡管理員協助。\n\n如問題持續發生，請提供查詢內容以便我們協助處理。", senderName, stickerURL)
}

// ErrorMessageWithDetail creates an error message with additional context for debugging.
// Use this when you want to show users what went wrong while keeping it user-friendly.
// The senderName is displayed as the message sender.
// The stickerURL is the avatar icon URL.
func ErrorMessageWithDetail(userMessage, senderName, stickerURL string) messaging_api.MessageInterface {
	return NewTextMessageWithSender(fmt.Sprintf("❌ %s\n\n請稍後再試，或聯絡管理員協助。", userMessage), senderName, stickerURL)
}

// ServiceUnavailableMessage creates a message indicating the service is unavailable.
// The senderName is displayed as the message sender.
// The stickerURL is the avatar icon URL.
func ServiceUnavailableMessage(senderName, stickerURL string) messaging_api.MessageInterface {
	return NewTextMessageWithSender("⚠️ 服務暫時無法使用\n\n系統正在維護中，請稍後再試。", senderName, stickerURL)
}

// NoResultsMessage creates a message indicating no search results were found.
// The senderName is displayed as the message sender.
// The stickerURL is the avatar icon URL.
func NoResultsMessage(senderName, stickerURL string) messaging_api.MessageInterface {
	return NewTextMessageWithSender("🔍 查無資料\n\n請檢查輸入的關鍵字是否正確，或嘗試其他搜尋條件。", senderName, stickerURL)
}

// DataExpiredWarningMessage creates a warning message for potentially outdated data.
// The year parameter indicates the data year that may be expired.
// The senderName is displayed as the message sender.
// The stickerURL is the avatar icon URL.
func DataExpiredWarningMessage(year int, senderName, stickerURL string) messaging_api.MessageInterface {
	if year >= 2024 {
		return NewTextMessageWithSender(fmt.Sprintf(
			"⚠️ 資料更新提醒\n\n%d 年度的資料可能尚未更新或已過期。\n如有疑問，請洽詢相關單位確認最新資訊。",
			year,
		), senderName, stickerURL)
	}
	return NewTextMessageWithSender(fmt.Sprintf(
		"ℹ️ 歷史資料提醒\n\n您查詢的是 %d 年度的資料，此資料可能已過時。\n建議查詢最新學年度的資訊。",
		year,
	), senderName, stickerURL)
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
func NewFlexMessage(altText string, contents messaging_api.FlexContainerInterface) *messaging_api.FlexMessage {
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
// The senderName is displayed as the message sender.
// The stickerURL is the avatar icon URL.
func ValidationErrorMessage(field, message, senderName, stickerURL string) messaging_api.MessageInterface {
	return NewTextMessageWithSender(fmt.Sprintf("❌ 輸入錯誤\n\n欄位：%s\n說明：%s", field, message), senderName, stickerURL)
}
