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
//	sender := lineutil.GetSender("NTPU 小工具", stickerManager)
//	msg1 := &messaging_api.TextMessageV2{Text: "訊息1", Sender: sender}
//	msg2 := &messaging_api.TextMessageV2{Text: "訊息2", Sender: sender}
func GetSender(name string, stickerManager *sticker.Manager) *messaging_api.Sender {
	iconURL := stickerManager.GetRandomSticker()
	return &messaging_api.Sender{
		Name:    name,
		IconUrl: iconURL,
	}
}

// NewTextMessageWithConsistentSender creates a text message (v2) using a pre-created sender.
// This is preferred over NewTextMessageWithSender when multiple messages need the same sender.
// LINE API limits: max 5000 characters per text message.
func NewTextMessageWithConsistentSender(text string, sender *messaging_api.Sender) *messaging_api.TextMessageV2 {
	text = TruncateRunes(text, 5000)
	return &messaging_api.TextMessageV2{
		Text:   text,
		Sender: sender,
	}
}

// ================================================
// Common Error Message Helpers
// ================================================
//
// Error messages:
//   1. Acknowledge the problem (not blame user)
//   2. Explain what happened briefly
//   3. Provide actionable next steps
//   4. Keep tone empathetic and helpful
//
// Reference: Nielsen Norman Group Heuristic #9 - Help users recognize,
// diagnose, and recover from errors.

const (
	// Generic error message template - used for unexpected system errors
	// Structure: emoji + acknowledgment + what to do + how to get help
	errorMessageTemplate = "😅 抱歉，系統暫時無法處理您的請求\n\n" +
		"這可能是暫時性的問題，建議您：\n" +
		"• 稍後再試一次\n" +
		"• 換個方式查詢\n\n" +
		"若問題持續發生，請告知查詢內容，我們將協助處理。"

	// Error message with detail template (prefix + detail + suffix)
	// For specific, contextual errors
	errorDetailPrefix = "😅 "
	errorDetailSuffix = "\n\n💡 建議稍後再試，或換個方式查詢。"
)

// ErrorMessageWithSender creates a user-friendly error message with a pre-created sender.
// Used for unexpected system errors where we don't have specific context.
func ErrorMessageWithSender(err error, sender *messaging_api.Sender) messaging_api.MessageInterface {
	return NewTextMessageWithConsistentSender(errorMessageTemplate, sender)
}

// ErrorMessageWithDetailAndSender creates an error message with additional context.
// Used when we know the specific issue (e.g., "搜尋課程時發生問題").
func ErrorMessageWithDetailAndSender(userMessage string, sender *messaging_api.Sender) messaging_api.MessageInterface {
	return NewTextMessageWithConsistentSender(errorDetailPrefix+userMessage+errorDetailSuffix, sender)
}

// ErrorMessageWithQuickReply creates an error message with quick reply actions.
// By default, it shows retry and help quick replies, but you can provide custom quick reply items.
// If no quickReplies are provided, it falls back to retry/help pattern.
//
// This is the preferred error message function as it provides actionable next steps.
func ErrorMessageWithQuickReply(userMessage string, sender *messaging_api.Sender, retryText string, quickReplies ...QuickReplyItem) *messaging_api.TextMessageV2 {
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
// This follows the UX pattern of providing alternatives when search fails.
//
// Parameters:
//   - searchTerm: The term that was searched for
//   - itemType: What was being searched (e.g., "課程", "聯絡資料", "學生")
//   - suggestions: Optional suggestion lines (will be formatted as bullet points)
//   - sender: The sender to use for the message
func NotFoundMessage(searchTerm, itemType string, suggestions []string, sender *messaging_api.Sender) *messaging_api.TextMessageV2 {
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

// ================================================
// Context-Specific Error Builders
// ================================================

// SystemErrorMessage creates a friendly system error message with recovery options.
// Used when something unexpected goes wrong during processing.
func SystemErrorMessage(operation string, sender *messaging_api.Sender) *messaging_api.TextMessageV2 {
	msg := NewTextMessageWithConsistentSender(
		"😅 "+operation+"時發生了一點問題\n\n"+
			"這可能是暫時性的，建議：\n"+
			"• 稍等幾秒後再試\n"+
			"• 換個關鍵字查詢",
		sender,
	)
	msg.QuickReply = NewQuickReply([]QuickReplyItem{
		QuickReplyHelpAction(),
	})
	return msg
}

// NetworkErrorMessage creates an error message for network-related issues.
// Used when scraping or external API calls fail.
func NetworkErrorMessage(target string, sender *messaging_api.Sender) *messaging_api.TextMessageV2 {
	msg := NewTextMessageWithConsistentSender(
		"🌐 無法連線到"+target+"\n\n"+
			"可能原因：\n"+
			"• 網站暫時維護中\n"+
			"• 網路連線不穩定\n\n"+
			"💡 建議稍後再試",
		sender,
	)
	msg.QuickReply = NewQuickReply([]QuickReplyItem{
		QuickReplyHelpAction(),
	})
	return msg
}
