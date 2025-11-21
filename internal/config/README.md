# Config

負責載入和驗證應用程式設定。

## 必要設定

- `LINE_CHANNEL_ACCESS_TOKEN` - LINE Messaging API Token
- `LINE_CHANNEL_SECRET` - LINE Channel Secret

## 可選設定

- `PORT` - HTTP 服務埠號（預設：10000）
- `LOG_LEVEL` - 日誌等級（預設：info）
- `SQLITE_PATH` - SQLite 資料庫路徑
- `CACHE_TTL` - 快取有效期限（預設：168h）
- `SCRAPER_WORKERS` - 爬蟲並發數（預設：5）
- `SCRAPER_TIMEOUT` - 爬蟲請求超時（預設：15s）
- `WARMUP_TIMEOUT` - 預熱超時時間（預設：20m）

## 使用方式

```go
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}

// 使用設定
log.Printf("Port: %s", cfg.Port)
```

## 設定來源優先順序

1. 系統環境變數（最高優先）
2. `.env` 檔案
3. 程式碼內預設值
