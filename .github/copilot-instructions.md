# NTPU LineBot Go - AI Agent Instructions

LINE chatbot for NTPU (National Taipei University) providing student ID lookup, contact directory, and course queries. Built with Go, emphasizing anti-scraping measures, persistent caching, and observability.

## Architecture: Async Webhook Processing (2025 Best Practice)

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
- **Context handling**: Bot operations use detached context (`context.WithoutCancel`) with 60s timeout, independent from HTTP request lifecycle
- **Detached context rationale**: LINE may close connection before processing completes; detached context ensures DB queries and scraping finish, reply token remains valid (~20 min)
- **Message batching**: LINE allows max 5 messages per reply; webhook auto-truncates to 4 + warning
- **Reference**: https://developers.line.biz/en/docs/partner-docs/development-guidelines/

## Bot Module Registration Pattern

**When adding new modules**:

1. **Implement `bot.Handler` interface** (`internal/bot/handler.go`)
2. **Register in webhook constructor** and dispatcher (`internal/webhook/handler.go`)
3. **Use prefix convention** for postback routing (e.g., `"course:"`, `"id:"`, `"contact:"`)
4. **Warmup support** is automatic if module implements cache warming

**Module-specific features**:

**ID Module**: Year query range (95-112, name search 101-112), department selection flow, student search (max 500 results), Flex Message cards

**Course Module**: Smart semester detection (`semester.go`), UID regex (`(?i)\d{3,4}[umnp]\d{4}`), max 40 results, Flex Message carousels
- **Semantic search**: `找課` keyword triggers embedding-based search using syllabus content (requires `GEMINI_API_KEY`)
- **Detached context**: Uses `context.WithoutCancel()` to prevent request context cancellation from aborting embedding API calls
- **Fallback**: Keyword search → semantic search (when no results and VectorDB enabled)

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

**Vector store** (`internal/rag/`):
- chromem-go (Pure Go, gob persistence to `data/chromem/syllabi/`)
- Gemini embedding API (`gemini-embedding-001`, 768 dimensions)
- Optional: only enabled when `GEMINI_API_KEY` is set

**Background Jobs** (`cmd/server/main.go`):
- **Daily Warmup**: Every day at 3:00 AM, refreshes all data modules unconditionally
  - **Concurrent**: id, contact, course - no dependencies between them
  - **Dependency**: syllabus waits for course to complete (needs course data), runs in parallel with others
  - **Sticker**: Handled separately by `refreshStickers` (every 24h, not included in daily warmup)
- **Cache Cleanup**: Every 12 hours, deletes data past Hard TTL (7 days) + VACUUM
- **Sticker Refresh**: Every 24 hours (separate from daily warmup)

**Data availability**:
- Student: 101-112 學年度 (≥113 shows deprecation notice)
- Course: Current + previous year (2 years, auto-detect based on month)
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
sum(rate(ntpu_cache_hits_total[5m])) / (sum(rate(ntpu_cache_hits_total[5m])) + sum(rate(ntpu_cache_misses_total[5m])))

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
- 9 intent functions: `course_search`, `course_semantic`, `course_uid`, `id_search`, `id_student_id`, `id_department`, `contact_search`, `contact_emergency`, `help`
- Group @Bot detection: Uses `mention.Index` and `mention.Length` for precise removal
- Fallback strategy: NLU failure → help message with warning log
- Metrics: `NLURequestsTotal`, `NLUDurationSeconds`, `NLUFallbackTotal`

**Interface Pattern**:
- `genai.IntentParser`: Interface defining Parse(), IsEnabled(), Close()
- `genai.GeminiIntentParser`: Gemini-based implementation of IntentParser
- `genai.ParseResult`: Module, Intent, Params, ClarificationText, FunctionName
- webhook imports genai package directly (no adapter needed)

## Key File Locations

- **Entry points**: `cmd/server/main.go`, `cmd/healthcheck/main.go`
- **Server organization**:
  - `cmd/server/main.go` - Entry point and initialization
  - `cmd/server/routes.go` - HTTP routes configuration
  - `cmd/server/middleware.go` - Security and logging middleware
  - `cmd/server/jobs.go` - Background jobs (cleanup, warmup, metrics)
- **Webhook handler**: `internal/webhook/handler.go:Handle()` (async processing)
- **Warmup module**: `internal/warmup/warmup.go` (background cache warming)
- **Bot module interface**: `internal/bot/handler.go`
- **DB schema**: `internal/storage/schema.go`
- **LINE utilities**: `internal/lineutil/builder.go` (use instead of raw SDK)
- **Sticker manager**: `internal/sticker/sticker.go` (avatar URLs for messages)
- **Semantic search**: `internal/rag/vectordb.go` (chromem-go wrapper)
- **Embedding client**: `internal/genai/embedding.go` (Gemini API)
- **NLU intent parser**: `internal/genai/intent.go` (Function Calling with Close method)
- **Syllabus scraper**: `internal/syllabus/scraper.go` (course syllabus extraction)
- **Timeout constants**: `internal/config/timeouts.go` (all timeout/interval constants)
