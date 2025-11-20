# Bot Modules

Bot 模組層負責處理 LINE 訊息事件，每個模組專注於特定領域的查詢功能。

## Handler Interface

所有 Bot 模組都實作統一的 `Handler` interface：

```go
type Handler interface {
    CanHandle(text string) bool
    HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface
    HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface
}
```

## 現有模組

- **ID Module**: 學號查詢、系所代碼、學生名單
- **Contact Module**: 聯絡資訊、緊急電話
- **Course Module**: 課程查詢、教師課表

## 新增模組步驟

1. 在 `internal/bot/<module>/` 建立 `handler.go` 與 `handler_test.go`
2. 實作 `Handler` interface 的三個方法
3. 在 `internal/webhook/handler.go` 註冊新模組
4. 若需要，在 `internal/storage/` 新增資料表和 repository 方法
5. 在 `cmd/warmup/main.go` 新增預熱邏輯
6. 撰寫 table-driven tests

## 開發慣例

- 使用 `lineutil` 建構訊息，不直接使用 LINE SDK
- 所有錯誤都需記錄 log 和 metrics
- Context timeout 設為 25 秒（符合 LINE webhook 限制）
- 每次回覆最多 5 則訊息（LINE API 限制）
- 使用 table-driven tests 測試所有公開方法
