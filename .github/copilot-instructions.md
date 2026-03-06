# NTPU LineBot Go — Copilot Instructions

## Big picture
- LINE webhook is async: handler returns 200 fast, then processes events in a goroutine with preserved tracing. See internal/webhook/handler.go and internal/ctxutil/context.go.
- Dispatch flow: registry first-match on CanHandle, then HandleMessage / HandlePostback. Handlers are registered in app.Initialize order. See internal/bot/registry.go and internal/app/app.go.
- Reply token is single-use; batch replies (max 5 messages) and keep a consistent sender per reply.

## Data & caching
- SQLite cache-first (WAL) with TTL for contacts/courses/programs/syllabi; students/stickers never expire. See internal/storage/.
- Refresh/cleanup jobs run on intervals; syllabus scraping happens ONLY in warmup/refresh (never in user queries). See internal/warmup/warmup.go and internal/syllabus/.
- Smart course search uses BM25 index rebuilt on startup from cached syllabi. See internal/rag/bm25.go.

## Module conventions
- Pure constructor DI; handlers depend on *storage.DB directly; optional deps are passed as nil when unused.
- Postbacks use module prefix "module:data". When parsing, extract the matched payload, not the full string.
- LINE message building uses internal/lineutil/* presets: QuickReply* and NewTextMessageWithConsistentSender. Use TruncateRunes only for LINE API limits; otherwise prefer wrap:true in Flex content.

## Config & env
- Config is validated at load time (internal/config/*). Required: NTPU_LINE_CHANNEL_SECRET and NTPU_LINE_CHANNEL_ACCESS_TOKEN.
- LLM features are optional (NTPU_LLM_ENABLED + provider API key) and power NLU + query expansion.
- Default data dir is OS-dependent (Windows ./data, Linux/Mac /data).

## Dev workflows (Taskfile.yml)
- task dev (debug server), task test (short), task test:full (includes network), task test:coverage (80% threshold), task lint, task fmt, task build, task compose:up.
- Tests use table-driven patterns; DB tests use in-memory SQLite via setupTestDB(); network tests must guard with testing.Short().

## Key entry points
- cmd/server/main.go (app entry), internal/app/app.go (DI + server), internal/bot/processor.go (routing).# NTPU LineBot Go - AI Agent Instructions

LINE chatbot "NTPU 小工具" for NTPU (National Taipei University) providing student ID lookup, contact directory, course queries, and academic program information. Built with Go, emphasizing anti-scraping measures, persistent caching, and observability.

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
4. Scraper Client (rate-limited HTTP client with per-domain rate limiting)
5. Sticker Manager (avatar URLs)
6. Chinese Segmenter (shared gse word segmenter for BM25 + suggest)
7. BM25 Index (load from DB syllabi, uses shared segmenter)
8. GenAI (IntentParser + QueryExpander with fallback, auto-enabled if API keys present)
9. LLMRateLimiter (per-user hourly token bucket, 60 burst, 30/hour refill)
10. UserRateLimiter (per-user request token bucket, webhook protection)
11. Handlers (id, course, contact, program with DI, segmenter injected)
12. Registry (handler registration and dispatch)
13. SessionStore (per-user conversation context, 3 intents, 5 min TTL)
14. Processor (message/intent routing with rate limiting and session context)
15. Webhook (LINE event handler with async processing)
16. HTTP Server (Gin with security headers, routes, graceful shutdown)
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
- **Data source**: Dual-source fusion during refresh task
  - List page (queryByKeyword): provides required/elective types
  - Detail page (queryguide): provides complete program names
  - Fuzzy matching merges types into full names
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
  - **Contacts/Courses/Programs**: 7-day TTL, refreshed on `NTPU_MAINTENANCE_REFRESH_INTERVAL`
  - **Syllabi**: 7-day TTL, auto-enabled when LLM API key is configured
- TTL enforced at SQL level for contacts/courses/programs: `WHERE cached_at > ?`
- **Syllabi table**: Stores syllabus content + SHA256 hash for incremental updates
- **course_programs table**: Junction table for course-program relationships (course_uid, program_name, course_type, cached_at)

**BM25 Index** (`internal/rag/`):
- [iwilltry42/bm25-go](https://github.com/iwilltry42/bm25-go) (k1=1.2, b=0.75)
- In-memory index rebuilt on startup from SQLite
- Chinese tokenization via shared `stringutil.Segmenter` (gse search-optimized word segmentation), 1 course = 1 document
- Min confidence filter (0.25) removes low-relevance noise
- Combined with LLM Query Expansion (auto-enabled when LLM API key configured)

**Background Jobs** (Taiwan time/Asia/Taipei):
- **Sticker**: Startup only
- **Refresh Task** (interval-based): contact, course+programs (always), syllabus (only most recent 2 semesters, auto-enabled if LLM API key)
- **Cleanup Task** (interval-based): Delete expired contacts/courses/programs/syllabi (7-day TTL) + VACUUM
- **Metrics/Rate Limiter Cleanup**: Every 5 minutes

**Data availability**:
- Student:
  - **Cache range**: 101-112 學年度 (refresh task auto-loads, complete data)
  - **Query range**: 94-112 學年度 (real-time scraping, complete data)
  - **Year 113**: Allowed with warning, extremely sparse data (only students with manual LMS 2.0 accounts)
    - **Academic year query** (`handleYearQuery`): Shows warning before proceeding
    - **Student ID query** (`handleStudentIDQuery`): Returns data if available (no warning); shows special explanation if empty
    - Empty results show special explanation message
  - **Year 114+**: Rejected with RIP image + `IDLMSDeprecatedMessage` (no data at all)
    - **Academic year query**: RIP image + deprecation message
    - **Student ID query**: Early rejection before database query
  - **Status**: Static data, year 114+ has no data due to LMS 2.0 deprecation
- Course:
  - **Cache range**: 4 most recent semesters (7-day TTL, refresh task auto-loads)
  - **Query range**: 90-current year (Course system launched 90, real-time scraping supported)
  - **Validation**: Uses `config.CourseSystemLaunchYear` as minimum, not limited by cache content
- Contact: 7-day TTL
- Sticker: Startup only, never expires
- Syllabus: ONLY scraped during refresh task for the most recent 2 semesters with cached data, 7-day TTL, auto-enabled when LLM API key configured

## Rate Limiting

**Scraper** (`internal/scraper/client.go`): Per-domain rate limiting (burst=3, 5 rps via token bucket), exponential backoff on failure (1s initial, max 10 retries, ±25% jitter), 60s HTTP timeout per request

**Webhook**: Per-user (15 tokens, 1 token/10s refill), global (100 rps), silently drops excess requests

**LLM API** (`internal/ratelimit/llm_limiter.go`): Per-user multi-layer limits (default 60 burst, 30/hr refill, 180/day cap) for NLU and query expansion operations
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
lineutil.QuickReplyMainNav()        // 課程→學程→學號→聯絡→緊急→說明→回報 (welcome, help)
lineutil.QuickReplyMainNavCompact() // 課程→學程→學號→聯絡→說明→回報 (errors, rate limit)
lineutil.QuickReplyMainFeatures()   // 課程→學程→學號→聯絡→緊急→回報 (instruction messages)
lineutil.QuickReplyContactNav()     // 聯絡→緊急→說明 (contact module)
lineutil.QuickReplyStudentNav()     // 學號→學年→系代碼→說明 (id module)
lineutil.QuickReplyCourseNav(bool)  // 課程→找課(if smart)→說明 (course module)
lineutil.QuickReplyProgramNav()     // 學程列表→學程→說明 (program module)
lineutil.QuickReplyErrorRecovery(retryText) // 重試→說明 (errors with retry)

// Sender pattern (REQUIRED)
// System/Help: "NTPU 小工具" (unified for bot-level messages)
// Modules: "課程小幫手", "學號小幫手", "聯繫小幫手", "學程小幫手" (module-specific)
// Special: "貼圖小幫手" (sticker responses only)
sender := lineutil.GetSender("NTPU 小工具", stickerManager)  // Once at handler start
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
  - `ColorButtonInternal` `#7C3AED` (紫色) - 內部指令/Postback (詳細資訊、教師課程、成員列表、查詢學號) - 4.6:1
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
- **資料說明**: 學號查詢結果的系所資訊由學號推測，可能與實際不符

- **輪播卡片設計模式**:
- 課程輪播 (Course): Colored Header (標題) → Body (標籤 + 資訊) → Footer
  - Header 使用 `NewColoredHeader()` 創建帶背景色的標題 (藍色/青色/灰色)
  - Body 第一列使用 `NewBodyLabel()` 顯示學期/相關性標籤 (文字顏色與 header 背景色一致)
  - 學期標籤: `🆕 最新學期` (明亮藍色), `📅 上個學期` (青色), `🗃️ 過去學期` (暗灰色)
  - 相關性標籤: `🎯 最佳匹配` (深青綠色), `✨ 高度相關` (青綠色), `📋 部分相關` (翠綠色) - 智慧搜尋
  - **Footer 按鈕**: 「詳細資訊」按鈕顏色與 header 同步 (`labelInfo.Color`)，增強視覺協調性
  - **視覺效果**: Header 背景色 = Body Label 文字色 = Footer 按鈕色，創造完整的視覺線索
- 聯絡人輪播 (Contact): Colored Header (姓名) → Body (標籤 + 資訊) → Footer
  - Header 使用 `NewColoredHeader()` 創建帶背景色的標題 (藍色/青綠色)
  - Body 第一列使用 `NewBodyLabel()` 顯示類型標籤 (文字顏色與 header 背景色一致)
  - 類型標籤: `🏢 組織單位` (明亮藍色), `👤 個人聯絡` (青色)
  - **Footer 按鈕**: 「成員列表」按鈕顏色與 header 同步 (`bodyLabel.Color`)，增強視覺協調性
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
**Required**: `NTPU_LINE_CHANNEL_SECRET`, `NTPU_LINE_CHANNEL_ACCESS_TOKEN`
**Optional**: `NTPU_LLM_ENABLED=true` + (`NTPU_GEMINI_API_KEY` or `NTPU_GROQ_API_KEY` or `NTPU_CEREBRAS_API_KEY`) enables NLU + Query Expansion with multi-provider fallback
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
task compose:up       # Start Docker Compose deployment (bot only)
```

**Test patterns**:
- Use `-short` flag to skip network tests: `if testing.Short() { t.Skip("skipping network test") }`
- Table-driven tests with `t.Run()` for parallel execution
- In-memory SQLite (`:memory:`) via `setupTestDB()` helper

**Environment variables** (`.env`):
- **Required**: `NTPU_LINE_CHANNEL_ACCESS_TOKEN`, `NTPU_LINE_CHANNEL_SECRET`
- **LLM** (Optional): `NTPU_LLM_ENABLED`, `NTPU_GEMINI_API_KEY`, `NTPU_GROQ_API_KEY`, `NTPU_CEREBRAS_API_KEY`, `NTPU_LLM_PROVIDERS`, `NTPU_*_INTENT_MODELS`, `NTPU_*_EXPANDER_MODELS`
- **Server**: `NTPU_PORT`, `NTPU_LOG_LEVEL`, `NTPU_SHUTDOWN_TIMEOUT`, `NTPU_SERVER_NAME`, `NTPU_INSTANCE_ID`
- **Data**: `NTPU_DATA_DIR` (default: `./data` on Windows, `/data` on Linux/Mac), `NTPU_CACHE_TTL`
- **Scraper**: `NTPU_SCRAPER_TIMEOUT`, `NTPU_SCRAPER_MAX_RETRIES`
- **Rate Limits**: `NTPU_USER_RATE_BURST`, `NTPU_USER_RATE_REFILL`, `NTPU_LLM_RATE_BURST`, `NTPU_LLM_RATE_REFILL`, `NTPU_LLM_RATE_DAILY`, `NTPU_GLOBAL_RATE_RPS`
- **Startup**: `NTPU_WARMUP_WAIT` (default: `false`, gates /webhook only), `NTPU_WARMUP_GRACE_PERIOD` (default: `10m`, readiness grace period)
- **Intervals**: `NTPU_MAINTENANCE_REFRESH_INTERVAL`, `NTPU_MAINTENANCE_CLEANUP_INTERVAL`, `NTPU_R2_SNAPSHOT_POLL_INTERVAL`
- **Metrics**: `NTPU_METRICS_AUTH_ENABLED`, `NTPU_METRICS_USERNAME`, `NTPU_METRICS_PASSWORD`

See `.env.example` for full documentation. Production: set `NTPU_WARMUP_WAIT=true` if you want /webhook to wait for warmup readiness.

## Error Handling

Wrap errors with context (`fmt.Errorf(..., %w)`), structured logging with fields, user-facing messages via `lineutil.ErrorMessage()`.

## Scraper Client

Multiple base URLs per domain (LMS/SEA), automatic failover on 500+ errors, URLCache for performance.
Per-domain rate limiting (burst=3, 5 rps) prevents overwhelming individual servers.

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
1. Builder: `golang:1.26.0-alpine` with CGO_ENABLED=0 for static binary
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
- Metrics: `ntpu_llm_total{provider,operation}`, `ntpu_llm_duration_seconds{provider}`, `ntpu_llm_fallback_total`, `ntpu_intent_total{module,intent,source}`

**Implementation Pattern**:
- `genai.IntentParser`: Interface for NLU parsing (implemented by Gemini and OpenAI-compatible)
- `genai.QueryExpander`: Interface for query expansion (implemented by Gemini and OpenAI-compatible)
- `genai.FallbackIntentParser`: Cross-provider failover wrapper
- `genai.FallbackQueryExpander`: Cross-provider failover wrapper
- `genai.CreateIntentParser()`: Factory function with provider selection (default: `["gemini", "groq", "cerebras"]`)
- `genai.ParseResult`: Module, Intent, Params, ClarificationText, FunctionName

**Default Models**:
- Gemini: `gemini-3.1-pro-preview` (intent), `gemini-3.1-pro-preview` (expander), with `gemini-2.5-pro` / `gemini-2.5-flash` fallbacks
- Groq: `openai/gpt-oss-120b` (primary), intent chain: `llama-3.3-70b-versatile` → `qwen/qwen3-32b` → `gpt-oss-20b` → `llama-3.1-8b-instant`; expander chain: `qwen/qwen3-32b` → `gpt-oss-20b` → `llama-3.3-70b-versatile` → `llama-3.1-8b-instant`
- Cerebras: intent `zai-glm-4.7` → `gpt-oss-120b` → `qwen-3-235b-a22b-instruct-2507` → `llama3.1-8b`; expander `qwen-3-235b-a22b-instruct-2507` → `zai-glm-4.7` → `gpt-oss-120b` → `llama3.1-8b`

## Syllabus Module

**CRITICAL: Syllabus and program data scraping is ONLY performed during refresh tasks - never in real-time user queries**

**Refresh Behavior** (`internal/warmup/warmup.go:warmupSyllabusModule()`):
1. Course refresh scrapes list pages and collects raw program requirements (name + type)
2. Identify most recent 2 semesters with cached course data via `GetDistinctRecentSemesters(ctx, 2)`
3. Load courses from those 2 semesters only via `GetCoursesByYearTerm(ctx, year, term)`
4. Scrape course detail page via `syllabus.ScrapeCourseDetail(ctx, course)` - returns syllabus content + matched programs
5. Match list-page types to detail-page full names (dual-source fusion)
6. Use SHA256 content hash for incremental updates (skip if content unchanged)
7. Save syllabus to database, save course-program relationships via `SaveCoursePrograms()`
8. Rebuild BM25 index

**Data Extraction (Dual-source)**:
- **List page (queryByKeyword)**: Extracts "應修系級" + "必選修別" pairs (may be abbreviated)
- **Detail page (queryguide)**: Extracts full program names from Major field (complete, accurate)
- **Fusion**: Fuzzy matching aligns list-page types to detail-page names

**User Query Behavior**:
- Smart search (`找課`) uses BM25 index built from cached syllabi (read-only)
- Course detail queries show cached syllabus if available (read-only)
- Program queries use course-program relationships populated during refresh (read-only)
- NO scraping occurs during user queries - all data is pre-cached

**Cache Strategy**:
- TTL: 7 days (enforced at SQL level: `WHERE cached_at > ?`)
- Scope: Only most recent 2 semesters with data
- Trigger: Interval-based refresh (auto-enabled when LLM API key configured)
- Cleanup: Expired entries deleted on `NTPU_MAINTENANCE_CLEANUP_INTERVAL`

**Data Flow**:
```
Refresh Task (interval-based)
  → ScrapeCourses (list page, 4 semesters) → RawProgramReqs
  → ScrapeCourseDetail (2 semesters) → Full Program Names
  → Match (types + names) → Save Programs + Syllabus → Rebuild BM25
                                   ↓
User Query (`找課`/`學程`) → BM25/SQL Search (read-only) → Return cached results
```

## Key File Locations

- **Entry point**: `cmd/server/main.go` - Application entry point (minimalist)
- **Application**: `internal/app/app.go` - Application lifecycle with DI, HTTP server, routes, middleware, background jobs
- **Webhook handler**: `internal/webhook/handler.go:Handle()` (async processing)
- **Warmup module**: `internal/warmup/warmup.go` (background data refresh, syllabus scraping)
- **Bot module interface**: `internal/bot/handler.go`
- **Context utilities**: `internal/ctxutil/context.go` (type-safe context values, PreserveTracing)
- **DB schema**: `internal/storage/schema.go`
- **LINE utilities**: `internal/lineutil/builder.go` (use instead of raw SDK)
- **Sticker manager**: `internal/sticker/sticker.go` (avatar URLs for messages)
- **Smart search**: `internal/rag/bm25.go` (BM25 index with Chinese tokenization, read-only during queries)
- **Query expander**: `internal/genai/gemini_expander.go` / `internal/genai/openai_expander.go` (LLM-based query expansion for Gemini/Groq/Cerebras)
- **NLU intent parser**: `internal/genai/gemini_intent.go` / `internal/genai/openai_intent.go` (Function Calling with Close method for Gemini/Groq/Cerebras)
- **Syllabus scraper**: `internal/syllabus/scraper.go` (extracts syllabus, parses full program names, and fuses with list-page types; ONLY called by refresh task)
- **Chinese segmenter**: `internal/stringutil/segmenter.go` (shared gse word segmenter for BM25 indexing + suggest features)
- **String utilities**: `internal/stringutil/strings.go` (SanitizeText, ContainsAllRunes, etc.)
- **Session store**: `internal/session/store.go` (per-user conversation context for NLU disambiguation)
- **Timeout constants**: `internal/config/timeouts.go` (all timeout/interval constants)
