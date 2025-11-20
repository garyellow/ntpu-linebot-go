# Config Package

負責載入和驗證應用程式設定，從 `.env` 檔案和環境變數讀取，支援類型轉換和平台自適應。

## 設定項目

### 必要設定

| 變數名稱 | 類型 | 說明 | 範例 |
|---------|------|------|------|
| `LINE_CHANNEL_ACCESS_TOKEN` | string | LINE Messaging API 存取令牌 | `YOUR_CHANNEL_ACCESS_TOKEN` |
| `LINE_CHANNEL_SECRET` | string | LINE Channel Secret（用於簽章驗證） | `YOUR_CHANNEL_SECRET` |

### 可選設定

| 變數名稱 | 類型 | 預設值 | 說明 |
|---------|------|--------|------|
| `PORT` | string | `10000` | HTTP 伺服器監聽埠號 |
| `LOG_LEVEL` | string | `info` | 日誌等級 (debug/info/warn/error) |
| `SQLITE_PATH` | string | 平台相關* | SQLite 資料庫檔案路徑 |
| `CACHE_TTL` | duration | `168h` (7天) | 快取資料存活時間 |
| `SCRAPER_TIMEOUT` | duration | `30s` | 爬蟲請求逾時時間 |
| `SCRAPER_WORKERS` | int | `5` | 爬蟲並發工作者數量 |
| `SCRAPER_MIN_DELAY` | duration | `100ms` | 爬蟲請求最小間隔（防爬機制） |
| `SCRAPER_MAX_DELAY` | duration | `500ms` | 爬蟲請求最大間隔（隨機延遲上限） |
| `SCRAPER_MAX_RETRIES` | int | `3` | 爬蟲失敗最大重試次數 |
| `SHUTDOWN_TIMEOUT` | duration | `30s` | 優雅關閉逾時時間 |

\* SQLITE_PATH 預設值：
- Windows: `./data/cache.db`
- Linux/macOS: `/data/cache.db`

## 使用方式

### 基本用法

```go
package main

import (
    "log"
    "github.com/garyellow/ntpu-linebot-go/internal/config"
)

func main() {
    // 載入設定
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // 使用設定
    log.Printf("Server will listen on port %s", cfg.Port)
    log.Printf("Log level: %s", cfg.LogLevel)
    log.Printf("Cache TTL: %v", cfg.CacheTTL)
}
```

### 設定來源優先順序

1. 系統環境變數（最高優先）
2. `.env` 檔案
3. 程式碼內預設值（最低優先）

### 環境變數設定範例

**Linux/macOS (.env)**
```bash
LINE_CHANNEL_ACCESS_TOKEN=your_token_here
LINE_CHANNEL_SECRET=your_secret_here
PORT=8080
LOG_LEVEL=debug
SQLITE_PATH=/var/lib/ntpu-linebot/cache.db
CACHE_TTL=72h
SCRAPER_TIMEOUT=45s
SCRAPER_WORKERS=10
```

**Windows (PowerShell)**
```powershell
$env:LINE_CHANNEL_ACCESS_TOKEN="your_token_here"
$env:LINE_CHANNEL_SECRET="your_secret_here"
$env:PORT="8080"
$env:LOG_LEVEL="debug"
```

## 設定驗證

`config.Validate()` 會在載入時自動執行以下檢查：

1. ✅ 必要欄位是否存在（LINE_CHANNEL_ACCESS_TOKEN、LINE_CHANNEL_SECRET）
2. ✅ 數值範圍是否合理（例如：PORT 必須 > 0）
3. ✅ Duration 是否可解析（例如：`30s`、`5m`、`2h`）
4. ✅ 邏輯一致性（例如：MAX_DELAY 必須 >= MIN_DELAY）

驗證失敗會返回詳細錯誤訊息，遵循快速失敗原則。

## 最佳實踐

1. **不要硬編碼敏感資訊**: 永遠透過環境變數傳遞 token 和 secret
2. **使用 .env 檔案開發**: 本機開發時將非敏感設定放在 `.env` 中
3. **生產環境用環境變數**: 容器化部署時透過 Docker/K8s 環境變數注入
4. **快取 Config 實例**: `Load()` 應該只在程式啟動時呼叫一次
5. **合理設定逾時**: 根據網路狀況和伺服器效能調整 `SCRAPER_TIMEOUT`

## 錯誤處理

```go
cfg, err := config.Load()
if err != nil {
    // 無法載入設定是致命錯誤，應該終止程式
    log.Fatalf("Configuration error: %v", err)
}

// 設定已通過驗證，可以安全使用
```

## 擴充設定

若需新增設定項目：

1. 在 `Config` 結構體中新增欄位
2. 在 `Load()` 函式中新增 `getEnv*()` 呼叫
3. 在 `Validate()` 中新增驗證邏輯
4. 更新此 README 文件

範例：

```go
// config.go
type Config struct {
    // ... existing fields
    NewFeatureEnabled bool
}

func Load() (*Config, error) {
    // ... existing code
    cfg.NewFeatureEnabled = getEnvBool("NEW_FEATURE_ENABLED", false)
    // ...
}
```

## 除錯提示

若遇到設定問題：

1. 檢查 `.env` 檔案是否存在且格式正確（無 BOM、LF 換行）
2. 確認環境變數名稱拼寫正確（大小寫敏感）
3. 使用 `LOG_LEVEL=debug` 查看詳細載入過程
4. 檢查檔案權限（特別是 SQLITE_PATH 的寫入權限）
5. Docker 部署時確認 volume mount 是否正確

## 相關檔案

- `config.go`: 設定結構定義與載入邏輯
- `config_test.go`: 單元測試
- `.env.example`: 設定範本檔案（專案根目錄）
- `docs/API.md`: API 文件（包含環境變數說明）
