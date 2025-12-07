# NTPU LineBot Go - AI Agent Instructions

LINE chatbot for NTPU (National Taipei University) providing student ID lookup, contact directory, and course queries. Built with Go, emphasizing anti-scraping measures, persistent caching, and observability.

## 🎯 Architecture Principles (Dec 8, 2025)

**Core Design:**
1. **Pure Dependency Injection** - Constructor-based injection with functional options pattern
2. **Direct Dependencies** - Handlers use `*storage.DB` directly, interfaces only when truly needed
3. **Typed Error Handling** - Sentinel errors and custom error types with wrapping
4. **Centralized Configuration** - Bot config with load-time validation
5. **Context Management (Go 1.21+)** - `context.WithoutCancel()` for async operations
6. **Functional Options** - Optional parameters via `HandlerOption` pattern
7. **Simplified Registry** - Direct dispatch without middleware overhead
8. **No Circular Dependencies** - Clean initialization order: Core → GenAI → Handlers → Webhook

**Code Style:**
- **Pure DI**: All dependencies via constructors, functional options for optional features
- **Concrete Types**: Handlers depend on `*storage.DB` directly (no mocking needed)
- **Interface Placement**: Defined inline where needed (Go convention: small interfaces)
- **Functional Options**: For optional GenAI features (BM25, query expansion, LLM limiter)
- **Context Values**: Minimal usage for request tracing only
- **Error Handling**: Typed errors with wrapping for context
- **Constants**: Centralized in config package
- **Async Operations**: `context.WithoutCancel()` preserves tracing
- **Validation**: Load-time config validation, runtime parameter checks

## Architecture: Async Webhook Processing

```
LINE Webhook → Gin Handler
                ↓ (signature validation - synchronous)
          HTTP 200 OK (< 2s)
                ↓
          [Goroutine] Async Event Processing
                ↓ (Loading Animation + rate limiting)
      Bot Module Dispatcher
                ↓ (keyword matching via CanHandle())
      Bot Handlers (id/contact/course)
                ↓ (detached context with 60s timeout)
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
- **Context handling**: Bot operations use `context.WithoutCancel()` (Go 1.21+) with 60s timeout
  - **Why WithoutCancel()**: Preserves parent context Values (request ID, user ID) for tracing/logging
  - **Why not Background()**: Background() creates clean context but loses all tracing data
  - **Cancellation independence**: Child timeout is independent from HTTP request lifecycle
- **Detached context rationale**: LINE may close connection before processing completes; detached cancellation ensures DB queries and scraping finish, reply token remains valid (~20 min)
- **Observability**: Request ID and user ID flow through entire async operation for log correlation
- **Message batching**: LINE allows max 5 messages per reply; webhook auto-truncates to 4 + warning
- **Reference**: https://developers.line.biz/en/docs/partner-docs/development-guidelines/

## Bot Module Registration Pattern

**When adding new modules**:

1. **Implement `bot.Handler` interface** (`internal/bot/handler.go`)
2. **Create handler in Container.initBotHandlers()** with required dependencies
3. **Register in Container.initWebhook()** via `registry.Register(handler)`
4. **Use prefix convention** for postback routing (e.g., `"course:"`, `"id:"`, `"contact:"`)
5. **Functional options** for optional features (create `options.go` if needed)
6. **Warmup support** is automatic if module implements cache warming

**Module-specific features**:

**ID Module**: Year query range (95-112, name search 101-112), department selection flow, student search (max 500 results), Flex Message cards

**Course Module**: Smart semester detection (`semester.go`), UID regex (`(?i)\\d{3,4}[umnp]\\d{4}`), max 40 results, Flex Message carousels
- **Precise search**: `課程` keyword triggers SQL LIKE + fuzzy search on course title and teachers
- **Smart search**: `找課` keyword triggers BM25 + Query Expansion search using syllabus content (requires `GEMINI_API_KEY` for Query Expansion)
- **BM25 search**: Keyword-based search with Chinese tokenization (unigram for CJK)
- **Confidence scoring**: Rank-based confidence (not similarity). Higher rank = higher confidence.
- **Query expansion**: LLM-based expansion for short queries and technical abbreviations (AWS→雲端運算, AI→人工智慧)
- **Detached context**: Uses `context.WithoutCancel()` to prevent request context cancellation from aborting API calls
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

**Background Jobs** (`cmd/server/main.go`):
- **Daily Warmup**: Every day at 3:00 AM, refreshes all data modules unconditionally
  - **Concurrent**: id, contact, course - no dependencies between them
  - **Dependency**: syllabus waits for course to complete (needs course data), runs in parallel with others
  - **Sticker**: Handled separately by `refreshStickers` (every 24h, not included in daily warmup)
- **Cache Cleanup**: Every 12 hours, deletes data past Hard TTL (7 days) + VACUUM
- **Sticker Refresh**: Every 24 hours (separate from daily warmup)

**Data availability**:
- Student: 101-112 學年度 (≥113 shows deprecation notice)
- Course: Cache: 2 years (4 semesters) | Query: 2 most recent semesters (auto-detect based on month)
- Contact: Real-time scraping

## Rate Limiting

**Scraper** (`internal/scraper/retry.go`): Fixed 2s delay after success, exponential backoff on failure (4s initial, max 5 retries, ±25% jitter), 60s HTTP timeout per request

**Webhook**: Per-user (6 tokens, 1 token/5s refill), global (80 rps), silently drops excess requests

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
- **間距**: Hero padding `24px`/`16px` (4-point grid), Body/Footer spacing `sm`, 按鈕高度 `sm`
- **文字**: 優先使用 `wrap: true` + `lineSpacing` 完整顯示資訊；僅 carousel 使用 `WithMaxLines()` 控制高度
- **截斷**: `TruncateRunes()` 僅用於 LINE API 限制 (altText 400 字, displayText 長度限制)
- **設計原則**: 對稱、現代、一致 - 確保視覺和諧，完整呈現資訊

**Postback format** (300 byte limit): Use module prefix `"module:data"` for routing. Reply token is single-use - batch all messages into one array.

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

Table-driven tests with `t.Run()`, in-memory SQLite for DB tests (`setupTestDB()` helper in test files)

## Configuration

Env vars loaded at startup (`internal/config/`). Requires LINE credentials. Platform-specific paths via `runtime.GOOS`.

## Task Commands

```powershell
task dev          # Run server
task ci           # Full CI pipeline
task test:coverage  # Coverage report
task compose:up   # Start monitoring stack
```

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

Multi-stage build (alpine builder + distroless runtime), init-data for permissions, healthcheck binary (no shell).

## NLU Intent Parser

**Location**: `internal/genai/` (types.go, intent.go, functions.go, prompts.go)

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
IntentParser.Parse() (Gemini 2.5 Flash Lite)
     ↓
dispatchIntent() → Route to Handler
     ↓ (failure)
Fallback → getHelpMessage() + Warning Log
```

**Key Features**:
- Function Calling (AUTO mode): Model chooses function call or text response
- 9 intent functions: `course_search`, `course_smart`, `course_uid`, `id_search`, `id_student_id`, `id_department`, `contact_search`, `contact_emergency`, `help`
- Group @Bot detection: Uses `mention.Index` and `mention.Length` for precise removal
- Fallback strategy: NLU failure → help message with warning log
- Metrics: `ntpu_llm_total{operation="nlu"}`, `ntpu_llm_duration_seconds{operation="nlu"}`

**Interface Pattern**:
- `genai.IntentParser`: Interface defining Parse(), IsEnabled(), Close()
- `genai.GeminiIntentParser`: Gemini-based implementation of IntentParser
- `genai.ParseResult`: Module, Intent, Params, ClarificationText, FunctionName
- webhook imports genai package directly (no adapter needed)

## Key File Locations

- **Entry point**: `cmd/server/main.go` - Application entry point (minimalist, ~28 lines)
- **Application class**: `internal/container/application.go` - Pure DI with HTTP server, routes, middleware, background jobs
- **Dependency container**: `internal/container/container.go` - Initialization lifecycle management
- **Webhook handler**: `internal/webhook/handler.go:Handle()` (async processing)
- **Warmup module**: `internal/warmup/warmup.go` (background cache warming)
- **Bot module interface**: `internal/bot/handler.go`
- **DB schema**: `internal/storage/schema.go`
- **LINE utilities**: `internal/lineutil/builder.go` (use instead of raw SDK)
- **Sticker manager**: `internal/sticker/sticker.go` (avatar URLs for messages)
- **Smart search**: `internal/rag/bm25.go` (BM25 index with Chinese tokenization)
- **Query expander**: `internal/genai/expander.go` (LLM-based query expansion)
- **NLU intent parser**: `internal/genai/intent.go` (Function Calling with Close method)
- **Syllabus scraper**: `internal/syllabus/scraper.go` (course syllabus extraction)
- **Timeout constants**: `internal/config/timeouts.go` (all timeout/interval constants)
