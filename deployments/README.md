# deployments/

部署設定與監控堆疊。使用預建映像從 Docker Hub 部署 (推薦生產環境)。

## 快速啟動

```bash
cd deployments
cp .env.example .env
# 編輯 .env 填入 LINE_CHANNEL_ACCESS_TOKEN 和 LINE_CHANNEL_SECRET
docker compose pull  # 從 Docker Hub 拉取最新映像
docker compose up -d
```

## 服務

- **init-data** - 初始化資料目錄權限
- **warmup** - 預熱快取（執行一次）
- **ntpu-linebot** - 主服務
- **prometheus** - 監控 (http://localhost:9090)
- **alertmanager** - 告警 (http://localhost:9093)
- **grafana** - 儀表板 (http://localhost:3000, admin/admin123)

## 環境變數

必填：
- `LINE_CHANNEL_ACCESS_TOKEN`
- `LINE_CHANNEL_SECRET`

可選：
- `IMAGE_TAG` - 映像版本（預設：latest）
- `WARMUP_MODULES` - 預熱模組（預設：id,contact,course,sticker，空字串跳過）
- `LOG_LEVEL` - 日誌層級（預設：info）
- `GRAFANA_PASSWORD` - Grafana 密碼（預設：admin123）

## 指定版本

```bash
# .env 檔案中設定
IMAGE_TAG=v1.2.3

# 或使用環境變數
IMAGE_TAG=v1.2.3 docker compose up -d
```

## 常用指令

### 1. 服務管理

```bash
# 啟動所有服務
task compose:up
# 或 cd deployments && docker compose up -d

# 查看日誌
task compose:logs -- ntpu-linebot
# 或 cd deployments && docker compose logs -f ntpu-linebot

# 停止所有服務
task compose:down
# 或 cd deployments && docker compose down

# 更新至最新版本
task compose:update
# 或 Windows: cd deployments && .\update.cmd
# 或 Linux/Mac: cd deployments && ./update.sh
```

### 2. 監控儀表板存取

監控服務預設不對外開放，需透過 access gateway 存取。啟用後會佔用本地 Port (3000, 9090, 9093)。使用以下方式開啟/關閉：

```bash
# 開啟監控儀表板 (Grafana:3000, Prometheus:9090, AlertManager:9093)
task access:up
# 或 Windows: cd deployments && .\access.cmd up
# 或 Linux/Mac: cd deployments && ./access.sh up

# 關閉監控儀表板
task access:down
# 或 Windows: cd deployments && .\access.cmd down
# 或 Linux/Mac: cd deployments && ./access.sh down
```

## 目錄結構

- **prometheus/** - Prometheus 設定
  - `prometheus.yml` - 主設定（scrape targets、AlertManager）
  - `alerts.yml` - 告警規則
- **alertmanager/** - AlertManager 設定
  - `alertmanager.yml` - 告警路由和接收器
- **grafana/** - Grafana 設定
  - `dashboards/ntpu-linebot.json` - 預設 Dashboard

## 告警規則

- **ScraperHighFailureRate** - 爬蟲失敗率 >30% 持續 3 分鐘
- **WebhookHighLatency** - Webhook P95 延遲 >3s 持續 5 分鐘
- **ServiceDown** - 服務停止回應持續 2 分鐘
- **HighMemoryUsage** - 記憶體使用 >410MB 持續 5 分鐘
- **CacheLowHitRate** - 快取命中率 <70% 持續 10 分鐘

## 配置告警通知

編輯 `alertmanager/alertmanager.yml`：

```yaml
receivers:
  - name: 'team'
    webhook_configs:
      - url: 'https://your-webhook-url'
```

重啟生效：
```bash
task compose:restart -- alertmanager
```

## 疑難排解

**權限錯誤**：
```bash
docker compose down
rm -rf ./data
docker compose up -d
```

**更新至最新版本**：
```bash
# 使用 Task
task compose:update

# 或直接執行腳本
# Windows: .\update.cmd
# Linux/Mac: ./update.sh

# 或手動執行
docker compose up -d --pull always
```

**本地建置** (開發用途):
```bash
# 回到專案根目錄
cd ..
docker build -t garyellow/ntpu-linebot-go:local .

# 使用本地映像
cd deployments
IMAGE_TAG=local docker compose up -d
```
