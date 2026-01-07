# NTPU LineBot Go - AI Agent Instructions

LINE chatbot for NTPU (National Taipei University) providing student ID lookup, contact directory, course queries, and academic program information. Built with Go, emphasizing anti-scraping measures, persistent caching, and observability.

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

**Initialization flow** (`internal/app/app.go:Initialize()`):
```
1. Logger (slog with custom ContextHandler)
2. Database (SQLite + WAL mode)
3. Metrics (Prometheus registry)
4. Scraper Client (rate-limited HTTP client)
5. Sticker Manager (avatar URLs)
6. BM25 Index (load from DB syllabi)
7. GenAI (IntentParser + QueryExpander with fallback, auto-enabled if API keys present)
8. LLMRateLimiter (per-user hourly token bucket, 120 burst, 60/hour refill)
9. UserRateLimiter (per-user request token bucket, webhook protection)
10. Handlers (id, course, contact, program with DI)
11. Registry (handler registration and dispatch)
12. Processor (message/intent routing with rate limiting)
13. Webhook (LINE event handler with async processing)
14. HTTP Server (Gin with security headers, routes, graceful shutdown)
```

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
      Bot Handlers (id/contact/course/program)
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
2. Optionally implement `DispatchIntent(ctx, intent, params)` for NLU support (checked via type assertion)
3. Create handler in app.Initialize() with dependencies (constructor-based DI)
4. Register via `registry.Register(handler)` (handlers matched in registration order)
5. Use prefix convention for postback routing (`"course:"`, `"id:"`, `"contact:"`, `"program:"`)
6. Pass nil for optional dependencies (e.g., `bm25Index`, `queryExpander`, `llmRateLimiter`)

**Registry dispatch flow**:
- **Message**: First-match wins (registration order), `CanHandle()` → `HandleMessage()`
- **Postback**: Module name lookup via `handlerMap`, `ParsePostback()` → `HandlePostback()`
- **NLU Intent**: Type assertion for `DispatchIntent()`, falls back to `HandleMessage()` if unsupported

**Course Module**:
- **Precise search** (`課程`): SQL LIKE + fuzzy search (2 recent semesters: 1st-2nd)
- **Extended search** (`更多學期`): SQL LIKE + fuzzy search (2 historical semesters: 3rd-4th)
- **Smart search** (`找課`): BM25 + Query Expansion (requires LLM API key)
- **Confidence scoring**: Relative BM25 score (0-1, first result always 1.0)
- **No cross-mode fallback**: Each search mode is independent and explicit

**Contact Module**:
- Emergency phones, multilingual keywords, Flex Message cards
- **2-tier SQL search**: SQL LIKE (name, title) + SQL Fuzzy `SearchContactsFuzzy()` (name, title, organization, superior)
- **Memory efficient**: Both searches use SQL-level character matching, no full-table loading
- **Sorting**: Organizations by hierarchy, individuals by match count

**ID Module**:
- **SQL character-set matching**: Dynamic LIKE clauses for each character (memory efficient)
- Supports non-contiguous character matching: "王明" and "明王" both match "王小明"
- Returns `StudentSearchResult{Students: []Student, TotalCount: int}` structure
- Displays "found X total, showing first 400" when results exceed limit

**Program Module**:
- **Pattern-Action Table**: Priority-sorted matchers (lower number = higher priority)
  - Priority 1 (highest): List (學程列表/program list/programs) - no parameters
  - Priority 2: Search (學程 XX/program XX) - extracts search term after keyword
  - Postback handlers: ViewCourses (`program:courses`), CourseProgramsList (`program:course_programs`)
- **2-tier search**: SQL LIKE + fuzzy `ContainsAllRunes()` (same as contact module)
- **Course ordering**: Required (必修) first, elective (選修) after, then by semester (newest first)
- **NLU intents**: `list` (no params), `search` (query), `courses` (programName)
- **Course detail integration**: "相關學程" button shows programs containing the course
- **Data source**: Parsed from course "應修系級" + "必選修別" fields (filters items ending with "學程")
- **Flex Message design**: Colored headers (blue for programs, green/cyan for courses by type)

**All modules**:
- Prefer text wrapping; use `TruncateRunes()` only for LINE API limits
- Consistent Sender pattern, cache-first strategy

## Data Layer: Cache-First Strategy

**SQLite cache** (`internal/storage/`):
- WAL mode, pure Go (`modernc.org/sqlite`)
- **Cache Strategy by Data Type**:
  - **Students**: Never expires, not refreshed (static data)
  - **Stickers**: Never expires, loaded once on startup
  - **Contacts/Courses/Programs**: 7-day TTL, refreshed daily at 3:00 AM Taiwan time
  - **Syllabi**: 7-day TTL, auto-enabled when LLM API key is configured
- TTL enforced at SQL level for contacts/courses/programs: `WHERE cached_at > ?`
- **Syllabi table**: Stores syllabus content + SHA256 hash for incremental updates
- **course_programs table**: Junction table for course-program relationships (course_uid, program_name, course_type, cached_at)

**BM25 Index** (`internal/rag/`):
- [iwilltry42/bm25-go](https://github.com/iwilltry42/bm25-go) (k1=1.5, b=0.75)
- In-memory index rebuilt on startup from SQLite
- Chinese tokenization (unigram for CJK), 1 course = 1 document
- Combined with LLM Query Expansion (auto-enabled when LLM API key configured)

**Background Jobs** (Taiwan time/Asia/Taipei):
- **Sticker**: Startup only
- **Daily Refresh** (3:00 AM): contact, course+programs (always), syllabus (only most recent 2 semesters, auto-enabled if LLM API key)
- **Cache Cleanup** (4:00 AM): Delete expired contacts/courses/programs/syllabi (7-day TTL) + VACUUM
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
- Syllabus: ONLY scraped during warmup for the most recent 2 semesters with cached data, 7-day TTL, auto-enabled when LLM API key configured

## Rate Limiting

**Scraper** (`internal/scraper/client.go`): No fixed delay between successful requests, exponential backoff on failure (1s initial, max 10 retries, ±25% jitter), 60s HTTP timeout per request

**Webhook**: Per-user (12 tokens, 1 token/5s refill), global (100 rps), silently drops excess requests

**LLM API** (`internal/ratelimit/llm_limiter.go`): Per-user multi-layer limits (default 120 burst, 60/hr refill, 300/day cap) for NLU and query expansion operations
- Token bucket with configurable burst capacity and hourly refill rate
- Daily sliding window to prevent quota exhaustion
- Independent from webhook rate limiter to control expensive LLM operations separately
- Auto-cleanup of inactive limiters every 5 minutes

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
lineutil.QuickReplyProgramNav()     // 學程列表→學程→說明 (program module)
lineutil.QuickReplyErrorRecovery(retryText) // 重試→說明 (errors with retry)

// Sender pattern (REQUIRED)
// System/Help: "北大小幫手" (unified for bot-level messages)
// Modules: "課程小幫手", "學號小幫手", "聯繫小幫手", "學程小幫手" (module-specific)
// Special: "貼圖小幫手" (sticker responses only)
sender := lineutil.GetSender("北大小幫手", stickerManager)  // Once at handler start
msg := lineutil.NewTextMessageWithConsistentSender(text, sender)
// Use same sender for all messages in one reply
```

**UX**:
- Always provide Quick Reply (including errors)
- Use `lineutil.QuickReply*` presets for consistency
- Show loading animation for long queries (> 1s)
- Use Flex Messages for rich content
- Include retry/help Quick Reply on errors
- Same sender throughout reply batch

**Flex Message 設計規範**:
- **配色** (WCAG AA 符合):
  - Hero 背景：模組特定色（課程藍、學生紫、聯絡青綠、緊急紅）、使用說明藍色系漸層、警告琥珀，標題白色
  - 主要文字 `#111111` (ColorText), 標籤 `#666666` (ColorLabel)
  - 次要文字 `#6B6B6B` (ColorSubtext), 備註 `#888888` (ColorNote)
  - 時間戳記 `#B7B7B7` (ColorGray400) - 僅用於不強調資訊
- **按鈕顏色** (語義化分類 - WCAG AA 符合):
  - `ColorButtonAction` `#10B981` (翠綠) - 主要操作 (複製學號、撥打電話、寄送郵件) - 4.5:1
  - `ColorButtonDanger` `#DC2626` (紅色) - 緊急操作 (緊急電話) - 4.7:1
  - `ColorWarning` `#D97706` (琥珀色) - 警告訊息 (配額達上限、限流提示) - 4.5:1
  - `ColorButtonExternal` `#3B82F6` (明亮藍) - 外部連結 (課程大綱、Dcard、選課大全、網站) - 4.6:1
  - `ColorButtonInternal` `#7C3AED` (紫色) - 內部指令/Postback (查看詳細、教師課程、查看成員、查詢學號) - 4.6:1
  - `ColorSuccess` `#059669` (青綠) - 成功狀態 (操作完成提示、確認訊息) - 4.5:1
  - `ColorDanger` `#E02D41` (深紅) - 危險狀態文字 (錯誤訊息、緊急聯絡標記) - 4.5:1
- **Header 顏色** (Colored Header 背景色 - 所有顏色符合 WCAG AA):
  - **設計理念**:
    - 學期: 藍色系**明度漸變** (明亮→標準→暗淡) 直覺表達時間的新→舊
    - 相關性: **青綠色系漸層** (深青綠→青綠→翠綠) 表達相關性強度，與學期藍色系明確區分
    - 使用說明: 藍紫色系**階層漸變** (主要→建議→資訊) 建立清晰的視覺層次
  - 學期標示: `ColorHeaderRecent` 明亮藍色 (最新學期), `ColorHeaderPrevious` 青色 (上個學期), `ColorHeaderHistorical` 暗灰 (過去學期)
  - 相關性標示: `ColorHeaderBest` 深青綠 (最佳匹配), `ColorHeaderHigh` 青綠 (高度相關), `ColorHeaderMedium` 翠綠 (部分相關) - 智慧搜尋
  - 聯絡類型: `ColorHeaderOrg` 明亮藍色 (組織單位), `ColorHeaderIndividual` 青色 (個人聯絡)
  - 詳情頁模組: `ColorHeaderCourse` 明亮藍色, `ColorHeaderContact` 青色, `ColorHeaderStudent` 紫色, `ColorHeaderEmergency` 紅色 (緊急聯絡)
  - 使用說明頁: `ColorHeaderPrimary` 皇家藍 (主要功能), `ColorHeaderTips` 明亮紫 (提示建議), `ColorHeaderInfo` 天空藍 (資訊展示)
  - **Header 文字顏色**: 所有 header 都使用白色文字 (ColorHeroText) 以確保 WCAG AA 對比度
- **Body Label 設計原則**:
  - **顏色協調**: Body label 文字顏色與 header 背景色一致，建立清晰的視覺關聯
  - **視覺層次**: Header 背景色 → Body label 文字色 (相同顏色)，創造連貫的視覺線索
  - **語義清晰**: 顏色強化語義含義 (藍=學術/組織, 青綠=相關性/個人, 紫=身份/建議, 紅=緊急等)
  - **設計一致**: 所有輪播卡片 (課程/聯絡人) 都遵循此模式，確保用戶體驗一致
- **間距**: Hero padding `24px`/`16px` (4-point grid), Body/Footer spacing `sm`, 按鈕高度 `sm`
- **文字**: 輪播卡片預設不換行 (緊湊顯示)；詳情頁可使用 `wrap: true` + `lineSpacing` 完整顯示資訊
- **截斷**: `TruncateRunes()` 僅用於 LINE API 限制 (altText 400 字, displayText 長度限制)
- **設計原則**: 對稱、現代、一致 - 確保視覺和諧，完整呈現資訊，所有顏色符合 WCAG AA 無障礙標準
- **資料說明**: 學號查詢結果的系所資訊由學號推測，若有轉系之類的情況可能與實際不符

- **輪播卡片設計模式**:
- 課程輪播 (Course): Colored Header (標題) → Body (標籤 + 資訊) → Footer
  - Header 使用 `NewColoredHeader()` 創建帶背景色的標題 (藍色/青色/灰色)
  - Body 第一列使用 `NewBodyLabel()` 顯示學期/相關性標籤 (文字顏色與 header 背景色一致)
  - 學期標籤: `🆕 最新學期` (明亮藍色), `📅 上個學期` (青色), `📦 過去學期` (暗灰色)
  - 相關性標籤: `🎯 最佳匹配` (深青綠色), `✨ 高度相關` (青綠色), `📋 部分相關` (翠綠色) - 智慧搜尋
  - **Footer 按鈕**: 「查看詳細」按鈕顏色與 header 同步 (`labelInfo.Color`)，增強視覺協調性
  - **視覺效果**: Header 背景色 = Body Label 文字色 = Footer 按鈕色，創造完整的視覺線索
- 聯絡人輪播 (Contact): Colored Header (姓名) → Body (標籤 + 資訊) → Footer
  - Header 使用 `NewColoredHeader()` 創建帶背景色的標題 (藍色/青綠色)
  - Body 第一列使用 `NewBodyLabel()` 顯示類型標籤 (文字顏色與 header 背景色一致)
  - 類型標籤: `🏢 組織單位` (明亮藍色), `👤 個人聯絡` (青色)
  - **Footer 按鈕**: 「查看成員」按鈕顏色與 header 同步 (`bodyLabel.Color`)，增強視覺協調性
  - **視覺效果**: Header 背景色 = Body Label 文字色 = Footer 按鈕色，與課程輪播保持一致
- 詳情頁 (所有模組): Colored Header (名稱) → Body (標籤 + 資訊) → Footer
  - **統一設計**: 所有模組 (Course/Contact/ID/Emergency) 都使用 `NewColoredHeader()` 呈現主要資訊
  - Course: 藍色 Header (課程名稱), Body 第一列顯示「📚 課程資訊」標籤 (明亮藍色文字)
  - Contact: 青色 Header (聯絡人姓名), Body 第一列顯示類型標籤（`🏢 組織單位` 或 `👤 個人聯絡`，文字色與 header 一致）
  - ID: 紫色 Header (學生姓名), Body 第一列顯示「🎓 國立臺北大學」標籤 (紫色文字)
  - Emergency: 紅色 Header (🚨 緊急聯絡電話), Body 第一列顯示「☎️ 校園緊急聯絡」標籤 (紅色文字)
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
**Optional**: `GEMINI_API_KEY` or `GROQ_API_KEY` or `CEREBRAS_API_KEY` (enables NLU + Query Expansion with multi-provider fallback)
**Platform paths**: `runtime.GOOS` determines default paths (Windows: `./data`, Linux/Mac: `/data`)

## Task Commands

```powershell
task dev              # Run server with debug logging (LOG_LEVEL=debug)
task test             # Run tests (skips network tests for speed, uses -short flag)
task test:full        # Run all tests including network tests
task test:race        # Run tests with race detector (requires CGO_ENABLED=1)
task test:coverage    # Coverage report with 80% threshold check (fails if < 80%)
task lint             # Run golangci-lint (5m timeout)
task fmt              # Format code and organize imports (goimports)
task build            # Build binaries to bin/ (CGO_ENABLED=0)
task clean            # Remove build artifacts (bin/, coverage files)
task compose:up       # Start monitoring stack (Prometheus/Grafana, see deployments/)
```

**Test patterns**:
- Use `-short` flag to skip network tests: `if testing.Short() { t.Skip("skipping network test") }`
- Table-driven tests with `t.Run()` for parallel execution
- In-memory SQLite (`:memory:`) via `setupTestDB()` helper

**Environment variables** (`.env`):
- **Required**: `LINE_CHANNEL_SECRET`, `LINE_CHANNEL_ACCESS_TOKEN`
- **Optional**: `GEMINI_API_KEY`, `GROQ_API_KEY`, `CEREBRAS_API_KEY` (enables NLU + Query Expansion with multi-provider fallback), `DATA_DIR` (default: `./data` on Windows, `/data` on Linux/Mac)
- **LLM Configuration**: `LLM_PROVIDERS` (comma-separated provider list, default: gemini,groq,cerebras), model override env vars (see `internal/config/config.go`)

Production warmup runs automatically on server startup (non-blocking). Database migration handled by `storage.New()`.

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

**Build stages**:
1. Builder: `golang:1.25.5-alpine` with CGO_ENABLED=0 for static binary
2. Runtime: `gcr.io/distroless/static-debian13:nonroot` (no shell, minimal attack surface)
3. Healthcheck: Custom binary (no `curl` dependency)
4. Volumes: `/data` (SQLite + cache), owned by nonroot:nonroot

## NLU Intent Parser (Multi-Provider)

**Location**: `internal/genai/` (types.go, gemini_intent.go, openai_intent.go, gemini_expander.go, openai_expander.go, factory.go, provider_fallback.go)

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
│ Gemini/Groq/       │→│ Groq/Cerebras/       │
│ Cerebras (retry)   │  │ Gemini (on failure)  │
└────────────────────┴──────────────────────────┘
     ↓
dispatchIntent() → Route to Handler
     ↓ (failure)
Fallback → getHelpMessage() + Warning Log
```

**Key Features**:
- **Multi-Provider Support**: Gemini, Groq, and Cerebras with automatic failover
- **Three-layer Fallback**: Model retry → Model chain fallback → Provider fallback → Graceful degradation
- **OpenAI v3 SDK**: Unified OpenAI-compatible implementation for Groq/Cerebras via custom BaseURL
- Function Calling (AUTO mode): Model chooses function call or text response
- 9 intent functions: `course_search`, `course_smart`, `course_uid`, `id_search`, `id_student_id`, `id_department`, `contact_search`, `contact_emergency`, `help`
- Group @Bot detection: Uses `mention.Index` and `mention.Length` for precise removal
- Metrics: `ntpu_llm_total{provider,operation}`, `ntpu_llm_duration_seconds{provider}`, `ntpu_llm_fallback_total`

**Implementation Pattern**:
- `genai.IntentParser`: Interface for NLU parsing (implemented by Gemini and OpenAI-compatible)
- `genai.QueryExpander`: Interface for query expansion (implemented by Gemini and OpenAI-compatible)
- `genai.FallbackIntentParser`: Cross-provider failover wrapper
- `genai.FallbackQueryExpander`: Cross-provider failover wrapper
- `genai.CreateIntentParser()`: Factory function with provider selection (default: `["gemini", "groq", "cerebras"]`)
- `genai.ParseResult`: Module, Intent, Params, ClarificationText, FunctionName

**Default Models**:
- Gemini: `gemini-2.5-flash` (primary), `gemini-2.5-flash-lite` (fallback)
- Groq: `meta-llama/llama-4-maverick-17b-128e-instruct` (intent), `meta-llama/llama-4-scout-17b-16e-instruct` (expander), with Llama 3.x Production fallbacks
- Cerebras: `llama-3.3-70b` (primary), `llama-3.1-8b` (fallback)

## Syllabus Module

**CRITICAL: Syllabus scraping is ONLY performed during warmup - never in real-time user queries**

**Warmup Behavior** (`internal/warmup/warmup.go:warmupSyllabusModule()`):
1. Identify most recent 2 semesters with cached course data via `GetDistinctRecentSemesters(ctx, 2)`
2. Load courses from those 2 semesters only via `GetCoursesByYearTerm(ctx, year, term)`
3. Scrape syllabus for each course via `syllabus.ScrapeSyllabus(ctx, course)`
4. Use SHA256 content hash for incremental updates (skip if content unchanged)
5. Save to database and rebuild BM25 index

**User Query Behavior**:
- Smart search (`找課`) uses BM25 index built from cached syllabi (read-only)
- Course detail queries show cached syllabus if available (read-only)
- NO scraping occurs during user queries - all data is pre-cached

**Cache Strategy**:
- TTL: 7 days (enforced at SQL level: `WHERE cached_at > ?`)
- Scope: Only most recent 2 semesters with data
- Trigger: Daily refresh at 3:00 AM (auto-enabled when LLM API key configured)
- Cleanup: Expired entries deleted at 4:00 AM

**Data Flow**:
```
Warmup (3:00 AM) → Scrape Syllabi (2 semesters) → Save to DB → Rebuild BM25
                                                      ↓
User Query (`找課`) → BM25 Search (read-only) → Return cached results
```

## Key File Locations

- **Entry point**: `cmd/server/main.go` - Application entry point (minimalist)
- **Application**: `internal/app/app.go` - Application lifecycle with DI, HTTP server, routes, middleware, background jobs
- **Webhook handler**: `internal/webhook/handler.go:Handle()` (async processing)
- **Warmup module**: `internal/warmup/warmup.go` (background cache warming, syllabus scraping)
- **Bot module interface**: `internal/bot/handler.go`
- **Context utilities**: `internal/ctxutil/context.go` (type-safe context values, PreserveTracing)
- **DB schema**: `internal/storage/schema.go`
- **LINE utilities**: `internal/lineutil/builder.go` (use instead of raw SDK)
- **Sticker manager**: `internal/sticker/sticker.go` (avatar URLs for messages)
- **Smart search**: `internal/rag/bm25.go` (BM25 index with Chinese tokenization, read-only during queries)
- **Query expander**: `internal/genai/gemini_expander.go` / `internal/genai/groq_expander.go` (LLM-based query expansion)
- **NLU intent parser**: `internal/genai/gemini_intent.go` / `internal/genai/groq_intent.go` (Function Calling with Close method)
- **Syllabus scraper**: `internal/syllabus/scraper.go` (ONLY called by warmup module)
- **Timeout constants**: `internal/config/timeouts.go` (all timeout/interval constants)
