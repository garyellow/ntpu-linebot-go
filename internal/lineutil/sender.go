package lineutil

import (
	"strings"

	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// GetSender creates a sender with consistent random sticker icon for a single reply session.
// This ensures all messages in a single reply use the same avatar icon, providing better UX.
//
// Usage:
//
//	sender := lineutil.GetSender("學號小幫手", stickerManager)
//	msg1 := &messaging_api.TextMessage{Text: "訊息1", Sender: sender}
//	msg2 := &messaging_api.TextMessage{Text: "訊息2", Sender: sender}
func GetSender(name string, stickerManager *sticker.Manager) *messaging_api.Sender {
	iconURL := stickerManager.GetRandomSticker()
	return &messaging_api.Sender{
		Name:    name,
		IconUrl: iconURL,
	}
}

// NewTextMessageWithConsistentSender creates a text message using a pre-created sender.
// This is preferred over NewTextMessageWithSender when multiple messages need the same sender.
//
// The text parameter is the message content.
// LINE API limits: max 5000 characters per text message
func NewTextMessageWithConsistentSender(text string, sender *messaging_api.Sender) *messaging_api.TextMessage {
	// Validate and truncate if necessary (LINE API limit: 5000 chars)
	if len(text) > 5000 {
		text = TruncateRunes(text, 4997) + "..."
	}

	return &messaging_api.TextMessage{
		Text:   text,
		Sender: sender,
	}
}

// ================================================
// Common Error Message Helpers
// ================================================

const (
	// Generic error message template
	errorMessageTemplate = "❌ 系統暫時無法處理您的請求\n\n請稍後再試，或聯絡管理員協助。\n\n如問題持續發生，請提供查詢內容以便我們協助處理。"
	// Error message with detail template (prefix + detail + suffix)
	errorDetailPrefix = "❌ "
	errorDetailSuffix = "\n\n請稍後再試，或聯絡管理員協助。"
)

// ErrorMessageWithSender creates a user-friendly error message with a pre-created sender.
func ErrorMessageWithSender(err error, sender *messaging_api.Sender) messaging_api.MessageInterface {
	return NewTextMessageWithConsistentSender(errorMessageTemplate, sender)
}

// ErrorMessageWithDetailAndSender creates an error message with additional context.
func ErrorMessageWithDetailAndSender(userMessage string, sender *messaging_api.Sender) messaging_api.MessageInterface {
	return NewTextMessageWithConsistentSender(errorDetailPrefix+userMessage+errorDetailSuffix, sender)
}

// ErrorMessageWithQuickReply creates an error message with quick reply actions.
// By default, it shows retry and help quick replies, but you can provide custom quick reply items.
// If no quickReplies are provided, it falls back to retry/help pattern.
func ErrorMessageWithQuickReply(userMessage string, sender *messaging_api.Sender, retryText string, quickReplies ...QuickReplyItem) *messaging_api.TextMessage {
	msg := NewTextMessageWithConsistentSender(errorDetailPrefix+userMessage+errorDetailSuffix, sender)
	if len(quickReplies) > 0 {
		msg.QuickReply = NewQuickReply(quickReplies)
	} else {
		msg.QuickReply = NewQuickReply([]QuickReplyItem{
			QuickReplyRetryAction(retryText),
			QuickReplyHelpAction(),
		})
	}
	return msg
}

// NotFoundMessage creates a standardized "not found" message with search suggestions.
// Parameters:
//   - searchTerm: The term that was searched for
//   - itemType: What was being searched (e.g., "課程", "聯絡資料", "學生")
//   - suggestions: Optional suggestion lines (will be formatted as bullet points)
//   - sender: The sender to use for the message
func NotFoundMessage(searchTerm, itemType string, suggestions []string, sender *messaging_api.Sender) *messaging_api.TextMessage {
	var builder strings.Builder
	if searchTerm != "" {
		builder.WriteString("🔍 查無包含「")
		builder.WriteString(searchTerm)
		builder.WriteString("」的")
		builder.WriteString(itemType)
	} else {
		builder.WriteString("🔍 查無")
		builder.WriteString(itemType)
	}

	if len(suggestions) > 0 {
		builder.WriteString("\n\n💡 建議：")
		for _, s := range suggestions {
			builder.WriteString("\n• ")
			builder.WriteString(s)
		}
	}

	return NewTextMessageWithConsistentSender(builder.String(), sender)
}
