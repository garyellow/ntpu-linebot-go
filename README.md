# NTPU LineBot (Go)

[![CI](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml/badge.svg)](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/garyellow/ntpu-linebot-go)](https://goreportcard.com/report/github.com/garyellow/ntpu-linebot-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go 1.25+](https://img.shields.io/badge/go-1.25+-blue.svg)](https://go.dev/dl/)

國立臺北大學 LINE 聊天機器人，提供學號查詢、通訊錄查詢、課程查詢等功能。

> **從 Python 遷移**: 本專案從 [ntpu-linebot-python](https://github.com/garyellow/ntpu-linebot-python) 重寫而來，選擇 Go 以獲得更好的並發處理、更低的資源消耗與完整的類型安全。詳見 [遷移說明](docs/migration.md)。

## 📋 目錄

- [功能特色](#-功能特色)
- [加入好友](#-加入好友)
- [快速開始](#-快速開始)
- [架構設計](#-架構設計)
- [環境變數](#-環境變數)
- [開發指南](#-開發指南)
- [Docker 部署](#-docker-部署)
- [疑難排解](#-疑難排解)
- [貢獻指南](#-貢獻指南)

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

使用預建映像從 Docker Hub 部署:

```bash
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go/deployments

# 設定環境變數
cp .env.example .env
# 編輯 .env 填入 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET

# 拉取並啟動服務
docker compose pull
docker compose up -d

# 查看日誌
docker compose logs -f ntpu-linebot
```

服務啟動後：
- LINE Bot Webhook: `http://localhost:10000/callback`
- Prometheus: `http://localhost:9090`
- AlertManager: `http://localhost:9093`
- Grafana: `http://localhost:3000` (admin/admin123)

**指定版本**: 在 `.env` 設定 `IMAGE_TAG=v1.2.3`

## 🏗️ 架構設計

```
LINE Webhook → Gin Handler → Bot Handlers → Storage Repository → Scraper → NTPU Websites
```

### 關鍵特性

- **Cache-First**: 優先查詢快取,避免重複爬取
- **Singleflight**: 重複查詢自動合併,減輕目標網站負擔
- **Rate Limiting**: 全域與每用戶限流,防止濫用
- **Context Timeout**: 25 秒超時控制,避免請求堆積

📖 **完整架構文件**: [docs/architecture.md](docs/architecture.md)

## 💬 使用範例

| 功能 | 指令範例 |
|------|---------|
| **學號查詢** | `學號 412345678` / `學生 王小明` / `系代碼 85` |
| **課程查詢** | `課程 資料結構` / `教師 王教授` / `課號 3141U0001` |
| **聯絡資訊** | `聯絡 資工系` / `緊急電話` |

## ⚙️ 環境變數

| 變數 | 說明 | 預設值 | 必填 |
|------|------|--------|------|
| `LINE_CHANNEL_ACCESS_TOKEN` | LINE Bot Access Token | - | ✅ |
| `LINE_CHANNEL_SECRET` | LINE Channel Secret | - | ✅ |
| `PORT` | HTTP 服務埠號 | `10000` | ❌ |
| `LOG_LEVEL` | 日誌等級 | `info` | ❌ |
| `SQLITE_PATH` | SQLite 資料庫路徑 | `/data/cache.db` | ❌ |

📖 **完整設定清單**: [internal/config/README.md](internal/config/README.md)

## 📊 監控

提供 Prometheus + Grafana + AlertManager 完整監控堆疊:

```bash
task compose:up  # 啟動監控服務
```

- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin123)
- AlertManager: http://localhost:9093

📖 **監控指標與告警設定**: [deployments/README.md](deployments/README.md)

## 🛠️ 開發指南

### 本機開發

```bash
# 1. Clone 專案
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go

# 2. 安裝 Task runner
go install github.com/go-task/task/v3/cmd/task@latest

# 3. 安裝依賴
go mod download

# 4. 設定環境變數
cp .env.example .env
# 編輯 .env 填入 LINE 憑證

# 5. 預熱快取（首次執行）
task warmup

# 6. 啟動開發服務
task dev
```

### 常用指令

```bash
task dev              # 開發模式執行
task build            # 編譯二進位
task test             # 執行測試
task lint             # 執行 linter
task ci               # 完整 CI (fmt + lint + test + build)
```

### 執行測試

```bash
# 執行所有測試
go test ./...

# 帶覆蓋率
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Race detector
go test -race ./...
```

### 新增 Bot 模組

1. 在 `internal/bot/` 建立新模組目錄
2. 實作 `Handler` 介面 (`CanHandle`, `HandleMessage`, `HandlePostback`)
3. 在 `internal/webhook/handler.go` 註冊模組
4. 撰寫單元測試

詳細架構說明請見 [docs/architecture.md](docs/architecture.md)

## 🐳 Docker 部署

### 使用預建映像 (推薦)

```bash
# 從 Docker Hub 拉取
docker pull garyellow/ntpu-linebot-go:latest

docker run -d \
  --name ntpu-linebot \
  -p 10000:10000 \
  -v ./data:/data \
  -e LINE_CHANNEL_ACCESS_TOKEN=your_token \
  -e LINE_CHANNEL_SECRET=your_secret \
  garyellow/ntpu-linebot-go:latest
```

### 本地建置

開發或客製化用途:

```bash
docker build -t garyellow/ntpu-linebot-go:local .

docker run -d \
  --name ntpu-linebot \
  -p 10000:10000 \
  -v ./data:/data \
  -e LINE_CHANNEL_ACCESS_TOKEN=your_token \
  -e LINE_CHANNEL_SECRET=your_secret \
  garyellow/ntpu-linebot-go:local
```

### 資料預熱

首次啟動建議預熱快取 (約 3-5 分鐘):

```bash
docker compose run --rm warmup
```

詳見 [cmd/warmup/README.md](cmd/warmup/README.md) 和 [deployments/README.md](deployments/README.md)

## 🔧 疑難排解

| 問題 | 解決方法 |
|------|----------|
| 服務無法啟動 | 檢查 `.env` 檔案是否正確設定 |
| 回應緩慢 | 執行 `task warmup` 預熱快取 |
| Webhook 驗證失敗 | 確認 `LINE_CHANNEL_SECRET` 正確 |

```bash
# 啟用詳細日誌
LOG_LEVEL=debug task dev

# 查看監控指標
curl http://localhost:10000/metrics
```

## 📚 文件

### 進階主題

- 📐 **[架構設計](docs/architecture.md)** - 系統架構與設計模式
- 🔄 **[Python 遷移說明](docs/migration.md)** - 為何選擇 Go

### 模組文件

各模組的詳細說明請見對應目錄:
- [Bot 模組](internal/bot/README.md) - 訊息處理與模組註冊
- [爬蟲系統](internal/scraper/README.md) - 限流、重試、Singleflight
- [資料層](internal/storage/README.md) - SQLite、Cache-First 策略
- [Webhook](internal/webhook/README.md) - LINE 事件處理
- [設定管理](internal/config/README.md) - 環境變數載入

## 🤝 貢獻指南

歡迎提交 Issue 和 Pull Request！

1. Fork 專案並建立功能分支
2. 開發與測試 (`task dev` / `task test`)
3. 執行完整 CI (`task ci`)
4. 遵循 [Conventional Commits](https://www.conventionalcommits.org/) 規範
5. 提交 Pull Request

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
