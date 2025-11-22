# Config

載入和驗證應用程式設定。

## 環境變數

### 必填
- `LINE_CHANNEL_ACCESS_TOKEN` - LINE Messaging API Token
- `LINE_CHANNEL_SECRET` - LINE Channel Secret

### 可選（含預設值）
- `PORT=10000` - HTTP 服務埠號
- `LOG_LEVEL=info` - 日誌等級（debug/info/warn/error）
- `SQLITE_PATH` - SQLite 路徑
  - Windows: `./data/cache.db`（預設）
  - Linux/Mac: `/data/cache.db`（預設）
- `CACHE_TTL=168h` - 快取有效期（7 天）
- `SCRAPER_WORKERS=5` - 爬蟲並發數
- `SCRAPER_MIN_DELAY=100ms` - 爬蟲最小延遲
- `SCRAPER_MAX_DELAY=500ms` - 爬蟲最大延遲
- `SCRAPER_TIMEOUT=15s` - 爬蟲請求超時
- `SCRAPER_MAX_RETRIES=3` - 最大重試次數
- `WARMUP_TIMEOUT=20m` - 預熱超時
- `SHUTDOWN_TIMEOUT=30s` - 優雅關閉超時

## 使用方式

```go
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}
```

完整環境變數列表請參考專案根目錄的 `.env.example`。
