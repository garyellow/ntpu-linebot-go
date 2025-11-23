package lineutil_test

import (
	"fmt"

	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// ExampleNewTextMessageWithSender demonstrates creating a text message with sender information.
func ExampleNewTextMessageWithSender() {
	msg := lineutil.NewTextMessageWithSender("Hello, World!", "魔法師", "https://example.com/avatar.png")
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

// ExampleTruncateText demonstrates text truncation.
func ExampleTruncateText() {
	text := "This is a very long text that needs to be truncated"
	truncated := lineutil.TruncateText(text, 20)
	fmt.Println(truncated)
	// Output: This is a very lo...
}

// ExampleSplitMessages demonstrates splitting messages into batches.
func ExampleSplitMessages() {
	messages := []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithSender("Message 1", "魔法師", "https://example.com/avatar.png"),
		lineutil.NewTextMessageWithSender("Message 2", "魔法師", "https://example.com/avatar.png"),
		lineutil.NewTextMessageWithSender("Message 3", "魔法師", "https://example.com/avatar.png"),
		lineutil.NewTextMessageWithSender("Message 4", "魔法師", "https://example.com/avatar.png"),
		lineutil.NewTextMessageWithSender("Message 5", "魔法師", "https://example.com/avatar.png"),
		lineutil.NewTextMessageWithSender("Message 6", "魔法師", "https://example.com/avatar.png"),
		lineutil.NewTextMessageWithSender("Message 7", "魔法師", "https://example.com/avatar.png"),
	}

	batches := lineutil.SplitMessages(messages, 5)
	fmt.Printf("Total batches: %d, First batch size: %d, Second batch size: %d",
		len(batches), len(batches[0]), len(batches[1]))
	// Output: Total batches: 2, First batch size: 5, Second batch size: 2
}

// ExampleErrorMessage demonstrates creating error messages.
func ExampleErrorMessage() {
	err := fmt.Errorf("database connection failed")
	msg := lineutil.ErrorMessage(err, "系統魔法師", "https://example.com/avatar.png")
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TextMessage
}

// ExampleDataExpiredWarningMessage demonstrates creating data expiration warnings.
func ExampleDataExpiredWarningMessage() {
	msg := lineutil.DataExpiredWarningMessage(2024, "魔法師", "https://example.com/avatar.png")
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TextMessage
}

// ExampleFormatList demonstrates formatting a list.
func ExampleFormatList() {
	items := []string{"課程A", "課程B", "課程C"}
	formatted := lineutil.FormatList("可選課程", items)
	fmt.Println(formatted)
	// Output: 可選課程
	//
	// 1. 課程A
	// 2. 課程B
	// 3. 課程C
}

// ExampleValidationErrorMessage demonstrates creating validation error messages.
func ExampleValidationErrorMessage() {
	msg := lineutil.ValidationErrorMessage("學號", "學號格式不正確，請輸入９位數字", "魔法師", "https://example.com/avatar.png")
	fmt.Printf("%T", msg)
	// Output: *messaging_api.TextMessage
}
