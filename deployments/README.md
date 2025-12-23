# Deployments

部署配置與監控堆疊。

## 部署模式

| 模式 | 描述 | 使用場景 |
|------|------|----------|
| **[Full Stack](./full/)** | Bot + 監控三件套 | 單機部署，所有服務在同一 Docker 網路 |
| **[Monitoring Only](./monitoring/)** | 僅監控三件套 | Bot 部署在雲端，監控在本地/其他伺服器 |
| **Local Go** | 直接執行 Go | 開發測試，不需要監控 |

---

## Mode 1: Full Stack

Bot 和監控三件套（Prometheus、Grafana、Alertmanager）在同一 Docker 網路。

```bash
cd deployments/full
cp .env.example .env
# 編輯 .env 填入 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET
docker compose up -d
```

**存取介面**：
- Grafana: http://localhost:3000 (admin/admin123)
- Prometheus: http://localhost:9090
- Alertmanager: http://localhost:9093
- Bot: http://localhost:10000

詳細說明請參閱 [full/README.md](./full/README.md)。

---

## Mode 2: Monitoring Only

Bot 部署在雲端（如 Cloud Run、Fly.io），監控三件套在本地。

### 步驟 1: 在雲端 Bot 設定 Metrics 驗證

```bash
# 在雲端 Bot 設定環境變數
METRICS_USERNAME=prometheus
METRICS_PASSWORD=your_secure_password
```

### 步驟 2: 啟動本地監控

```bash
cd deployments/monitoring
cp .env.example .env
# 編輯 .env，設定 BOT_HOST 和 METRICS_PASSWORD

# 產生 Prometheus 配置
./setup.sh  # 或 Windows: .\setup.cmd

# 啟動監控
docker compose up -d
```

詳細說明請參閱 [monitoring/README.md](./monitoring/README.md)。

---

## Mode 3: Local Go Execution

開發或測試時直接執行 Go 程式，不需要 Docker 或監控。

### 使用 Task

```bash
# 設定環境變數（或建立 .env 檔案）
cp .env.example .env
# 編輯 .env

# 執行
task dev
```

### 直接執行

```bash
# 設定必要環境變數
export LINE_CHANNEL_ACCESS_TOKEN=xxx
export LINE_CHANNEL_SECRET=xxx

# 執行
go run ./cmd/server
```

### 可選: 設定 Metrics 驗證

```bash
# 如果需要保護 /metrics 端點
export METRICS_USERNAME=prometheus
export METRICS_PASSWORD=your_password
```

---

## 目錄結構

```
deployments/
├── README.md            # 本文件
├── full/                # Mode 1: Bot + 監控
│   ├── compose.yaml
│   ├── .env.example
│   └── README.md
├── monitoring/          # Mode 2: 僅監控
│   ├── compose.yaml
│   ├── prometheus/
│   │   ├── prometheus.yml.template
│   │   └── .gitignore
│   ├── setup.sh / setup.cmd
│   ├── .env.example
│   └── README.md
└── shared/              # 共用配置
    ├── prometheus/
    │   ├── prometheus-internal.yml
    │   └── alerts.yml
    ├── alertmanager/
    │   └── alertmanager.yml
    └── grafana/
        ├── dashboards/
        │   ├── dashboard.yml
        │   └── ntpu-linebot.json
        └── datasources/
            └── datasource.yml
```

---

## 告警規則

告警規則配置在 `shared/prometheus/alerts.yml`，包含：

| 告警名稱 | 條件 | 嚴重度 |
|---------|------|--------|
| ServiceDown | 服務停止回應 2 分鐘 | Critical |
| WebhookLatencyHigh | P95 延遲 > 2s | Warning |
| WebhookErrorRateHigh | 錯誤率 > 5% | Warning |
| ScraperFailureRateHigh | 爬蟲失敗率 > 30% | Warning |
| LLMErrorRateHigh | LLM 錯誤率 > 20% | Warning |
| CacheHitRateLow | 快取命中率 < 50% | Info |

完整告警規則請參閱 [shared/prometheus/alerts.yml](./shared/prometheus/alerts.yml)。

---

## 常用 Task 指令

```bash
# Full Stack
task compose:up       # 啟動 full stack
task compose:down     # 停止 full stack
task compose:logs     # 查看日誌

# Monitoring Only
task monitoring:setup # 產生 prometheus.yml（首次或更新認證後）
task monitoring:up    # 啟動監控
task monitoring:down  # 停止監控

# 開發
task dev              # 本地執行 (go run)
```

---

## 安全最佳實踐

1. **Metrics 驗證**
   - 外部部署時務必設定 `METRICS_PASSWORD`
   - 使用強密碼（20+ 字元）
   - 內部 Docker 網路可不設密碼（向後相容）

2. **Grafana**
   - 更改預設密碼 `admin123`
   - 生產環境設定 `GF_SECURITY_COOKIE_SECURE=true`

3. **網路**
   - 僅暴露必要端口
   - 使用防火牆限制監控端口存取

4. **憑證檔案**
   - 不要將 `monitoring/prometheus/prometheus.yml` 提交到 Git
   - 使用 `.gitignore` 保護敏感配置
