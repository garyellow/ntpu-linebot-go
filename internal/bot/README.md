# Bot Modules

Bot 模組層負責處理 LINE 訊息事件。

## Handler Interface

```go
type Handler interface {
    CanHandle(text string) bool
    HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface
    HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface
}
```

## 現有模組

- **id/** - 學號查詢、系所代碼
- **contact/** - 聯絡資訊、緊急電話
- **course/** - 課程查詢、教師課表

## 新增模組

1. 在 `internal/bot/<module>/` 建立 `handler.go` 與 `handler_test.go`
2. 實作 `Handler` interface
3. 在 `internal/webhook/handler.go` 註冊
4. 在 `internal/storage/` 新增資料表（若需要）
5. 在 `cmd/warmup/main.go` 新增預熱邏輯

## 開發慣例

- 使用 `lineutil` 建構訊息
- Context timeout 25 秒（LINE webhook 限制）
- 每次回覆最多 5 則訊息（LINE API 限制）
- 使用 table-driven tests
