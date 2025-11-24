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
- [使用範例](#-使用範例)
- [開發指南](#-開發指南)
- [監控](#-監控)
- [疑難排解](#-疑難排解)

## ✨ 功能特色

### 核心功能
- 🔍 **學號查詢**: 依姓名或學號查詢學生資訊、系代碼對照、按學年度查詢
- 📞 **通訊錄查詢**: 校內人員聯絡方式（分機、Email、辦公室）、緊急電話（含校安中心）
- 📚 **課程查詢**: 課程資訊（課號、教師、時間、地點、大綱連結）、教師授課課程查詢

### 技術特色
- 🚀 **高效能**: Goroutine 並發處理，Worker Pool 限流保護
- 💾 **智能快取**: SQLite WAL 模式，7 天 TTL，Cache-First 策略
- 🛡️ **反爬蟲**: User-Agent 輪替、Singleflight 去重、指數退避重試
- 📊 **可觀測性**: Prometheus + Grafana 監控儀表板，結構化日誌
- 🔒 **安全性**: Webhook 簽章驗證、Rate Limiting、SQL Injection 防護

## 📞 加入好友

**LINE ID**: [@148wrcch](https://lin.ee/QiMmPBv)

[![加入好友](add_friend/S_add_friend_button.png)](https://lin.ee/QiMmPBv)

![QR Code](add_friend/S_gainfriends_qr.png)

## 🚀 快速開始

### 方案 A: Docker Compose (推薦)

```bash
# 1. Clone 專案
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go/deployments

# 2. 設定環境變數
cp .env.example .env
# 編輯 .env 填入你的 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET

# 3. 啟動服務（server 會自動在背景預熱快取）
docker compose up -d
```

**服務端點**：
- Webhook: http://localhost:10000/callback （設定為 LINE Webhook URL）
- Liveness: http://localhost:10000/healthz
- Readiness: http://localhost:10000/ready
- Metrics: http://localhost:10000/metrics

**注意**：若本機測試，需使用 ngrok 或 localtunnel 等工具將 localhost 轉發至公網 IP。

### 方案 B: 本機開發

**前置需求**: Go 1.25+

```bash
# 1. Clone 專案
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go

# 2. 安裝依賴
go mod download

# 3. 設定環境變數
cp .env.example .env
# 編輯 .env 填入你的 LINE 憑證
# Windows: SQLITE_PATH=./data/cache.db
# Linux/Mac: SQLITE_PATH=/data/cache.db

# 4. 啟動服務（會自動在背景預熱快取）
go run ./cmd/server
```

### 取得 LINE Bot 憑證

1. 前往 [LINE Developers Console](https://developers.line.biz/console/)
2. 建立 Messaging API Channel
3. 取得 **Channel Secret** (Basic settings 頁面)
4. 發行 **Channel Access Token** (Messaging API 頁面)

## 💬 使用範例

### 學號查詢

| 查詢方式 | 指令範例 | 說明 |
|---------|---------|------|
| 直接輸入學號 | `412345678` | 支援 8-9 位學號直接查詢 |
| 關鍵字查詢 | `學號 412345678` / `學生 王小明` | 使用關鍵字 + 查詢內容 |
| 部分姓名 | `學生 小明` / `學生 王` | 支援姓氏或名字查詢 |
| 系所資訊 | `系代碼 85` / `科系 資工` | 查詢系代碼對照 |
| 年度查詢 | `學年 112` | 按學年度查詢學生名單（支援即時爬取） |
| 所有系代碼 | `所有系代碼` | 列出所有系所代碼 |

**資料範圍**：
- ✅ 101-112 學年度資料（完整）
- ℹ️ 113 學年度起無新資料（數位學苑 2.0 停用）
- 💡 支援按學年度查詢特定系所學生名單

### 課程查詢

| 查詢方式 | 指令範例 | 說明 |
|---------|---------|------|
| 直接輸入課號 | `3141U0001` | 支援課號直接查詢（年度+學期+課號）|
| 課程名稱 | `課程 資料結構` / `微積分課` | 搜尋課程名稱（前後皆可）|
| 教師查詢 | `教師 王教授` / `李老師` | 搜尋教師授課課程（可只輸入姓氏）|

**查詢範圍**：系統會自動搜尋當前學期及上一學期的課程資料（例如：2月時搜尋該學年下學期及上學期）

### 聯絡資訊

| 查詢方式 | 指令範例 | 說明 |
|---------|---------|------|
| 緊急電話 | `緊急` / `緊急電話` | 📱 顯示三峽/台北校區緊急聯絡電話（含校安中心、警消）|
| 單位查詢 | `聯絡 資工系` / `聯繫 圖書館` | 查詢單位聯絡方式（電話、分機、地點）|
| 人員查詢 | `聯絡 王` / `電話 李教授` | 查詢特定人員聯絡資訊 |
| 部分關鍵字 | `分機 學務` / `信箱 資工` | 支援模糊搜尋 |

**提示**：聯絡資訊為即時抓取，如查無結果可嘗試使用單位全名或簡稱

## 📊 監控

Docker Compose 部署自動包含 Prometheus + Grafana + AlertManager 監控堆疊。

### 開啟監控儀表板

**Windows**:
```powershell
cd deployments
.\access.cmd up
```

**Linux / Mac**:
```bash
cd deployments
./access.sh up
```

**使用 Task (通用)**:
```bash
task access:up
```

### 存取網址
- **Grafana**: http://localhost:3000 (帳號: admin / 密碼: admin123)
- **Prometheus**: http://localhost:9090
- **AlertManager**: http://localhost:9093

### 關閉監控儀表板
```bash
task access:down
# 或 Windows: .\deployments\access.cmd down
# 或 Linux/Mac: ./access.sh down
```

## 🛠️ 開發指南

### 使用 Task Runner（推薦）

安裝 Task：
```bash
go install github.com/go-task/task/v3/cmd/task@latest
```

常用指令：
```bash
task dev              # 啟動開發服務
task warmup           # 預熱快取
task test             # 執行測試
task test:coverage    # 測試覆蓋率報告
task lint             # 程式碼檢查
task fmt              # 格式化程式碼
task ci               # 完整 CI (fmt + lint + test)
```

### 使用原生 Go 指令

```bash
go run ./cmd/server                                     # 啟動服務
go test ./...                                           # 執行測試
go test -race -coverprofile=coverage.out ./...          # 測試 + 覆蓋率
go run ./cmd/warmup -reset                              # 手動預熱（選用）
```

### Docker 操作

```bash
# Docker Compose
cd deployments
docker compose up -d                     # 啟動所有服務
docker compose logs -f ntpu-linebot      # 查看日誌
docker compose down                      # 停止服務

# 更新至最新版本
task compose:update                      # 使用 Task
# 或 Windows: .\update.cmd
# 或 Linux/Mac: ./update.sh

# 單一容器
docker pull garyellow/ntpu-linebot-go:latest
docker run -d --name ntpu-linebot \
  -p 10000:10000 -v ./data:/data \
  -e LINE_CHANNEL_ACCESS_TOKEN=your_token \
  -e LINE_CHANNEL_SECRET=your_secret \
  garyellow/ntpu-linebot-go:latest
```

## 🔧 疑難排解

| 問題 | 解決方法 |
|------|----------|
| 服務無法啟動 | 檢查 `.env` 檔案是否正確設定 LINE 憑證 |
| 首次啟動回應緩慢 | 服務啟動後會在背景預熱快取（約 5-10 分鐘），期間首次查詢可能較慢 |
| Webhook 驗證失敗 | 確認 `LINE_CHANNEL_SECRET` 正確 |
| Docker 權限錯誤 | `docker compose down && rm -rf ./data && docker compose up -d` |

**啟用詳細日誌**：
```bash
LOG_LEVEL=debug go run ./cmd/server
```

## 📚 文件

- 📐 [架構設計](docs/architecture.md) - 系統設計與實作細節
- 🔄 [Python 遷移說明](docs/migration.md) - 為何從 Python 遷移到 Go
- 📊 [監控設定](deployments/README.md) - Prometheus/Grafana 配置
- 🔧 [配置說明](internal/config/README.md) - 環境變數完整清單

## 📄 授權條款

本專案採用 [MIT License](LICENSE) 授權。

**重要提示**:
- 本專案僅供學術研究與教育用途
- 請遵守 NTPU 網站使用條款

---

Made with ❤️ by NTPU Students
