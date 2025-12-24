# Full Stack Deployment

Bot + 監控三件套（Prometheus、Grafana、Alertmanager）在同一 Docker 網路。

## 快速啟動

```bash
cd deployments/full
cp .env.example .env
# 編輯 .env 填入必要參數
docker compose pull
docker compose up -d
```

## 服務

| 服務 | 端口 | 說明 |
|------|------|------|
| **ntpu-linebot** | 10000 | LINE Bot 主服務 |
| **prometheus** | 9090 | 監控資料收集 |
| **grafana** | 3000 | 儀表板 (預設：admin/admin123) |
| **alertmanager** | 9093 | 告警管理 |

## 環境變數

### 必填

| 變數 | 說明 |
|------|------|
| `LINE_CHANNEL_ACCESS_TOKEN` | LINE Bot Access Token |
| `LINE_CHANNEL_SECRET` | LINE Bot Channel Secret |

### LLM 設定（可選）

| 變數 | 說明 |
|------|------|
| `GEMINI_API_KEY` | Gemini API Key（啟用 NLU 和智慧搜尋）|
| `GROQ_API_KEY` | Groq API Key（備援 LLM）|

### 監控設定

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `GRAFANA_USER` | admin | Grafana 管理員帳號 |
| `GRAFANA_PASSWORD` | admin123 | Grafana 管理員密碼 |
| `GRAFANA_PORT` | 3000 | Grafana 端口 |
| `PROMETHEUS_PORT` | 9090 | Prometheus 端口 |
| `ALERTMANAGER_PORT` | 9093 | Alertmanager 端口 |

### Metrics 驗證

| 變數 | 預設值 | 說明 |
|------|--------|------|
| `METRICS_USERNAME` | prometheus | /metrics 端點帳號 |
| `METRICS_PASSWORD` | (空) | /metrics 端點密碼（空=停用驗證）|

> **注意**：內部 Docker 網路不需要 metrics 驗證，保持 `METRICS_PASSWORD` 為空即可。

## 常用指令

```bash
# 啟動
docker compose up -d

# 查看日誌
docker compose logs -f ntpu-linebot

# 重啟
docker compose restart

# 停止
docker compose down

# 更新到最新版本
docker compose pull && docker compose up -d
```

## 資料持久化

- **data**: Bot 的 SQLite 資料庫
- **prometheus-data**: Prometheus 時序資料（保留 15 天或 2GB）
- **alertmanager-data**: Alertmanager 靜默/抑制狀態
- **grafana-data**: Grafana 儀表板和設定

## 網路架構

```
┌─────────────────────────────────────────────────────────┐
│                   ntpu_bot_network                       │
│  ┌─────────────┐  ┌────────────┐  ┌───────────────────┐ │
│  │ ntpu-linebot│←─│ prometheus │←─│ grafana          │ │
│  │   :10000    │  │   :9090    │  │   :3000          │ │
│  └─────────────┘  └────────────┘  └───────────────────┘ │
│                          ↓                               │
│                   ┌────────────┐                         │
│                   │alertmanager│                         │
│                   │   :9093    │                         │
│                   └────────────┘                         │
└─────────────────────────────────────────────────────────┘
```

所有服務透過 Docker 內部網路 `ntpu_bot_network` 通訊，Prometheus 使用內部位址 `ntpu-linebot:10000` 拉取 metrics。

---

## 訪問監控介面

預設不暴露監控端口，需要時才開啟：

### 開啟監控訪問

```bash
cd access
docker compose up -d
# 或
task access:up
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
task access:down
```

---

## 架構

```
┌─────────────────────────────────────────────────────────┐
│                   ntpu_bot_network                       │
│  ┌─────────────┐  ┌────────────┐  ┌───────────────────┐ │
│  │ ntpu-linebot│  │ prometheus │  │ grafana           │ │
│  │   :10000    │  │ (內部)     │  │ (內部)            │ │
│  └─────────────┘  └────────────┘  └───────────────────┘ │
│                          ↑                ↑              │
│                   ┌──────┴────────────────┘              │
│              ┌────────────┐                              │
│              │nginx-gateway│ ← 按需啟動                   │
│              │:3000 :9090 :9093                          │
│              └────────────┘                              │
└─────────────────────────────────────────────────────────┘
```

Nginx gateway 在需要時啟動，代理請求到內部服務，關閉後釋放端口。
