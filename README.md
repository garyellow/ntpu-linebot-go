# NTPU Line Bot (Go)

[![CI](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/garyellow/ntpu-linebot-go)](https://goreportcard.com/report/github.com/garyellow/ntpu-linebot-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

國立臺北大學 LINE 聊天機器人的 Go 語言實作版本,提供學號查詢、通訊錄查詢、課程查詢等功能。

## ✨ 功能特色

### 核心功能
- 🔍 **學號查詢**: 依姓名或學號查詢學生資訊，支援系代碼查詢與年度篩選
- 📞 **通訊錄查詢**: 校內人員聯絡方式（分機、Email、辦公室位置）
- 📚 **課程查詢**: 課程資訊查詢（課號、教師、時間、地點、大綱）
- 🆘 **緊急電話**: 三峽/臺北校區緊急聯絡電話

### 技術特色
- 💾 **智慧快取**: SQLite + 7天 TTL，Cache-First 策略，快取命中率 >90%
- 🛡️ **防爬蟲機制**:
  - Singleflight 去重（10個請求合併為1次爬蟲）
  - Token Bucket 限流（5 req/s）
  - 隨機延遲（100-500ms）
  - 指數退避重試（1s/2s/4s，最多3次）
  - User-Agent 輪替
- 📊 **完整監控**:
  - Prometheus 指標收集
  - Grafana 視覺化儀表板
  - 告警規則（高失敗率、高延遲、服務停止）
- 📋 **結構化日誌**: JSON 格式，便於 ELK/Loki 集中分析
- 🚀 **高效能**:
  - 使用 Go 1.25 並發特性
  - Worker Pool 控制並發數
  - Context 超時控制（25s）
  - 平均回應時間 < 500ms

## 📋 前置需求

- **Go 1.25+**: 用於本機開發
- **Docker & Docker Compose**: 用於容器化部署
- **LINE Bot Credentials**: 需要 Channel Access Token 與 Channel Secret

### 取得 LINE Bot Credentials

1. 前往 [LINE Developers Console](https://developers.line.biz/console/)
2. 建立 Messaging API Channel
3. 取得 **Channel Secret** (Basic settings)
4. 發行 **Channel Access Token** (Messaging API settings)

## 🚀 快速開始

### 使用 Docker Compose (推薦)

```bash
# 1. Clone 專案
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go

# 2. 設定環境變數
cp .env.example .env
# 編輯 .env 填入 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET

# 3. 啟動所有服務
docker-compose -f docker/docker-compose.yml up -d

# 4. 查看日誌
docker-compose -f docker/docker-compose.yml logs -f ntpu-linebot
```

服務啟動後:
- LINE Bot Webhook: `http://localhost:10000/callback`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (帳號: admin / 密碼: admin123)

### 本機開發

```bash
# 1. 安裝依賴
go mod download

# 2. 設定環境變數
cp .env.example .env
# 編輯 .env 填入必要變數

# 3. 資料預熱 (首次執行)
task warmup
# 或
go run ./cmd/warmup --modules=id,contact,course

# 4. 執行服務
task dev
# 或
go run ./cmd/server
```

## 🏗️ 架構設計

### 三層架構

```
LINE Platform (Webhook)
         ↓
[Webhook Layer] - Gin HTTP Server
  • 簽章驗證 (X-Line-Signature)
  • Rate Limiting (80 rps global + 10 rps/user)
  • Context Timeout (25s)
         ↓
[Bot Module Layer] - Handler Interface
  • ID Module (學號查詢)
  • Contact Module (通訊錄)
  • Course Module (課程)
         ↓
[Repository Layer] - Cache-First Strategy
  • SQLite Cache (7-day TTL)
  • Singleflight 去重
         ↓
[Scraper Layer] - Rate-Limited HTTP Client
  • Token Bucket (5 req/s)
  • Exponential Backoff
  • User-Agent Rotation
         ↓
NTPU Websites (LMS / SEA)
```

### 關鍵設計模式

1. **Cache-First 策略**: 優先查詢快取，Miss 時觸發爬蟲
2. **Singleflight 模式**: 10個用戶同時查詢→只爬1次
3. **Repository 模式**: 資料存取邏輯與業務邏輯分離
4. **Worker Pool**: 限制並發數避免資源耗盡

詳細架構說明請見 [docs/architecture.md](docs/architecture.md)

---

### 完整架構圖

```
┌─────────────────────────────────────────┐
│  Gin HTTP Server (port 10000)          │
├─────────────────────────────────────────┤
│  ┌────────────┐  ┌──────────────────┐  │
│  │  Webhook   │  │  Metrics         │  │
│  │  Handler   │  │  /metrics        │  │
│  └─────┬──────┘  └──────────────────┘  │
│        │                                 │
│        ▼                                 │
│  ┌─────────────────────────────────┐   │
│  │  Bot Module Dispatcher          │   │
│  │  ┌──────┐ ┌────────┐ ┌────────┐│   │
│  │  │ ID   │ │Contact │ │Course  ││   │
│  │  │Module│ │Module  │ │Module  ││   │
│  │  └───┬──┘ └────┬───┘ └────┬───┘│   │
│  └──────┼─────────┼─────────┼────┘   │
└─────────┼─────────┼─────────┼────────┘
          │         │         │
          ▼         ▼         ▼
    ┌─────────────────────────────┐
    │  Repository Layer           │
    │  (Cache-First Strategy)     │
    └──────┬──────────────────────┘
           │ Cache Miss
           ▼
    ┌─────────────────────────────┐
    │  Web Scraper Layer          │
    │  ┌────────────────────────┐ │
    │  │ Rate Limiter           │ │
    │  │ (Token Bucket)         │ │
    │  └───────┬────────────────┘ │
    │          │                   │
    │          ▼                   │
    │  ┌────────────────────────┐ │
    │  │ HTTP Client            │ │
    │  │ (User-Agent Rotation)  │ │
    │  └───────┬────────────────┘ │
    └──────────┼───────────────────┘
               │
               ▼
        ┌────────────┐
        │  NTPU      │
        │  Websites  │
        └────────────┘

Storage:              Monitoring:
┌──────────────┐     ┌────────────────┐
│  SQLite DB   │     │  Prometheus    │
│  (WAL mode)  │◄────┤  (scrapes      │
│  /data/cache │     │   /metrics)    │
└──────────────┘     └────────┬───────┘
                              │
                              ▼
                     ┌────────────────┐
                     │  Grafana       │
                     │  (Dashboard)   │
                     └────────────────┘
```

### 資料流程

1. **Webhook 接收**: Gin 接收 LINE Webhook 事件
2. **模組分派**: 依關鍵字判斷由哪個 Bot Module 處理
3. **快取查詢**: Repository 先查詢 SQLite 快取 (TTL: 7 天)
4. **爬蟲抓取**: Cache Miss 時觸發 Web Scraper
5. **限流控制**: Rate Limiter 確保不過度請求 NTPU 網站
6. **資料回傳**: 格式化訊息回覆給 LINE 使用者
7. **指標記錄**: 記錄 Prometheus 指標供監控使用

## 🤖 Bot 模組說明

### ID Module (學號查詢)

**觸發關鍵字**: `學號`, `id`, `名字`, `姓名`, `學生`, `系`, `系代碼`

**功能**:
- 依學號查詢學生資訊 (8-9 位數字)
- 依姓名搜尋學生 (最多 500 筆)
- 查詢系代碼對應的系所名稱
- 查詢特定年度的學生資料 (110-113 學年度)

**範例**:
```
使用者: 學號 412345678
Bot: 姓名: 王小明
     學號: 412345678
     年級: 112 學年度入學
     系所: 資訊工程學系

使用者: 查詢王小明
Bot: 找到 3 位學生：
     1. 王小明 (412345678) - 資工系
     2. 王小明 (411234567) - 電機系
     ...
```

### Contact Module (通訊錄查詢)

**觸發關鍵字**: `聯繫`, `聯絡`, `contact`, `電話`, `分機`, `email`

**功能**:
- 緊急電話查詢 (三峽/臺北校區)
- 依姓名搜尋校內人員聯絡方式
- 顯示分機、Email、辦公室位置等資訊

**範例**:
```
使用者: 緊急電話
Bot: 📞 國立臺北大學緊急聯絡電話

     三峽校區：
     總機: 02-8674-1111
     24H 校安: 02-2673-2123

     臺北校區：
     總機: 02-2502-4654

使用者: 聯絡陳教授
Bot: 找到 2 筆聯絡人：

     👤 陳大華 教授
     單位: 資訊工程學系
     分機: 88888
     Email: chen@gm.ntpu.edu.tw
     ...
```

### Course Module (課程查詢)

**觸發關鍵字**: `課`, `課程`, `class`, `course`, `老師`, `教授`, `teacher`

**功能**:
- 依課程代碼查詢 (UID)
- 依課程名稱搜尋
- 依教師姓名搜尋課程
- 顯示上課時間、地點、課程大綱連結

**範例**:
```
使用者: 課程 3141U0001
Bot: 📚 資料結構
     課號: 3141U0001
     學年期: 113-1
     授課教師: 王教授
     上課時間: 星期二 3-4 節
     上課地點: 資訊大樓 101
     🔗 課程大綱

使用者: 王教授的課
Bot: 找到 5 門課程：
     1. 資料結構 (3141U0001)
     2. 演算法 (3141U0002)
     ...
```

## ⚙️ 環境變數設定

| 變數名稱 | 說明 | 預設值 | 必填 |
|---------|------|--------|------|
| `LINE_CHANNEL_ACCESS_TOKEN` | LINE Channel Access Token | - | ✅ |
| `LINE_CHANNEL_SECRET` | LINE Channel Secret | - | ✅ |
| `PORT` | HTTP 服務埠號 | `10000` | ❌ |
| `LOG_LEVEL` | 日誌等級 (debug/info/warn/error) | `info` | ❌ |
| `SQLITE_PATH` | SQLite 資料庫路徑 | `/data/cache.db` | ❌ |
| `CACHE_TTL` | 快取有效期限 | `168h` (7 天) | ❌ |
| `SCRAPER_WORKERS` | 爬蟲 Worker 數量 | `5` | ❌ |
| `SCRAPER_MIN_DELAY` | 最小請求延遲 | `100ms` | ❌ |
| `SCRAPER_MAX_DELAY` | 最大請求延遲 | `500ms` | ❌ |
| `SCRAPER_TIMEOUT` | HTTP 請求超時時間 | `15s` | ❌ |
| `SCRAPER_MAX_RETRIES` | 最大重試次數 | `3` | ❌ |
| `SHUTDOWN_TIMEOUT` | 優雅關機超時時間 | `30s` | ❌ |
| `WARMUP_TIMEOUT` | 資料預熱超時時間 | `5m` | ❌ |

## 📊 監控與可觀測性

### Prometheus 指標

存取 `http://localhost:10000/metrics` 查看所有指標。

**核心指標**:

| 指標類別 | 指標名稱 | 說明 |
|----------|------------|------|
| **請求量** | `ntpu_webhook_requests_total` | Webhook 請求總數 (labels: event_type, status) |
| | `ntpu_scraper_requests_total` | 爬蟲請求總數 (labels: module, status) |
| **延遲** | `ntpu_webhook_duration_seconds` | Webhook 處理耗時分佈 (Histogram) |
| | `ntpu_scraper_duration_seconds` | 爬蟲請求耗時分佈 (Histogram) |
| **快取** | `ntpu_cache_hits_total` | 快取命中次數 (labels: module) |
| | `ntpu_cache_misses_total` | 快取未命中次數 (labels: module) |
| | `ntpu_cache_entries` | 快取項目數量 (labels: module) |
| **系統** | `ntpu_active_goroutines` | 活躍 Goroutine 數量 |
| | `ntpu_memory_bytes` | 記憶體使用量 (bytes) |

**常用 PromQL 查詢**:

```promql
# Webhook 成功率
sum(rate(ntpu_webhook_requests_total{status="success"}[5m]))
/ sum(rate(ntpu_webhook_requests_total[5m]))

# P95 延遲
histogram_quantile(0.95,
  sum(rate(ntpu_webhook_duration_seconds_bucket[5m])) by (le, event_type)
)

# 快取命中率 (by module)
sum(rate(ntpu_cache_hits_total[5m])) by (module)
/ (sum(rate(ntpu_cache_hits_total[5m])) + sum(rate(ntpu_cache_misses_total[5m]))) by (module)

# 每秒請求數 (QPS)
sum(rate(ntpu_webhook_requests_total[1m]))
```

### Grafana Dashboard

1. 開啟 `http://localhost:3000`
2. 使用帳號 `admin` / 密碼 `admin123` 登入
3. 預設 Dashboard 已自動匯入：`deploy/grafana/dashboard.json`

**Dashboard 面板**:
- 📊 **Overview**: QPS、成功率、平均延遲
- ⏱️ **Latency**: P50/P95/P99 延遲分佈 (Webhook & Scraper)
- ✅ **Success Rate**: 爬蟲成功率 vs 錯誤率（依模組）
- 💾 **Cache Performance**: 命中率、Miss 率、Cache 大小
- 🔧 **System Resources**: Goroutines、Memory、CPU
- 🚨 **Error Tracking**: 錯誤趨勢與分類

**查看監控數據**:
```bash
# 啟動完整監控堆疊
task compose:up

# 存取服務
open http://localhost:9090  # Prometheus
open http://localhost:3000  # Grafana
open http://localhost:10000/metrics  # Bot Metrics
```

### 告警規則

Prometheus 告警規則 (`deploy/prometheus/alerts.yml`):

- **ScraperHighFailureRate**: 爬蟲失敗率 > 50% 持續 5 分鐘
- **WebhookHighLatency**: Webhook P95 延遲 > 5 秒持續 5 分鐘
- **ServiceDown**: 服務停止回應持續 2 分鐘
- **HighMemoryUsage**: 記憶體使用 > 500MB 持續 10 分鐘
- **CacheLowHitRate**: 快取命中率 < 50% 持續 15 分鐘

### 結構化日誌

日誌格式為 JSON,範例:

```json
{
  "level": "info",
  "msg": "Webhook received",
  "time": "2024-01-15T10:30:45+08:00",
  "request_id": "abc123",
  "event_type": "message",
  "user_id": "U1234567890abcdef"
}
```

## 🛠️ 開發指南

### 專案結構

```
.
├── cmd/
│   ├── server/          # 主服務入口
│   └── warmup/          # 資料預熱工具
├── internal/
│   ├── bot/             # Bot 模組
│   │   ├── id/          # 學號查詢模組
│   │   ├── contact/     # 通訊錄查詢模組
│   │   └── course/      # 課程查詢模組
│   ├── config/          # 設定管理
│   ├── logger/          # 日誌系統
│   ├── metrics/         # Prometheus 指標
│   ├── scraper/         # 爬蟲系統
│   │   └── ntpu/        # NTPU 特定爬蟲
│   ├── storage/         # 資料儲存層
│   └── webhook/         # LINE Webhook 處理
├── pkg/
│   └── lineutil/        # LINE 訊息工具
├── deploy/
│   ├── prometheus/      # Prometheus 設定
│   └── grafana/         # Grafana 設定
├── docker/
│   └── docker-compose.yml
├── .github/
│   └── workflows/       # CI/CD 設定
├── Dockerfile
├── Makefile
└── go.mod
```

### 開發指令

本專案使用 [Task](https://taskfile.dev) 作為任務執行器：

```bash
# 安裝 Task (選擇其一)
go install github.com/go-task/task/v3/cmd/task@latest
# Windows: choco install go-task
# macOS: brew install go-task
# Linux: sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin

# 查看所有可用指令
task --list

# 常用指令
task dev            # 執行開發伺服器
task build          # 編譯二進位檔
task test           # 執行測試
task lint           # 執行 Linter
task fmt            # 格式化程式碼
task ci             # 執行完整 CI 流程
task warmup         # 執行資料預熱
task clean          # 清除建置產物

# Docker 相關
task docker:build   # 建置 Docker image
task compose:up     # 啟動 docker-compose
task compose:down   # 停止服務
task compose:logs   # 查看日誌
```

### 新增 Bot 模組

1. 在 `internal/bot/<module>/` 建立新模組
2. 實作 `Handler` interface:
   ```go
   type Handler interface {
       CanHandle(text string) bool
       HandleMessage(ctx context.Context, event *webhook.MessageEvent) ([]messaging_api.MessageInterface, error)
       HandlePostback(ctx context.Context, event *webhook.PostbackEvent) ([]messaging_api.MessageInterface, error)
   }
   ```
3. 在 `internal/webhook/handler.go` 註冊模組
4. 更新 `cmd/warmup/main.go` 新增預熱邏輯

### 執行測試

```bash
# 執行所有測試
go test ./...

# 執行特定套件測試
go test ./internal/storage

# 產生 Coverage 報告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# 執行 Race Detector
go test -race ./...
```

### 程式碼品質檢查

```bash
# 安裝 golangci-lint
task tools

# 執行 Linter
task lint

# 格式化程式碼
task fmt

# 執行完整 CI（驗證、格式化、Lint、測試、編譯）
task ci
```

## 🐳 Docker 部署

### 單獨服務

```bash
# 建置 Image
docker build -t ntpu-linebot:latest .

# 執行容器
docker run -d \
  --name ntpu-linebot \
  -p 10000:10000 \
  -v ./data:/data \
  -e LINE_CHANNEL_ACCESS_TOKEN=your_token \
  -e LINE_CHANNEL_SECRET=your_secret \
  ntpu-linebot:latest
```

### Docker Compose (完整監控)

```bash
# 啟動所有服務 (包含 Prometheus + Grafana)
docker-compose -f docker/docker-compose.yml up -d

# 查看服務狀態
docker-compose -f docker/docker-compose.yml ps

# 查看特定服務日誌
docker-compose -f docker/docker-compose.yml logs -f ntpu-linebot

# 停止所有服務
docker-compose -f docker/docker-compose.yml down

# 停止並刪除資料卷
docker-compose -f docker/docker-compose.yml down -v
```

### 資料預熱 (Warmup)

**為什麼需要 Warmup?**
- 首次查詢時需等待爬蟲抓取資料 (10-30秒)
- 提前建立快取可提升使用者體驗
- 建議在系統啟動時或定期執行

**執行方式**:

```bash
# Docker Compose 方式 (推薦)
docker-compose -f docker/docker-compose.yml run --rm warmup

# 完整重新抓取 (清除舊資料)
docker-compose -f docker/docker-compose.yml run --rm warmup --reset

# 僅抓取特定模組
docker-compose -f docker/docker-compose.yml run --rm warmup --modules=id
docker-compose -f docker/docker-compose.yml run --rm warmup --modules=contact,course

# 本機執行
go run ./cmd/warmup
go run ./cmd/warmup --modules=id,contact,course --reset

# 使用 Task
task warmup
```

**Warmup 涵蓋範圍**:
- **ID 模組**: 110-113 學年度 × 所有系所 (約 200 個組合)
- **Contact 模組**: 行政單位 + 學術單位聯絡人
- **Course 模組**: 最近 3 學期課程 (113-1, 113-2, 112-2)

**執行時間**:
- 完整 warmup: 約 3-5 分鐘 (視網路速度)
- 單一模組: 約 1-2 分鐘

**Worker Pool 設定**:
```bash
# 調整並發數 (預設 5)
go run ./cmd/warmup --workers=10

# 環境變數控制
SCRAPER_WORKERS=8 go run ./cmd/warmup
```

## 📊 監控設定指南 (Monitoring Setup)

### Prometheus 設定

**啟動 Prometheus**:
```bash
# Docker Compose 已包含
docker-compose -f docker/docker-compose.yml up -d prometheus

# 存取: http://localhost:9090
```

**查詢範例**:
```promql
# 請求成功率
sum(rate(ntpu_webhook_requests_total{status="success"}[5m]))
/
sum(rate(ntpu_webhook_requests_total[5m]))

# P95 延遲
histogram_quantile(0.95,
  sum(rate(ntpu_webhook_duration_seconds_bucket[5m])) by (le, event_type)
)

# 快取命中率
sum(rate(ntpu_cache_hits_total[5m])) by (module)
/
(sum(rate(ntpu_cache_hits_total[5m])) + sum(rate(ntpu_cache_misses_total[5m]))) by (module)
```

### Grafana Dashboard 匯入

1. 開啟 Grafana: `http://localhost:3000`
2. 登入 (admin / admin123)
3. 左側選單 → Dashboards → Import
4. 選擇 `deploy/grafana/dashboard.json`
5. 選擇 Prometheus 資料源

**Dashboard 包含**:
- 📊 Request Rate (QPS)
- ⏱️ Latency (P50/P95/P99)
- ✅ Success Rate / ❌ Error Rate
- 💾 Cache Hit Rate
- 🔧 System Resources (Memory/Goroutines)

### 告警通知設定

**Alertmanager 設定** (`deploy/prometheus/alertmanager.yml`):
```yaml
global:
  resolve_timeout: 5m

route:
  receiver: 'line-notify'
  group_by: ['alertname', 'severity']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h

receivers:
  - name: 'line-notify'
    webhook_configs:
      - url: 'https://notify-api.line.me/api/notify'
        send_resolved: true
```

**告警規則說明**:
- ⚠️ **ScraperHighFailureRate**: 失敗率 >30% 持續 3 分鐘
- ⏰ **WebhookHighLatency**: P95 延遲 >3s 持續 5 分鐘
- 🔴 **ServiceDown**: 服務停止 2 分鐘
- 💾 **HighMemoryUsage**: 記憶體使用 >80% 持續 5 分鐘
- 📉 **CacheLowHitRate**: 快取命中 <70% 持續 10 分鐘
- 🐛 **HighGoroutineCount**: Goroutine 數量 >1000
- 🗄️ **DatabaseConnectionError**: 資料庫錯誤增加
- 🎨 **StickerLoadingFailure**: Sticker 載入失敗

### 日誌聚合 (選用)

**使用 Loki + Promtail**:
```bash
# 新增至 docker-compose.yml
docker-compose -f docker/docker-compose-full.yml up -d
```

**在 Grafana 中查詢日誌**:
```logql
{job="ntpu-linebot"} |= "error"
{job="ntpu-linebot"} | json | level="error"
```

## 🔧 疑難排解 (Troubleshooting)

### 常見問題快速診斷

| 問題 | 可能原因 | 解決方法 |
|------|----------|----------|
| 🔴 服務無法啟動 | 環境變數未設定 | 檢查 `.env` 檔案，確認 `LINE_CHANNEL_*` 已設定 |
| ⏰ 回應緩慢 | Cache 未預熱 | 執行 `task warmup` 建立快取 |
| 🚫 Webhook 驗證失敗 | Channel Secret 錯誤 | 檢查 `LINE_CHANNEL_SECRET` 是否正確 |
| 💾 資料庫鎖定 | 多實例寫入 | 確認只有一個服務實例運行 |
| 🕷️ 爬蟲失敗率高 | NTPU 網站異常 | 檢查 Prometheus metrics 確認失敗模組 |
| 📊 Grafana 無資料 | Prometheus 未連線 | 確認 `docker-compose` 服務都正常運行 |

### 詳細問題解決

#### 1. SQLite 資料庫鎖定

**錯誤訊息**: `database is locked`

**診斷步驟**:
```bash
# 檢查是否有多個實例
ps aux | grep ntpu-linebot

# 檢查資料庫檔案
ls -lh data/cache.db*

# 驗證 WAL 模式
sqlite3 data/cache.db "PRAGMA journal_mode;"
# 應該輸出: wal
```

**解決方法**:
- 確認只有一個服務實例存取資料庫
- 檢查 `busy_timeout` 設定是否足夠 (預設 5000ms)
- 確認 SQLite 使用 WAL 模式

### 問題: 爬蟲請求失敗率高

**錯誤訊息**: `scraper request failed` 或 `timeout exceeded`

**解決方法**:
- 調高 `SCRAPER_TIMEOUT` (預設 15 秒)
- 增加 `SCRAPER_MAX_DELAY` 減少請求頻率
- 檢查 NTPU 網站是否正常運作
- 查看 Prometheus 指標確認失敗模組

### 問題: 記憶體使用過高

**解決方法**:
- 降低 `SCRAPER_WORKERS` 數量 (預設 5)
- 縮短 `CACHE_TTL` 清理舊資料
- 定期執行 `DELETE FROM ... WHERE cached_at < ?`
- 監控 Grafana 記憶體面板

### 問題: Webhook 簽章驗證失敗

**錯誤訊息**: `invalid signature`

**解決方法**:
- 確認 `LINE_CHANNEL_SECRET` 正確無誤
- 檢查 Webhook URL 設定是否正確
- 確認使用 HTTPS (LINE 要求)
- 查看 LINE Developers Console 的錯誤日誌

### 問題: Docker Compose 啟動失敗

**錯誤訊息**: `warmup service exited with code 1`

**解決方法**:
- 查看 warmup 容器日誌: `docker-compose logs warmup`
- 檢查網路連線是否正常
- 確認 NTPU 網站可存取
- 增加 `WARMUP_TIMEOUT` (預設 5 分鐘)

## 📚 專案文件

- **[架構設計](docs/architecture.md)** - 系統架構、設計模式、技術決策
- **[API 文件](docs/API.md)** - HTTP 端點、LINE Webhook、Prometheus 指標
- **[Copilot 指引](.github/copilot-instructions.md)** - AI Agent 開發指引
- **[部署指南](deploy/README.md)** - Prometheus/Grafana 監控設定
- **[Docker Compose](docker/README.md)** - 容器化部署說明
- **[Warmup 工具](cmd/warmup/README.md)** - 資料預熱使用方式

### 模組文件

- [Bot Modules](internal/bot/README.md) - Bot 模組開發指南
- [Scraper System](internal/scraper/README.md) - 爬蟲系統設計
- [Storage Layer](internal/storage/README.md) - 資料庫設計與快取策略
- [Webhook Handler](internal/webhook/README.md) - Webhook 處理邏輯
- [LINE Utilities](internal/lineutil/README.md) - LINE 訊息工具
- [Logger](internal/logger/README.md) - 結構化日誌系統
- [Metrics](internal/metrics/README.md) - Prometheus 指標
- [Sticker](internal/sticker/README.md) - 貼圖管理系統
- [Config](internal/config/README.md) - 設定管理

---

## 🤝 貢獻指南

歡迎提交 Issue 和 Pull Request！

### 開發流程

1. **Fork & Clone**
   ```bash
   git clone https://github.com/YOUR_USERNAME/ntpu-linebot-go.git
   cd ntpu-linebot-go
   ```

2. **建立功能分支**
   ```bash
   git checkout -b feature/amazing-feature
   ```

3. **開發與測試**
   ```bash
   task dev              # 執行開發伺服器
   task test             # 執行測試
   task ci               # 執行完整 CI（fmt + lint + test + build）
   ```

4. **提交變更**（遵循 [Conventional Commits](https://www.conventionalcommits.org/)）
   ```bash
   git commit -m 'feat(bot): add amazing feature'
   git commit -m 'fix(scraper): fix rate limiting issue'
   git commit -m 'docs: update README'
   ```

5. **推送並建立 PR**
   ```bash
   git push origin feature/amazing-feature
   # 在 GitHub 上開啟 Pull Request
   ```

### 程式碼品質要求

- ✅ 遵循 [Effective Go](https://go.dev/doc/effective_go) 指南
- ✅ 使用 `gofmt` 格式化（`task fmt`）
- ✅ 通過 `golangci-lint` 檢查（`task lint`）
- ✅ 測試覆蓋率 > 80%（`task test:coverage`）
- ✅ 為新功能撰寫 table-driven tests
- ✅ 更新相關文件（README、模組 README）
- ✅ 通過 CI 檢查

### Commit Message 規範

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Type:**
- `feat`: 新功能
- `fix`: Bug 修復
- `docs`: 文件更新
- `refactor`: 重構（不影響功能）
- `test`: 測試相關
- `chore`: 維護性變更（依賴更新、工具設定）

**Scope:**
- `bot`: Bot 模組
- `scraper`: 爬蟲系統
- `storage`: 資料庫層
- `webhook`: Webhook 處理
- `config`: 設定管理

**範例:**
```
feat(bot): add course teacher search

Implement teacher name search in course module.
Users can now query "王教授的課" to find all courses.

Closes #123
```

## 📄 授權條款

本專案採用 [MIT License](LICENSE) 授權。

## 📞 聯絡方式

- **專案維護者**: [garyellow](https://github.com/garyellow)
- **問題回報**: [GitHub Issues](https://github.com/garyellow/ntpu-linebot-go/issues)

## ⚡ 效能優化建議

### 1. 快取策略優化

**問題**: 首次查詢回應慢（10-30秒）
**解決**:
```bash
# 部署前執行 warmup 建立快取
task warmup

# 或使用 Docker Compose（自動執行 warmup）
task compose:up
```

### 2. 爬蟲並發調整

**低流量場景** (< 100 users):
```bash
SCRAPER_WORKERS=3
SCRAPER_MIN_DELAY=200ms
SCRAPER_MAX_DELAY=800ms
```

**高流量場景** (> 1000 users):
```bash
SCRAPER_WORKERS=10
SCRAPER_MIN_DELAY=100ms
SCRAPER_MAX_DELAY=500ms
```

### 3. 快取 TTL 調整

**學期中**（資料穩定）:
```bash
CACHE_TTL=336h  # 14 天
```

**學期初/末**（資料變動頻繁）:
```bash
CACHE_TTL=72h   # 3 天
```

### 4. 記憶體優化

**監控記憶體使用**:
```bash
# Prometheus 查詢
ntpu_memory_bytes / 1024 / 1024  # MB

# 或使用 Grafana Dashboard
```

**記憶體過高時**:
- 降低 `SCRAPER_WORKERS`
- 縮短 `CACHE_TTL`
- 定期執行 `VACUUM`

### 5. 資料庫優化

```bash
# 定期清理過期資料
sqlite3 data/cache.db "DELETE FROM students WHERE cached_at < strftime('%s', 'now') - 604800;"

# 回收空間
sqlite3 data/cache.db "VACUUM;"

# 重建索引
sqlite3 data/cache.db "REINDEX;"
```

---

## 🙏 致謝

- [LINE Developers](https://developers.line.biz/) - LINE Bot SDK
- [Gin Web Framework](https://gin-gonic.com/) - HTTP 框架
- [Prometheus](https://prometheus.io/) - 監控系統
- [Grafana](https://grafana.com/) - 視覺化工具
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) - Pure Go SQLite
- [goquery](https://github.com/PuerkitoBio/goquery) - HTML 解析

---

## 📝 授權與版權

本專案採用 [MIT License](LICENSE) 授權。

**重要提示**:
- 本專案僅供學術研究與教育用途
- 請遵守 NTPU 網站使用條款
- 爬蟲請求務必遵守 rate limiting
- 不得用於商業用途

---

Made with ❤️ by NTPU Students

**維護者**: [garyellow](https://github.com/garyellow)
**專案連結**: https://github.com/garyellow/ntpu-linebot-go
**問題回報**: https://github.com/garyellow/ntpu-linebot-go/issues
