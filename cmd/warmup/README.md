# Warmup Tool - 手動快取預熱工具

> **生產環境不需要此工具** - Server 啟動時會自動在背景執行 warmup。

此工具主要用於：
- 🔧 開發/除錯：驗證爬蟲功能
- 🔄 手動維護：重置特定模組快取
- ⏰ 定期更新：Cron job 定期更新快取
- 🧪 測試環境：獨立測試 warmup 邏輯

## 快速使用

```bash
# 基本用法
go run ./cmd/warmup

# 重置所有快取
go run ./cmd/warmup -reset

# 只更新特定模組
go run ./cmd/warmup -modules=contact,course

# 使用更多 workers 加速
go run ./cmd/warmup -workers=10
```

## 參數說明

### `-modules` (預設: WARMUP_MODULES 環境變數，預設 "id,contact,course,sticker")

支援的模組：
- `id` - 101-112 學年 × 22 系所 = 264 任務
- `contact` - 行政 + 學術單位
- `course` - 3 學期課程（113-1, 113-2, 112-2）
- `sticker` - 頭像貼圖（Spy Family + Ichigo Production）

```bash
go run ./cmd/warmup -modules=id
go run ./cmd/warmup -modules=contact,course
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
| **ID** | 1-2 萬筆 | 系所代碼、101-112 學年學生 |
| **Contact** | 500-1000 筆 | 行政與學術單位聯絡資訊 |
| **Course** | 5000-1 萬筆 | 近 3 年課程（U/M/N/P 學制） |
| **總計** | **~2.4 萬筆** | |

## 使用建議

### 執行時機
- 推薦: 夜間或週末
- 中斷後可繼續，已快取資料不重複 (TTL 7 天)

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

### 部署流程
```bash
# 直接啟動服務（會自動在背景執行 warmup）
task dev
# 或 go run ./cmd/server

# 若需手動預熱（測試/除錯用）
go run ./cmd/warmup -reset
```

### 定期更新 (Cron)
```cron
# 每週一凌晨 3 點更新快取
0 3 * * 1 cd /path/to/ntpu-linebot-go && go run ./cmd/warmup -reset
```

### Docker Compose
Server 啟動時會自動在背景執行 warmup，不需手動執行此工具。

## 環境變數

```bash
LOG_LEVEL=debug                       # 詳細日誌
SQLITE_PATH=/tmp/cache.db             # 資料庫路徑
SCRAPER_WORKERS=10                    # Worker 數
WARMUP_MODULES=id,contact,course,sticker  # 預設模組
WARMUP_TIMEOUT=30m                    # 超時時間
```
