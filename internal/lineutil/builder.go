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

// TruncateText truncates text to a maximum length and adds "..." if truncated.
// The text parameter is the original text, and maxLen is the maximum allowed length.
// Uses rune slicing to properly handle multi-byte UTF-8 characters (e.g., Chinese).
func TruncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}

	if maxLen <= 3 {
		return string(runes[:maxLen])
	}

	return string(runes[:maxLen-3]) + "..."
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
