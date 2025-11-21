# Scraper System

網頁爬蟲系統負責從 NTPU 官方網站抓取資料，包含防爬蟲機制、錯誤處理、與快取整合。

## 核心元件

- **Client**: HTTP 客戶端封裝，支援多 URL Failover、User-Agent 輪替、指數退避重試
- **Rate Limiter**: Token bucket 演算法限流（預設 5 req/s）
- **Singleflight**: 去重複請求，多使用者同時查詢時只執行一次爬蟲

## NTPU 爬蟲實作

### 學號查詢 (`ntpu/id_scraper.go`)
- 資料來源：LMS 數位學苑 2.0
- 支援學號查詢、年度/系所批次查詢
- 資料範圍：100-112 學年度

### 聯絡資訊 (`ntpu/contact_scraper.go`)
- 資料來源：SEA 校園聯絡簿
- 搜尋字��需編碼為 Big5
- 支援行政單位、學術單位查詢

### 課程查詢 (`ntpu/course_scraper.go`)
- 資料來源：SEA 課程查詢系統
- 支援課號、課程名稱搜尋
- Education Codes：U(大學部)、M(碩士)、N(在職碩士)、P(博士)

## 防爬蟲策略

1. **User-Agent Rotation**: 使用 `corpix/uarand` 輪替瀏覽器標識
2. **Random Delay**: 每次請求隨機延遲 100-500ms
3. **Exponential Backoff**: 重試等待時間：1s → 2s → 4s（最多 3 次）
4. **Rate Limiting**: Token bucket 限流（預設 5 req/s）
5. **Failover URLs**: 自動切換備用網址

## 使用建議

- 爬蟲應只在快取未命中時執行
- 所有 I/O 操作需傳遞 `context.Context`
- 使用 worker pool 控制並發數（預設 5）
- 記錄所有爬蟲請求的 metrics 和 logs
