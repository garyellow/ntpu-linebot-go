# lineutil

LINE 訊息建構工具，基於 LINE Bot SDK v8。

## 檔案結構

- `builder.go` - 訊息建構（Template、Actions、Quick Reply）
- `flex.go` - Flex Message 元件（Bubble、Box、Text、Button、Carousel）
- `sender.go` - Sender 管理（一致性頭像、錯誤訊息）
- `colors.go` - LINE 設計系統顏色常數

## 主要功能

### 訊息類型
- 文字訊息：`NewTextMessage()`, `NewTextMessageWithConsistentSender()`
- 圖片訊息：`NewImageMessage()`
- Flex 訊息：`NewFlexMessage()`, `NewFlexBubble()`
- 輪播訊息：`NewCarouselTemplate()`, `NewFlexCarousel()`, `BuildCarouselMessages()`
- 按鈕訊息：`NewButtonsTemplate()`, `NewButtonsTemplateWithImage()`
- 確認訊息：`NewConfirmTemplate()`

### Flex Message 元件
- 容器：`NewFlexBubble()`, `NewFlexCarousel()`, `NewHeroBox()`, `NewCompactHeroBox()`
- 內容：`NewFlexBox()`, `NewFlexText()`, `NewFlexButton()`, `NewFlexSeparator()`
- 佈局：`NewInfoRow()`, `NewButtonRow()`, `NewButtonFooter()`
- 建構器：`NewBodyContentBuilder()` (自動分隔線)
- 輔助函數：`BuildCarouselMessages()` (自動分割大量 Bubbles)

### 互動元件
- Quick Reply：`NewQuickReply()` (最多 13 個按鈕)
- 預設 Quick Reply：`QuickReplyHelpAction()`, `QuickReplyCourseAction()` 等
- Actions：`NewMessageAction()`, `NewPostbackAction()`, `NewURIAction()`, `NewClipboardAction()`

### Sender 管理
- `GetSender(name, stickerManager)` - 取得一致性頭像的 Sender
- `NewTextMessageWithConsistentSender()` - 使用預設 Sender 的文字訊息

### 錯誤處理
- `ErrorMessageWithSender()` - 通用錯誤訊息
- `ErrorMessageWithDetailAndSender()` - 帶詳情的錯誤訊息
- `ErrorMessageWithQuickReply()` - 帶重試按鈕的錯誤訊息
- `NotFoundMessage()` - 查無結果訊息

## LINE API 限制

| 項目 | 限制 | 常數 |
|------|------|------|
| 每次回覆訊息數 | 5 則 | - |
| Quick Reply 按鈕 | 13 個 | - |
| Flex Carousel bubbles | 10 個 | `MaxBubblesPerCarousel` |
| Buttons 動作 | 4 個 | - |
| 文字訊息長度 | 5000 字元 | - |
| altText 長度 | 400 字元 | - |
| Postback data | 300 bytes | - |

## 最佳實踐

1. **Sender 一致性**：同一回覆中使用相同的 Sender（一次 `GetSender()` 調用）
2. **Quick Reply 引導**：在訊息結尾加入快速回覆選項
3. **Flex Message 優先**：使用卡片式介面提升體驗
4. **完整顯示資訊**：使用 `wrap: true` + `lineSpacing` 讓文字換行
5. **截斷僅限 API**：`TruncateRunes()` 僅用於 LINE API 硬性限制

### Flex Carousel 範例

```go
// 使用 BuildCarouselMessages 自動分割 (每 10 個 bubbles 一則訊息)
var bubbles []messaging_api.FlexBubble
for _, item := range items {
    bubble := lineutil.NewFlexBubble(...)
    bubbles = append(bubbles, *bubble.FlexBubble)
}
sender := lineutil.GetSender("模組名", stickerManager)
messages := lineutil.BuildCarouselMessages("搜尋結果", bubbles, sender)
```

### Flex Bubble 範例

```go
// 使用 BodyContentBuilder 自動處理分隔線
body := lineutil.NewBodyContentBuilder()
body.AddInfoRow("🆔", "學號", student.ID, lineutil.BoldInfoRowStyle())
body.AddInfoRow("🏫", "系所", student.Department, lineutil.DefaultInfoRowStyle())

// 建立完整 Bubble
bubble := lineutil.NewFlexBubble(header, hero.FlexBox, body.Build(), footer)
msg := lineutil.NewFlexMessage("學生資訊", bubble.FlexBubble)
msg.Sender = sender
```

詳細範例請參考 `example_test.go`。
