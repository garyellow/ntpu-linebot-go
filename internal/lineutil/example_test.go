package lineutil_test

import (
	"fmt"

	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// ExampleNewTextMessageWithConsistentSender demonstrates creating a text message with sender information.
func ExampleNewTextMessageWithConsistentSender() {
	sender := &messaging_api.Sender{Name: "魔法師", IconUrl: "https://example.com/avatar.png"}
	msg := lineutil.NewTextMessageWithConsistentSender("Hello, World!", sender)
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TextMessage
}

// ExampleNewCarouselTemplate demonstrates creating a carousel with multiple columns.
func ExampleNewCarouselTemplate() {
	columns := []lineutil.CarouselColumn{
		{
			ThumbnailImageURL: "https://example.com/image1.jpg",
			Title:             "選項 1",
			Text:              "這是第一個選項的說明",
			Actions: []lineutil.Action{
				lineutil.NewMessageAction("選擇", "選擇選項1"),
				lineutil.NewURIAction("查看詳情", "https://example.com/1"),
			},
		},
		{
			ThumbnailImageURL: "https://example.com/image2.jpg",
			Title:             "選項 2",
			Text:              "這是第二個選項的說明",
			Actions: []lineutil.Action{
				lineutil.NewMessageAction("選擇", "選擇選項2"),
				lineutil.NewURIAction("查看詳情", "https://example.com/2"),
			},
		},
	}

	msg := lineutil.NewCarouselTemplate("請選擇一個選項", columns)
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TemplateMessage
}

// ExampleNewButtonsTemplate demonstrates creating a buttons template.
func ExampleNewButtonsTemplate() {
	actions := []lineutil.Action{
		lineutil.NewMessageAction("是", "確認"),
		lineutil.NewMessageAction("否", "取消"),
		lineutil.NewURIAction("了解更多", "https://example.com"),
	}

	msg := lineutil.NewButtonsTemplate(
		"請選擇操作",
		"操作確認",
		"您確定要執行此操作嗎?",
		actions,
	)
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TemplateMessage
}

// ExampleNewConfirmTemplate demonstrates creating a confirmation dialog.
func ExampleNewConfirmTemplate() {
	msg := lineutil.NewConfirmTemplate(
		"確認操作",
		"您確定要刪除此項目嗎?",
		lineutil.NewPostbackAction("確定", "action=delete&confirm=yes"),
		lineutil.NewPostbackAction("取消", "action=delete&confirm=no"),
	)
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TemplateMessage
}

// ExampleNewQuickReply demonstrates creating quick reply buttons.
func ExampleNewQuickReply() {
	items := []lineutil.QuickReplyItem{
		{
			Action: lineutil.NewMessageAction("課程查詢", "查詢課程"),
		},
		{
			Action: lineutil.NewMessageAction("聯絡資訊", "查詢聯絡資訊"),
		},
		{
			Action: lineutil.NewMessageAction("學號查詢", "查詢學號"),
		},
	}

	quickReply := lineutil.NewQuickReply(items)
	fmt.Printf("%T", quickReply)
	// Output: *messaging_api.QuickReply
}

// ExampleTruncateRunes demonstrates text truncation.
func ExampleTruncateRunes() {
	text := "This is a very long text that needs to be truncated"
	truncated := lineutil.TruncateRunes(text, 20)
	fmt.Println(truncated)
	// Output: This is a very lo...
}

// ExampleErrorMessageWithSender demonstrates creating error messages.
func ExampleErrorMessageWithSender() {
	err := fmt.Errorf("database connection failed")
	sender := &messaging_api.Sender{Name: "系統魔法師", IconUrl: "https://example.com/avatar.png"}
	msg := lineutil.ErrorMessageWithSender(err, sender)
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TextMessage
}
