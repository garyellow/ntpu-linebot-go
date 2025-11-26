# 從 Python 遷移到 Go

本專案原為 Python 實作 ([ntpu-linebot-python](https://github.com/garyellow/ntpu-linebot-python))，重寫為 Go 版本。

## 遷移原因

| 指標 | Python | Go | 改善 |
|------|--------|-----|------|
| **啟動時間** | ~3-5 秒 | ~0.1 秒 | **50x** |
| **記憶體使用** | ~150-200 MB | ~30-50 MB | **4x** |
| **容器大小** | ~500 MB | ~15 MB | **33x** |
| **並發處理** | asyncio (單執行緒) | goroutines (多核心) | 顯著提升 |

## 技術對照

| 功能 | Python | Go |
|------|--------|-----|
| Web 框架 | Flask | Gin |
| 快取 | Dict (記憶體) | SQLite (持久化) |
| 並發控制 | asyncio | goroutines + channels |
| 監控 | 無 | Prometheus + Grafana |

## 主要改進

- **快取策略**: 從記憶體字典改為 SQLite WAL 模式（重啟後資料保留）
- **TTL 機制**: Soft TTL 5 天 + Hard TTL 7 天，支援主動 warmup
- **防爬蟲**: Token Bucket + Singleflight + 指數退避重試
- **錯誤處理**: 強制處理（`if err != nil`）+ 錯誤包裝（`%w`）

## 保留設計

- Cache-First 查詢策略
- 三層模組架構（ID/Contact/Course）
- Failover URLs（多個備援網址）
- 關鍵字匹配邏輯

---

Python 版本作為歷史參考：https://github.com/garyellow/ntpu-linebot-python
