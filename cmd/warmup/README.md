# Warmup Tool - 快取預熱工具

預先從 NTPU 網站抓取資料並存入 SQLite 快取，提升首次回應速度，減少對上游網站的負擔。

## 快速使用

```bash
# 使用 Task (推薦)
task warmup

# 或直接執行
go run ./cmd/warmup

# 只預熱特定模組
go run ./cmd/warmup -modules=id,contact

# 重置快取後預熱
go run ./cmd/warmup -reset

# 自訂 Worker 數量
go run ./cmd/warmup -workers=10
```

## 參數說明

### `-modules` (預設: "id,contact,course")
指定要預熱的模組（逗號分隔）：
- `id` - 學號資料（系所代碼、近 4 年學生）
- `contact` - 通訊錄（行政與學術單位聯絡資訊）
- `course` - 課程資料（近 3 年課程）

範例：
```bash
go run ./cmd/warmup -modules=id              # 只預熱學號
go run ./cmd/warmup -modules=contact,course  # 預熱聯絡與課程
```

### `-reset` (預設: false)
預熱前刪除所有快取。用於更新過期資料或修復損壞的快取。

範例：
```bash
go run ./cmd/warmup -reset
```

### `-workers` (預設: 0 = 使用設定檔)
並發爬蟲數量。數值越高速度越快，但對 NTPU 伺服器負擔越大。

建議值：
- `3-5` - 保守，尊重限流（推薦）
- `8-10` - 平衡，適合離峰時段
- `0` - 使用設定檔預設值（3 workers）

範例：
```bash
go run ./cmd/warmup -workers=8
```

## 快取內容

| 模組 | 資料量 | 說明 |
|------|--------|------|
| **ID** | 1-2 萬筆 | 系所代碼、近 4 年學生（110-113 學年） |
| **Contact** | 500-1000 筆 | 行政與學術單位聯絡資訊 |
| **Course** | 5000-1 萬筆 | 近 3 年課程（U/M/N/P 學制） |
| **總計** | **~2.4 萬筆** | |

## 使用建議

### 執行時機
- ✅ **推薦**: 夜間 2-6 點或週末
- ⚠️ **避免**: 平日上班時間（9-17 點）

### 中斷處理
若預熱中斷（Ctrl+C）：
- 已儲存的資料會保留
- 可直接重新執行，不需 `-reset`
- 已快取資料不會重複爬取（TTL 7 天）

### 驗證快取
```bash
# Windows (需安裝 sqlite3)
sqlite3 .\data\cache.db "SELECT COUNT(*) FROM students;"

# Linux / Mac
sqlite3 data/cache.db "SELECT COUNT(*) FROM students;"
sqlite3 data/cache.db "SELECT COUNT(*) FROM contacts;"
sqlite3 data/cache.db "SELECT COUNT(*) FROM courses;"
```

## 常見情境

```bash
# 首次部署
go run ./cmd/warmup -reset

# 每週更新（TTL 7 天）
go run ./cmd/warmup -reset

# 修復損壞資料
go run ./cmd/warmup -reset -modules=id

# 僅更新聯絡資訊
go run ./cmd/warmup -modules=contact
```

## 疑難排解

| 問題 | 原因 | 解決方法 |
|------|------|----------|
| 爬蟲失敗 | NTPU 網站無法連線或限流 | 降低 workers (`-workers=1`)、稍後重試 |
| 預熱過慢 | Worker 太少或網路延遲 | 增加 workers (`-workers=8`)、離峰執行 |
| Database locked | 服務正在使用資料庫 | 停止服務後再執行 warmup |
| 記憶體不足 | 並發數過高 | 降低 workers (`-workers=3`) |

## 部署整合

### 推薦部署流程
```bash
# 1. 預熱快取
task warmup
# 或 go run ./cmd/warmup -reset

# 2. 啟動服務
task dev
# 或 go run ./cmd/server
```

### 定期更新 (Cron)
```cron
# 每週一凌晨 3 點更新快取
0 3 * * 1 cd /path/to/ntpu-linebot-go && go run ./cmd/warmup -reset
```

### Docker Compose
Docker Compose 部署會自動執行 warmup（見 `deployments/docker-compose.yml`）。

## 進階用法

### 自訂資料庫路徑
```bash
go run ./cmd/warmup -sqlite-path=/custom/path/cache.db
```

### 環境變數
```bash
export LOG_LEVEL=debug           # 詳細日誌
export SQLITE_PATH=/tmp/cache.db
export SCRAPER_WORKERS=10

go run ./cmd/warmup
```

## 相關文件
- [爬蟲系統](../../internal/scraper/README.md)
- [資料庫結構](../../internal/storage/README.md)
- [設定說明](../../internal/config/README.md)
