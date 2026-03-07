# NTPU 小工具

<div align="center">

[![服務狀態](https://img.shields.io/uptimerobot/status/m802132556-5a95fc71d4f9260bdcd036db?logo=line&logoColor=white)](https://ntpubot-status.garyellow.app/)
[![CI](https://img.shields.io/github/actions/workflow/status/garyellow/ntpu-linebot-go/ci.yml?branch=main&label=CI&logo=github)](https://github.com/garyellow/ntpu-linebot-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/garyellow/ntpu-linebot-go)](https://goreportcard.com/report/github.com/garyellow/ntpu-linebot-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/garyellow/ntpu-linebot-go.svg)](https://pkg.go.dev/github.com/garyellow/ntpu-linebot-go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/garyellow/ntpu-linebot-go?logo=go&logoColor=white)](https://go.dev/dl/)
[![Docker Version](https://img.shields.io/docker/v/garyellow/ntpu-linebot-go?sort=semver&logo=docker&logoColor=white)](https://hub.docker.com/r/garyellow/ntpu-linebot-go)
[![License](https://img.shields.io/github/license/garyellow/ntpu-linebot-go)](https://opensource.org/licenses/MIT)
[![Ko-fi](https://img.shields.io/badge/Ko--fi-%E8%AB%8B%20AI%20%E5%90%83%E9%BB%9E%20Token-72a4f2?logo=ko-fi&logoColor=white)](https://ko-fi.com/garyellow)

[![加入好友](https://img.shields.io/badge/LINE-加入好友-00C300?style=for-the-badge&logo=line&logoColor=white)](https://line.me/R/ti/p/@148wrcch)

</div>

國立臺北大學 LINE 聊天機器人「NTPU 小工具」，提供課程查詢、智慧找課、學號查詢、學程查詢、校內聯絡資訊、緊急電話與使用額度查詢。

它的定位很單純：把 NTPU 常用的公開資訊整理成一個在 LINE 裡就能直接查的工具。對 LINE 使用者來說，加好友就能用；對開發者來說，後半段也保留了自架、維運與監控背景。

> [!IMPORTANT]
> 本專案是社群維護的開源工具，並非學校官方資訊系統，也不是 LINE 官方服務。查詢結果主要來自 NTPU 公開網站與公開資料整理；若來源網站資料不完整、停止提供或改版，結果也可能受到影響。

---

## 立即使用

LINE ID：[ @148wrcch ](https://line.me/R/ti/p/@148wrcch)

<p align="center">
  <a href="https://line.me/R/ti/p/@148wrcch">
    <img src="add_friend/M_add_friend_button.png" alt="加入好友" width="200">
  </a>
</p>

<p align="center">
  <img src="add_friend/M_gainfriends_qr.png" alt="加入好友 QR Code" width="200">
</p>

相關連結：

- [服務狀態頁](https://ntpubot-status.garyellow.app/)
- [問題回報 / 功能建議](https://github.com/garyellow/ntpu-linebot-go/issues/new/choose)
- [Go 套件文件](https://pkg.go.dev/github.com/garyellow/ntpu-linebot-go)

---

## LINE 使用者看這裡

| 功能 | 說明 |
|------|------|
| 學號查詢 | 依姓名、學號、學年度、系所或系代碼查學生資訊 |
| 課程查詢 | 查近期課程、指定學年課程與較早學期課程 |
| 智慧找課 | 不知道課名時，可用描述依課綱內容找課 |
| 學程查詢 | 查學程列表、學程內容與課程對應學程 |
| 聯絡資訊 | 查校內單位、老師聯絡方式與緊急電話 |
| 配額查詢 | 查看訊息額度與 AI 功能額度 |

### 最常用的查法

| 類別 | 直接傳給 Bot | 說明 |
|------|--------------|------|
| 學號 | `學號 王小明` | 依姓名查學號 |
| 學號 | `412345678` | 直接輸入學號查學生 |
| 學號 | `系 資工`、`系代碼 85` | 查系所或系代碼 |
| 課程 | `課程 資料結構` | 查最近學期的課 |
| 課程 | `課程 110 微積分` | 查指定學年課程 |
| 課程 | `更多學期 微積分` | 往前擴展查歷史學期 |
| 智慧找課 | `找課 我想學資料分析` | 依課綱內容找課 |
| 學程 | `學程列表`、`學程 人工智慧` | 查學程與學程課程 |
| 聯絡 | `聯絡 資工系`、`教授 王小明` | 查單位或老師聯絡資訊 |
| 緊急 | `緊急` | 查緊急聯絡電話 |
| 額度 | `配額`、`用量`、`額度` | 查看目前額度 |
| 說明 | `使用說明` | 顯示完整操作說明 |

> [!NOTE]
> 關鍵字查詢通常最快也最穩定。若部署者有啟用 AI 功能，也可以直接用口語輸入，例如「我想找微積分的課」或「資工系電話」。

### 使用前先知道

- 這不是學校官方服務，而是整理公開資訊的社群工具。
- 學號、課程、聯絡資訊都會受原始資料來源完整度影響。
- `找課` 與自然語言理解屬於選用 AI 功能，並會消耗 AI 額度。

---

## 常見問題

<details>
<summary><strong>一定要記住指令格式嗎？</strong></summary>
<br />

不一定。最穩定的方式是直接輸入關鍵字，例如 `課程 微積分`、`學號 王小明`、`聯絡 資工系`。

如果部署者有啟用 AI 功能，也可以直接用口語輸入，例如「王小明的學號」或「我想找資料分析相關的課」。
</details>

<details>
<summary><strong>課程查詢、找課、更多學期有什麼差別？</strong></summary>
<br />

- `課程 關鍵字`：查最近 2 學期，適合你已經知道課名、老師或明確關鍵字
- `更多學期 關鍵字`：把查詢範圍往前延伸 2 個歷史學期
- `找課 描述`：依課綱內容與語意找課，適合你只知道想學什麼、不知道課名

如果你已經知道課名或老師，通常直接用 `課程` 會更快。
</details>

<details>
<summary><strong>為什麼我查不到某位同學，或 114 學年度以後沒有資料？</strong></summary>
<br />

多半不是 Bot 壞掉，而是原始資料來源本身已經沒有完整提供。

- 94 到 112 學年度資料相對完整
- 113 學年度資料零散
- 114 學年度起沒有新資料可供查詢

如果你查的是姓名，也可能因為同名、簡稱或資料不完整而沒有結果，這時建議改用學號、學年度或更完整的姓名再試一次。
</details>

<details>
<summary><strong>學號與系所資訊的範圍是什麼？</strong></summary>
<br />

- 姓名查詢的完整快取資料主要涵蓋 101 到 112 學年度
- 直接查學號可查到 94 到 112 學年度的完整資料
- 部分系所資訊是依學號規則推測，可能與實際資料不完全一致

如果你需要更穩定的結果，直接輸入學號通常比姓名查詢更準。
</details>

<details>
<summary><strong>課程與課綱資料的範圍是什麼？</strong></summary>
<br />

- `課程` 會優先顯示最近 2 個學期
- `更多學期` 會往前再查第 3 到第 4 個學期
- 指定學年課程可查到 90 學年度以後
- `找課` 主要依據近期已快取的課程大綱內容，不保證每門歷史課都有完整課綱可比對

如果你知道課名，直接用 `課程` 通常會比 `找課` 更快。
</details>

<details>
<summary><strong>這個 Bot 會保存我的聊天內容嗎？</strong></summary>
<br />

不會以對話紀錄的形式長期保存你的聊天內容，也不會把你的 LINE 訊息當成資料庫累積。

系統快取的是來自公開網站的資料，例如課程、聯絡簿、學程、課綱等，目的在於提升查詢速度與穩定性。
</details>

<details>
<summary><strong>我的 AI 額度是怎麼算的？</strong></summary>
<br />

每則訊息都會消耗訊息額度；自然語言理解與 `找課` 智慧搜尋會再消耗 AI 額度。

你可以輸入 `配額` 查看剩餘額度，輸入 `額度說明` 查看更細的扣點規則。
</details>

<details>
<summary><strong>這是 NTPU 官方服務嗎？</strong></summary>
<br />

不是。這是社群維護的開源專案，目的是把公開資訊整理成較容易使用的查詢介面。

如果學校網站改版、來源資料缺漏或停止維護，查詢結果也會同步受影響。
</details>

<details>
<summary><strong>我想自己架一個版本，可以嗎？</strong></summary>
<br />

可以。這個專案支援直接用 Go 啟動、Docker 單容器執行，以及 Docker Compose 部署；如果你是開發者，可以直接往下看「自行部署」。
</details>

---

## 隱私與資料來源

| 項目 | 說明 |
|------|------|
| 對話紀錄 | 不以使用者聊天紀錄作為長期資料保存 |
| 個人資料蒐集 | 不以 Bot 身分額外建立使用者個資檔案 |
| 資料來源 | 課程查詢系統、數位學苑 2.0、校園聯絡簿與其他公開資料 |
| 快取用途 | 只用來減少重複抓取、提升速度與穩定性 |
| 開源透明 | 程式碼公開，可自行檢視功能與資料流向 |

> [!NOTE]
> 智慧搜尋使用的是已快取的課綱內容與查詢擴展，不會因為你問了一句話，就把你的聊天內容寫成永久知識庫。

---

## 自行部署

如果你想自己架設一個版本，下面先給最短路徑；更完整的部署背景、維運選項與多節點同步細節也都保留在後面。

### 需要準備什麼

- Go 1.26.1，或 Docker / Docker Compose
- LINE Messaging API 的 Channel Secret 與 Channel Access Token
- 若要啟用 AI 功能，需設定 `NTPU_LLM_ENABLED=true` 並提供至少一組 LLM API Key

### 快速開始

#### 方式 1：本機開發

```bash
git clone https://github.com/garyellow/ntpu-linebot-go.git
cd ntpu-linebot-go
cp .env.example .env
# 編輯 .env，填入 LINE 憑證
task dev
```

#### 方式 2：Docker Compose

```bash
cd deployments
cp .env.example .env
# 編輯 .env，填入 LINE 憑證
docker compose up -d
```

#### 方式 3：Docker 單容器

```bash
docker run -d -p 10000:10000 \
  -e NTPU_LINE_CHANNEL_ACCESS_TOKEN=xxx \
  -e NTPU_LINE_CHANNEL_SECRET=xxx \
  -v ./data:/data \
  garyellow/ntpu-linebot-go:latest
```

### 常用端點

| 端點 | 用途 |
|------|------|
| `/webhook` | LINE Webhook 接收點 |
| `/livez` | 存活檢查 |
| `/readyz` | 就緒檢查 |
| `/metrics` | Prometheus 指標 |

> [!TIP]
> 在本機測試 LINE Webhook 時，通常還需要 ngrok 之類的工具把 localhost 暴露到公網。

### 開發常用指令

```bash
task dev
task test
task lint
task build
task compose:up
```

### 進階部署與維運背景

<details>
<summary><strong>展開完整背景與選用功能</strong></summary>
<br />

#### LINE 憑證取得方式

1. 前往 [LINE Developers Console](https://developers.line.biz/console/)
2. 建立 Messaging API Channel
3. 取得 Channel Secret
4. 發行 Channel Access Token

#### AI 功能

設定 `NTPU_LLM_ENABLED=true`，再搭配至少一組 API Key：

- `NTPU_GEMINI_API_KEY`
- `NTPU_GROQ_API_KEY`
- `NTPU_CEREBRAS_API_KEY`
- 或自訂 OpenAI-compatible endpoint

#### Metrics、Sentry、Better Stack

- Metrics 保護：`NTPU_METRICS_AUTH_ENABLED=true`
- Sentry：`NTPU_SENTRY_ENABLED=true`
- Better Stack：`NTPU_BETTERSTACK_ENABLED=true`

如果你是單機或簡單部署，這些都不是必填；如果你要長期營運、觀察錯誤或集中收 log，再啟用即可。

#### 多節點與 R2 快照同步

若你是多節點或多容器部署，建議啟用 R2 快照同步：

- `NTPU_R2_ENABLED=true`
- `NTPU_R2_ACCOUNT_ID`
- `NTPU_R2_ACCESS_KEY_ID`
- `NTPU_R2_SECRET_ACCESS_KEY`
- `NTPU_R2_BUCKET_NAME`

啟用後會有這些效果：

- 啟動時可先下載 SQLite 快照，減少冷啟動刷新時間
- 多節點可共享刷新進度與清理排程
- follower 可輪詢新快照並熱切換資料
- 不建議多容器直接共用同一個 SQLite 檔案，較建議透過 R2 同步

#### 多點部署識別

可選設定：

- `NTPU_SERVER_NAME`
- `NTPU_INSTANCE_ID`

這些主要用在 log、metrics 與 Sentry 上，幫你分辨不同節點或不同容器實例。

#### 更多環境變數與細節

更完整的設定請看：

- [環境變數範例](.env.example)
- [Docker Compose 說明](deployments/README.md)
- [API 文件](docs/API.md)
- [架構說明](docs/architecture.md)

</details>

### 文件索引

- [環境變數範例](.env.example)
- [Docker Compose 說明](deployments/README.md)
- [API 文件](docs/API.md)
- [架構說明](docs/architecture.md)
- [文件索引](docs/README.md)

---

## 貢獻

歡迎用以下方式幫這個專案變得更好：

- 開 [Issue](https://github.com/garyellow/ntpu-linebot-go/issues) 回報問題
- 提出 [Pull Request](https://github.com/garyellow/ntpu-linebot-go/pulls)
- 協助補充文件、修正文案、改善查詢體驗

---

## 贊助

覺得好用的話，請我的 AI 吃一點 Token 吧。

<a href="https://ko-fi.com/garyellow">
  <img src="https://img.shields.io/badge/Ko--fi-請 AI 吃點 Token-72a4f2?style=for-the-badge&logo=ko-fi&logoColor=white" alt="在 Ko-fi 上贊助開發者" height="36" />
</a>

你的支持會直接拿來支付這個開源專案的開發、維護、部署與 AI Token 成本。

---

## 授權

本專案採用 [MIT License](LICENSE) 授權。
