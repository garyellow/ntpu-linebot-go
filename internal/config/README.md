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
- `CACHE_TTL=168h` - Hard TTL：快取絕對過期時間（7 天）
- `SOFT_TTL=120h` - Soft TTL：觸發主動刷新時間（5 天）
- `SCRAPER_TIMEOUT=60s` - 每次 HTTP 請求超時
- `SCRAPER_MAX_RETRIES=5` - 失敗時最大重試次數（指數退避）
- `WARMUP_TIMEOUT=30m` - 預熱超時
- `WARMUP_MODULES=sticker,id,contact,course` - 預熱模組列表（逗號分隔，空字串跳過，**並行執行無順序限制**）
- `SHUTDOWN_TIMEOUT=30s` - 優雅關閉超時
- `USER_RATE_LIMIT_TOKENS=10` - 每個使用者的 Rate Limiter Token 數量
- `USER_RATE_LIMIT_REFILL_RATE=0.333...` - 每秒補充的 Token 數（預設 1/3，即每 3 秒補充 1 個 Token）

## 使用方式

```go
// 載入並驗證所有必填欄位（包含 LINE 憑證）
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}
```

## 驗證

`Load()` 會驗證以下必填欄位：
- LINE 憑證（`LINE_CHANNEL_ACCESS_TOKEN`, `LINE_CHANNEL_SECRET`）
- 伺服器設定（`PORT`）
- 資料庫設定（`SQLITE_PATH`）
- 超時和重試設定

完整環境變數列表請參考專案根目錄的 `.env.example`。
