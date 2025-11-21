# Storage Module

資料庫存取層，使用 SQLite 實作快取機制，TTL 7 天。

## 資料表

- **students**: 學生資料（id, name, year, department, cached_at）
- **contacts**: 聯絡人（uid, type, name, organization, extension, email, ..., cached_at）
- **courses**: 課程（uid, year, term, title, teachers[], times[], locations[], cached_at）
- **stickers**: 貼圖 URL（url, source, cached_at, success/failure_count）

所有資料表均有 `cached_at` 欄位，索引包含 name/title 搜尋和 cached_at TTL 檢查。

## Cache-First 策略

1. 優先查詢 SQLite 快取（TTL: 7 天）
2. Cache miss 時觸發爬蟲
3. 儲存新資料到快取
4. 每小時自動清理過期資料

## 安全性

- 所有查詢使用 prepared statements 防止 SQL injection
- LIKE 查詢前調用 `sanitizeSearchTerm()` 轉義特殊字元
- SQLite WAL mode 支援並發讀寫
- Busy timeout 5000ms 避免 database locked
