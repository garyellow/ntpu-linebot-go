# Sticker

提供隨機貼圖 URL 給 LINE 訊息使用，從 Spy x Family 和 Ichigo Production 爬取，快取 7 天。

## 架構

```
Warmup Tool → Web Scraping → SQLite Cache (7-day TTL)
                                    ↓
Server Startup → LoadStickers() → In-Memory Array
                                    ↓
User Request → GetRandomSticker() → Random Selection
```

## 使用方式

```go
// 建立 manager
manager := sticker.NewManager(db, scraperClient, log)

// 載入貼圖（cache-first）
if err := manager.LoadStickers(ctx); err != nil {
    log.Fatal(err)
}

// 取得隨機貼圖
stickerURL := manager.GetRandomSticker()
```

## 預熱快取

```bash
# 推薦：部署前預熱
go run ./cmd/warmup -reset

# 驗證快取
sqlite3 data/cache.db "SELECT COUNT(*) FROM stickers;"
```

## 資料來源

- **Spy x Family** (7 pages) - ~100-150 貼圖
- **Ichigo Production** (1 page) - ~20-30 貼圖
- **Fallback** - 20 個 UI avatars（當所有來源失敗時）

## 效能特性

| 指標 | 有預熱 | 無預熱（冷啟動） |
|------|--------|-----------------|
| 服務啟動 | <1 秒 | 90+ 秒 |
| 貼圖數量 | 100-200 | 100-200 |

建議生產環境部署前執行 `task warmup`。
