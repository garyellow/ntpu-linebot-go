# NTPU LineBot

<p align="center">
  <a href="https://lin.ee/QiMmPBv"><img src="https://img.shields.io/badge/LINE-加入好友-00C300?style=for-the-badge&logo=line&logoColor=white" alt="加入好友"></a>
</p>

<p align="center">
  <a href="https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/garyellow/ntpu-linebot-go/ci.yml?branch=main&label=CI&logo=github" alt="CI"></a>
  <a href="https://stats.uptimerobot.com/OqI3euBWoF"><img src="https://img.shields.io/uptimerobot/status/m795331349-bc2923cffedbf48e2a93c6f9?style=flat&label=Uptime" alt="Uptime"></a>
  <a href="https://goreportcard.com/report/github.com/garyellow/ntpu-linebot-go"><img src="https://goreportcard.com/badge/github.com/garyellow/ntpu-linebot-go" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/garyellow/ntpu-linebot-go"><img src="https://pkg.go.dev/badge/github.com/garyellow/ntpu-linebot-go.svg" alt="Go Reference"></a>
  <a href="https://go.dev/dl/"><img src="https://img.shields.io/github/go-mod/go-version/garyellow/ntpu-linebot-go?logo=go&logoColor=white" alt="Go Version"></a>
  <a href="https://hub.docker.com/r/garyellow/ntpu-linebot-go"><img src="https://img.shields.io/docker/pulls/garyellow/ntpu-linebot-go?logo=docker&logoColor=white" alt="Docker Pulls"></a>
  <a href="https://github.com/garyellow/ntpu-linebot-go/pkgs/container/ntpu-linebot-go"><img src="https://img.shields.io/badge/GHCR-latest-blue?logo=github&logoColor=white" alt="GHCR"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
</p>

國立臺北大學 LINE 聊天機器人，提供學號查詢、通訊錄查詢、課程查詢等功能。

> **💡 一般使用者**：直接[加入好友](#-立即使用)即可使用，無需任何設定！
>
> **🛠️ 開發者 / 自架需求**：請參閱[自架部署](#%EF%B8%8F-自架部署)章節。

---

## 📋 目錄

- [立即使用](#-立即使用)
- [功能介紹](#-功能介紹)
- [使用教學](#-使用教學)
- [隱私說明](#-隱私說明)
- [自架部署](#%EF%B8%8F-自架部署)
- [授權條款](#-授權條款)

---

## 📱 立即使用

**加入好友即可使用，完全免費！**

**LINE ID**: [@148wrcch](https://lin.ee/QiMmPBv)

<p align="center">
  <a href="https://lin.ee/QiMmPBv">
    <img src="add_friend/M_add_friend_button.png" alt="加入好友">
  </a>
</p>

<p align="center">
  <img src="add_friend/M_gainfriends_qr.png" alt="QR Code" width="200">
</p>

---

## ✨ 功能介紹

| 功能 | 說明 |
|------|------|
| 🔍 **學號查詢** | 依姓名或學號查詢學生資訊、系代碼對照、按學年度查詢 |
| 📞 **通訊錄查詢** | 校內人員聯絡方式（分機、Email、辦公室）、緊急電話 |
| 📚 **課程查詢** | 課程資訊（課號、教師、時間、地點）、課程名稱或教師姓名搜尋 |
| 🤖 **自然語言** | 支援口語化查詢，例如「我想找微積分的課」 |
| 🔮 **智慧搜尋** | 輸入「找課 + 描述」，根據課程大綱內容智慧匹配 |

---

## 💬 使用教學

### 🔍 學號查詢

| 查詢方式 | 輸入範例 | 說明 |
|---------|---------|------|
| 直接輸入學號 | `412345678` | 8-9 位數字 |
| 姓名查詢 | `學號 王小明` 或 `學生 王小明` | 支援關鍵字：學號/學生/姓名 |
| 系所查詢 | `系 資工` | 支援關鍵字：系/所/系所/科系 |
| 系所代碼 | `系代碼 85` | 支援關鍵字：系代碼/系所代碼 |
| 學年度查詢 | `學年 112` | 支援關鍵字：學年/年度/學年度 |
| 列出所有系代碼 | `所有系代碼` | 顯示完整對照表 |

> **📌 資料範圍**
> - 姓名查詢：101-113 學年度（warmup 快取範圍，可查詢 94-113）
> - 學年度查詢：94-113 學年度（即時爬取）
> - 114 學年度起因數位學苑 2.0 停用，無新資料

### 📚 課程查詢

| 查詢方式 | 輸入範例 | 說明 |
|---------|---------|------|
| 課程名稱 | `課程 資料結構` | 支援關鍵字：課/課程/科目 |
| 教師查詢 | `老師 王教授` | 支援關鍵字：師/老師/教師/教授 |
| 組合查詢 | `課程 微積分 王` | 同時搜尋課程名稱與教師 |
| 課號查詢 | `U0001` 或 `1131U0001` | 完整課號直接查詢，簡碼搜最近四學期 |
| 智慧搜尋 | `找課 線上實體混合` | 根據課程大綱內容智慧匹配 |

> **📌 查詢範圍**：最近四個學期（例如：114-1、113-2、113-1、112-2），智慧偵測學期資料可用性

### 📞 聯絡資訊

| 查詢方式 | 輸入範例 | 說明 |
|---------|---------|------|
| 緊急電話 | `緊急` 或 `緊急電話` | 顯示校安中心等緊急聯絡方式 |
| 單位查詢 | `聯絡 資工系` | 支援關鍵字：聯繫/聯絡/連繫/連絡 |
| 電話查詢 | `電話 王教授` | 支援關鍵字：電話/分機 |
| 信箱查詢 | `信箱 學務處` | 支援關鍵字：email/信箱 |

### 🤖 自然語言查詢

不需要記住指令格式，直接用口語描述即可：

| 你可以這樣說 | 機器人會理解為 |
|-------------|---------------|
| 我想找微積分的課 | 課程搜尋 |
| 王小明的學號是多少 | 學生查詢 |
| 資工系的電話 | 聯絡資訊查詢 |

> **💡 小提示**
> - 建議先嘗試關鍵字查詢（如 `課程 微積分`），速度較快
> - 若關鍵字無法匹配，系統會自動啟用 AI 理解你的意圖
> - 群組聊天中需 **@機器人** 才會回應

---

## 🔒 隱私說明

- **不儲存對話紀錄**：本機器人不會保存您的聊天內容
- **不蒐集個人資料**：僅處理您發送的查詢，不會追蹤或記錄用戶身份
- **資料來源公開**：所有查詢結果皆來自 NTPU 公開網站（數位學苑 2.0、課程查詢系統、校園聯絡簿）
- **快取機制**：為提升效能，會暫存公開網站的查詢結果並定期更新（課程/聯絡等），不會儲存對話內容或追蹤用戶
- **系所資訊說明**：學號查詢的系所資訊由學號推測，若學生已轉系可能有所不同

---

## 🛠️ 自架部署

<details>
<summary><strong>點擊展開開發者專區</strong></summary>

以下內容適用於想要自行架設的開發者。一般使用者直接加好友即可使用。

### 環境需求

- Go 1.25+（本機開發）
- Docker + Docker Compose（推薦）
- （可選）[Gemini API Key](https://aistudio.google.com/apikey)：啟用自然語言理解與智慧搜尋功能

### 取得 LINE Bot 憑證

1. 前往 [LINE Developers Console](https://developers.line.biz/console/)
2. 建立 Messaging API Channel
3. 取得 **Channel Secret**（Basic settings）
4. 發行 **Channel Access Token**（Messaging API）

### 方案 A：Docker Compose（推薦）

```bash
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go/deployments

cp .env.example .env
# 編輯 .env 填入 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET
# （可選）填入 GEMINI_API_KEY 或 GROQ_API_KEY 以啟用 AI 功能

docker compose up -d
```

**服務端點**：

| 端點 | 說明 |
|------|------|
| `/webhook` | LINE Webhook URL |
| `/livez` | Liveness (進程存活,不檢查外部依賴) |
| `/readyz` | Readiness (服務就緒,檢查 DB + cache) |
| `/metrics` | Prometheus 指標 |

> ⚠️ 本機測試需使用 [ngrok](https://ngrok.com/) 等工具將 localhost 轉發至公網。

### 方案 B：本機開發

```bash
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go

go mod download

cp .env.example .env
# 編輯 .env 填入 LINE 憑證
# Windows: DATA_DIR=./data | Linux/Mac: DATA_DIR=/data

go run ./cmd/server
```

### 開發指令

使用 [Task](https://taskfile.dev/) 執行常用指令：

```bash
task dev              # 啟動開發伺服器
task test             # 執行測試
task test:coverage    # 測試覆蓋率報告
task lint             # 程式碼檢查
task ci               # 完整 CI 流程
```

### 監控（可選）

部署自動包含 Prometheus + Grafana + Alertmanager：

```bash
task access:up        # 開啟監控儀表板
task access:down      # 關閉監控儀表板
```

| 服務 | 網址 | 帳密 |
|------|------|------|
| Grafana | `http://localhost:3000` | admin / 請設定 `GRAFANA_PASSWORD` |
| Prometheus | `http://localhost:9090` | - |
| Alertmanager | `http://localhost:9093` | - |

### 疑難排解

| 問題 | 解決方法 |
|------|----------|
| 服務無法啟動 | 檢查 `.env` 是否正確設定 LINE 憑證 |
| 首次回應緩慢 | 服務啟動後會在背景預熱快取（約 5-10 分鐘） |
| Webhook 驗證失敗 | 確認 `LINE_CHANNEL_SECRET` 正確 |
| Docker 權限錯誤 | 執行 `docker compose down && rm -rf ./data && docker compose up -d` |

啟用詳細日誌：

```bash
LOG_LEVEL=debug go run ./cmd/server
```

### 更多文件

- 📐 [架構設計](docs/architecture.md)
- 📊 [監控設定](deployments/README.md)
- 🔧 [環境變數](.env.example)

</details>

---

## 📄 授權條款

本專案採用 [MIT License](LICENSE) 授權。

---

<p align="center">
  Made with ❤️ by NTPU Students
</p>
