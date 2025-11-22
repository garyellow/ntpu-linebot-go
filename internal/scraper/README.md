# Scraper System

從 NTPU 官方網站抓取資料，包含防爬蟲機制、錯誤處理與快取整合。

## 核心元件

- **Client**: HTTP 客戶端，支援多 URL Failover、User-Agent 輪替、指數退避重試
- **Rate Limiter**: Token bucket 限流（3 workers，15 秒補充週期）
- **Singleflight**: 去重複請求（多人同時查詢只執行一次爬蟲）

## 資料來源

| 模組 | 資料來源 | 說明 |
|------|---------|------|
| **ID** | LMS 數位學苑 2.0 | 學號、系所代碼 |
| **Contact** | SEA 校園聯絡簿 | 行政、學術單位聯絡資訊（Big5 編碼） |
| **Course** | SEA 課程查詢 | 課號、課程名稱（U/M/N/P 學制） |

## 防爬蟲機制

- User-Agent 輪替
- 隨機延遲 2-5 秒（每次請求間隨機延遲，實際間隔還會受到 Token bucket 全域限流影響，總延遲可能遠大於 2-5 秒）
- 指數退避重試（最多 3 次）
- Token bucket 限流（3 workers，15 秒補充週期，與隨機延遲疊加）
- URL Failover（自動切換備用網址）
