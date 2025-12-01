# Warmup

背景快取預熱模組，負責在伺服器啟動和每日排程時主動抓取並快取資料。

## 功能

### 模組支援
- `id` - 學號查詢資料（101-112 學年度）
- `contact` - 通訊錄資料（行政單位 + 學術單位）
- `course` - 課程資料（當前學年 + 前一學年）
- `sticker` - 貼圖資料（LINE 貼圖 URL）
- `syllabus` - 課程大綱（需要 `GEMINI_API_KEY`，依賴 course 完成）

### 執行模式

#### 1. 啟動預熱 (RunInBackground)
伺服器啟動時非阻塞執行，立即返回讓伺服器繼續啟動。

```go
warmup.RunInBackground(ctx, db, client, stickerMgr, log, warmup.Options{
    Modules:  warmup.ParseModules(cfg.WarmupModules),
    Reset:    false,
    Metrics:  m,
    VectorDB: vectorDB,
})
```

#### 2. 每日預熱 (proactiveWarmup)
每天凌晨 3:00 自動執行，確保資料新鮮度。

## 並行策略

```
┌─────────────┬─────────────┬─────────────┬─────────────┐
│     id      │   contact   │   sticker   │   course    │
│  (並行)     │   (並行)    │   (並行)    │   (並行)    │
└─────────────┴─────────────┴─────────────┴──────┬──────┘
                                                 │
                                                 ▼
                                          ┌─────────────┐
                                          │  syllabus   │
                                          │ (等待course)│
                                          └─────────────┘
```

- **獨立模組**：id, contact, sticker 可並行執行
- **依賴模組**：syllabus 依賴 course 完成後才開始

## 配置

環境變數：
- `WARMUP_MODULES=sticker,id,contact,course` - 預熱模組列表（逗號分隔）
- 加入 `syllabus` 需同時設定 `GEMINI_API_KEY`

## 錯誤處理

- 單一模組失敗不會中斷其他模組
- 返回部分成功的統計資料和合併的錯誤
- 使用 `errors.Join()` 合併多個錯誤

## 指標監控

- `ntpu_warmup_tasks_total{module, status}` - 各模組任務計數
- `ntpu_warmup_duration_seconds` - 預熱總耗時

## 使用範例

```go
// 解析模組列表
modules := warmup.ParseModules("id,contact,course")

// 執行預熱（阻塞）
stats, err := warmup.Run(ctx, db, client, stickerMgr, log, warmup.Options{
    Modules:  modules,
    Reset:    false, // 不清除現有快取
    Metrics:  m,
    VectorDB: vectorDB,
})

// 檢查結果
if err != nil {
    log.WithError(err).Warn("Warmup finished with errors")
}
log.WithField("students", stats.Students.Load()).
    WithField("courses", stats.Courses.Load()).
    Info("Warmup complete")
```
