# Bot Core

Bot 核心架構，提供訊息處理、模組註冊、意圖分發等功能。

## 目錄結構

```
internal/bot/
├── handler.go    # Handler 介面定義
├── processor.go  # 訊息處理器（NLU、Fallback）
├── registry.go   # 模組註冊與分發
└── utils.go      # 共用工具（關鍵字匹配）
```

## 相關模組

功能模組實作位於 `internal/modules/`，每個模組都有獨立的 README：
- [course](../modules/course/README.md) - 課程查詢
- [id](../modules/id/README.md) - 學號查詢
- [contact](../modules/contact/README.md) - 聯絡資訊
- [program](../modules/program/README.md) - 學程查詢
- [usage](../modules/usage/README.md) - 配額查詢

## Handler 介面

```go
type Handler interface {
    CanHandle(text string) bool
    HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface
    HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface
}
```

### NLU DispatchIntent（可選功能）

各模組額外實作 `DispatchIntent` 方法支援 NLU 意圖分發（需設定 `GEMINI_API_KEY` 或 `GROQ_API_KEY` 或 `CEREBRAS_API_KEY`）：

```go
// DispatchIntent 處理 NLU 解析後的意圖
// intent: 意圖名稱（如 "search", "smart", "uid"）
// params: 解析出的參數（如 {"keyword": "微積分"}）
func (h *Handler) DispatchIntent(ctx context.Context, intent string, params map[string]string) ([]messaging_api.MessageInterface, error)
```

**為何不在 `Handler` 介面中定義？**

NLU 是**可選功能**（需要 `GEMINI_API_KEY` 或 `GROQ_API_KEY` 或 `CEREBRAS_API_KEY`），不是所有部署環境都啟用。遵循 Go 的介面設計原則：

1. **介面最小化**：`Handler` 介面只包含必要方法（`CanHandle`, `HandleMessage`, `HandlePostback`）
2. **可選性檢測**：Webhook 使用類型斷言 `.(interface{ DispatchIntent(...) })` 動態檢測支援
3. **零依賴原則**：未啟用 NLU 時，模組完全獨立運作，不依賴 `genai` 套件

**實作模式**：
```go
// webhook/handler.go - 動態檢測
if dispatcher, ok := handler.(interface{
    DispatchIntent(context.Context, string, map[string]string) ([]messaging_api.MessageInterface, error)
}); ok {
    // 支援 NLU，使用 DispatchIntent
    return dispatcher.DispatchIntent(ctx, intent, params)
}
// 不支援 NLU，fallback 到 HandleMessage
return handler.HandleMessage(ctx, rawText)
```

詳見 `internal/genai/README.md` 了解 NLU 架構。



## 核心功能

### 訊息處理流程

```
LINE Webhook Event
    ↓
Processor.ProcessEvent()
    ↓
┌─ Message ─────────────┐  ┌─ Postback ────────────┐
│ 1. Rate Limiting      │  │ 1. Parse prefix       │
│ 2. Keyword Matching   │  │ 2. Route to handler   │
│ 3. NLU (if no match)  │  │ 3. Execute action     │
│ 4. Handler dispatch   │  │                       │
└───────────────────────┘  └───────────────────────┘
```

### Registry（模組註冊）

所有功能模組透過 `Registry` 註冊和分發：

```go
// 註冊模組（app.Initialize）
registry.Register(courseHandler)
registry.Register(idHandler)
registry.Register(contactHandler)
registry.Register(programHandler)
registry.Register(usageHandler)

// 訊息分發（first-match wins）
handler := registry.FindHandler(text)

// Postback 路由（prefix-based）
handler := registry.GetHandler("course")
```

### NLU 意圖分發

當關鍵字無法匹配時，使用 NLU（需要 LLM API Key）：

```go
// 1. NLU 解析意圖
result := intentParser.Parse(ctx, userInput)
// result.Module = "course", result.Intent = "search"

// 2. 分發到模組（via type assertion）
if dispatcher, ok := handler.(interface{
    DispatchIntent(context.Context, string, map[string]string) ([]messaging_api.MessageInterface, error)
}); ok {
    return dispatcher.DispatchIntent(ctx, result.Intent, result.Params)
}
```

詳見 [genai/README.md](../genai/README.md) 了解 NLU 架構。

## 共用工具 (utils.go)

```go
// 建立關鍵字正則（按長度排序避免短詞先匹配）
// 重要：關鍵字必須在開頭，且後面必須有空格或是文字結尾
regex := bot.BuildKeywordRegex([]string{"課程", "課"})

// 使用 MatchKeyword 取得匹配的關鍵字（不含尾部空格）
keyword := bot.MatchKeyword(regex, "課程 微積分") // → "課程"
keyword := bot.MatchKeyword(regex, "課程微積分")  // → "" (無空格，不匹配)
keyword := bot.MatchKeyword(regex, "課程")       // → "課程" (文字結尾，匹配)

// 提取搜尋詞（移除匹配的關鍵字）
term := bot.ExtractSearchTerm("課程 微積分", "課程") // → "微積分"
```

### 💡 關鍵字匹配規則

| 輸入 | 匹配結果 | 原因 |
|------|---------|------|
| `課程 微積分` | ✅ `課程` | 關鍵字在開頭，後有空格 |
| `課程` | ✅ `課程` | 關鍵字就是整個文字 |
| `課程微積分` | ❌ 不匹配 | 關鍵字後沒有空格 |
| `課程表` | ❌ 不匹配 | 關鍵字後沒有空格（複合詞） |
| `王老師` | ❌ 不匹配 | 關鍵字不在開頭 |
| `老師 王小明` | ✅ `老師` | 關鍵字在開頭，後有空格 |

**設計理念**：
- 避免誤判複合詞（如「課程表」不觸發「課程」）
- 確保用戶意圖明確（必須以空格分隔關鍵字與搜尋詞）
- Tab、換行符等空白字元也視為有效分隔



## Fallback 策略

當無法理解使用者輸入時，提供情境化的錯誤訊息：

| Context | 情境 | 訊息策略 |
|---------|------|----------|
| `FallbackGeneric` | 群組聊天僅 @Bot 無內容 | 提供主要功能列表 |
| `FallbackNLUDisabled` | NLU 未啟用且無關鍵字匹配 | 引導使用關鍵字查詢 |
| `FallbackNLUFailed` | NLU 解析失敗 | 建議換個說法或用關鍵字 |
| `FallbackDispatchFailed` | Intent 分發失敗 | 顯示系統錯誤訊息 |
| `FallbackUnknownModule` | NLU 返回未知模組 | 顯示系統錯誤訊息 |

設計遵循 [Nielsen Norman Group Error Message Guidelines](https://www.nngroup.com/articles/error-message-guidelines/)。

## 開發指南

### 模組開發規範

1. **訊息建構**：使用 `lineutil` 而非直接使用 LINE SDK
2. **Sender 一致性**：同一回覆使用相同 Sender
3. **Context timeout**：60 秒（LINE loading animation 上限）
4. **訊息限制**：最多 5 則訊息/回應
5. **Postback 前綴**：使用 `{module}:` 格式路由
6. **Quick Reply**：最後訊息附加導航按鈕

### 新增模組步驟

1. 在 `internal/modules/{module}/` 建立 handler
2. 實作 `Handler` 介面（`CanHandle`, `HandleMessage`, `HandlePostback`）
3. （可選）實作 `DispatchIntent` 支援 NLU
4. 在 `app.Initialize()` 註冊模組
5. 編寫 README.md 說明功能

詳細架構參考主 README 和 [copilot-instructions.md](../../.github/copilot-instructions.md)。
