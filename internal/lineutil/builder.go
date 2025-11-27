// Package lineutil provides utility functions for building LINE messages and actions.
package lineutil

import (
	"fmt"
	"regexp"
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
	// Validate and truncate if necessary (LINE API limit: 5000 chars)
	if len(text) > 5000 {
		text = TruncateRunes(text, 4997) + "..."
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
		altText = TruncateRunes(altText, 397) + "..."
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
		text = TruncateRunes(text, maxTextLen-3) + "..."
	}

	// Validate title length (max 40 characters)
	if len(title) > 40 {
		title = TruncateRunes(title, 37) + "..."
	}

	// Validate altText length (max 400 characters)
	if len(altText) > 400 {
		altText = TruncateRunes(altText, 397) + "..."
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

// ContainsAllRunes checks if string s contains all runes from string chars.
// Example: ContainsAllRunes("資訊工程學系", "資工系") returns true
// because all characters in "資工系" exist in "資訊工程學系".
// This is case-insensitive for ASCII characters.
func ContainsAllRunes(s, chars string) bool {
	if chars == "" {
		return true
	}
	if s == "" {
		return false
	}

	// Convert to lowercase for case-insensitive matching (for ASCII)
	sLower := strings.ToLower(s)
	charsLower := strings.ToLower(chars)

	// Build a set of runes from s
	runeSet := make(map[rune]struct{})
	for _, r := range sLower {
		runeSet[r] = struct{}{}
	}

	// Check if all runes in chars exist in s
	for _, r := range charsLower {
		if _, exists := runeSet[r]; !exists {
			return false
		}
	}
	return true
}

// NewFlexMessage creates a flex message with the given alt text and flex container.
// Flex messages allow for rich, customizable layouts.
func NewFlexMessage(altText string, contents messaging_api.FlexContainerInterface) *messaging_api.FlexMessage {
	return &messaging_api.FlexMessage{
		AltText:  altText,
		Contents: contents,
	}
}

// SetSender sets the Sender field on a message.
// This is a helper function to add consistent sender information to any message type.
// Returns the same message for method chaining.
// Supports: TextMessage, FlexMessage, TemplateMessage, ImageMessage
func SetSender(msg messaging_api.MessageInterface, sender *messaging_api.Sender) messaging_api.MessageInterface {
	if sender == nil {
		return msg
	}

	switch m := msg.(type) {
	case *messaging_api.TextMessage:
		m.Sender = sender
	case *messaging_api.FlexMessage:
		m.Sender = sender
	case *messaging_api.TemplateMessage:
		m.Sender = sender
	case *messaging_api.ImageMessage:
		m.Sender = sender
	}

	return msg
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

// BuildTelURI creates a tel: URI for phone dialing with optional extension.
// Format: tel:+886286741111,12345 (E.164 format without dashes, comma for pause dial)
//
// Parameters:
//   - mainPhone: The main phone number (e.g., "0286741111")
//   - extension: Optional extension number (e.g., "12345")
//
// Returns: tel: URI string that works on iOS/Android
// Example: BuildTelURI("0286741111", "12345") -> "tel:+886286741111,12345"
func BuildTelURI(mainPhone, extension string) string {
	// Remove any existing formatting (dashes, spaces)
	phone := strings.ReplaceAll(mainPhone, "-", "")
	phone = strings.ReplaceAll(phone, " ", "")

	// Convert to international format (Taiwan +886)
	// Remove leading 0 and add +886
	if strings.HasPrefix(phone, "0") {
		phone = "+886" + phone[1:]
	} else if !strings.HasPrefix(phone, "+") {
		phone = "+886" + phone
	}

	// Add extension with comma (pause dial)
	if extension != "" {
		// Use first 5 digits of extension (standard practice)
		ext := strings.TrimSpace(extension)
		if len(ext) > 5 {
			ext = ext[:5]
		}
		return "tel:" + phone + "," + ext
	}

	return "tel:" + phone
}

// FormatDisplayName formats Chinese and English names for display.
// Rules:
//   - If English name is empty or same as Chinese name: return Chinese name only
//   - If different: return "ChineseName EnglishName" with a space separator
//
// Parameters:
//   - nameCN: Chinese name
//   - nameEN: English name (may be empty or same as Chinese)
//
// Returns: Formatted display name
func FormatDisplayName(nameCN, nameEN string) string {
	nameCN = strings.TrimSpace(nameCN)
	nameEN = strings.TrimSpace(nameEN)

	// If English name is empty or same as Chinese, return Chinese only
	if nameEN == "" || nameEN == nameCN {
		return nameCN
	}

	// Return combined name with space
	return nameCN + " " + nameEN
}

// ExtractCourseCode extracts the course code from a UID string.
// UID format: {year}{term}{code} where:
//   - year: 2-3 digits (e.g., 113, 12)
//   - term: 1 digit (1=上學期, 2=下學期)
//   - code: U/M/N/P + 4 digits (e.g., U0001, M0002)
//
// Returns the code part (e.g., "U0001" from "11312U0001")
// Returns empty string if pattern not found.
var courseCodeRegex = regexp.MustCompile(`(?i)([umnp]\d{4})`)

// ExtractCourseCode extracts the course code from a course UID.
func ExtractCourseCode(uid string) string {
	matches := courseCodeRegex.FindStringSubmatch(uid)
	if len(matches) >= 2 {
		return strings.ToUpper(matches[1])
	}
	return ""
}

// FormatCourseTitle formats a course title with optional course code.
// If courseCode is empty, returns just the title.
// Otherwise returns "title (code)" format.
//
// Example:
//
//	FormatCourseTitle("程式設計", "U0001") -> "程式設計 (U0001)"
//	FormatCourseTitle("程式設計", "") -> "程式設計"
func FormatCourseTitle(title, courseCode string) string {
	if courseCode == "" {
		return title
	}
	return fmt.Sprintf("%s (%s)", title, courseCode)
}

// FormatCourseTitleWithUID formats a course title with course code extracted from UID.
// This is a convenience function that combines ExtractCourseCode and FormatCourseTitle.
//
// Example:
//
//	FormatCourseTitleWithUID("程式設計", "1132U0001") -> "程式設計 (U0001)"
func FormatCourseTitleWithUID(title, uid string) string {
	return FormatCourseTitle(title, ExtractCourseCode(uid))
}

// FormatSemester formats year and term into a readable semester string.
// Parameters:
//   - year: Academic year in ROC calendar (e.g., 113)
//   - term: 1 for 上學期, 2 for 下學期
//
// Returns: Formatted string like "113 學年度 上學期"
func FormatSemester(year, term int) string {
	termStr := "上學期"
	if term == 2 {
		termStr = "下學期"
	}
	return fmt.Sprintf("%d 學年度 %s", year, termStr)
}

// FormatTeachers formats teacher names with optional truncation.
// If more than maxCount teachers, shows first maxCount names + "等 N 人".
// Parameters:
//   - teachers: List of teacher names
//   - maxCount: Maximum teachers to show before truncation (0 = no limit)
//
// Returns: Formatted string like "王教授、李教授 等 3 人"
func FormatTeachers(teachers []string, maxCount int) string {
	if len(teachers) == 0 {
		return ""
	}
	if maxCount <= 0 || len(teachers) <= maxCount {
		return strings.Join(teachers, "、")
	}
	remaining := len(teachers) - maxCount
	return strings.Join(teachers[:maxCount], "、") + fmt.Sprintf(" 等 %d 人", remaining)
}

// FormatTimes formats time slots with optional truncation.
// If more than maxCount time slots, shows first maxCount slots + "等 N 節".
// Parameters:
//   - times: List of time slot strings (e.g., "週一1-2", "週三3-4")
//   - maxCount: Maximum time slots to show before truncation (0 = no limit)
//
// Returns: Formatted string like "週一1-2、週三3-4 等 2 節"
func FormatTimes(times []string, maxCount int) string {
	if len(times) == 0 {
		return ""
	}
	if maxCount <= 0 || len(times) <= maxCount {
		return strings.Join(times, "、")
	}
	remaining := len(times) - maxCount
	return strings.Join(times[:maxCount], "、") + fmt.Sprintf(" 等 %d 節", remaining)
}

// ================================================
// Common QuickReply Actions (pre-defined for reuse)
// ================================================

// QuickReplyHelpAction returns a "使用說明" quick reply item
func QuickReplyHelpAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("📖 使用說明", "使用說明")}
}

// QuickReplyCourseAction returns a "課程" quick reply item
func QuickReplyCourseAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("📚 課程", "課程")}
}

// QuickReplyTeacherAction returns a "教師" quick reply item
func QuickReplyTeacherAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("👨‍🏫 教師", "教師")}
}

// QuickReplyStudentAction returns a "學號" quick reply item
func QuickReplyStudentAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("🎓 學號", "學號")}
}

// QuickReplyYearAction returns a "學年" quick reply item
func QuickReplyYearAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("📅 學年", "學年")}
}

// QuickReplyContactAction returns a "聯絡" quick reply item
func QuickReplyContactAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("📞 聯絡", "聯絡")}
}

// QuickReplyEmergencyAction returns a "緊急" quick reply item
func QuickReplyEmergencyAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("🚨 緊急", "緊急")}
}

// QuickReplyDeptCodeAction returns a "所有系代碼" quick reply item
func QuickReplyDeptCodeAction() QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("📋 所有系代碼", "所有系代碼")}
}

// QuickReplyRetryAction creates a retry quick reply item with custom text
func QuickReplyRetryAction(retryText string) QuickReplyItem {
	return QuickReplyItem{Action: NewMessageAction("🔄 重試", retryText)}
}

// ================================================
// Message Helper Functions
// ================================================

// NewTextMessageWithQuickReply creates a text message with quick reply items.
// This is a convenience function for the common pattern of creating a text message
// and attaching quick replies.
func NewTextMessageWithQuickReply(text string, sender *messaging_api.Sender, items ...QuickReplyItem) *messaging_api.TextMessage {
	msg := NewTextMessageWithConsistentSender(text, sender)
	if len(items) > 0 {
		msg.QuickReply = NewQuickReply(items)
	}
	return msg
}

// NewFlexMessageWithQuickReply creates a flex message with quick reply items and sender.
// This is a convenience function for the common pattern of creating a flex message
// with sender and quick replies.
func NewFlexMessageWithQuickReply(altText string, contents messaging_api.FlexContainerInterface, sender *messaging_api.Sender, items ...QuickReplyItem) *messaging_api.FlexMessage {
	msg := NewFlexMessage(altText, contents)
	msg.Sender = sender
	if len(items) > 0 {
		msg.QuickReply = NewQuickReply(items)
	}
	return msg
}

// AddQuickReplyToMessages attaches quick reply items to the last message in a slice.
// If the slice is empty or the last message doesn't support quick replies, it's a no-op.
// This is useful for adding quick replies to the final message of multi-message responses.
func AddQuickReplyToMessages(messages []messaging_api.MessageInterface, items ...QuickReplyItem) {
	if len(messages) == 0 || len(items) == 0 {
		return
	}
	lastMsg := messages[len(messages)-1]
	qr := NewQuickReply(items)
	switch m := lastMsg.(type) {
	case *messaging_api.TextMessage:
		m.QuickReply = qr
	case *messaging_api.FlexMessage:
		m.QuickReply = qr
	case *messaging_api.TemplateMessage:
		m.QuickReply = qr
	}
}
