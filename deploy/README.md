# deploy/

部署相關設定檔。

## prometheus/

Prometheus 監控設定：
- `prometheus.yml` - Prometheus 主設定，定義 scrape targets
- `alerts.yml` - 告警規則（高失敗率、高延遲、服務停止等）

## grafana/

Grafana 視覺化設定：
- `dashboard.json` - 預設 Dashboard，包含 QPS、延遲、錯誤率、快取命中率等面板

## 使用方式

這些設定會自動載入到 Docker Compose 環境：

```bash
# 啟動完整監控堆疊
task compose:up

# 存取服務
# Prometheus: http://localhost:9090
# Grafana: http://localhost:3000 (admin/admin123)
```

## 自訂告警

編輯 `prometheus/alerts.yml` 新增告警規則，重啟 Prometheus 生效：

```bash
task compose:restart prometheus
```
