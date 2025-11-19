# NTPU Line Bot (Go)

[![CI](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/garyellow/ntpu-linebot-go)](https://goreportcard.com/report/github.com/garyellow/ntpu-linebot-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

國立台北大學 LINE 聊天機器人的 Go 語言實作版本,提供學號查詢、通訊錄查詢、課程查詢等功能。

## ✨ 功能特色

- 🔍 **學號查詢**: 依姓名或學號查詢學生資訊,支援系代碼查詢
- 📞 **通訊錄查詢**: 查詢校內人員聯絡方式,包含分機、Email 等資訊
- 📚 **課程查詢**: 查詢課程資訊,包含授課教師、上課時間與地點
- 💾 **SQLite 快取**: 使用外部資料庫儲存,避免記憶體溢出
- 📊 **Prometheus 監控**: 完整的效能指標與告警機制
- 📋 **結構化日誌**: JSON 格式日誌,便於集中式分析
- 🔄 **資料預熱**: Docker 初始化容器自動抓取最新資料
- 🛡️ **防爬蟲機制**: Token bucket 限流、隨機延遲、指數退避重試
- 🚀 **高效能**: 使用 Go 並發特性,平均回應時間 < 500ms

## 📋 前置需求

- **Go 1.23+**: 用於本機開發
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
make warmup
# 或
go run cmd/warmup/main.go --modules=id,contact,course

# 4. 執行服務
make run
# 或
go run cmd/server/main.go
```

## 🏗️ 架構設計

```
┌─────────────┐
│  LINE       │
│  Platform   │
└──────┬──────┘
       │ Webhook
       ▼
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
- 緊急電話查詢 (三峽/台北校區)
- 依姓名搜尋校內人員聯絡方式
- 顯示分機、Email、辦公室位置等資訊

**範例**:
```
使用者: 緊急電話
Bot: 📞 國立臺北大學緊急聯絡電話

     三峽校區：
     總機: 02-8674-1111
     24H 校安: 02-2673-2123

     台北校區：
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

## 📊 監控與日誌

### Prometheus 指標

存取 `http://localhost:9090/metrics` 可查看以下指標:

- `ntpu_scraper_requests_total{module,status}`: 爬蟲請求次數
- `ntpu_scraper_duration_seconds{module}`: 爬蟲請求耗時 (Histogram)
- `ntpu_cache_hits_total{module}`: 快取命中次數
- `ntpu_cache_misses_total{module}`: 快取未命中次數
- `ntpu_cache_entries{module}`: 快取項目數量
- `ntpu_webhook_requests_total{event_type,status}`: Webhook 請求次數
- `ntpu_webhook_duration_seconds{event_type}`: Webhook 處理耗時
- `ntpu_active_goroutines`: 活躍的 Goroutine 數量
- `ntpu_memory_bytes`: 記憶體使用量

### Grafana Dashboard

1. 開啟 `http://localhost:3000`
2. 使用帳號 `admin` / 密碼 `admin123` 登入
3. 匯入預設 Dashboard: `deployments/grafana/dashboard.json`

Dashboard 包含以下面板:
- 📈 請求 QPS (依事件類型)
- ⏱️ Webhook 延遲 (P50/P95/P99)
- ✅ 爬蟲成功率 / ❌ 錯誤率 (依模組)
- 💾 快取命中率 (依模組)
- 🔧 系統資源 (Goroutines / Memory)
- 🚨 錯誤趨勢

### 告警規則

Prometheus 告警規則 (`deployments/prometheus/alerts.yml`):

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
├── deployments/
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

### Makefile 指令

```bash
make build          # 編譯二進位檔
make test           # 執行測試 (含 coverage)
make lint           # 執行 Linter
make docker-build   # 建置 Docker image
make run            # 執行主服務
make warmup         # 執行資料預熱
make clean          # 清除建置產物
make deps           # 下載依賴
make install-tools  # 安裝開發工具
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
# golangci-lint (需先安裝)
make install-tools
make lint

# 檢查漏洞
govulncheck ./...

# 格式化程式碼
go fmt ./...
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

### 資料預熱

```bash
# 使用 warmup 容器重新抓取資料
docker-compose -f docker/docker-compose.yml run --rm warmup \
  --modules=id,contact,course --reset

# 僅抓取特定模組
docker-compose -f docker/docker-compose.yml run --rm warmup \
  --modules=id
```

## 🔧 疑難排解

### 問題: SQLite 資料庫鎖定

**錯誤訊息**: `database is locked`

**解決方法**:
- 確認只有一個服務實例存取資料庫
- 檢查 `busy_timeout` 設定是否足夠 (預設 5 秒)
- 確認 SQLite 使用 WAL 模式 (`PRAGMA journal_mode=WAL`)

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

## 🤝 貢獻指南

歡迎提交 Issue 和 Pull Request!

### 提交 Pull Request

1. Fork 本專案
2. 建立功能分支: `git checkout -b feature/amazing-feature`
3. 提交變更: `git commit -m 'Add amazing feature'`
4. 推送分支: `git push origin feature/amazing-feature`
5. 開啟 Pull Request

### 程式碼風格

- 遵循 [Effective Go](https://go.dev/doc/effective_go) 指南
- 使用 `gofmt` 格式化程式碼
- 通過 `golangci-lint` 檢查
- 為新功能撰寫測試 (目標 Coverage > 80%)
- 更新相關文件

## 📄 授權條款

本專案採用 [MIT License](LICENSE) 授權。

## 📞 聯絡方式

- **專案維護者**: [garyellow](https://github.com/garyellow)
- **原始 Python 專案**: [ntpu-linebot-python](https://github.com/garyellow/ntpu-linebot-python)
- **問題回報**: [GitHub Issues](https://github.com/garyellow/ntpu-linebot-go/issues)

## 🙏 致謝

- [LINE Developers](https://developers.line.biz/) - LINE Bot SDK
- [Gin Web Framework](https://gin-gonic.com/) - HTTP 框架
- [Prometheus](https://prometheus.io/) - 監控系統
- [Grafana](https://grafana.com/) - 視覺化工具
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) - Pure Go SQLite

---

Made with ❤️ by NTPU Students
