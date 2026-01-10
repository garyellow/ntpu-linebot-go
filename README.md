# NTPU LineBot

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

<p align="center">
  <a href="https://line.me/R/ti/p/@148wrcch"><img src="https://img.shields.io/badge/LINE-加入好友-00C300?style=for-the-badge&logo=line&logoColor=white" alt="加入好友"></a>
</p>

國立臺北大學 LINE 聊天機器人「NTPU 小工具」，提供學號查詢、通訊錄查詢、課程查詢等功能。

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

**LINE ID**: [@148wrcch](https://line.me/R/ti/p/@148wrcch)

<p align="center">
  <a href="https://line.me/R/ti/p/@148wrcch">
    <img src="add_friend/M_add_friend_button.png" alt="加入好友" width="200">
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
| 🎯 **學程查詢** | 瀏覽所有學程、查詢學程課程、查看課程相關學程 |
| 🤖 **自然語言** | 支援口語化查詢，例如「我想找微積分的課」 |
| 🔮 **智慧搜尋** | 輸入「找課 + 描述」，根據課程大綱內容智慧匹配 |

---

## 💬 使用教學

### 🔍 學號/姓名查詢

| 查詢方式 | 輸入範例 | 說明 |
|---------|---------|------|
| 學號查詢 | `412345678` | 直接輸入 8-9 位數字 |
| 姓名查詢 | `學號 王小明` | **關鍵字**：學號/學生/姓名/id |
| 系所名稱 | `系名 資工` | **關鍵字**：系名/系/系所<br>查詢系代碼（含碩博士班）|
| 系代碼 | `系代碼 85` | **關鍵字**：系代碼<br>查詢系名稱 |
| 學年度 | `學年 112` | **關鍵字**：學年<br>按學年度搜尋學生 |
| 所有系代碼 | `所有系代碼` | 顯示大學部系代碼對照表 |

> **📌 資料範圍**
> - 姓名查詢：日間部大學部 101-112 學年度（完整資料）
> - 學號查詢：94-112 學年度（完整資料）
> - ⚠️ 113 學年度資料極不完整（僅極少數學生）
> - ⚠️ 114 學年度起因數位學苑 2.0 停用，無新資料

> **💡 系代碼說明**
> - 大學部與碩博士班使用**不同的代碼系統**
> - 例如：資工系大學部 `85`，碩士班 `83`
> - 輸入 `系 資工` 可查看所有學制的代碼

### 📚 課程查詢

| 查詢方式 | 輸入範例 | 說明 |
|---------|---------|------|
| 課程名稱 | `課程 資料結構` | **關鍵字**：課程/課/科目<br>搜尋近 2 學期課程 |
| 課號查詢 | `U0001`<br>`1131U0001` | 直接輸入課號<br>搜尋近 2 學期課程 |
| 智慧搜尋 | `找課 線上實體混合` | **關鍵字**：找課/找課程/搜課<br>根據課程大綱內容匹配 |
| 指定學年 | `課程 110 微積分` | 指定學年度（90 年至今）|
| 更多學期 | `更多學期 微積分` | **關鍵字**：更多學期/更多課程/歷史課程<br>擴展搜尋第 3-4 學期 |

> **📌 查詢範圍**
> - 一般搜尋：近 2 學期（依資料庫實際資料判斷）
> - 更多學期：第 3-4 學期
> - 智慧搜尋：近 2 學期（根據課程大綱）

### 🎯 學程查詢

| 查詢方式 | 輸入範例 | 說明 |
|---------|---------|------|
| 學程列表 | `學程列表`<br>`所有學程` | **關鍵字**：學程列表/所有學程<br>顯示所有可選學程 |
| 搜尋學程 | `學程 人工智慧` | **關鍵字**：學程/program<br>依名稱模糊搜尋 |

> **💡 功能特色**
> - 點擊學程可查看課程（必修在前、選修在後）
> - 課程頁面有「相關學程」按鈕
> - 支援模糊搜尋（如「智財」→「智慧財產學程」）
> - 學程課程顯示近 2 學期（與精確搜尋範圍一致）

### 📞 聯絡資訊

| 查詢方式 | 輸入範例 | 說明 |
|---------|---------|------|
| 緊急電話 | `緊急` | 校安中心、派出所、醫院 |
| 單位/人員 | `聯絡 資工系`<br>`教授 王小明` | **關鍵字**：聯絡/聯繫/連繫/電話/分機/信箱/找老師/老師/教授<br>支援組織單位與教師個人查詢 |

> **💡 功能特色**
> - 組織單位可點擊「查看成員」
> - 電話/信箱可直接撥打或複製
> - 教師聯絡可查看「授課課程」

### 🤖 自然語言查詢

不需記住指令格式，直接用口語描述：

| 你可以這樣說 | NTPU 小工具會理解為 |
|-------------|---------------|
| 我想找微積分的課 | 課程搜尋 |
| 人工智慧學程有什麼課 | 學程查詢 |
| 王小明的學號 | 學生查詢 |
| 資工系電話 | 聯絡資訊 |
| 找王小明老師 | 教師聯絡資訊 |

> **💡 提示**：關鍵字查詢速度較快，建議優先使用

---

## 🔒 隱私說明

- **不儲存對話紀錄**：NTPU 小工具不會保存您的聊天內容
- **不蒐集個人資料**：僅處理您發送的查詢，不會追蹤或記錄用戶身份
- **資料來源公開**：所有查詢結果皆來自 NTPU 公開網站（數位學苑 2.0、課程查詢系統、校園聯絡簿）
- **快取機制**：為提升效能，會暫存公開網站的查詢結果並定期更新，不會儲存對話內容或追蹤用戶
- **系所資訊說明**：學號查詢的系所資訊由學號推測，若有轉系等情況可能與實際不符

---

## 🛠️ 自架部署

<details>
<summary><strong>點擊展開開發者專區</strong></summary>

以下內容適用於想要自行架設的開發者。一般使用者直接加好友即可使用。

### 執行方式

| 類別 | 模式 | 說明 |
|------|------|------|
| **僅 Bot** | Go 直接執行 | `go run ./cmd/server` |
| | Docker Container | `docker run garyellow/ntpu-linebot-go` |
| **Bot + 監控** | Full Stack | Bot + 監控同 Docker 網路 |
| | Monitoring Only | Bot 在雲端，監控在本地 |

> 完整部署說明請參閱 [deployments/README.md](deployments/README.md)。

### 環境需求

- Go 1.25+（Go 直接執行）
- Docker + Docker Compose（容器部署）
- LLM API Key（可選，啟用 AI 功能）：
  - [Gemini](https://aistudio.google.com/apikey)
  - [Groq](https://console.groq.com/keys)
  - [Cerebras](https://cloud.cerebras.ai/)

### 取得 LINE Bot 憑證

1. 前往 [LINE Developers Console](https://developers.line.biz/console/)
2. 建立 Messaging API Channel
3. 取得 **Channel Secret**（Basic settings）
4. 發行 **Channel Access Token**（Messaging API）

### 快速開始

**Go 直接執行：**

```bash
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go

cp .env.example .env
# 編輯 .env 填入 LINE 憑證

task dev
```

**Docker Container：**

```bash
# Distroless（推薦）
docker run -d -p 10000:10000 \
  -e LINE_CHANNEL_ACCESS_TOKEN=xxx \
  -e LINE_CHANNEL_SECRET=xxx \
  -v ./data:/data \
  garyellow/ntpu-linebot-go:latest

# Alpine（debug 用）- 進入容器 shell
docker run -it --rm garyellow/ntpu-linebot-go:alpine sh
```

**Full Stack（含監控）：**

```bash
cd deployments/full
cp .env.example .env
docker compose up -d
```

### 服務端點

| 端點 | 說明 |
|------|------|
| `/webhook` | LINE Webhook URL |
| `/livez` | Liveness |
| `/readyz` | Readiness |
| `/metrics` | Prometheus 指標 |

> ⚠️ 本機測試需使用 [ngrok](https://ngrok.com/) 等工具將 localhost 轉發至公網。

### 開發指令

```bash
task dev              # 啟動開發伺服器（預設 debug 日誌）
task test             # 執行測試
task lint             # 程式碼檢查
task ci               # 完整 CI 流程
```

### 更多文件

- 📐 [架構設計](docs/architecture.md)
- 📊 [部署說明](deployments/README.md)
- 🔧 [環境變數](.env.example)

</details>

---

## 📄 授權條款

本專案採用 [MIT License](LICENSE) 授權。

---

<p align="center">
  Made with ❤️ by NTPU Students
</p>
