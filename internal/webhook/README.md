# Webhook Module

處理 LINE Platform Webhook 事件的核心模組。

## 核心功能

- **請求驗證**: 使用 `X-Line-Signature` 驗證簽名，請求體限制 1MB，處理超時 25 秒
- **Rate Limiting**: 全域 80 rps + 單一用戶 10 rps（自動清理過期限流器）
- **事件分發**: 支援 Message/Postback/Follow/Join 事件
- **Bot 路由**: ID Module → Contact Module → Course Module → Help
- **訊息限制**: 每次最多 5 則，文字最多 5000 字元

## 監控指標

- `ntpu_webhook_requests_total{status}` - 請求總數
- `ntpu_webhook_duration_seconds` - 處理耗時
- `ntpu_webhook_ratelimit_hits_total{type}` - 限流次數
