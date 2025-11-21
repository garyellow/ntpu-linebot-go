# NTPU LineBot (Go)

[![CI](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/garyellow/ntpu-linebot-go)](https://goreportcard.com/report/github.com/garyellow/ntpu-linebot-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go 1.25+](https://img.shields.io/badge/go-1.25+-blue.svg)](https://go.dev/dl/)

國立臺北大學 LINE 聊天機器人，提供學號查詢、通訊錄查詢、課程查詢等功能。使用 Go 重寫，強調高效能、可維護性與完整監控。

## ✨ 功能特色

### 核心功能
- 🔍 **學號查詢**: 依姓名或學號查詢學生資訊、系代碼對照
- 📞 **通訊錄查詢**: 校內人員聯絡方式（分機、Email、辦公室）、緊急電話
- 📚 **課程查詢**: 課程資訊（課號、教師、時間、地點、大綱連結）

### 技術特色
- 💾 **智慧快取**: SQLite WAL 模式、7 天 TTL、Cache-First 策略
- 🛡️ **防爬蟲機制**: Singleflight 去重、Token Bucket 限流（5 req/s）、指數退避重試
- 📊 **完整監控**: Prometheus + Grafana + AlertManager
- 🚀 **高效能**: Go 並發、Worker Pool、Context 超時控制（25s）

## 📞 加入好友

**LINE ID**: [@148wrcch](https://lin.ee/QiMmPBv)

[![加入好友](add_friend/S_add_friend_button.png)](https://lin.ee/QiMmPBv)

![QR Code](add_friend/S_gainfriends_qr.png)

## 📋 前置需求

- **Go 1.25+** (本機開發)
- **Docker & Docker Compose** (容器部署)
- **LINE Bot 憑證**: Channel Access Token 與 Channel Secret

### 取得 LINE Bot 憑證

1. 前往 [LINE Developers Console](https://developers.line.biz/console/)
2. 建立 Messaging API Channel
3. 取得 **Channel Secret** (Basic settings)
4. 發行 **Channel Access Token** (Messaging API settings)

## 🚀 快速開始

### Docker Compose (推薦)

```bash
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go/docker

# 設定環境變數
cp .env.example .env
# 編輯 .env 填入 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET

# 啟動服務
docker compose up -d

# 查看日誌
docker compose logs -f ntpu-linebot
```

服務啟動後：
- LINE Bot Webhook: `http://localhost:10000/callback`
- Prometheus: `http://localhost:9090`
- AlertManager: `http://localhost:9093`
- Grafana: `http://localhost:3000` (admin/admin123)

### 本機開發

```bash
# 安裝 Task runner
go install github.com/go-task/task/v3/cmd/task@latest

# 安裝依賴
go mod download

# 設定環境變數
cp .env.example .env
# 編輯 .env

# 預熱快取（首次執行）
task warmup

# 啟動開發服務
task dev
```

## 🏗️ 架構設計

```
LINE Webhook → Gin Handler (25s timeout)
                 ↓ (簽章驗證、限流)
           Bot Handlers (id/contact/course)
                 ↓ (關鍵字匹配)
          Storage Repository (cache-first)
                 ↓ (7天 TTL 檢查)
       Scraper Client (限流、singleflight)
                 ↓ (指數退避、failover URLs)
            NTPU Websites (lms/sea)
```

### 關鍵設計

- **Cache-First**: 優先查詢快取，Miss 時觸發爬蟲
- **Singleflight**: 10 個並發查詢 → 1 次爬蟲執行
- **Rate Limiting**: 全域 5 req/s + 每用戶 10 req/s
- **Worker Pool**: 限制並發數避免資源耗盡

詳細架構說明請見 [docs/architecture.md](docs/architecture.md)

## 📖 使用範例

### 學號查詢
```
學號 412345678          # 依學號查詢
學生 王小明             # 依姓名查詢
系代碼 85               # 查詢系所名稱
```

### 課程查詢
```
課程 資料結構           # 依課程名稱搜尋
教師 王教授             # 依教師姓名搜尋
課號 3141U0001          # 依課號查詢
```

### 聯絡資訊
```
聯絡 資工系             # 查詢系所聯絡方式
緊急電話                # 顯示緊急聯絡電話
```

## ⚙️ 環境變數

| 變數 | 說明 | 預設值 | 必填 |
|------|------|--------|------|
| `LINE_CHANNEL_ACCESS_TOKEN` | LINE Bot Access Token | - | ✅ |
| `LINE_CHANNEL_SECRET` | LINE Channel Secret | - | ✅ |
| `PORT` | HTTP 服務埠號 | `10000` | ❌ |
| `LOG_LEVEL` | 日誌等級 (debug/info/warn/error) | `info` | ❌ |
| `SQLITE_PATH` | SQLite 資料庫路徑 | `/data/cache.db` | ❌ |
| `CACHE_TTL` | 快取有效期限 | `168h` | ❌ |
| `SCRAPER_WORKERS` | 爬蟲並發數 | `5` | ❌ |
| `SCRAPER_TIMEOUT` | 爬蟲請求超時 | `15s` | ❌ |
| `WARMUP_TIMEOUT` | 預熱超時時間 | `20m` | ❌ |

完整設定請見 [internal/config/README.md](internal/config/README.md)

## 📊 監控與可觀測性

### Prometheus 指標

| 類別 | 指標 | 說明 |
|------|------|------|
| **請求** | `ntpu_webhook_requests_total` | Webhook 請求總數 |
| **延遲** | `ntpu_webhook_duration_seconds` | Webhook 處理耗時 (Histogram) |
| **快取** | `ntpu_cache_hits_total` | 快取命中次數 |
| | `ntpu_cache_misses_total` | 快取未命中次數 |
| **系統** | `ntpu_memory_bytes` | 記憶體使用量 |
| | `ntpu_active_goroutines` | 活躍 Goroutine 數 |

### 存取監控服務

```bash
# 啟動完整監控堆疊
task compose:up

# 存取服務
open http://localhost:9090  # Prometheus
open http://localhost:9093  # AlertManager
open http://localhost:3000  # Grafana (admin/admin123)
```

### Grafana Dashboard

預設 Dashboard 包含：
- 📊 QPS、成功率、平均延遲
- ⏱️ P50/P95/P99 延遲分佈
- 💾 快取命中率
- 🔧 系統資源使用

詳細監控設定請見 [deploy/README.md](deploy/README.md)

## 🛠️ 開發指南

### 專案結構

```
.
├── cmd/                    # 應用程式入口
│   ├── server/            # 主服務
│   ├── warmup/            # 預熱工具
│   └── healthcheck/       # 健康檢查
├── internal/              # 內部套件
│   ├── bot/               # Bot 模組 (id/contact/course)
│   ├── config/            # 設定管理
│   ├── logger/            # 結構化日誌
│   ├── metrics/           # Prometheus 指標
│   ├── scraper/           # 爬蟲系統
│   ├── storage/           # SQLite 資料層
│   ├── sticker/           # 貼圖管理
│   ├── webhook/           # LINE Webhook 處理
│   └── lineutil/          # LINE 訊息工具
├── deploy/                # 監控設定
│   ├── prometheus/
│   ├── alertmanager/
│   └── grafana/
├── docker/                # Docker 部署
└── docs/                  # 文件
```

### Task 指令

```bash
task dev              # 開發模式執行
task build            # 編譯二進位
task test             # 執行測試
task lint             # 執行 linter
task ci               # 完整 CI (fmt + lint + test + build)
task warmup           # 預熱快取
task compose:up       # 啟動 docker compose
task compose:logs     # 查看日誌
```

### 執行測試

```bash
# 執行所有測試
go test ./...

# 帶 coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Race detector
go test -race ./...
```

## 🐳 Docker 部署

### 建置與執行

```bash
# 建置 image
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

### Docker Compose

```bash
cd docker

# 啟動所有服務
docker compose up -d

# 查看狀態
docker compose ps

# 查看日誌
docker compose logs -f ntpu-linebot

# 停止服務
docker compose down
```

### 資料預熱

為避免首次查詢緩慢（10-30秒），建議啟動前預熱快取：

```bash
# 使用 docker compose
docker compose run --rm warmup

# 完整重新抓取
docker compose run --rm warmup -reset

# 本機執行
go run ./cmd/warmup -modules=id,contact,course
```

預熱涵蓋：
- **ID 模組**: 110-113 學年度 × 所有系所
- **Contact 模組**: 行政 + 學術單位聯絡人
- **Course 模組**: 最近 3 學期課程

執行時間：約 3-5 分鐘

詳細說明請見 [cmd/warmup/README.md](cmd/warmup/README.md)

## 🔧 疑難排解

### 常見問題

| 問題 | 原因 | 解決方法 |
|------|------|----------|
| 服務無法啟動 | 環境變數未設定 | 檢查 `.env` 檔案 |
| 回應緩慢 | Cache 未預熱 | 執行 `task warmup` |
| Webhook 驗證失敗 | Channel Secret 錯誤 | 檢查 `LINE_CHANNEL_SECRET` |
| 資料庫鎖定 | 多實例寫入 | 確認只有一個服務實例 |

### 除錯提示

```bash
# 啟用 debug 日誌
LOG_LEVEL=debug task dev

# 檢查快取狀態
sqlite3 data/cache.db "SELECT COUNT(*) FROM students;"

# 查看 metrics
curl http://localhost:10000/metrics

# 驗證 docker compose 設定
cd docker && docker compose config
```

## 📚 專案文件

- **[架構設計](docs/architecture.md)** - 系統架構、設計模式
- **[API 文件](docs/API.md)** - HTTP 端點、Prometheus 指標
- **[部署指南](deploy/README.md)** - Prometheus/Grafana 設定
- **[Docker Compose](docker/README.md)** - 容器化部署

### 模組文件

- [Bot Modules](internal/bot/README.md) - Bot 模組開發
- [Scraper System](internal/scraper/README.md) - 爬蟲系統
- [Storage Layer](internal/storage/README.md) - 資料庫與快取
- [Webhook Handler](internal/webhook/README.md) - Webhook 處理
- [Config](internal/config/README.md) - 設定管理

## 🤝 貢獻指南

歡迎提交 Issue 和 Pull Request！

### 開發流程

1. Fork 專案並建立功能分支
2. 開發與測試 (`task dev` / `task test`)
3. 執行完整 CI (`task ci`)
4. 遵循 [Conventional Commits](https://www.conventionalcommits.org/) 規範
5. 提交 Pull Request

### Commit 規範

```
feat(bot): add course search by teacher
fix(scraper): handle timeout correctly
docs: update README
refactor(storage): simplify cache logic
test: add missing unit tests
```

## ⚡ 效能優化

### 快取策略

```bash
# 學期中（資料穩定）
CACHE_TTL=336h  # 14 天

# 學期初/末（資料變動頻繁）
CACHE_TTL=72h   # 3 天
```

### 爬蟲並發

```bash
# 低流量（< 100 users）
SCRAPER_WORKERS=3

# 高流量（> 1000 users）
SCRAPER_WORKERS=10
```

## 📄 授權條款

本專案採用 [MIT License](LICENSE) 授權。

**重要提示**:
- 本專案僅供學術研究與教育用途
- 請遵守 NTPU 網站使用條款
- 不得用於商業用途

---

Made with ❤️ by NTPU Students

**維護者**: [garyellow](https://github.com/garyellow)
**專案連結**: https://github.com/garyellow/ntpu-linebot-go
**問題回報**: https://github.com/garyellow/ntpu-linebot-go/issues
