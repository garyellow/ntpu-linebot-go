# Bot Modules

處理 LINE 訊息事件的核心模組，實作 `Handler` 介面。

## 架構

```
bot/
├── handler.go      # Handler 介面定義
├── utils.go        # 共用工具（關鍵字匹配、搜尋詞提取）
├── id/             # 學號查詢模組
├── contact/        # 聯絡資訊模組
└── course/         # 課程查詢模組
```

## Handler 介面

```go
type Handler interface {
    CanHandle(text string) bool
    HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface
    HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface
}
```

## 模組概覽

### id/ - 學號查詢
- **關鍵字**：學號、學生、姓名、科系、系代碼、學年
- **功能**：
  - 學號查詢（直接輸入 8-9 位數字）
  - 姓名搜尋（2-tier 並行搜尋，最多 500 筆）
  - 系代碼對照
  - 學年度學生名單（按學院→科系選擇）
- **搜尋策略**：SQL LIKE (name) + 模糊 ContainsAllRunes (name)
- **Postback 前綴**：`id:`
- **Sender 名稱**：學號小幫手

### contact/ - 聯絡資訊
- **關鍵字**：聯繫、聯絡、電話、分機、緊急
- **功能**：
  - 單位/人員搜尋（2-tier 並行搜尋）
  - 緊急聯絡電話（三峽/台北校區）
  - 組織成員查詢
- **搜尋策略**：SQL LIKE (name, title) + 模糊 ContainsAllRunes (name, title, organization, superior)
- **Postback 前綴**：`contact:`
- **Sender 名稱**：聯繫小幫手

### course/ - 課程查詢
- **關鍵字**：課程、課、教師、老師（統一查詢）
- **功能**：
  - 課程名稱搜尋（最多 50 門）
  - 課程編號查詢（UID 格式）
  - 統一查詢（2-tier 並行搜尋：同時搜尋課程名稱和教師姓名）
  - 歷史課程查詢（`課程 {年度} {關鍵字}`）
- **搜尋策略**：SQL LIKE (title, teachers) + 模糊 ContainsAllRunes (title, teachers)
- **Postback 前綴**：`course:`
- **Sender 名稱**：課程小幫手

## 共用工具 (utils.go)

```go
// 建立關鍵字正則（按長度排序避免短詞先匹配）
regex := bot.BuildKeywordRegex([]string{"課程", "課"})

// 提取搜尋詞（移除匹配的關鍵字）
term := bot.ExtractSearchTerm("課程 微積分", "課程") // → "微積分"
```

## 搜尋策略：2-Tier Parallel Search

所有模組統一採用「2-tier 並行搜尋」策略，確保結果的完整性與一致性：

```
┌─────────────────────────────────────────────────────────────┐
│                   使用者查詢                                  │
└─────────────────────┬───────────────────────────────────────┘
                      │
        ┌─────────────┴─────────────┐
        │                           │
        ▼                           ▼
┌───────────────────┐     ┌───────────────────────────┐
│   Tier 1: Cache   │     │   如果 Cache 無資料       │
│   (SQL LIKE +     │     │   → Tier 2: Scraper       │
│    ContainsAll)   │     │      (爬取後存入 Cache)    │
└─────────┬─────────┘     └─────────────┬─────────────┘
          │                             │
          └─────────────┬───────────────┘
                        │
                        ▼
                ┌───────────────┐
                │  合併 + 去重   │
                │  (by UID/ID)  │
                └───────────────┘
```

**核心邏輯**：
1. **SQL LIKE**: 傳統子字串匹配，適合連續字元搜尋（如「線性代數」）
2. **ContainsAllRunes**: 模糊字元集匹配，適合簡稱/縮寫搜尋（如「線代」→「線性代數」）
3. **兩者並行執行**，結果合併後去重（依 UID/ID）

**為何不用 fallback？**
原本的 fallback 策略（SQL LIKE 無結果時才執行 fuzzy search）會導致：
- 「線代」→ SQL LIKE 無結果 → fuzzy 找到「線性代數」✅
- 「線性代數」→ SQL LIKE 有結果 → 跳過 fuzzy → 漏掉其他匹配 ❌

**函數位置**：
- `lineutil.ContainsAllRunes(text, search)` - 判斷 text 是否包含 search 的所有字元（rune）

## 開發規範

1. **使用 `lineutil`**：所有訊息建構透過 lineutil，不直接使用 LINE SDK
2. **Sender 一致性**：每次回覆使用同一個 `GetSender()` 返回的 Sender
3. **Context timeout**：25 秒（LINE webhook 限制）
4. **訊息限制**：每次最多 5 則訊息（LINE API 限制）
5. **Postback 前綴**：使用 `{module}:` 前綴便於 webhook dispatcher 路由
6. **Table-driven tests**：所有測試使用 table-driven 模式
7. **Quick Reply 引導**：在最後一則訊息附加 Quick Reply 引導下一步

## 新增模組步驟

1. 建立 `internal/bot/{module}/handler.go`
2. 實作 `Handler` 介面
3. 在 `webhook/handler.go` 註冊新 handler
4. 定義 Postback 前綴並在 dispatcher 加入路由

詳細架構請參考 `.github/copilot-instructions.md`。
