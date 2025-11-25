# Storage Module

SQLite 快取層，採用 Soft TTL / Hard TTL 雙層策略。

## 資料表

- **students** - 學生資料
- **contacts** - 聯絡人資訊
- **courses** - 課程資訊
- **stickers** - 貼圖 URL

## 快取策略（Soft TTL / Hard TTL）

| TTL | 預設 | 說明 |
|-----|------|------|
| Soft TTL | 5 天 | 觸發主動刷新，資料仍可用 |
| Hard TTL | 7 天 | 絕對過期，資料刪除 |

**Cache-First 流程**:
1. 查詢時檢查 Hard TTL
2. Cache hit → 直接返回
3. Cache miss → 觸發爬蟲 → 儲存新資料
4. 背景主動刷新 Soft TTL 過期的資料

## 特性

- WAL mode 支援並發讀寫
- Prepared statements 防止 SQL injection
- Busy timeout 30000ms（支援 warmup 期間並發寫入）
- `CountExpiring*` 方法支援主動 warmup 判斷
