# Metrics

Prometheus 指標收集。

## 核心指標

- **ntpu_webhook_requests_total** - Webhook 請求總數
- **ntpu_webhook_duration_seconds** - Webhook 處理耗時
- **ntpu_cache_hits_total** - 快取命中次數
- **ntpu_cache_misses_total** - 快取未命中次數
- **ntpu_scraper_requests_total** - 爬蟲請求總數
- **ntpu_system_memory_bytes** - 記憶體使用量
- **ntpu_system_goroutines** - Goroutine 數量

## 使用方式

```go
m := metrics.New(registry)

// 記錄請求
m.RecordWebhookRequest("success", 0.123)

// 記錄快取
m.RecordCacheHit("id")
m.RecordCacheMiss("contact")

// 記錄爬蟲
m.RecordScraperRequest("course", "success", 1.234)
```

## Prometheus 查詢

```promql
# 快取命中率
sum(rate(ntpu_cache_hits_total[5m])) /
  (sum(rate(ntpu_cache_hits_total[5m])) + sum(rate(ntpu_cache_misses_total[5m])))

# 錯誤率
sum(rate(ntpu_scraper_requests_total{status="error"}[5m])) /
  sum(rate(ntpu_scraper_requests_total[5m]))
```
