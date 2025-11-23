# Metrics

Prometheus 指標收集。

## 核心指標

### Webhook Metrics
- **ntpu_webhook_requests_total** - Webhook 請求總數 (labels: event_type, status)
- **ntpu_webhook_duration_seconds** - Webhook 處理耗時直方圖 (labels: event_type)

### Cache Metrics
- **ntpu_cache_hits_total** - 快取命中次數 (labels: module)
- **ntpu_cache_misses_total** - 快取未命中次數 (labels: module)

### Scraper Metrics
- **ntpu_scraper_requests_total** - 爬蟲請求總數 (labels: module, status)
  - status: success, error, timeout, not_found
- **ntpu_scraper_duration_seconds** - 爬蟲請求耗時直方圖 (labels: module)

### HTTP Metrics
- **ntpu_http_errors_total** - HTTP 錯誤總數 (labels: error_type, module)

### Data Integrity Metrics
- **ntpu_course_data_integrity_issues_total** - 課程資料完整性問題 (labels: issue_type)

### Rate Limiter Metrics
- **ntpu_rate_limiter_wait_duration_seconds** - Rate limiter 等待時間直方圖 (labels: limiter_type)
- **ntpu_rate_limiter_dropped_total** - Rate limiter 丟棄的請求數 (labels: limiter_type)

### Singleflight Metrics
- **ntpu_singleflight_dedup_total** - Singleflight 去重請求數 (labels: module)

### Warmup Metrics
- **ntpu_warmup_tasks_total** - Warmup 任務總數 (labels: module, status)
- **ntpu_warmup_duration_seconds** - Warmup 總耗時直方圖

### Go Runtime Metrics (標準收集器)
- **go_goroutines** - Goroutine 數量
- **go_memstats_alloc_bytes** - 記憶體使用量
- **go_gc_duration_seconds** - GC 耗時

## 使用方式

```go
m := metrics.New(registry)

// Webhook metrics
m.RecordWebhook("message", "success", 0.123)

// Cache metrics
m.RecordCacheHit("id")
m.RecordCacheMiss("contact")

// Scraper metrics
m.RecordScraperRequest("course", "success", 1.234)

// HTTP error metrics
m.RecordHTTPError("invalid_signature", "webhook")

// Data integrity metrics
m.RecordCourseIntegrityIssue("missing_no")

// Rate limiter metrics
m.RecordRateLimiterWait("scraper", 0.5)
m.RecordRateLimiterDrop("user")

// Singleflight metrics
m.RecordSingleflightDedup("id")

// Warmup metrics
m.RecordWarmupTask("id", "success")
m.RecordWarmupDuration(120.5)
```

## 查看指標

- **Prometheus**: http://localhost:9090
- **Grafana Dashboard**: http://localhost:3000 (帳號密碼: admin/admin)
- **Metrics Endpoint**: http://localhost:10000/metrics

## 最佳實踐

1. **使用有意義的 labels**：避免高基數 labels (如 user_id)
2. **選擇正確的 metric 類型**：
   - Counter: 單調遞增的累計值 (請求總數、錯誤數)
   - Gauge: 可增可減的瞬時值 (當前連線數、記憶體使用)
   - Histogram: 分佈分析 (請求耗時、回應大小)
3. **適當的 bucket 設定**：根據實際數據範圍調整 histogram buckets
4. **避免過度使用 histograms**：每個 histogram 會產生多個時間序列
