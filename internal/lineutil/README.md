# lineutil

LINE 訊息建構工具，使用 LINE Bot SDK v8。

## 訊息建構器

```go
// 文字訊息
msg := lineutil.NewTextMessage("Hello")

// 輪播訊息
columns := []lineutil.CarouselColumn{{
    Title: "課程 A",
    Text:  "課程說明",
    Actions: []lineutil.Action{
        lineutil.NewMessageAction("選擇", "選課程A"),
        lineutil.NewURIAction("詳情", "https://example.com"),
    },
}}
msg := lineutil.NewCarouselTemplate("選擇課程", columns)

// 按鈕訊息
actions := []lineutil.Action{
    lineutil.NewMessageAction("查詢", "課程查詢"),
    lineutil.NewURIAction("網站", "https://www.ntpu.edu.tw"),
}
msg := lineutil.NewButtonsTemplate("標題", "主要文字", "說明", actions)
```

## 錯誤訊息模板

```go
lineutil.ErrorMessage(err)            // 一般錯誤
lineutil.ServiceUnavailableMessage()  // 服務無法使用
lineutil.NoResultsMessage()           // 查無資料
```

## 輔助函式

```go
lineutil.TruncateText(text, 40)        // 截斷文字
lineutil.SplitMessages(msgs, 5)        // 訊息分批（LINE 限制 5 則）
lineutil.FormatList("標題", items)     // 格式化列表
```

## 注意事項

- 訊息數量：每次最多 5 則
- Carousel 標題：最多 40 字
- Carousel 內容：最多 60 字
- 按鈕標籤：最多 20 字
- Carousel 動作：最多 3 個
- Buttons 動作：最多 4 個
