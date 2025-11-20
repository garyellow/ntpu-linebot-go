# NTPU LineBot Go - AI Agent Instructions

This is a LINE chatbot for NTPU (National Taipei University) that provides student ID lookup, contact directory, and course queries. Built with Go, it emphasizes anti-scraping measures, caching, and observability.

## Architecture Overview

**3-Layer Architecture:**
1. **Webhook Layer** (`internal/webhook/`) - Gin HTTP server receives LINE webhook events, validates signatures, dispatches to bot modules
2. **Bot Module Layer** (`internal/bot/{id,contact,course}/`) - Each module implements `Handler` interface with `CanHandle()`, `HandleMessage()`, `HandlePostback()` methods
3. **Data Layer** - Repository pattern with cache-first strategy: SQLite cache (7-day TTL) → Web scraper (rate-limited) → NTPU websites

**Key Design Patterns:**
- **Singleflight**: `internal/scraper/singleflight.go` deduplicates concurrent requests for same data (e.g., 10 users query same student ID → only 1 scrape)
- **Rate Limiting**: Two-tier system - token bucket for scrapers (`internal/scraper/ratelimiter.go`) + per-user webhook limiter (`internal/webhook/ratelimiter.go`)
- **Graceful Degradation**: Scraper has exponential backoff with max 3 retries, random delays (100-500ms), and User-Agent rotation

## Development Workflow

### Local Development
```powershell
# Use Task runner (preferred) - see Taskfile.yml
task dev              # Run server with hot reload
task warmup          # Pre-populate cache (3-5 min for all modules)
task test            # Run all tests
task ci              # Full CI: fmt + lint + test + build

# Direct go commands (fallback)
go run ./cmd/server
go run ./cmd/warmup --modules=id,contact,course --reset
```

### Testing Conventions
- Use table-driven tests (see `internal/scraper/singleflight_test.go`)
- Test files live alongside implementation (`handler.go` → `handler_test.go`)
- Use `context.Background()` for tests, `context.WithTimeout()` for production
- Mock external dependencies (LINE API, HTTP requests)
- Target 80%+ coverage (`task test:coverage`)

### Adding a New Bot Module
1. **Create handler**: `internal/bot/newmodule/handler.go`
   ```go
   type Handler struct {
       db             *storage.DB
       scraper        *scraper.Client
       metrics        *metrics.Metrics
       logger         *logger.Logger
       stickerManager *sticker.Manager // For message avatars
   }

   func (h *Handler) CanHandle(text string) bool {
       // Check keywords/patterns using regex
       // See existing modules for pattern examples
   }

   func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
       // Process query, return LINE messages
       // Use lineutil helpers for consistent message formatting
   }

   func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
       // Handle button clicks from carousel/quick reply
       // Postback data should use prefix routing: "newmodule:param"
   }
   ```

2. **Register in webhook**: `internal/webhook/handler.go` - add to `NewHandler()` constructor and event dispatcher
3. **Add warmup logic**: `cmd/warmup/main.go` - add module to `--modules` flag handler
4. **Database schema**: Add tables in `internal/storage/schema.go`, repository methods in `internal/storage/repository.go`
5. **Update tests**: Create `handler_test.go` with table-driven tests for all public methods

## Critical Conventions

### Error Handling
- **Always wrap errors**: `fmt.Errorf("failed to X: %w", err)` for stack traces
- **Log before returning**: Use `logger.WithError(err).Error("context")`
- **User-facing errors**: Use `lineutil` helpers like `lineutil.ErrorMessage("❌ Title", "details")`

### Database Patterns
- **Cache TTL**: 7 days hardcoded (`168h`), checked with `cached_at + 604800 <= now()`
- **SQLite pragmas**: WAL mode, busy_timeout=5000ms (see `internal/storage/db.go`)
- **NULL handling**: Use `sql.NullString` for optional fields, `nullString()` helper for inserts
- **Search sanitization**: Always call `sanitizeSearchTerm()` before LIKE queries to escape `%`, `_`, `\`

### LINE Message Limits (Enforced in Code)
- Max 5 messages per reply (webhook truncates to 4 + warning message)
- Text message: 5000 chars (auto-truncated in `lineutil.NewTextMessage()`)
- Carousel: 10 columns max, 4 actions per column
- Postback data: 300 bytes max

### Prometheus Metrics
- **Naming**: `ntpu_{component}_{metric}_{unit}` e.g., `ntpu_scraper_duration_seconds`
- **Labels**: Always include `module` (id/contact/course), `status` (success/error)
- **Instrument**: Call `metrics.RecordWebhook()`, `metrics.RecordScrape()` at function end
- **Custom metrics**: Register in `internal/metrics/metrics.go` constructor

### Configuration
- **All config via env vars**: See `internal/config/config.go`
- **Required vars**: `LINE_CHANNEL_ACCESS_TOKEN`, `LINE_CHANNEL_SECRET`
- **Validation**: `config.Validate()` fails fast on missing required fields
- **Platform-specific defaults**: Use `runtime.GOOS` for paths (e.g., Windows `./data/cache.db` vs Linux `/data/cache.db`)

## Common Gotchas

1. **LINE Reply Token**: Single-use only. Attempting to reply twice causes `Invalid reply token` error. Solution: Batch messages into single `ReplyMessage()` call.

2. **Context Timeouts**: Webhook processing has 25s timeout (derived from request context). Long operations need background goroutines or increase timeout.

3. **SQLite Locking**: Only one writer at a time. Use WAL mode (enabled by default) and avoid long transactions. If `database is locked`, check for concurrent writes.

4. **Scraper Race Conditions**: Always use `CacheWrapper.DoScrape()` to leverage singleflight. Direct HTTP calls bypass deduplication.

5. **Postback Data Format**: Use prefix routing (`id:`, `contact:`, `course:`) for multi-module disambiguation. Example: `course:3141U0001` routes to course handler with `3141U0001` parameter.

## Quick Reference

### Key File Locations
- **Entry points**: `cmd/server/main.go`, `cmd/warmup/main.go`, `cmd/verify/main.go`
- **Webhook dispatcher**: `internal/webhook/handler.go:handleMessageEvent()`
- **Scraper URLs**: `internal/scraper/client.go` - failover list per target
- **DB schema**: `internal/storage/schema.go`
- **LINE message builders**: `internal/lineutil/builder.go` - use these instead of raw SDK
- **Sticker management**: `internal/sticker/sticker.go` - handles avatar URLs for messages

### Useful Commands
```powershell
# Build for production (static binary)
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/ntpu-linebot ./cmd/server

# Run with custom config
$env:LOG_LEVEL="debug"; $env:PORT="8080"; task dev

# Test specific package
go test -v ./internal/bot/id

# Docker Compose with all monitoring
docker-compose -f docker/docker-compose.yml up -d
```

### Dependencies Rationale
- **modernc.org/sqlite**: Pure Go SQLite (no CGo) for cross-compilation, slightly slower than mattn/go-sqlite3
- **github.com/corpix/uarand**: User-Agent rotation to avoid scraper detection
- **github.com/line/line-bot-sdk-go/v8**: Official LINE SDK, v8 has improved type safety
- **github.com/gin-gonic/gin**: HTTP framework, chosen for middleware ecosystem and performance

## Debugging Tips

- **Enable debug logs**: `$env:LOG_LEVEL="debug"; task dev` - shows all SQL queries, scraper timing
- **Check metrics**: `http://localhost:10000/metrics` - see cache hit rates, error counts
- **Readiness probe**: `http://localhost:10000/ready` - validates DB, scraper URLs, cache stats
- **Database inspection**: Use `sqlite3 data/cache.db` CLI or `task ci` to run tests which print row counts

## Migration Context

This codebase was migrated from Python (`migrate-from-python` branch). Key changes:
- Python's asyncio → Go's goroutines with context cancellation
- Python dict cache → SQLite persistent cache
- Flask → Gin for HTTP routing
- Centralized config loading (no more scattered env access)
