# Sticker Module

提供隨機貼圖 URL 給 LINE 訊息使用，從 Spy x Family 和 Ichigo Production 網站爬取，快取 7 天。

## Architecture

### Data Flow
```
Warmup Tool → Web Scraping → SQLite Cache (7-day TTL)
                                    ↓
Server Startup → LoadStickers() → In-Memory Array
                                    ↓
User Request → GetRandomSticker() → Random Selection
```

### Cache Strategy
- **Cache First**: Always check SQLite before web scraping
- **TTL**: 7 days (168 hours)
- **Fallback**: If all web sources fail, generate 20 UI avatar stickers
- **Concurrency**: All 8 sources scraped in parallel with retry logic

### Web Sources
**Spy x Family** (7 pages):
- `https://spy-family.net/tvseries/special/special1_season1.php`
- `https://spy-family.net/tvseries/special/special2_season1.php`
- `https://spy-family.net/tvseries/special/special9_season1.php`
- `https://spy-family.net/tvseries/special/special13_season1.php`
- `https://spy-family.net/tvseries/special/special16_season1.php`
- `https://spy-family.net/tvseries/special/special17_season1.php`
- `https://spy-family.net/tvseries/special/special3.php`

**Ichigo Production** (1 page):
- `https://ichigoproduction.com/special/present_icon.html`

Expected yield: ~100-200 unique sticker URLs

## Usage

### Server Initialization
```go
// 1. Create manager
manager := sticker.NewManager(db, scraperClient)

// 2. Load stickers (cache-first, falls back to web scraping if cache empty/expired)
if err := manager.LoadStickers(ctx); err != nil {
    log.Fatalf("Failed to load stickers: %v", err)
}

// 3. Get random sticker
stickerURL := manager.GetRandomSticker()
```

### Warmup Tool Integration
```bash
# Pre-populate sticker cache (recommended before production deployment)
go run ./cmd/warmup --reset

# This will:
# 1. Delete old sticker cache
# 2. Scrape all 8 web sources concurrently
# 3. Save ~100-200 stickers to SQLite
# 4. Takes ~10-30 seconds depending on network
```

### Cache Validation
```bash
# Check sticker count in database
sqlite3 data/cache.db "SELECT COUNT(*) FROM stickers;"

# Check TTL expiration
sqlite3 data/cache.db "SELECT COUNT(*) FROM stickers WHERE cached_at + 604800 > strftime('%s', 'now');"
```

## Implementation Details

### Manager Structure
```go
type Manager struct {
    stickers []string      // In-memory cache
    mu       sync.RWMutex  // Thread-safe access
    loaded   bool          // Initialization flag
    db       *storage.DB   // SQLite connection
    client   *scraper.Client // HTTP client with rate limiting
}
```

### Database Schema
```sql
CREATE TABLE IF NOT EXISTS stickers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT UNIQUE NOT NULL,           -- Sticker image URL
    source TEXT NOT NULL,                -- 'spy_family', 'ichigo', or 'fallback'
    cached_at INTEGER NOT NULL,          -- Unix timestamp
    success_count INTEGER DEFAULT 0,     -- Usage tracking
    failure_count INTEGER DEFAULT 0      -- Error tracking
);
```

### Scraping Logic
```go
// Concurrent fetching with retry (max 3 attempts per source)
func (m *Manager) FetchAndSaveStickers(ctx context.Context) error {
    // 1. Launch 8 goroutines (7 Spy Family + 1 Ichigo)
    // 2. Each goroutine retries up to 3 times with exponential backoff
    // 3. Parse HTML using goquery to extract <img> tags
    // 4. Save each sticker URL to SQLite
    // 5. If all sources fail, generate 20 fallback stickers using ui-avatars.com
}
```

### Retry Logic
- **Max Retries**: 3 attempts per source
- **Backoff**: Exponential (1s, 2s, 4s)
- **Timeout**: Inherits from scraper client (default 30s)

## Testing

### Run Tests
```bash
# Run sticker tests (WARNING: Takes 90+ seconds due to web scraping)
go test ./internal/sticker

# Skip long-running tests
go test -short ./internal/sticker

# With coverage
go test -coverprofile=coverage.out ./internal/sticker
go tool cover -html=coverage.out
```

### Test Coverage
Current: ~73% coverage (284/338 lines)
- Main bottleneck: 90-second web scraping tests
- Coverage includes: LoadStickers, FetchAndSaveStickers, retry logic, fallback generation

## Best Practices

### Production Deployment
```bash
# 1. Run warmup before deploying server
go run ./cmd/warmup --reset

# 2. Verify sticker cache populated
sqlite3 data/cache.db "SELECT COUNT(*) FROM stickers;"
# Expected: 100-200 stickers

# 3. Start server (will use cached stickers, <1s initialization)
go run ./cmd/server
```

### Periodic Refresh
Set up weekly cron job to refresh stickers:
```cron
# Every Monday at 3 AM, refresh sticker cache
0 3 * * 1 cd /path/to/ntpu-linebot-go && go run ./cmd/warmup --reset
```

### Monitoring
Check sticker health:
```sql
-- Stickers expiring soon (< 24 hours)
SELECT COUNT(*) FROM stickers
WHERE cached_at + 604800 - strftime('%s', 'now') < 86400;

-- Stickers by source
SELECT source, COUNT(*) FROM stickers GROUP BY source;

-- Most used stickers
SELECT url, success_count FROM stickers ORDER BY success_count DESC LIMIT 10;
```

## Troubleshooting

### Problem: Server startup takes 90+ seconds
**Cause**: Sticker cache empty, triggering web scraping on first LoadStickers()

**Solution**: Run warmup tool before server startup
```bash
go run ./cmd/warmup --reset
```

### Problem: Only 20 fallback stickers loaded
**Cause**: All 8 web sources failed (network issue or website down)

**Solution**:
1. Check network connectivity
2. Verify websites accessible: `curl -I https://spy-family.net/tvseries/special/special1_season1.php`
3. Re-run warmup during better network conditions

### Problem: Stickers expired but not refreshing
**Cause**: Server only checks cache on startup, doesn't auto-refresh

**Solution**: Restart server or run warmup tool to force refresh
```bash
# Option 1: Restart server (loads new stickers)
pkill -f ntpu-linebot; go run ./cmd/server

# Option 2: Update cache while server running (requires server restart to load)
go run ./cmd/warmup --reset
```

### Problem: Test suite takes 90+ seconds
**Cause**: Tests scrape real websites

**Solution**: Use `-short` flag to skip long-running tests
```bash
go test -short ./internal/sticker
```

## Performance Characteristics

| Metric | With Warmup | Without Warmup (Cold Start) |
|--------|-------------|----------------------------|
| **Server Startup** | <1 second | 90+ seconds |
| **Cache Hit** | Instant (in-memory) | N/A |
| **Cache Miss** | 10-30 seconds (web scraping) | 10-30 seconds |
| **Sticker Variety** | 100-200 unique URLs | 100-200 unique URLs |

**Recommendation**: Always run warmup tool before production deployment to ensure <1s startup.
