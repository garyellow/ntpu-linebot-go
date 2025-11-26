# NTPU LineBot Go - AI Agent Instructions

LINE chatbot for NTPU (National Taipei University) providing student ID lookup, contact directory, and course queries. Built with Go, emphasizing anti-scraping measures, persistent caching, and observability.

## Architecture: 3-Layer Request Flow

```
LINE Webhook → Gin Handler (25s timeout) → Bot Module Dispatcher
                ↓ (signature validation, rate limiting)
      Bot Handlers (id/contact/course)
                ↓ (keyword matching via CanHandle())
      Storage Repository (cache-first)
                ↓ (7-day TTL check)
      Scraper Client (rate-limited)
                ↓ (exponential backoff, failover URLs)
          NTPU Websites (lms/sea)
```

**Critical Flow Details:**
- **Context timeout**: All bot operations inherit 25s deadline from webhook (`internal/webhook/handler.go:214`)
- **Message batching**: LINE allows max 5 messages per reply; webhook auto-truncates to 4 + warning (`handler.go:159`)

## Bot Module Registration Pattern

**When adding new modules**:

1. **Implement `bot.Handler` interface** (`internal/bot/handler.go`)
2. **Register in webhook constructor** and dispatcher (`internal/webhook/handler.go`)
3. **Use prefix convention** for postback routing (e.g., `"course:"`, `"id:"`, `"contact:"`)
4. **Warmup support** is automatic if module implements cache warming

**Module-specific features**:

**ID Module**: Year validation (89-130, AD↔ROC), department selection flow, student search (max 500 results), Flex Message cards

**Course Module**: Smart semester detection (`semester.go`), UID regex (`(?i)\d{3,4}[umnp]\d{4}`), max 50 results, Flex Message carousels

**Contact Module**: Emergency phones (hardcoded), multilingual keywords, organization/individual contacts, Flex Message cards
- **Search fields**: name, title (職稱)
- **Sorting**: Organizations by hierarchy (top-level first), individuals by match count → name → title → organization

**All modules**: Prefer text wrapping over truncation for complete info display, use `TruncateRunes()` only for LINE API limits (altText, displayText), consistent Sender pattern, cache-first strategy

## Data Layer: Cache-First Strategy with Soft/Hard TTL

**SQLite cache** (`internal/storage/`):
- WAL mode, Soft/Hard TTL (configurable), pure Go (`modernc.org/sqlite`)
- **Soft TTL (5 days)**: Data considered stale, triggers proactive warmup
- **Hard TTL (7 days)**: Data absolutely expired, must be deleted
- TTL enforced at SQL level: `WHERE cached_at > ?`


**Background Jobs** (`cmd/server/main.go`):
- **Proactive Warmup**: Daily 3:00 AM, refreshes data past Soft TTL
- **Cache Cleanup**: Every 12 hours, deletes data past Hard TTL + VACUUM
- **Sticker Refresh**: Every 24 hours

**Data availability**:
- Student: 101-112 學年度 (≥113 shows deprecation notice)
- Course: Current + previous semester (auto-detect based on month)
- Contact: Real-time scraping

## Rate Limiting

**Scraper** (`internal/scraper/ratelimiter.go`): Token bucket (3 workers), 5-10s random delays, 120s timeout, max 3 retries with exponential backoff

**Webhook**: Per-user (10 req/s, burst 2), global (80 rps), silently drops excess requests

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
- **配色**: Hero 背景 `#1DB446` (NTPU 綠), 標題白色, Body 內容灰色
- **間距**: Hero padding `15px`, Body/Footer spacing `sm`, 按鈕高度 `sm`
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
lineutil.NewInfoRow("標籤", value).WithWrap(true).WithLineSpacing("4px")  // ✅ Full display
lineutil.TruncateRunes(value, 20)                                         // ❌ Hides information
```

## Testing

Table-driven tests with `t.Run()`, in-memory SQLite for DB tests (`storage.NewTestDB()`)

## Configuration

Env vars loaded at startup (`internal/config/`): ServerMode (requires LINE creds) or WarmupMode (scraper only). Platform-specific paths via `runtime.GOOS`.

## Task Commands

```powershell
task dev          # Run server
task warmup       # Manual warmup
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

**Logging**: `$env:LOG_LEVEL="debug"; task dev`

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

## Key File Locations

- **Entry points**: `cmd/server/main.go`, `cmd/warmup/main.go`, `cmd/healthcheck/main.go`
- **Warmup module**: `internal/warmup/warmup.go` (background cache warming)
- **Webhook router**: `internal/webhook/handler.go:handleMessageEvent()`
- **Bot module interface**: `internal/bot/handler.go`
- **DB schema**: `internal/storage/schema.go`
- **LINE utilities**: `internal/lineutil/builder.go` (use instead of raw SDK)

- **Sticker manager**: `internal/sticker/sticker.go` (avatar URLs for messages)
