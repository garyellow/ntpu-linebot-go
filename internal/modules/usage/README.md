# Usage Module

配額查詢模組 - 提供使用者查詢自己的功能配額狀態（訊息頻率限制、AI 功能配額）。

## 功能特性

### 支援的查詢方式

1. **關鍵字查詢**
   - 中文：`用量`、`配額`、`額度`
   - 英文：`quota`、`usage`、`limit`

2. **NLU 自然語言查詢**（需要 LLM API Key）
   - 範例：「我的配額還剩多少」、「還可以用幾次」、「查詢用量」

3. **Postback 動作**
   - 透過按鈕觸發：`usage:query` 或 `usage:配額`

### 配額資訊顯示

#### 訊息頻率限制
- 當前可用次數 / 最大次數
- 視覺化進度條（綠色 > 50%、黃色 20-50%、紅色 < 20%）
- 恢復速度說明

#### AI 功能配額
- **短期配額**（Token Bucket）
  - 當前可用次數 / 最大次數
  - 每小時恢復次數
  - 視覺化進度條
- **每日配額**（Sliding Window，如果啟用）
  - 當日剩餘次數 / 每日最大次數
  - 視覺化進度條
  - 每日凌晨重置提示

## 設計模式

### Handler 接口實作

```go
type Handler struct {
    userLimiter    *ratelimit.KeyedLimiter  // Webhook 訊息頻率限制器
    llmLimiter     *ratelimit.KeyedLimiter  // LLM API 配額限制器
    logger         *logger.Logger
    stickerManager *sticker.Manager
}
```

### 依賴注入
- 使用 Constructor Pattern 注入所有依賴
- 可選依賴：通過 nil 檢查處理（limiters 可能未啟用）

### Rate Limiter 整合
- 使用 `GetUsageStats(chatID)` 獲取配額資訊
- 返回 `UsageStats` 結構包含：
  - `BurstAvailable`, `BurstMax`, `BurstRefillRate`
  - `DailyRemaining`, `DailyMax`（如果啟用）

### Context 使用
- 優先使用 `chatID`（與 rate limiter 一致）
- Fallback 至 `userID`（向後兼容）
- 找不到時返回完整配額（新用戶情況）

## Flex Message 設計

### 結構
1. **Hero Section**（藍色背景）
   - 標題：📊 使用配額狀態
   - Sender: 配額小幫手

2. **Body Section**
   - 訊息頻率限制（如果啟用）
   - AI 功能配額（如果啟用）
   - 每個區塊包含：
     - 標題（粗體）
     - 可用/最大次數
     - 視覺化進度條
     - 恢復/重置說明

3. **Footer Section**
   - 📚 課程查詢 按鈕
   - 📖 使用說明 按鈕

4. **Quick Reply**
   - 使用 `QuickReplyUsageNav()`
   - 包含：📚 課程、🎓 學號、📞 聯絡、📖 說明

### 進度條實作
- 使用 Flex Box 的 flex ratio 顯示百分比
- 顏色邏輯：
  - 綠色（#4CAF50）：> 50%
  - 黃色（#FFC107）：20-50%
  - 紅色（#F44336）：< 20%
- 最小可見度：確保至少 2% 的 flex 值
- 邊界處理：0% 和 100% 的特殊情況

## NLU 整合

### Intent 定義
- **Function Name**: `usage_query`
- **Module**: `usage`
- **Intent**: `query`
- **Parameters**: 無參數

### Intent Mapping
在 `genai/functions.go` 中定義：
```go
"usage_query": {"usage", "query"}
```

### 處理流程
1. NLU Parser 識別 `usage_query` 函數
2. Dispatch 到 `DispatchIntent(ctx, "query", nil)`
3. 調用 `HandleMessage(ctx, "")` 生成回應

## 測試覆蓋

### 測試案例
1. **CanHandle** 測試
   - 中文/英文關鍵字
   - 大小寫不敏感
   - 前後空格處理
   - 關鍵字位置驗證

2. **HandleMessage** 測試
   - 基本訊息生成
   - Limiter 整合

3. **DispatchIntent** 測試
   - `query` intent 處理
   - 未知 intent 錯誤處理

4. **HandlePostback** 測試
   - `query` postback
   - `配額` postback
   - 前綴處理（`usage:query`）

5. **UsageStats** 測試
   - 配額消耗後的數值
   - 新用戶完整配額
   - 每日限制禁用情況

## 最佳實踐

### 錯誤處理
- Limiter 為 nil 時優雅降級（不顯示該區塊）
- Context 找不到 chatID 時返回完整配額

### 性能考量
- 讀取配額狀態無需鎖定（使用 atomic 操作）
- 輕量級操作，不涉及資料庫或網路請求

### UX 設計
- 清晰的視覺化進度條
- 明確的恢復/重置說明
- 提供快速操作按鈕
- 一致的 Sender 使用

### 一致性
- 遵循其他模組的設計模式
- 統一的 Quick Reply 導航
- 標準的 Flex Message 結構
- 完整的 NLU 支援

## 相關檔案
- Handler: `internal/modules/usage/handler.go`
- Tests: `internal/modules/usage/handler_test.go`
- NLU Functions: `internal/genai/functions.go`
- Quick Reply: `internal/lineutil/builder.go`
- Help Message: `internal/bot/processor.go`
