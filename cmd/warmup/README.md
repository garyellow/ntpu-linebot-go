# Warmup Tool

## Overview
The warmup tool pre-populates the SQLite cache with data from NTPU websites, improving initial response times and reducing load on upstream services.

## Quick Start

```bash
# Basic usage - warmup all modules
go run ./cmd/warmup

# Or using Task runner
task warmup

# Warmup specific modules only
go run ./cmd/warmup -modules=id,course

# Reset cache and warmup
go run ./cmd/warmup -reset

# Custom worker pool size
go run ./cmd/warmup -workers=10
```

## Command-Line Options

### `-modules` (default: "id,contact,course")
Comma-separated list of modules to warmup:
- `id` - Student ID data (department mappings, recent year students)
- `contact` - Contact directory (administrative and academic units)
- `course` - Course information (recent year courses)

**Example:**
```bash
# Only warmup student data
go run ./cmd/warmup -modules=id

# Warmup contact and course data
go run ./cmd/warmup -modules=contact,course
```

### `-reset` (default: false)
Delete all existing cache data before warmup. Use this to refresh stale data or fix corrupted cache.

**Example:**
```bash
# Fresh start with empty cache
go run ./cmd/warmup -reset
```

### `-workers` (default: 0 = use config)
Number of concurrent workers for scraping. Higher values = faster warmup but more load on NTPU servers.

**Recommended values:**
- `5-10` - Conservative, respects rate limits
- `15-20` - Aggressive, use during off-peak hours only
- `0` - Use default from config (recommended)

**Example:**
```bash
# Use 8 workers for faster warmup
go run ./cmd/warmup -workers=8
```

## What Gets Cached?

### ID Module (~3-5 minutes)
- Department code mappings (all departments)
- Student data for recent 5 years (year 108-112)
- ~10,000-20,000 student records

**Data sources:**
- Department page: `https://lms.ntpu.edu.tw/portfolio/search.php`
- Student search by year/dept

### Contact Module (~2-3 minutes)
- Administrative units (行政單位)
- Academic units (學術單位)
- Individual contacts with emails/phones
- ~500-1000 contact records

**Data sources:**
- Directory: `https://sea.cc.ntpu.edu.tw/pls/ld/CAMPUS_DIR_M.p1`

### Course Module (~5-10 minutes)
- Course data for recent 3 years
- All education codes (U/M/N/P types)
- Teacher assignments
- ~5,000-10,000 course records

**Data sources:**
- Course query: `https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.queryByKeyword`

## Performance Tips

### 1. Run During Low-Traffic Hours
Best time to run warmup:
- **Recommended**: 2 AM - 6 AM Taiwan time (夜間 2-6 點)
- **Acceptable**: Weekends, holidays
- **Avoid**: Weekday 9 AM - 5 PM (peak hours)

### 2. Monitor Progress
The tool shows real-time progress:
```
[ID] Warmup starting...
[ID] Year 112: Processing department 85 (資工系)...
[ID] Year 112: Saved 150 students
[ID] Year 111: Processing department 87 (電機系)...
...
[ID] Warmup completed: 12,543 students cached
```

### 3. Handle Interruptions
If warmup is interrupted (Ctrl+C):
- Partial data is already saved to database
- Safe to restart without `-reset`
- Already cached data won't be re-scraped (7-day TTL)

### 4. Verify Cache
After warmup, check cache statistics:
```bash
# Count cached records
sqlite3 data/cache.db "SELECT COUNT(*) FROM students;"
sqlite3 data/cache.db "SELECT COUNT(*) FROM contacts;"
sqlite3 data/cache.db "SELECT COUNT(*) FROM courses;"

# Check TTL expiration
sqlite3 data/cache.db "SELECT COUNT(*) FROM students WHERE cached_at + 604800 > strftime('%s', 'now');"
```

## Common Scenarios

### Scenario 1: Initial Deployment
```bash
# First-time setup (no LINE credentials needed)
go run ./cmd/warmup -reset -modules=id,contact,course -workers=10
```

### Scenario 2: Weekly Refresh
```bash
# Refresh all data (TTL = 7 days)
go run ./cmd/warmup -reset
```

### Scenario 3: Fix Corrupt Data
```bash
# Reset and re-warmup
go run ./cmd/warmup -reset -modules=id
```

### Scenario 4: Update Contact Info Only
```bash
# Quick contact update
go run ./cmd/warmup -modules=contact
```

## Troubleshooting

### Problem: "Failed to scrape" errors
**Cause**: NTPU website unavailable or rate limit hit

**Solutions:**
1. Reduce workers: `-workers=3`
2. Try alternative time
3. Check network connectivity
4. Verify NTPU website is accessible

### Problem: Warmup too slow
**Cause**: Too few workers or network latency

**Solutions:**
1. Increase workers: `-workers=15`
2. Increase timeout: `WARMUP_TIMEOUT=30m` (default is 20m)
3. Run during off-peak hours
4. Check internet speed

### Problem: Database locked
**Cause**: Another process (server) is using the database

**Solutions:**
1. Stop server first: `pkill -f ntpu-linebot`
2. Or use separate database: `-sqlite-path=/tmp/warmup.db`

### Problem: Out of memory
**Cause**: Too many concurrent operations

**Solutions:**
1. Reduce workers: `-workers=5`
2. Warmup one module at a time: `-modules=id`

## Integration with Server

### Auto-warmup on startup
The server does NOT auto-warmup on startup (by design). This prevents slow startup times.

**Recommended deployment flow:**
```bash
# 1. Run warmup before deployment
go run ./cmd/warmup -reset

# 2. Start server with pre-warmed cache
go run ./cmd/server
```

### Cron job for periodic refresh
Set up weekly cache refresh:
```cron
# Every Monday at 3 AM, refresh cache
0 3 * * 1 cd /path/to/ntpu-linebot-go && go run ./cmd/warmup -reset
```

## Advanced Usage

### Custom SQLite Path
```bash
# Use non-default database location
go run ./cmd/warmup -sqlite-path=/custom/path/cache.db
```

### Environment Variables
The warmup tool respects these environment variables:
```bash
export LOG_LEVEL=debug        # Enable verbose logging
export SQLITE_PATH=/tmp/cache.db
export SCRAPER_WORKERS=10

go run ./cmd/warmup
```

### Parallel Multi-Module Warmup
```bash
# Run multiple warmups in parallel (use with caution!)
go run ./cmd/warmup -modules=id & \
go run ./cmd/warmup -modules=contact & \
go run ./cmd/warmup -modules=course & \
wait
```

## Performance Benchmarks

**Test environment:** Intel i7, 50 Mbps connection, -workers=10

| Module | Records | Time | Rate |
|--------|---------|------|------|
| ID | 15,000 students | 4m 30s | 55/sec |
| Contact | 800 contacts | 2m 15s | 6/sec |
| Course | 8,000 courses | 8m 00s | 17/sec |
| **Total** | **23,800** | **~15 min** | **26/sec** |

**With -workers=20:**
- Total time reduced to ~8 minutes
- Risk of rate limiting increases

**Note:** Default `WARMUP_TIMEOUT` is set to 20 minutes to accommodate varying network conditions and NTPU website load.

## Best Practices

1. **Always test in staging first** before production warmup
2. **Monitor NTPU website load** - be a good citizen
3. **Schedule regular refreshes** - weekly or bi-weekly
4. **Keep warmup logs** for debugging
5. **Verify data integrity** after warmup
6. **Use -reset sparingly** - only when data is corrupt
7. **Start with low workers** then increase if needed

## Related Documentation
- [Scraper Rate Limiting](../../internal/scraper/README.md)
- [Database Schema](../../internal/storage/README.md)
- [Configuration Guide](../../internal/config/README.md)
