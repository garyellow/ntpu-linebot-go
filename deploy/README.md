# deploy/

部署相關設定檔。

## prometheus/

Prometheus 監控設定：
- `prometheus.yml` - Prometheus 主設定，定義 scrape targets 和 AlertManager 連接
- `alerts.yml` - 告警規則（高失敗率、高延遲、服務停止等）

## alertmanager/

AlertManager 告警管理設定：
- `alertmanager.yml` - 告警路由和接收器配置
  - 支援按嚴重程度（critical/warning/info）路由
  - 可配置 Email、Webhook、Slack 等通知渠道
  - 包含告警抑制規則避免重複通知

## grafana/

Grafana 視覺化設定：
- `dashboards/ntpu-linebot.json` - 預設 Dashboard，包含 QPS、延遲、錯誤率、快取命中率等面板

## 使用方式

這些設定會自動載入到 docker compose 環境：

```bash
# 啟動完整監控堆疊
task compose:up

# 存取服務
# Prometheus: http://localhost:9090
# Grafana: http://localhost:3000 (admin/admin123)
```

## 自訂告警規則

編輯 `prometheus/alerts.yml` 新增告警規則，重啟 Prometheus 生效：

```bash
task compose:restart -- prometheus
```

## 配置告警通知 ✨ 新功能

編輯 `alertmanager/alertmanager.yml` 配置通知渠道：

### 範例：Slack 通知
```yaml
receivers:
  - name: 'critical'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/WEBHOOK/URL'
        channel: '#alerts'
        title: '🚨 {{ .GroupLabels.alertname }}'
        text: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'
```

### 範例：Email 通知
```yaml
receivers:
  - name: 'warning'
    email_configs:
      - to: 'team@example.com'
        from: 'alertmanager@ntpu-linebot.local'
        smarthost: 'smtp.gmail.com:587'
        auth_username: 'your-email@gmail.com'
        auth_password: 'your-app-password'
```

配置後重啟 AlertManager：
```bash
task compose:restart -- alertmanager
```
