# Logger Module

結構化日誌系統，基於 logrus，輸出 JSON 格式，支援 debug/info/warn/error/fatal 級別。

## 使用方式

### 基本記錄

```go
log := logger.New("info")

// 一般記錄
log.Info("Server started")
log.Infof("Listening on port %d", 10000)

// 錯誤記錄
log.WithError(err).Error("Failed to connect to database")
log.Fatal("Critical error, shutting down")
```

### 結構化欄位

```go
// 單一欄位
log.WithField("user_id", "U123456").Info("User message received")

// 多個欄位
log.WithFields(map[string]interface{}{
    "method": "POST",
    "path":   "/callback",
    "status": 200,
}).Info("Request completed")
```

### 模組分類

```go
log := logger.New("info")

// ID 模組
log.WithModule("id").Info("Processing student ID query")

// Contact 模組
log.WithModule("contact").Warn("Empty search result")

// Course 模組
log.WithModule("course").Error("Failed to scrape course data")
```

### 日誌級別

```go
log := logger.New("debug")

log.Debug("Detailed debug information")  // 開發階段
log.Info("Normal operation")              // 一般資訊
log.Warn("Warning condition")             // 警告訊息
log.Error("Error occurred")               // 錯誤資訊
log.Fatal("Critical failure")             // 嚴重錯誤（會結束程式）
```

## 輸出範例

```json
{
  "level": "info",
  "message": "Cache hit for student ID",
  "module": "id",
  "student_id": "41247001",
  "timestamp": "2025-11-21T04:30:00+08:00"
}

{
  "level": "error",
  "message": "Failed to scrape contacts",
  "module": "contact",
  "error": "context deadline exceeded",
  "search_term": "資工系",
  "timestamp": "2025-11-21T04:30:05+08:00"
}
```

## 環境變數

```bash
# 設定日誌級別
LOG_LEVEL=debug  # debug, info, warn, error
```

## 最佳實踐

1. **使用結構化欄位**: 避免字串拼接，使用 WithField
2. **標記模組**: 所有 bot handler 都應使用 WithModule
3. **記錄錯誤**: 使用 WithError 附加錯誤資訊
4. **避免敏感資訊**: 不要記錄 token, password 等

## 相關檔案

- [Logger Implementation](./logger.go)
- [Configuration](../config/config.go)
