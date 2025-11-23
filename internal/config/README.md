# Config

載入和驗證應用程式設定。

## 環境變數

### 必填（Server Mode）
- `LINE_CHANNEL_ACCESS_TOKEN` - LINE Messaging API Token
- `LINE_CHANNEL_SECRET` - LINE Channel Secret

### 可選（含預設值）
- `PORT=10000` - HTTP 服務埠號
- `LOG_LEVEL=info` - 日誌等級（debug/info/warn/error）
- `SQLITE_PATH` - SQLite 路徑
  - Windows: `./data/cache.db`（預設）
  - Linux/Mac: `/data/cache.db`（預設）
- `CACHE_TTL=168h` - 快取有效期（7 天）
- `SCRAPER_WORKERS=3` - 爬蟲並發數
- `SCRAPER_MIN_DELAY=5s` - 爬蟲最小延遲
- `SCRAPER_MAX_DELAY=10s` - 爬蟲最大延遲
- `SCRAPER_TIMEOUT=60s` - 爬蟲請求超時
- `SCRAPER_MAX_RETRIES=5` - 最大重試次數
- `WARMUP_TIMEOUT=20m` - 預熱超時
- `WARMUP_MODULES=sticker,id,contact,course` - 預熱模組列表（逗號分隔，空字串跳過，**並行執行無順序限制**）
- `SHUTDOWN_TIMEOUT=30s` - 優雅關閉超時

## 使用方式

### Server Mode（需要 LINE 憑證）

```go
// 載入並驗證所有必填欄位（包含 LINE 憑證）
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}
```

### Warmup Mode（不需要 LINE 憑證）

```go
// 載入設定但跳過 LINE 憑證驗證（用於快取預熱工具）
cfg, err := config.LoadForMode(config.WarmupMode)
if err != nil {
    log.Fatal(err)
}
```

## 驗證模式

專案使用 `ValidationMode` 來支援不同執行環境：

- **ServerMode**: Webhook 伺服器，需要完整的 LINE 憑證
- **WarmupMode**: 快取預熱工具，只需要爬蟲和資料庫設定

這種模式化的設計遵循 Go 的最佳實踐：
- 單一載入邏輯，避免程式碼重複
- 明確的驗證需求，透過類型安全的 enum 控制
- Fail-fast 原則，在啟動時就發現設定錯誤

完整環境變數列表請參考專案根目錄的 `.env.example`。
