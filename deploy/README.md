# deploy/

部署相關設定檔。

## 目錄結構

- **prometheus/** - Prometheus 設定
  - `prometheus.yml` - 主設定（scrape targets、AlertManager）
  - `alerts.yml` - 告警規則
- **alertmanager/** - AlertManager 設定
  - `alertmanager.yml` - 告警路由和接收器
- **grafana/** - Grafana 設定
  - `dashboards/ntpu-linebot.json` - 預設 Dashboard

## 使用方式

```bash
# 啟動監控堆疊
task compose:up

# 存取服務
# Prometheus: http://localhost:9090
# AlertManager: http://localhost:9093
# Grafana: http://localhost:3000 (admin/admin123)
```

## 告警規則

- **ScraperHighFailureRate** - 爬蟲失敗率 >50% 持續 5 分鐘
- **WebhookHighLatency** - Webhook P95 延遲 >5s 持續 5 分鐘
- **ServiceDown** - 服務停止回應持續 2 分鐘
- **HighMemoryUsage** - 記憶體使用 >500MB 持續 10 分鐘
- **CacheLowHitRate** - 快取命中率 <50% 持續 15 分鐘

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
