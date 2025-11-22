# Sticker

提供隨機貼圖 URL 給 LINE 訊息使用。

## 資料來源

- Spy x Family (~100-150 貼圖)
- Ichigo Production (~20-30 貼圖)
- Fallback: UI avatars (20 個)

## 工作流程

1. Warmup Tool → 爬取貼圖 → SQLite (TTL 7 天)
2. Server Startup → 載入至記憶體
3. User Request → 隨機選擇

## 效能

- **有預熱**: <1 秒啟動
- **無預熱**: 90+ 秒啟動

**建議**: 生產環境部署前執行 `task warmup`。
