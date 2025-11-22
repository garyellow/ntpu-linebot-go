# Storage Module

SQLite 快取層，TTL 7 天。

## 資料表

- **students** - 學生資料
- **contacts** - 聯絡人資訊
- **courses** - 課程資訊
- **stickers** - 貼圖 URL

## Cache-First 策略

1. 優先查詢 SQLite（TTL 7 天）
2. Cache miss → 觸發爬蟲
3. 儲存新資料

## 特性

- WAL mode 支援並發讀寫
- Prepared statements 防止 SQL injection
- Busy timeout 5000ms
