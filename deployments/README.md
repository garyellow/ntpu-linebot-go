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
- `WARMUP_MODULES` - 預熱模組（預設：id,contact,course，空字串跳過）
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

```bash
task compose:up                      # 啟動
task compose:down                    # 停止
task compose:pull                    # 更新映像
task compose:logs                    # 查看所有日誌
task compose:logs -- ntpu-linebot    # 查看特定服務日誌
task compose:restart -- ntpu-linebot # 重啟服務
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

**更新映像**：
```bash
docker compose pull && docker compose up -d --force-recreate
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
