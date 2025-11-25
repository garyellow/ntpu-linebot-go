# lineutil

LINE 訊息建構工具，基於 LINE Bot SDK v8。

## 主要功能

### 訊息類型
- 文字訊息：`NewTextMessage()`, `NewTextMessageWithSender()`
- 圖片訊息：`NewImageMessage()`
- Flex 訊息：`NewFlexMessage()` (卡片式互動介面)
- 輪播訊息：`NewCarouselTemplate()`
- 按鈕訊息：`NewButtonsTemplate()`, `NewButtonsTemplateWithImage()`
- 確認訊息：`NewConfirmTemplate()`

### 互動元件
- Quick Reply：`NewQuickReply()` (快速回覆按鈕，最多 13 個)
- Actions：`NewMessageAction()`, `NewPostbackAction()`, `NewURIAction()`, `NewClipboardAction()`

### 錯誤處理
- 錯誤模板：`ErrorMessage()`, `ErrorMessageWithDetail()`, `ServiceUnavailableMessage()`, `NoResultsMessage()`
- 驗證錯誤：`ValidationErrorMessage()`

## LINE API 限制

- 每次最多 **5 則**訊息
- Quick Reply 最多 **13 個**按鈕
- Carousel 最多 **10 個** columns，標題最多 **40 字**
- Buttons 動作最多 **4 個**
- Postback data 最多 **300 bytes**

## 最佳實踐

1. **使用 Quick Reply 提升體驗**：在訊息結尾加入快速回覆選項，引導用戶下一步操作
2. **顯示 Loading Animation**：長查詢前顯示載入動畫 (由 webhook handler 處理)
3. **Flex Message 優先**：使用 Flex Message 提供豐富的卡片式介面
4. **錯誤訊息友善化**：隱藏技術細節，提供可操作的 Quick Reply
5. **完整顯示資訊**：優先使用 `wrap: true` + `lineSpacing` 讓文字換行，避免截斷文字
6. **截斷限制**：`TruncateRunes()` 僅用於 LINE API 硬性限制 (altText、displayText)

### Flex Message 文字處理

```go
// ✅ 推薦：完整顯示資訊
lineutil.NewInfoRow("標籤", value).WithWrap(true).WithLineSpacing("4px")

// ❌ 避免：隱藏資訊
lineutil.NewFlexText(lineutil.TruncateRunes(value, 20))

// ✅ 僅在 LINE API 限制時使用截斷
altText := lineutil.TruncateRunes(title, 400)  // LINE altText 限制
```

詳細範例請參考 `example_test.go`。
