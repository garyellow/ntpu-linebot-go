# Logger

結構化日誌系統，基於 logrus，輸出 JSON 格式。

## 使用方式

```go
log := logger.New("info")

// 基本記錄
log.Info("Server started")
log.Error("Failed to connect")

// 結構化欄位
log.WithField("user_id", "U123").Info("Message received")
log.WithModule("id").Info("Processing query")

// 錯誤記錄
log.WithError(err).Error("Scrape failed")
```

## 日誌級別

- `debug` - 詳細除錯資訊
- `info` - 一般資訊
- `warn` - 警告訊息
- `error` - 錯誤資訊

## 環境變數

```bash
LOG_LEVEL=debug  # debug, info, warn, error
```
