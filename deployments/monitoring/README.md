# Monitoring Only Deployment

僅監控三件套（Prometheus、Grafana、Alertmanager），透過 HTTPS + Basic Auth 拉取外部雲端部署的 Bot。

## 使用場景

當你的 LINE Bot 已經部署在雲端（如 Cloud Run、Fly.io、Render 等），想要在本地或其他伺服器運行監控堆疊。

## 架構

```
┌─────────────────────────────────────────────────────────────────┐
│                     Cloud (你的 Bot)                             │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ ntpu-linebot (your-bot.a.run.app)                           ││
│  │   /metrics (HTTPS + Basic Auth)                             ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                              ↑ HTTPS Pull
                              │ (Basic Auth)
┌─────────────────────────────────────────────────────────────────┐
│                     Local/Other Server                          │
│   ┌────────────┐  ┌───────────────┐  ┌───────────────┐          │
│   │ prometheus │→ │    grafana    │  │ alertmanager  │          │
│   │  (內部)    │  │    (內部)     │  │    (內部)     │          │
│   └────────────┘  └───────────────┘  └───────────────┘          │
│                          ↑                  ↑                    │
│                   ┌──────┴──────────────────┘                    │
│              ┌────────────────┐                                  │
│              │  nginx-gateway │ ← 按需啟動 (monitoring:access:up) │
│              │:3000 :9090 :9093                                  │
│              └────────────────┘                                  │
└─────────────────────────────────────────────────────────────────┘
```

## 設定步驟

### 1. 在雲端 Bot 設定 Metrics 驗證

在你的雲端 Bot 設定以下環境變數：

```bash
METRICS_USERNAME=prometheus
METRICS_PASSWORD=your_secure_password  # 使用強密碼
```

驗證 metrics 端點是否正常運作：

```bash
curl -u prometheus:your_secure_password https://your-bot.a.run.app/metrics
```

### 2. 設定本地監控

```bash
cd deployments/monitoring
cp .env.example .env
```

編輯 `.env` 檔案：

```bash
# 你的 Bot 公開網址（不含 https://）
BOT_HOST=your-bot.a.run.app

# 與 Bot 相同的 metrics 認證
METRICS_USERNAME=prometheus
METRICS_PASSWORD=your_secure_password
```

### 3. 啟動監控

Prometheus 配置會在容器啟動時自動從 `.env` 生成：

```bash
docker compose up -d
```

## 存取介面

| 服務 | URL | 說明 |
|------|-----|------|
| Grafana | http://localhost:3000 | 儀表板（預設：admin/admin123）|
| Prometheus | http://localhost:9090 | 監控資料（查看 Targets 確認連線）|
| Alertmanager | http://localhost:9093 | 告警管理 |

## 驗證連線

1. 開啟 Prometheus: http://localhost:9090
2. 前往 Status → Targets
3. 確認 `ntpu-linebot` target 顯示 **UP**

如果顯示 **DOWN**，請檢查：
- Bot 的公開網址是否正確
- Bot 是否已設定 `METRICS_PASSWORD`
- 認證資訊是否匹配

## 環境變數

| 變數 | 必填 | 說明 |
|------|------|------|
| `BOT_HOST` | ✅ | Bot 公開網址（不含 https://）|
| `METRICS_USERNAME` | ⬚ | Metrics 帳號（預設：prometheus）|
| `METRICS_PASSWORD` | ✅ | Metrics 密碼 |
| `GRAFANA_USER` | ⬚ | Grafana 帳號（預設：admin）|
| `GRAFANA_PASSWORD` | ⬚ | Grafana 密碼（預設：admin123）|

## 常用指令

```bash
# 啟動
docker compose up -d

# 查看日誌
docker compose logs -f

# 更新認證（修改 .env 後）
./setup.sh  # 或 .\setup.cmd
docker compose restart prometheus

# 停止
docker compose down
```

## 安全考量

- ✅ 使用 HTTPS 傳輸 metrics
- ✅ Basic Auth 保護 /metrics 端點
- ✅ 密碼使用強密碼（建議 20+ 字元）
- ⚠️ `prometheus.yml` 包含明文密碼，請妥善保管
- ⚠️ 請勿將產生的 `prometheus/prometheus.yml` 提交到版本控制

## TLS 設定

如果你的 Bot 使用自簽憑證，請編輯 `prometheus/prometheus.yml.template`：

```yaml
tls_config:
  insecure_skip_verify: true  # 允許自簽憑證
```

然後重新執行 `./setup.sh`。

---

## 訪問監控介面

預設不暴露監控端口，需要時才開啟：

### 開啟監控訪問

```bash
cd access
docker compose up -d
# 或
task monitoring:access:up
```

現在可以訪問：
- Grafana: http://localhost:3000
- Prometheus: http://localhost:9090
- Alertmanager: http://localhost:9093

### 關閉監控訪問（釋放端口）

```bash
cd access
docker compose down
# 或
task monitoring:access:down
```
