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
                ↓ (7-day TTL check)
      Scraper Client (rate-limited)
                ↓ (exponential backoff, failover URLs)
          NTPU Websites (lms/sea)
                ↓
      Reply via Reply Token (< 30s)
```

**Critical Flow Details:**
- **Async processing**: HTTP 200 returned immediately after signature verification (< 2s), events processed in goroutine
- **LINE Best Practice**: Responds within 2s to prevent request_timeout errors, processes asynchronously to handle long operations
- **Context handling**: Webhook uses `context.Background()` for async processing, Bot operations use `ctxutil.PreserveTracing()` with 60s timeout
  - **Webhook layer**: Uses `context.Background()` directly (no parent context)
  - **Processor layer**: Calls `ctxutil.PreserveTracing(ctx)` to preserve tracing values from event source
  - **PreserveTracing()**: Creates new context with only necessary tracing values (userID, chatID, requestID)
  - **Why not WithoutCancel()**: Avoids memory leaks from parent references (see Go issue #64478)
  - **Why not Background()**: `Background()` loses all tracing data needed for log correlation, so it's not suitable for the processor layer even though it's used in the webhook layer
  - **Cancellation independence**: Detached context timeout is independent from HTTP request lifecycle
- **Detached context rationale**: LINE may close connection before processing completes; detached cancellation ensures DB queries and scraping finish, reply token remains valid (~20 min)
- **Observability**: Request ID and user ID flow through entire async operation for log correlation
- **Message batching**: LINE allows max 5 messages per reply; webhook auto-truncates to 4 + warning
- **References**:
  - LINE guidelines: https://developers.line.biz/en/docs/partner-docs/development-guidelines/
  - Context safety: https://github.com/golang/go/issues/64478

## Bot Module Registration Pattern

**When adding new modules**:

1. **Implement `bot.Handler` interface** (`internal/bot/handler.go`)
2. **Create handler in app.Initialize()** with required dependencies
3. **Register via `registry.Register(handler)` before creating webhook handler
4. **Use prefix convention** for postback routing (e.g., `"course:"`, `"id:"`, `"contact:"`)
5. **Direct constructors** - Pass all dependencies (including optional nil values) directly in constructor
6. **Warmup support** is automatic if module implements cache warming

**Handler constructor patterns**:
- **All handlers**: Direct constructors with all dependencies as parameters
- **Optional dependencies**: Pass nil if feature disabled (e.g., `bm25Index`, `queryExpander`, `llmRateLimiter`)
- **No setter injection**: All dependencies passed at construction time to avoid circular dependencies
- **BotConfig**: Embedded in main Config as `cfg.Bot` (no separate constructor needed)

**Module-specific features**:

- **Precise search**: `課程` keyword triggers SQL LIKE + fuzzy search on course title and teachers
- **Smart search**: `找課` keyword triggers BM25 + Query Expansion search using syllabus content (requires `GEMINI_API_KEY` or `GROQ_API_KEY` for Query Expansion)
- **BM25 search**: Keyword-based search with Chinese tokenization (unigram for CJK)
- **Confidence scoring**: Relative BM25 score (score / maxScore), not similarity. Range: 0-1, first result always 1.0.
- **Query expansion**: LLM-based expansion for short queries and technical abbreviations (AWS→雲端運算, AI→人工智慧)
- **Detached context**: Uses `ctxutil.PreserveTracing()` to prevent request context cancellation from aborting API calls (safer than WithoutCancel)
- **Fallback**: Precise search → BM25 smart search (when no results and BM25Index enabled)
- **UX terminology**: Uses "精確搜尋" (precise) for keyword search, "智慧搜尋" (smart) for BM25 search

**Contact Module**: Emergency phones (hardcoded), multilingual keywords, organization/individual contacts, Flex Message cards
- **SQL LIKE fields**: name, title (fast path)
- **Fuzzy search fields**: name, title, organization, superior (complete matching)
- **Sorting**: Organizations by hierarchy (top-level first), individuals by match count → name → title → organization

**All modules**:
- Prefer text wrapping over truncation for complete info display
- Use `TruncateRunes()` only for LINE API limits (altText, displayText)
- Consistent Sender pattern, cache-first strategy
- **2-tier parallel search**: SQL LIKE + fuzzy `ContainsAllRunes()` always run together, results merged and deduplicated

## Data Layer: Cache-First Strategy with Daily Warmup

**SQLite cache** (`internal/storage/`):
- WAL mode, Hard TTL (7 days), pure Go (`modernc.org/sqlite`)
- **Hard TTL (7 days)**: Data absolutely expired, must be deleted
- TTL enforced at SQL level: `WHERE cached_at > ?`
- **Syllabi table**: Stores syllabus content + SHA256 hash for incremental updates

**BM25 Index** (`internal/rag/`):
- Uses [iwilltry42/bm25-go](https://github.com/iwilltry42/bm25-go) (k1=1.5, b=0.75)
- Maintained by k3d-io/k3d (⭐6.1k) maintainer - reliable and actively fixed
- In-memory index (rebuilt on startup from SQLite)
- Chinese tokenization with unigram for CJK characters
- Single document strategy: 1 course = 1 document (no chunking)
- Combined with LLM Query Expansion for effective retrieval
- Optional: Gemini API Key enables Query Expansion
- **Important**: BM25 requires syllabus data - add `syllabus` to `WARMUP_MODULES` to enable smart search (找課)

**Background Jobs** (`internal/app/app.go`):
All maintenance tasks use **fixed Taiwan time (Asia/Taipei)** for predictable scheduling:
- **Sticker Refresh**: Runs on startup, then daily at **2:00 AM Taiwan time**
  - Updates sticker URLs from external sources
  - Runs first to ensure fresh sticker data before warmup
- **Daily Warmup** (proactiveWarmup): Runs on startup, then daily at **3:00 AM Taiwan time**
  - Refreshes modules specified in `WARMUP_MODULES` (default: sticker, id, contact, course)
  - **Concurrent**: id, contact, sticker, course - no dependencies between them
  - **Optional - syllabus**: If manually added to `WARMUP_MODULES`, waits for course to complete (needs course data), then runs in parallel with others. Removed from default due to infrequent updates.
    - **BM25 dependency**: Syllabus module rebuilds BM25 index after saving syllabi. Without syllabus warmup, smart search (找課) remains disabled even if Gemini API key is configured.
  - **Note**: sticker can be included in warmup modules for initial population
- **Cache Cleanup**: Runs on startup, then daily at **4:00 AM Taiwan time**
  - Deletes data past Hard TTL (7 days) + VACUUM for space reclamation
  - Runs after warmup to avoid deleting freshly cached data
- **Metrics Update**: Every 5 minutes (uses Ticker for high-frequency monitoring)
- **Rate Limiter Cleanup**: Every 5 minutes (uses Ticker for high-frequency cleanup)

**Data availability**:
- Student: 94-113 學年度 (≥114 shows deprecation notice)
- Course: Query: 4 most recent semesters with intelligent detection (checks if current semester has any data)
  - Warmup strategy: Scrapes 4 semesters individually using ScrapeCourses (4 requests per semester, 16 total)
  - Delayed upload tolerance: If current semester has no data yet, falls back to previous semester
- Contact: Real-time scraping

## Rate Limiting

**Scraper** (`internal/scraper/retry.go`): Fixed 2s delay after success, exponential backoff on failure (4s initial, max 5 retries, ±25% jitter), 60s HTTP timeout per request

**Webhook**: Per-user (6 tokens, 1 token/5s refill), global (100 rps), silently drops excess requests

**LINE SDK Conventions**

**Message builders** (`internal/lineutil/`):
```go
lineutil.NewTextMessage(text)                    // Simple text
lineutil.NewFlexMessage(altText, contents)       // Flex Message
lineutil.NewQuickReply(items)                    // Quick Reply (max 13)

// Consistent Sender pattern (REQUIRED)
sender := lineutil.GetSender("模組名", stickerManager)  // Once at handler start
msg := lineutil.NewTextMessageWithConsistentSender(text, sender)
// Use same sender for all messages in one reply
```

**UX Best Practices**: Quick Reply for guidance, Loading Animation for long queries, Flex Messages for rich UI, actionable error options

**Flex Message 設計規範**:
- **配色** (WCAG AA 符合):
  - Hero 背景 `#06C755` (LINE 綠), 標題白色
  - 主要文字 `#111111` (ColorText), 標籤 `#666666` (ColorLabel)
  - 次要文字 `#6B6B6B` (ColorSubtext), 備註 `#888888` (ColorNote)
  - 時間戳記 `#B7B7B7` (ColorGray400) - 僅用於不強調資訊
- **按鈕顏色** (語義化分類):
  - `ColorButtonPrimary` `#06C755` (LINE 綠) - 主要操作 (複製學號、撥打電話、寄送郵件)
  - `ColorDanger` `#FF334B` (紅色) - 緊急操作 (校安電話)
  - `ColorButtonExternal` `#469FD6` (藍色) - 外部連結 (課程大綱、Dcard、選課大全、網站)
  - `ColorButtonInternal` `#8B5CF6` (紫色) - 內部指令/Postback (教師課程、查看成員、查詢學號)
  - `secondary` 預設灰色 - 次要操作 (複製號碼、複製信箱)
- **間距**: Hero padding `24px`/`16px` (4-point grid), Body/Footer spacing `sm`, 按鈕高度 `sm`
- **文字**: 優先使用 `wrap: true` + `lineSpacing` 完整顯示資訊；僅 carousel 使用 `WithMaxLines()` 控制高度
- **截斷**: `TruncateRunes()` 僅用於 LINE API 限制 (altText 400 字, displayText 長度限制)
- **設計原則**: 對稱、現代、一致 - 確保視覺和諧，完整呈現資訊

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
- Groq: `meta-llama/llama-4-scout-17b-16e-instruct` (intent), `meta-llama/llama-4-maverick-17b-128e-instruct` (expander), with Llama 3.x Production fallbacks

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
