# Webhook Module

處理 LINE Webhook 事件。

## 核心功能

- **請求驗證**: `X-Line-Signature` 簽名驗證
- **Rate Limiting**: 全域 80 rps + 單一用戶 10 rps
- **事件分發**: Message / Postback / Follow / Join
- **Bot 路由**: ID → Contact → Course → Help（依序匹配）
- **限制**: 超時 25 秒，每次最多 5 則訊息
