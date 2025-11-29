# syllabus

課程大綱擷取與處理模組，為語意搜尋提供資料來源。

## 功能

- **Scraper**: 從課程詳細頁面擷取大綱內容
- **Fields**: 解析後的大綱欄位 (教學目標、內容綱要、教學進度)
- **ChunksForEmbedding**: 為語意搜尋產生分塊內容
- **ComputeContentHash**: SHA256 雜湊用於增量更新偵測

## 擷取欄位

| 欄位 | 來源 | 用途 |
|------|------|------|
| 教學目標 | `<td>` 含 "教學目標" 標籤 | 回答「這門課學什麼」類查詢 |
| 內容綱要 | `<td>` 含 "內容綱要" 標籤 | 回答主題/內容類查詢 |
| 教學進度 | `<td>` 含 "教學進度" 標籤 | 包含考試週、特殊活動等資訊 |

## 使用

```go
// 建立 scraper
syllabusScraper := syllabus.NewScraper(scraperClient)

// 擷取課程大綱
fields, err := syllabusScraper.ScrapeSyllabus(ctx, &course)

// 產生分塊內容（用於 embedding）
chunks := fields.ChunksForEmbedding(course.Title)
for _, chunk := range chunks {
    // chunk.Type: objectives, outline, schedule
    // chunk.Content: 包含課程名稱前綴的完整內容
}

// 計算 hash 用於增量更新
contentForHash := fields.Objectives + "\n" + fields.Outline + "\n" + fields.Schedule
hash := syllabus.ComputeContentHash(contentForHash)
```

## Chunking 策略 (2025 最佳實踐)

- **語意分塊**: 每個大綱欄位本身就是語意完整的單元
- **不截斷內容**: Gemini embedding 支援 2048 tokens (~8000 字元)，完整保留所有內容
- **課程名稱前綴**: 每個 chunk 包含 `【課程名稱】` 前綴，提升檢索準確度
- **非對稱搜尋優化**: 短查詢對長文件的場景，分塊比整篇文件 embedding 效果更好

## 增量更新

- 使用 `content_hash` 偵測內容變更
- 僅重新產生 embedding 有變更的大綱
- 減少 API 呼叫與運算成本

## 依賴

- `internal/scraper`: HTTP 客戶端
- `internal/storage`: Course 資料模型
