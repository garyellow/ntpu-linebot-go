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
      Scraper Client (rate-limited, singleflight)
                ↓ (exponential backoff, failover URLs)
          NTPU Websites (lms/sea)
```

**Critical Flow Details:**
- **Context timeout**: All bot operations inherit 25s deadline from webhook (`internal/webhook/handler.go:214`)
- **Message batching**: LINE allows max 5 messages per reply; webhook auto-truncates to 4 + warning (`handler.go:159`)
- **Singleflight deduplication**: 10 concurrent queries for same data → 1 scrape execution (`internal/scraper/singleflight.go`)

## Bot Module Registration Pattern

**When adding new modules**, follow this registration sequence (breaks if skipped):

1. **Implement `bot.Handler` interface** (`internal/bot/handler.go`):
   ```go
   func (h *Handler) CanHandle(text string) bool {
       // Match keywords/patterns - called in registration order
   }
   ```

2. **Register in webhook constructor** (`internal/webhook/handler.go:40-42`):
   ```go
   idHandler := id.NewHandler(db, scraperClient, m, log, stickerManager)
   contactHandler := contact.NewHandler(...)
   courseHandler := course.NewHandler(...)
   ```

3. **Add to dispatcher** (`internal/webhook/handler.go:209-219`):
   ```go
   if h.idHandler.CanHandle(text) {
       return h.idHandler.HandleMessage(processCtx, text), nil
   }
   // Order matters: first match wins
   ```

4. **Postback routing** uses prefix convention (`handler.go:255-263`):
   ```go
   if strings.HasPrefix(data, "course:") {
       return h.courseHandler.HandlePostback(ctx, strings.TrimPrefix(data, "course:"))
   }
   ```

5. **Warmup module** (`internal/warmup/warmup.go`) handles background cache population automatically on server startup

## Data Layer: Cache-First Strategy

**SQLite as primary cache** (not ephemeral like Redis):
- **WAL mode** (`internal/storage/db.go:39`) - allows concurrent reads during writes
- **7-day TTL**: Configurable via CACHE_TTL env (default: 168h), checked in all repository methods
- **Busy timeout**: 5000ms (`db.go:44`) - waits for lock instead of failing

**Repository pattern** (`internal/storage/repository.go`):
```go
// Always check cache first
students := db.GetStudentsByName(name)
if len(students) > 0 && !expired(students[0].CachedAt) {
    return students // Cache hit
}
// Cache miss → trigger scraper
```

**Avoiding cache stampede** - use singleflight wrapper:
```go
wrapper := scraper.NewCacheWrapper()
result, err := wrapper.DoScrape(ctx, "key", func() (interface{}, error) {
    return ntpu.ScrapeStudents(ctx, client, year, dept)
})
// Concurrent calls to same key wait for single execution
```

## Rate Limiting: Two-Tier System

**Global scraper rate limit** (`internal/scraper/ratelimiter.go`):
- Token bucket: `workers` tokens (default: 3), refills at rate of workers/15.0 tokens/sec (~15s for full refill)
- Enforced in `RateLimiter.Wait(ctx)` - blocks until token available
- Random delays: 5s-10s (5000-10000ms) between requests by default (configurable via `SCRAPER_MIN_DELAY`/`SCRAPER_MAX_DELAY`)
- Timeout: 60s for all scraper operations (configurable via `SCRAPER_TIMEOUT`)

**Per-user webhook limit** (`internal/webhook/ratelimiter.go`):
```go
userLimiter := NewUserRateLimiter(5 * time.Minute) // Cleanup interval
if !h.userLimiter.Allow(chatID, 10.0, 2.0) {       // 10 req/s, burst 2
    // Drop request silently (LINE doesn't support 429 responses)
}
```

**Global webhook rate limit**: 80 rps (LINE API supports 100 rps, using 80 for safety margin)

**Exponential backoff** (`scraper/backoff.go`):
- Max 5 retries (configurable via `SCRAPER_MAX_RETRIES`)
- Backoff: 1s → 2s → 4s → 8s → 16s (base 1s, max 30s)
- Applies to HTTP errors + context cancellation

## LINE SDK Conventions

**Message builders** (`internal/lineutil/builder.go`) - always use these:
```go
lineutil.NewTextMessage(text)                              // Auto-truncates at 5000 chars
lineutil.NewTextMessageWithSender(text, name, iconURL)     // With avatar
lineutil.NewFlexMessage(altText, contents)                 // Flex Message for rich UI
lineutil.NewCarouselTemplate(altText, columns)             // Max 10 columns
lineutil.NewButtonsTemplate(altText, title, text, actions) // Max 4 actions
lineutil.NewQuickReply(items)                              // Max 13 items
```

**UX Best Practices**:
- Add **Quick Reply** to all messages for next-step guidance
- Show **Loading Animation** before long-running queries (webhook handles this)
- Use **Flex Messages** for rich card-based interfaces
- Provide **actionable options** in error messages

**Postback data format** (300 byte limit):
- Use module prefix: `"course:3141U0001"`, `"id:select_year_112"`
- Parsed in `handlePostbackEvent()` with `strings.HasPrefix()`

**Reply token gotcha**: Single-use only. If you call `ReplyMessage()` twice:
```
Error: "Invalid reply token"
```
Solution: Batch all messages into one `messages []MessageInterface` array.

## Testing: Table-Driven Pattern

**Standard test structure** (see `internal/scraper/singleflight_test.go:12`):
```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"case1", "input1", "expected1", false},
        {"case2", "input2", "expected2", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test logic using tt.input, tt.want, tt.wantErr
        })
    }
}
```

**Database testing**: Use in-memory SQLite (`internal/storage/db.go:99`):
```go
db, err := storage.NewTestDB() // Creates :memory: database with schema
defer db.Close()
```

## Configuration: Env Var → Struct

**All config loaded once** at startup (`internal/config/config.go`):
```go
// Server mode (requires LINE credentials)
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)  // Fail fast if required vars missing
}

// Warmup mode (skips LINE credentials validation)
cfg, err := config.LoadForMode(config.WarmupMode)
if err != nil {
    log.Fatal(err)
}
```

**Validation modes** (`config.ValidationMode`):
- `ServerMode`: Requires LINE_CHANNEL_ACCESS_TOKEN, LINE_CHANNEL_SECRET
- `WarmupMode`: Only requires scraper and database fields

**Platform detection**: `runtime.GOOS` for paths (`config.go`)
```go
if runtime.GOOS == "windows" {
    defaultPath = "./data/cache.db"
} else {
    defaultPath = "/data/cache.db"
}
```

## Task Commands (Taskfile.yml)

```powershell
task dev                    # Run server (hot reload)
task warmup                 # Pre-populate cache
task ci                     # Full CI: fmt + lint + test + build
task test:coverage          # Generate coverage.html
task compose:up             # Start with Prometheus + Grafana
task compose:logs -- <svc>  # View specific service logs
```

**Standalone warmup** (for testing/debugging):
```powershell
go run ./cmd/warmup -modules=id,contact,course -workers=10 -reset
```

**Production warmup** (automatic):
- Server runs `warmup.RunInBackground()` on startup
- Non-blocking: webhook accepts requests immediately
- Cache misses trigger on-demand scraping
- Modules: ID (264 tasks for years 101-112), Contact (admin + academic), Course (10 terms = 5 years), Sticker (avatars)
- Default: "id,contact,course,sticker"
- Same scraper settings as regular requests (5-10s delay, 60s timeout)

## Error Handling: Context + Wrapping

**Always wrap errors with context**:
```go
students, err := ntpu.ScrapeStudents(ctx, client, year, dept)
if err != nil {
    return fmt.Errorf("failed to scrape %d/%s: %w", year, dept, err)
}
```

**Structured logging** before returning errors:
```go
h.logger.WithError(err).
    WithField("year", year).
    WithField("dept", dept).
    Error("Scrape failed")
```

**User-facing error messages** (hide implementation details):
```go
return []messaging_api.MessageInterface{
    lineutil.ErrorMessage(err),  // Generic: "系統暫時無法處理您的請求"
}
```

## Scraper Failover URLs

**Multiple base URLs per domain** (`internal/scraper/client.go:36-48`):
```go
baseURLs := map[string][]string{
    "lms": {
        "http://120.126.197.52",      // IP fallback
        "https://120.126.197.52",     // HTTPS IP
        "https://lms.ntpu.edu.tw",    // Primary domain
    },
    "sea": {...},
}
```

**Failover logic**: Try URLs sequentially on HTTP 500+ errors (`client.go:131-152`)

## Debugging: Metrics + Logs

**Enable debug logging**:
```powershell
$env:LOG_LEVEL="debug"; task dev
```
Shows: SQL queries, scraper timing, cache hit/miss, rate limiter waits

**Prometheus metrics** (`http://localhost:10000/metrics`):
- `ntpu_cache_hits_total{module="id"}` - Cache hit count by module
- `ntpu_scraper_duration_seconds` - Histogram of scrape latencies
- `ntpu_webhook_requests_total{status="success"}` - Total webhook success/error

**Common queries**:
```promql
# Cache hit rate
sum(rate(ntpu_cache_hits_total[5m])) by (module)
/ (sum(rate(ntpu_cache_hits_total[5m])) + sum(rate(ntpu_cache_misses_total[5m]))) by (module)

# P95 webhook latency
histogram_quantile(0.95, sum(rate(ntpu_webhook_duration_seconds_bucket[5m])) by (le))
```

## Docker & Image Architecture

**Multi-stage build** (`Dockerfile`):
- Builder: golang:1.25-alpine with CGO_ENABLED=0 (static binaries)
- Runtime: gcr.io/distroless/static-debian12:nonroot (uid=65532)
- Binaries: `/app/ntpu-linebot`, `/app/ntpu-linebot-warmup`, `/app/healthcheck`

**Container startup flow** (`docker-compose.yml`):
1. `init-data` - Creates `/data` with uid=65532 ownership (alpine with shell)
2. `ntpu-linebot` - Main service starts and runs warmup in background automatically
3. Monitoring stack (prometheus/alertmanager/grafana)

**Healthcheck**:
- Binary: `cmd/healthcheck/main.go` (no wget/curl in distroless)
- Endpoint: `/healthz` (liveness) or `/ready` (readiness with dependency checks)
- Timeout: 8s client timeout < 10s Docker timeout

**Key constraints**:
- Distroless has no shell - use CMD form not CMD-SHELL
- nonroot user cannot create directories - use init-data container
- All paths use `/data` prefix in container (not `./data`)

## Key File Locations

- **Entry points**: `cmd/server/main.go`, `cmd/warmup/main.go`, `cmd/healthcheck/main.go`
- **Warmup module**: `internal/warmup/warmup.go` (background cache warming)
- **Webhook router**: `internal/webhook/handler.go:handleMessageEvent()`
- **Bot module interface**: `internal/bot/handler.go`
- **DB schema**: `internal/storage/schema.go`
- **LINE utilities**: `internal/lineutil/builder.go` (use instead of raw SDK)
- **Singleflight wrapper**: `internal/scraper/singleflight.go`
- **Sticker manager**: `internal/sticker/sticker.go` (avatar URLs for messages)

## Migration Notes (Python → Go)

This codebase migrated from Python (`migrate-from-python` branch):
- Python's asyncio → Go's goroutines with context cancellation
- Python dict cache → SQLite persistent cache with WAL mode
- Flask → Gin with middleware chaining
- Centralized config loading (no scattered `os.getenv()` calls)
