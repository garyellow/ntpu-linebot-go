# Metrics Module

Prometheus 指標收集，記錄 webhook/scraper/cache/system 效能資訊。

## 核心功能

### 1. Webhook 指標

- **ntpu_webhook_requests_total** - Webhook 請求總數（按狀態：success/error）
- **ntpu_webhook_duration_seconds** - Webhook 處理耗時分佈

### 2. Bot 模組指標

- **ntpu_bot_messages_total** - 訊息處理總數（按模組：id/contact/course）
- **ntpu_bot_duration_seconds** - 模組處理耗時分佈

### 3. 爬蟲指標

- **ntpu_scraper_requests_total** - 爬蟲請求總數（按模組和狀態）
- **ntpu_scraper_duration_seconds** - 爬蟲耗時分佈

### 4. 快取指標

- **ntpu_cache_hits_total** - 快取命中次數（按模組）
- **ntpu_cache_misses_total** - 快取未命中次數（按模組）

### 5. 系統指標

- **ntpu_system_memory_bytes** - 記憶體使用量（heap_alloc/heap_sys/sys）
- **ntpu_system_goroutines** - Goroutine 數量
- **ntpu_database_size_bytes** - SQLite 資料庫大小

## 使用方式

```go
// 建立 metrics
registry := prometheus.NewRegistry()
m := metrics.New(registry)

// 記錄 webhook 請求
m.RecordWebhookRequest("success", 0.123)

// 記錄快取命中/未命中
m.RecordCacheHit("id")
m.RecordCacheMiss("contact")

// 記錄爬蟲請求
m.RecordScraperRequest("course", "success", 1.234)

// 更新系統指標
m.UpdateSystemMetrics()
```

## Prometheus 查詢範例

```promql
# 快取命中率
sum(rate(ntpu_cache_hits_total[5m])) /
  (sum(rate(ntpu_cache_hits_total[5m])) + sum(rate(ntpu_cache_misses_total[5m])))

# 平均處理時間
rate(ntpu_webhook_duration_seconds_sum[5m]) /
  rate(ntpu_webhook_duration_seconds_count[5m])

# 錯誤率
sum(rate(ntpu_scraper_requests_total{status="error"}[5m])) /
  sum(rate(ntpu_scraper_requests_total[5m]))
```

## 相關文件

- [Prometheus Configuration](../../deploy/prometheus/prometheus.yml)
- [Alert Rules](../../deploy/prometheus/alerts.yml)
- [Grafana Dashboard](../../deploy/grafana/dashboard.json)
