# NTPU LineBot Go - AI Agent Instructions

LINE chatbot for NTPU (National Taipei University) providing student ID lookup, contact directory, and course queries. Built with Go, emphasizing anti-scraping measures, persistent caching, and observability.

## 🎯 Architecture Principles

**Core Design:**
1. **Pure Dependency Injection** - Constructor-based injection with all dependencies explicit at construction time
2. **Direct Dependencies** - Handlers use `*storage.DB` directly, interfaces only when truly needed
3. **Typed Error Handling** - Sentinel errors (`errors.ErrNotFound`) with standard wrapping
4. **Centralized Configuration** - Bot config with load-time validation
5. **Context Management** - `ctxutil.PreserveTracing()` for safe async operations with tracing
6. **Simplified Registry** - Direct dispatch without middleware overhead
7. **Clean Initialization** - Core → GenAI → LLMRateLimiter → Handlers → Webhook (linear flow)

**Code Style:**
- **Pure DI**: All dependencies via constructors (no functional options)
- **Concrete Types**: Handlers depend on `*storage.DB` directly (no mocking needed)
- **Interface Placement**: Defined inline where needed (Go convention: accept interfaces, return structs)
- **Optional Parameters**: Pass nil for optional dependencies (e.g., `bm25Index`, `intentParser`)
- **Context Values**: Minimal usage for request tracing only (userID, chatID, requestID)
- **Error Handling**: Sentinel errors with standard `fmt.Errorf` wrapping
- **Constants**: Centralized in config package
- **Async Operations**: `ctxutil.PreserveTracing()` for safe detached contexts (avoids memory leaks)
- **Validation**: Load-time config validation, runtime parameter checks

## Architecture: Async Webhook Processing

```
LINE Webhook → Gin Handler
                ↓ (signature validation - synchronous)
          HTTP 200 OK (< 2s)
                ↓
          [Goroutine] Async Event Processing (context.Background())
                ↓ (Loading Animation + rate limiting)
      Bot Module Dispatcher
                ↓ (keyword matching via CanHandle())
      Bot Handlers (id/contact/course)
                ↓ (ctxutil.PreserveTracing() with 60s timeout)
      Storage Repository (cache-first)
                ↓ (TTL check for contacts/courses only)
      Scraper Client (rate-limited)
                ↓ (exponential backoff, failover URLs)
          NTPU Websites (lms/sea)
                ↓
      Reply via Reply Token (< 30s)
```

**Critical Flow Details:**
- **Async processing**: HTTP 200 returned immediately (< 2s), events processed in goroutine
- **Context handling**:
  - Webhook: `context.Background()` for async processing
  - Bot operations: `ctxutil.PreserveTracing()` preserves tracing (userID, chatID, requestID) with 60s timeout
  - Prevents memory leaks while maintaining log correlation (Go issue #64478)
- **Message batching**: Max 5 messages per reply; auto-truncates to 4 + warning
- **References**: [LINE guidelines](https://developers.line.biz/en/docs/partner-docs/development-guidelines/), [Context safety](https://github.com/golang/go/issues/64478)

## Bot Module Registration Pattern

**When adding new modules**:

1. Implement `bot.Handler` interface (`internal/bot/handler.go`)
2. Create handler in app.Initialize() with dependencies
3. Register via `registry.Register(handler)`
4. Use prefix convention for postback routing (`"course:"`, `"id:"`, `"contact:"`)
5. Pass nil for optional dependencies (e.g., `bm25Index`, `queryExpander`, `llmRateLimiter`)

**Course Module**:
- **Precise search** (`課程`): SQL LIKE + fuzzy search
- **Smart search** (`找課`): BM25 + Query Expansion (requires LLM API key)
- **Confidence scoring**: Relative BM25 score (0-1, first result always 1.0)
- **Fallback**: Precise → Smart search (if BM25Index enabled)

**Contact Module**:
- Emergency phones, multilingual keywords, Flex Message cards
- **2-tier parallel search**: SQL LIKE + fuzzy `ContainsAllRunes()`, merged and deduplicated
- **Sorting**: Organizations by hierarchy, individuals by match count

**All modules**:
- Prefer text wrapping; use `TruncateRunes()` only for LINE API limits
- Consistent Sender pattern, cache-first strategy

## Data Layer: Cache-First Strategy

**SQLite cache** (`internal/storage/`):
- WAL mode, pure Go (`modernc.org/sqlite`)
- **Cache Strategy by Data Type**:
  - **Students**: Never expires, not refreshed (static data)
  - **Stickers**: Never expires, loaded once on startup
  - **Contacts/Courses**: 7-day TTL, refreshed daily at 3:00 AM Taiwan time
  - **Syllabi**: 7-day TTL, auto-enabled when LLM API key is configured
- TTL enforced at SQL level for contacts/courses: `WHERE cached_at > ?`
- **Syllabi table**: Stores syllabus content + SHA256 hash for incremental updates

**BM25 Index** (`internal/rag/`):
- [iwilltry42/bm25-go](https://github.com/iwilltry42/bm25-go) (k1=1.5, b=0.75)
- In-memory index rebuilt on startup from SQLite
- Chinese tokenization (unigram for CJK), 1 course = 1 document
- Combined with LLM Query Expansion (auto-enabled when LLM API key configured)

**Background Jobs** (Taiwan time/Asia/Taipei):
- **Sticker**: Startup only
- **Daily Refresh** (3:00 AM): contact, course (always), syllabus (auto-enabled if LLM API key)
- **Cache Cleanup** (4:00 AM): Delete expired contacts/courses/syllabi (7-day TTL) + VACUUM
- **Metrics/Rate Limiter Cleanup**: Every 5 minutes

**Data availability**:
- Student:
  - **Cache range**: 101-113 學年度 (warmup auto-loads)
  - **Query range**: 94-113 學年度 (real-time scraping, hard limit due to LMS 2.0 deprecated)
  - **Status**: Static data, no new data after 114
- Course:
  - **Cache range**: 4 most recent semesters (7-day TTL, warmup auto-loads)
  - **Query range**: 90-current year (Course system launched 90, real-time scraping supported)
  - **Validation**: Uses `config.CourseSystemLaunchYear` as minimum, not limited by cache content
- Contact: 7-day TTL
- Sticker: Startup only, never expires
- Syllabus: Auto-enabled when LLM API key configured

## Rate Limiting

**Scraper** (`internal/scraper/client.go`): 2s rate limiting between requests, exponential backoff on failure (4s initial, max 5 retries, ±25% jitter), 60s HTTP timeout per request

**Webhook**: Per-user (6 tokens, 1 token/5s refill), global (100 rps), silently drops excess requests

**LINE SDK Conventions**

**Message builders** (`internal/lineutil/`):
```go
lineutil.NewTextMessage(text)                    // Simple text
lineutil.NewFlexMessage(altText, contents)       // Flex Message
lineutil.NewQuickReply(items)                    // Quick Reply (max 13)

// Quick Reply Presets (use these for consistency)
lineutil.QuickReplyMainNav()        // 課程→學號→聯絡→緊急→說明 (welcome, help)
lineutil.QuickReplyMainNavCompact() // 課程→學號→聯絡→說明 (errors, rate limit)
lineutil.QuickReplyMainFeatures()   // 課程→學號→聯絡→緊急 (instruction messages)
lineutil.QuickReplyContactNav()     // 聯絡→緊急→說明 (contact module)
lineutil.QuickReplyStudentNav()     // 學號→學年→系代碼→說明 (id module)
lineutil.QuickReplyCourseNav(bool)  // 課程→找課(if smart)→說明 (course module)
lineutil.QuickReplyErrorRecovery(retryText) // 重試→說明 (errors with retry)

// Sender pattern (REQUIRED)
// System/Help: "北大小幫手" (unified for bot-level messages)
// Modules: "課程小幫手", "學號小幫手", "聯繫小幫手" (module-specific)
// Special: "貼圖小幫手" (sticker responses only)
sender := lineutil.GetSender("北大小幫手", stickerManager)  // Once at handler start
msg := lineutil.NewTextMessageWithConsistentSender(text, sender)
// Use same sender for all messages in one reply
```

**UX Best Practices**:
- Always provide Quick Reply (including errors)
- Use `lineutil.QuickReply*` presets for consistency
- Show loading animation for long queries (> 1s)
- Use Flex Messages for rich content
- Include retry/help Quick Reply on errors
- Same sender throughout reply batch

**Flex Message 設計規範**:
- **配色** (WCAG AA 符合):
  - Hero 背景 `#06C755` (LINE 綠), 標題白色
  - 主要文字 `#111111` (ColorText), 標籤 `#666666` (ColorLabel)
  - 次要文字 `#6B6B6B` (ColorSubtext), 備註 `#888888` (ColorNote)
  - 時間戳記 `#B7B7B7` (ColorGray400) - 僅用於不強調資訊
- **按鈕顏色** (語義化分類 - WCAG AA 符合):
  - `ColorButtonPrimary` `#06C755` (LINE 綠) - 主要操作 (複製學號、撥打電話、寄送郵件) - 4.9:1
  - `ColorDanger` `#E02D41` (深紅) - 緊急操作 (校安電話) - 4.5:1
  - `ColorWarning` `#D97706` (琥珀色) - 警告訊息 (配額達上限、限流提示) - 4.5:1
  - `ColorButtonExternal` `#2563EB` (深藍) - 外部連結 (課程大綱、Dcard、選課大全、網站) - 4.8:1
  - `ColorButtonInternal` `#7C3AED` (深紫) - 內部指令/Postback (教師課程、查看成員、查詢學號) - 4.6:1
  - `ColorSuccess` `#059669` (深翠綠) - 成功狀態 (操作完成提示、確認訊息) - 4.5:1 WCAG AA
  - `ColorButtonSecondary` `#6B7280` (灰色) - 次要操作 (複製號碼、複製信箱) - 5.9:1
- **Header 顏色** (Colored Header 背景色 - 所有顏色符合 WCAG AA):
  - 學期標示: `ColorHeaderRecent` 白色 (最新學期), `ColorHeaderPrevious` 藍色 (上個學期), `ColorHeaderHistorical` 深灰 (過去學期)
  - 相關性標示: `ColorHeaderBest` 白色 (最佳匹配), `ColorHeaderHigh` 紫色 (高度相關), `ColorHeaderMedium` 琥珀色 (部分相關)
  - 聯絡類型: `ColorHeaderOrg` 藍色 (組織單位), `ColorHeaderIndividual` 綠色 (個人聯絡)
  - 詳情頁模組: `ColorHeaderCourse` 琥珀色, `ColorHeaderContact` 藍色, `ColorHeaderStudent` 綠色
  - **Header 文字顏色**: 白色背景用深色文字 (ColorText)，彩色背景用白色文字 (ColorHeroText)
- **Body Label 設計原則**:
  - **統一使用 LINE 綠色** (`ColorPrimary`): 所有輪播卡片的 body label 都使用 LINE 綠色，確保視覺一致性和品牌辨識度
  - **視覺層次**: Header 背景色用於區分類別 (學期/相關性/類型)，Body Label 用綠色強調重點標記
  - **簡化邏輯**: 移除複雜的顏色繼承，body label 永遠是綠色，更易於維護和理解
- **間距**: Hero padding `24px`/`16px` (4-point grid), Body/Footer spacing `sm`, 按鈕高度 `sm`
- **文字**: 優先使用 `wrap: true` + `lineSpacing` 完整顯示資訊；僅 carousel 使用 `WithMaxLines()` 控制高度
- **截斷**: `TruncateRunes()` 僅用於 LINE API 限制 (altText 400 字, displayText 長度限制)
- **設計原則**: 對稱、現代、一致 - 確保視覺和諧，完整呈現資訊，所有顏色符合 WCAG AA 無障礙標準
- **資料說明**: 學號查詢結果的系所資訊由學號推測，可能因轉系等原因有所不同

**輪播卡片設計模式**:
- 課程輪播 (Course): Colored Header (標題) → Body (標籤 + 資訊) → Footer
  - Header 使用 `NewColoredHeader()` 創建帶背景色的標題 (白色/藍色/灰色等)
  - Body 第一列使用 `NewBodyLabel()` 顯示學期/相關性標籤 (統一 LINE 綠色文字)
  - 學期標籤: `🆕 最新學期` (綠色), `📅 上個學期` (綠色), `📦 過去學期` (綠色)
  - 相關性標籤: `🎯 最佳匹配` (綠色), `✨ 高度相關` (綠色), `📋 部分相關` (綠色) - 智慧搜尋
  - **視覺效果**: Header 背景色顯示類別，Body Label 綠色文字強調標記，層次分明
- 聯絡人輪播 (Contact): Colored Header (姓名) → Body (標籤 + 資訊) → Footer
  - Header 使用 `NewColoredHeader()` 創建帶背景色的標題 (藍色/綠色)
  - Body 第一列使用 `NewBodyLabel()` 顯示類型標籤 (統一 LINE 綠色文字)
  - 類型標籤: `🏢 組織單位`, `👤 個人聯絡`（Header 背景色分別為藍/綠）
  - **視覺效果**: 與課程輪播一致，Header 背景色顯示類型，Body Label 強調標記
- 詳情頁 (所有模組): Colored Header (名稱) → Body (標籤 + 資訊) → Footer
  - **統一設計**: 所有模組 (Course/Contact/ID) 都使用 `NewColoredHeader()` 呈現主要資訊
  - Course: 琥珀色 Header (課程名稱), Body 第一列顯示「📚 課程資訊」標籤
  - Contact: 藍色/綠色 Header (聯絡人姓名), Body 第一列顯示類型標籤（`🏢 組織單位` 或 `👤 個人聯絡`，與輪播一致）
  - ID: 綠色 Header (學生姓名), Body 第一列顯示「🎓 國立臺北大學」標籤
  - **移除 Hero**: 不再使用 `NewDetailPageLabel()` + `NewHeroBox()` 的舊設計，改為統一的 Colored Header 模式
  - **節省空間**: 資訊更緊湊，視覺一致性更好

**Postback format** (300 byte limit): Use module prefix `"module:data"` for routing (e.g., `"course:1132U2236"`). Reply token is single-use - batch all messages into one array.

**Postback processing**: Handlers must extract actual data from prefixed format:
```go
// ✅ Correct: Extract matched portion
if uidRegex.MatchString(data) {
    uid := uidRegex.FindString(data)  // "course:1132U2236" -> "1132U2236"
    return h.handleQuery(ctx, uid)
}
// ❌ Wrong: Pass entire data string
if uidRegex.MatchString(data) {
    return h.handleQuery(ctx, data)  // "course:1132U2236" causes parsing errors
}
```

## URL Failover

**URLCache** (`internal/scraper/urlcache.go`): Thread-safe URL caching with automatic failover
- `atomic.Value` for lock-free reads, auto-recovery on errors
- Scrapers use `getWorkingBaseURL()` helper, call `clearCache()` on failures

## UTF-8 Handling

**Use `TruncateRunes()` only for LINE API limits** (altText, displayText) - byte slicing breaks multi-byte CJK characters:
```go
lineutil.TruncateRunes(text, maxChars)  // ✅ Safe for API limits
text[:10] + "..."                       // ❌ Corrupts UTF-8
```

**Prefer text wrapping** for Flex Message content - use `wrap: true` with `lineSpacing` for readability:
```go
lineutil.NewInfoRow("標籤", value).WithWrap(true).WithLineSpacing(lineutil.SpacingXS)  // ✅ Full display
lineutil.TruncateRunes(value, 20)                                                    // ❌ Hides information
```

## Testing

**Patterns**:
- Table-driven tests with `t.Run()` for parallel execution
- In-memory SQLite (`:memory:`) for DB tests via `setupTestDB()` helper
- Network tests skip by default (`-short` flag): Use `testing.Short()` guard for scraper integration tests
- Test files follow `*_test.go` convention alongside implementation files

**Coverage requirements**: 80% threshold enforced in CI (`task test:coverage`)

## Configuration

**Load-time validation**: All env vars loaded at startup (`internal/config/`) with validation before server starts
**Required**: `LINE_CHANNEL_SECRET`, `LINE_CHANNEL_ACCESS_TOKEN`
**Optional**: `GEMINI_API_KEY` or `GROQ_API_KEY` (enables NLU + Query Expansion with multi-provider fallback)
**Platform paths**: `runtime.GOOS` determines default paths (Windows: `./data`, Linux/Mac: `/data`)

## Task Commands

```powershell
task dev              # Run server with debug logging
task test             # Run tests (skips network tests for speed)
task test:full        # Run all tests including network tests
task test:race        # Run tests with race detector
task test:coverage    # Coverage report with 80% threshold check
task lint             # Run golangci-lint
task fmt              # Format code and organize imports
task build            # Build binaries to bin/
task clean            # Remove build artifacts
task compose:up       # Start monitoring stack (Prometheus/Grafana)
```

**Environment variables** (`.env`):
- **Required**: `LINE_CHANNEL_SECRET`, `LINE_CHANNEL_ACCESS_TOKEN`
- **Optional**: `GEMINI_API_KEY`, `GROQ_API_KEY` (enables NLU + Query Expansion with multi-provider fallback), `DATA_DIR` (default: `./data` on Windows, `/data` on Linux/Mac)

Production warmup runs automatically on server startup (non-blocking).

## Error Handling

Wrap errors with context (`fmt.Errorf(..., %w)`), structured logging with fields, user-facing messages via `lineutil.ErrorMessage()`.

## Scraper Client

Multiple base URLs per domain (LMS/SEA), automatic failover on 500+ errors, URLCache for performance.

## Debugging

**Logging**: `task dev` (debug level enabled by default in dev mode)

**Prometheus** (`http://localhost:10000/metrics`):
- Webhook: requests, latency
- Cache: hits, misses
- Scraper: requests (success/error/timeout), latency
- Rate limiter: wait time, dropped requests

**Common queries**:
```promql
# Cache hit rate
sum(rate(ntpu_cache_operations_total{result="hit"}[5m])) / sum(rate(ntpu_cache_operations_total[5m]))

# P95 latency
histogram_quantile(0.95, sum(rate(ntpu_webhook_duration_seconds_bucket[5m])) by (le))
```

## Docker

Multi-stage build (alpine builder + distroless runtime), healthcheck binary (no shell), volume permissions handled by application.

## NLU Intent Parser (Multi-Provider)

**Location**: `internal/genai/` (types.go, gemini_intent.go, groq_intent.go, factory.go, provider_fallback.go)

**Architecture**:
```
User Input → Keyword Matching (existing handlers)
     ↓ (no match)
handleUnmatchedMessage()
     ↓
┌─ Group Chat ─┐     ┌─ Personal Chat ─┐
│ No @Bot → silent │  NLU Parser       │
│ Has @Bot → remove│                   │
│ mention & process│                   │
└─────────────────┴───────────────────┘
     ↓
FallbackIntentParser.Parse()
     ↓
┌─ Primary Provider ─┐  ┌─ Fallback Provider ─┐
│ Gemini/Groq        │→│ Groq/Gemini          │
│ (with retry)       │  │ (on failure)         │
└────────────────────┴──────────────────────────┘
     ↓
dispatchIntent() → Route to Handler
     ↓ (failure)
Fallback → getHelpMessage() + Warning Log
```

**Key Features**:
- **Multi-Provider Support**: Gemini and Groq with automatic failover
- **Three-layer Fallback**: Model retry → Provider fallback → Graceful degradation
- Function Calling (AUTO mode): Model chooses function call or text response
- 9 intent functions: `course_search`, `course_smart`, `course_uid`, `id_search`, `id_student_id`, `id_department`, `contact_search`, `contact_emergency`, `help`
- Group @Bot detection: Uses `mention.Index` and `mention.Length` for precise removal
- Metrics: `ntpu_llm_total{provider,operation}`, `ntpu_llm_duration_seconds{provider}`, `ntpu_llm_fallback_total`

**Implementation Pattern**:
- `genai.IntentParser`: Interface for NLU parsing (implemented by Gemini and Groq)
- `genai.FallbackIntentParser`: Cross-provider failover wrapper
- `genai.CreateIntentParser()`: Factory function with provider selection
- `genai.ParseResult`: Module, Intent, Params, ClarificationText, FunctionName

**Default Models**:
- Gemini: `gemini-2.5-flash` (primary), `gemini-2.5-flash-lite` (fallback)
- Groq: `meta-llama/llama-4-maverick-17b-128e-instruct` (intent), `meta-llama/llama-4-scout-17b-16e-instruct` (expander), with Llama 3.x Production fallbacks

## Key File Locations

- **Entry point**: `cmd/server/main.go` - Application entry point (minimalist)
- **Application**: `internal/app/app.go` - Application lifecycle with DI, HTTP server, routes, middleware, background jobs
- **Webhook handler**: `internal/webhook/handler.go:Handle()` (async processing)
- **Warmup module**: `internal/warmup/warmup.go` (background cache warming)
- **Bot module interface**: `internal/bot/handler.go`
- **Context utilities**: `internal/ctxutil/context.go` (type-safe context values, PreserveTracing)
- **DB schema**: `internal/storage/schema.go`
- **LINE utilities**: `internal/lineutil/builder.go` (use instead of raw SDK)
- **Sticker manager**: `internal/sticker/sticker.go` (avatar URLs for messages)
- **Smart search**: `internal/rag/bm25.go` (BM25 index with Chinese tokenization)
- **Query expander**: `internal/genai/gemini_expander.go` / `internal/genai/groq_expander.go` (LLM-based query expansion)
- **NLU intent parser**: `internal/genai/gemini_intent.go` / `internal/genai/groq_intent.go` (Function Calling with Close method)
- **Syllabus scraper**: `internal/syllabus/scraper.go` (course syllabus extraction)
- **Timeout constants**: `internal/config/timeouts.go` (all timeout/interval constants)
