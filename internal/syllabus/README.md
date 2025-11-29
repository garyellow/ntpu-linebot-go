# syllabus

課程大綱擷取與處理模組，為語意搜尋提供資料來源。

## 功能

- **Scraper**: 從課程詳細頁面擷取大綱內容
- **Fields**: 解析後的大綱欄位 (教學目標、內容綱要、教學進度)
- **ComputeContentHash**: SHA256 雜湊用於增量更新偵測

## 擷取欄位

| 欄位 | 來源 |
|------|------|
| 教學目標 | `<td>` 含 "教學目標" 標籤 |
| 內容綱要 | `<td>` 含 "內容綱要" 標籤 |
| 教學進度 | `<td>` 含 "教學進度" 標籤 |

## 使用

```go
// 建立 scraper
syllabusScraper := syllabus.NewScraper(scraperClient)

// 擷取課程大綱
fields, err := syllabusScraper.ScrapeSyllabus(ctx, &course)

// 合併為 embedding 用內容
content := fields.MergeForEmbedding()

// 計算 hash 用於增量更新
hash := syllabus.ComputeContentHash(content)
```

## 增量更新

- 使用 `content_hash` 偵測內容變更
- 僅重新產生 embedding 有變更的大綱
- 減少 API 呼叫與運算成本

## 依賴

- `internal/scraper`: HTTP 客戶端
- `internal/storage`: Course 資料模型
