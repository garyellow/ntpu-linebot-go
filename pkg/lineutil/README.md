# lineutil - LINE Message Builder Utility

這個套件提供了便於建立 LINE 訊息的工具函式,使用 LINE Bot SDK v8。

## 功能特色

### 1. 訊息建構器 (Message Builders)

#### 文字訊息
```go
msg := lineutil.NewTextMessage("Hello, World!")
```

#### 輪播訊息 (Carousel Template)
```go
columns := []lineutil.CarouselColumn{
    {
        ThumbnailImageURL: "https://example.com/image1.jpg",
        Title:             "課程 A",
        Text:              "課程說明",
        Actions: []lineutil.Action{
            lineutil.NewMessageAction("選擇", "我要選課程A"),
            lineutil.NewURIAction("詳細資訊", "https://example.com/course-a"),
        },
    },
}

msg := lineutil.NewCarouselTemplate("請選擇課程", columns)
```

#### 按鈕訊息 (Buttons Template)
```go
actions := []lineutil.Action{
    lineutil.NewMessageAction("查詢課程", "課程查詢"),
    lineutil.NewMessageAction("查詢聯絡資訊", "聯絡資訊"),
    lineutil.NewURIAction("官方網站", "https://www.ntpu.edu.tw"),
}

msg := lineutil.NewButtonsTemplate(
    "選擇功能",
    "NTPU 查詢系統",
    "請選擇您要使用的功能",
    actions,
)
```

#### 確認訊息 (Confirm Template)
```go
msg := lineutil.NewConfirmTemplate(
    "確認操作",
    "您確定要執行此操作嗎?",
    lineutil.NewPostbackAction("確定", "action=confirm&value=yes"),
    lineutil.NewPostbackAction("取消", "action=confirm&value=no"),
)
```

#### 快速回覆 (Quick Reply)
```go
items := []lineutil.QuickReplyItem{
    {
        Action: lineutil.NewMessageAction("課程", "查詢課程"),
    },
    {
        Action: lineutil.NewMessageAction("聯絡", "查詢聯絡資訊"),
    },
}

quickReply := lineutil.NewQuickReply(items)

// 附加到文字訊息
textMsg := &messaging_api.TextMessage{
    Type: "text",
    Text: "請選擇功能",
    QuickReply: quickReply,
}
```

#### Flex 訊息
```go
// 建立 flex container (需自行構建)
flexContainer := &messaging_api.FlexContainer{ /* ... */ }

msg := lineutil.NewFlexMessage("Flex 訊息", flexContainer)
```

### 2. 動作建構器 (Action Builders)

#### 訊息動作
```go
action := lineutil.NewMessageAction("點我", "使用者會發送這個訊息")
```

#### Postback 動作
```go
action := lineutil.NewPostbackAction("確認", "action=submit&id=123")
```

#### URI 動作
```go
action := lineutil.NewURIAction("開啟網頁", "https://www.ntpu.edu.tw")
```

### 3. 錯誤訊息模板

#### 一般錯誤
```go
err := fmt.Errorf("資料庫連線失敗")
msg := lineutil.ErrorMessage(err)
// 輸出: ❌ 發生錯誤：資料庫連線失敗\n\n請稍後再試或聯絡管理員。
```

#### 服務無法使用
```go
msg := lineutil.ServiceUnavailableMessage()
// 輸出: ⚠️ 服務暫時無法使用\n\n系統正在維護中,請稍後再試。
```

#### 查無資料
```go
msg := lineutil.NoResultsMessage()
// 輸出: 🔍 查無資料\n\n請檢查輸入的關鍵字是否正確,或嘗試其他搜尋條件。
```

#### 資料過期警告
```go
msg := lineutil.DataExpiredWarningMessage(2024)
// 輸出警告訊息提醒 2024 年度資料可能已過期
```

### 4. 輔助函式

#### 截斷文字
```go
text := "這是一段很長的文字需要被截斷"
truncated := lineutil.TruncateText(text, 10)
// 輸出: 這是一段很...
```

#### 訊息分批 (LINE 限制每次最多 5 則訊息)
```go
messages := []messaging_api.MessageInterface{
    lineutil.NewTextMessage("Message 1"),
    lineutil.NewTextMessage("Message 2"),
    // ... 更多訊息
}

batches := lineutil.SplitMessages(messages, 5)
// 每個 batch 最多 5 則訊息

for _, batch := range batches {
    // 發送每個 batch
    client.PushMessage(userID, batch...)
}
```

#### 格式化列表
```go
items := []string{"課程 A", "課程 B", "課程 C"}
formatted := lineutil.FormatList("可選課程", items)
// 輸出:
// 可選課程
//
// 1. 課程 A
// 2. 課程 B
// 3. 課程 C
```

#### 驗證錯誤訊息
```go
msg := lineutil.ValidationErrorMessage("學號", "學號格式不正確")
// 輸出: ❌ 輸入錯誤\n\n欄位：學號\n說明：學號格式不正確
```

## 使用範例

### 課程查詢結果
```go
func sendCourseResults(courses []Course) messaging_api.MessageInterface {
    if len(courses) == 0 {
        return lineutil.NoResultsMessage()
    }

    columns := make([]lineutil.CarouselColumn, 0, len(courses))
    for _, course := range courses {
        col := lineutil.CarouselColumn{
            Title: lineutil.TruncateText(course.Name, 40),
            Text:  fmt.Sprintf("教師: %s\n學分: %d", course.Teacher, course.Credits),
            Actions: []lineutil.Action{
                lineutil.NewMessageAction("查看詳情", fmt.Sprintf("課程:%s", course.ID)),
                lineutil.NewURIAction("課程大綱", course.SyllabusURL),
            },
        }
        columns = append(columns, col)
    }

    return lineutil.NewCarouselTemplate("課程查詢結果", columns)
}
```

### 互動式選單
```go
func sendMainMenu() messaging_api.MessageInterface {
    items := []lineutil.QuickReplyItem{
        {Action: lineutil.NewMessageAction("📚 課程查詢", "查詢課程")},
        {Action: lineutil.NewMessageAction("📞 聯絡資訊", "查詢聯絡資訊")},
        {Action: lineutil.NewMessageAction("🎓 學號查詢", "查詢學號")},
        {Action: lineutil.NewMessageAction("ℹ️ 使用說明", "說明")},
    }

    msg := &messaging_api.TextMessage{
        Type:       "text",
        Text:       "您好！我是 NTPU 查詢機器人\n請選擇您需要的功能：",
        QuickReply: lineutil.NewQuickReply(items),
    }

    return msg
}
```

### 錯誤處理
```go
func handleError(err error) messaging_api.MessageInterface {
    switch {
    case errors.Is(err, ErrNotFound):
        return lineutil.NoResultsMessage()
    case errors.Is(err, ErrServiceDown):
        return lineutil.ServiceUnavailableMessage()
    default:
        return lineutil.ErrorMessage(err)
    }
}
```

## 型別定義

### CarouselColumn
```go
type CarouselColumn struct {
    ThumbnailImageURL    string   // 縮圖 URL
    ImageBackgroundColor string   // 背景顏色 (hex)
    Title                string   // 標題 (最多 40 字)
    Text                 string   // 內容文字 (最多 60 字)
    Actions              []Action // 動作按鈕 (最多 3 個)
}
```

### QuickReplyItem
```go
type QuickReplyItem struct {
    ImageURL string // 圖示 URL (選填)
    Action   Action // 動作
}
```

### ValidationError
```go
type ValidationError struct {
    Field   string // 欄位名稱
    Message string // 錯誤訊息
}
```

## 注意事項

1. **訊息數量限制**: LINE API 每次最多發送 5 則訊息,使用 `SplitMessages` 來處理
2. **文字長度限制**:
   - Carousel 標題: 最多 40 字
   - Carousel 內容: 最多 60 字
   - 按鈕標籤: 最多 20 字
3. **動作數量限制**:
   - Carousel 每欄: 最多 3 個動作
   - Buttons Template: 最多 4 個動作
   - Quick Reply: 最多 13 個項目

## 依賴套件

- `github.com/line/line-bot-sdk-go/v8` - LINE Bot SDK v8

## 授權

此專案遵循與主專案相同的授權條款。
