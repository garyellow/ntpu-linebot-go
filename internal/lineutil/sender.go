package lineutil

import (
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// GetSender creates a sender with consistent random sticker icon for a single reply session.
// This ensures all messages in a single reply use the same avatar icon, providing better UX.
//
// Usage:
//
//	sender := lineutil.GetSender("學號魔法師", stickerManager)
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
	// Validate and truncate if necessary
	if len(text) > 5000 {
		text = TruncateText(text, 4997) + "..."
	}

	return &messaging_api.TextMessage{
		Text:   text,
		Sender: sender,
	}
}

// ErrorMessageWithSender creates a user-friendly error message with a pre-created sender.
func ErrorMessageWithSender(err error, sender *messaging_api.Sender) messaging_api.MessageInterface {
	return NewTextMessageWithConsistentSender("❌ 系統暫時無法處理您的請求\n\n請稍後再試，或聯絡管理員協助。\n\n如問題持續發生，請提供查詢內容以便我們協助處理。", sender)
}

// ErrorMessageWithDetailAndSender creates an error message with additional context.
func ErrorMessageWithDetailAndSender(userMessage string, sender *messaging_api.Sender) messaging_api.MessageInterface {
	return NewTextMessageWithConsistentSender("❌ "+userMessage+"\n\n請稍後再試，或聯絡管理員協助。", sender)
}
