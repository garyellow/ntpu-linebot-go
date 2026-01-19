# Deployments

部署配置與執行方式說明。

## 執行方式總覽

| 類別 | 模式 | 說明 | 使用場景 |
|------|------|------|----------|
| **僅 Bot** | [Go 直接執行](#go-直接執行) | `go run ./cmd/server` | 開發測試 |
| | [Docker Container](#docker-container) | `docker run` 單獨跑 Bot | 簡單部署 |
| **Bot + 監控** | [Full Stack](#full-stack) | Bot + 監控三件套同網路 | 單機完整部署 |
| | [Monitoring Only](#monitoring-only) | Bot 在雲端，本地只跑監控 | 雲端部署 + 本地監控 |

---

# 僅 Bot

不含監控堆疊，適合開發測試或簡單部署。

## Go 直接執行

開發或測試時直接執行 Go 程式。

### 使用 Task（推薦）

```bash
cp .env.example .env
# 編輯 .env 填入 LINE 憑證

task dev
```

### 直接執行

```bash
# 設定必要環境變數
export LINE_CHANNEL_ACCESS_TOKEN=xxx
export LINE_CHANNEL_SECRET=xxx

# Windows 需設定 DATA_DIR（預設 /data 在 Windows 無效）
# Windows: DATA_DIR=./data

go run ./cmd/server
```

### 可選設定

```bash
# 啟用詳細日誌
LOG_LEVEL=debug go run ./cmd/server

# 保護 /metrics 端點
METRICS_USERNAME=prometheus METRICS_PASSWORD=xxx go run ./cmd/server
```

---

## Docker Container

單獨執行 Bot 容器，不含監控。

### 快速啟動

```bash
docker run -d \
  -p 10000:10000 \
  -e LINE_CHANNEL_ACCESS_TOKEN=xxx \
  -e LINE_CHANNEL_SECRET=xxx \
  -v ./data:/data \
  garyellow/ntpu-linebot-go:latest
```

### 可用映像標籤

提供兩種映像變體：

| 變體 | Base Image | 適用場景 |
|------|------------|----------|
| **Distroless（預設）** | `gcr.io/distroless/static-debian13` | 生產環境（最小攻擊面） |
| **Alpine** | `alpine:3.23` | 需要 shell/debug 的特殊場景 |

| 標籤 | Distroless | Alpine |
|------|------------|--------|
| 最新穩定版 | `latest` | `alpine` |
| 特定版本 | `1.2.3` | `1.2.3-alpine` |
| Minor 版本 | `1.2` | `1.2-alpine` |
| Major 版本 | `1` | `1-alpine` |
| 開發版 | `dev` | `dev-alpine` |

映像同時發布至：
- Docker Hub: `garyellow/ntpu-linebot-go`
- GHCR: `ghcr.io/garyellow/ntpu-linebot-go`

> **建議**：生產環境使用 Distroless（無後綴標籤），僅在需要進入容器 debug 時使用 Alpine。

### 環境變數

詳見 [.env.example](../.env.example)。

#### 必填（LINE Bot）

| 變數 | 說明 |
|------|------|
| `LINE_CHANNEL_ACCESS_TOKEN` | LINE Bot Access Token |
| `LINE_CHANNEL_SECRET` | LINE Bot Channel Secret |

#### 日誌整合（可選）

| 變數 | 說明 |
|------|------|
| `BETTERSTACK_SOURCE_TOKEN` | Better Stack Source Token（空字串=不啟用） |
| `BETTERSTACK_ENDPOINT` | Better Stack Ingesting Endpoint |

### 服務端點

| 端點 | 說明 |
|------|------|
| `/webhook` | LINE Webhook URL |
| `/livez` | Liveness（進程存活）|
| `/readyz` | Readiness（服務就緒 - 可配置等待 Warmup）|
| `/metrics` | Prometheus 指標 |

### 常見問題

**Q: 為什麼剛啟動時 `/webhook` 回傳 503？**
A: 當 `WAIT_FOR_WARMUP=true` 時，服務會等待 initial warmup 完成（約 2-5 分鐘）：
- `/readyz` 回傳 503
- `/webhook` 回傳 503（LINE 會自動重試）
- 這是為了避免在資料不完整時處理請求。
- **預設行為**（`WAIT_FOR_WARMUP=false`）：不等待 warmup，資料庫連線後立即 Ready。

**Q: Warmup 卡住怎麼辦？**
A: 系統設有 grace period（`WARMUP_GRACE_PERIOD`，預設 10 分鐘）。即使 warmup 未完成，超過後也會強制開放流量，避免永久無法服務。此設定僅當 `WAIT_FOR_WARMUP=true` 時生效。

**Q: 如何啟用 warmup 等待？**
A: 設定 `WAIT_FOR_WARMUP=true`。可搭配 `WARMUP_GRACE_PERIOD` 調整等待時間（預設 10m）。

---

# Bot + 監控

包含 Prometheus + Grafana + Alertmanager 監控堆疊。

## Full Stack

Bot 和監控三件套在同一 Docker 網路，適合單機部署。

### 快速啟動

```bash
cd deployments/full
cp .env.example .env
# 編輯 .env 填入 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET

docker compose up -d
```

### 存取介面

監控端口預設不對外暴露，需要時啟動 Gateway：

```bash
task access:up      # 開啟
task access:down    # 關閉（釋放端口）
```

| 服務 | URL | 說明 |
|------|-----|------|
| Grafana | http://localhost:3000 | 儀表板 (admin/admin123) |
| Prometheus | http://localhost:9090 | 監控資料 |
| Alertmanager | http://localhost:9093 | 告警管理 |
| Bot | http://localhost:10000 | 始終可用 |

詳細說明請參閱 [full/README.md](./full/README.md)。

---

## Monitoring Only

Bot 部署在雲端（如 Cloud Run、Fly.io），監控三件套在本地。

### 步驟 1：在雲端 Bot 設定 Metrics 驗證

```bash
# 在雲端 Bot 設定環境變數
METRICS_USERNAME=prometheus
METRICS_PASSWORD=your_secure_password
```

### 步驟 2：啟動本地監控

```bash
cd deployments/monitoring
cp .env.example .env
# 編輯 .env，設定 BOT_HOST 和 METRICS_PASSWORD

docker compose up -d
```

### 存取介面

```bash
task monitoring:access:up      # 開啟
task monitoring:access:down    # 關閉
```

詳細說明請參閱 [monitoring/README.md](./monitoring/README.md)。

---

# 常用 Task 指令

```bash
# 僅 Bot
task dev                       # Go 直接執行

# Full Stack
task compose:up                # 啟動
task compose:down              # 停止
task compose:logs              # 查看日誌
task access:up                 # 開啟監控訪問
task access:down               # 關閉監控訪問

# Monitoring Only
task monitoring:up             # 啟動
task monitoring:down           # 停止
task monitoring:access:up      # 開啟監控訪問
task monitoring:access:down    # 關閉監控訪問
```

---

# 目錄結構

```
deployments/
├── README.md            # 本文件
├── full/                # Full Stack：Bot + 監控
│   ├── compose.yaml
│   ├── access/          # nginx gateway
│   ├── .env.example
│   └── README.md
├── monitoring/          # Monitoring Only：僅監控
│   ├── compose.yaml
│   ├── access/          # nginx gateway
│   ├── prometheus/
│   ├── .env.example
│   └── README.md
└── shared/              # 共用配置
    ├── prometheus/
    ├── alertmanager/
    ├── grafana/
    └── nginx/
```

---

# 告警規則

告警規則配置在 `shared/prometheus/alerts.yml`：

| 告警名稱 | 條件 | 嚴重度 |
|---------|------|--------|
| ServiceDown | 服務停止回應 2 分鐘 | Critical |
| WebhookLatencyHigh | P95 延遲 > 2s | Warning |
| WebhookErrorRateHigh | 錯誤率 > 5% | Warning |
| ScraperFailureRateHigh | 爬蟲失敗率 > 30% | Warning |
| LLMErrorRateHigh | LLM 錯誤率 > 20% | Warning |
| CacheHitRateLow | 快取命中率 < 50% | Info |

---

# 安全最佳實踐

1. **Metrics 驗證**
   - 外部部署時務必設定 `METRICS_PASSWORD`
   - 使用強密碼（20+ 字元）
   - 內部 Docker 網路可不設密碼

2. **Grafana**
   - 更改預設密碼 `admin123`
   - 生產環境設定 `GF_SECURITY_COOKIE_SECURE=true`

3. **網路**
   - 僅暴露必要端口
   - 使用防火牆限制監控端口存取
