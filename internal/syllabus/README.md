# syllabus

課程大綱擷取與處理模組，為語意搜尋提供資料來源。

## 功能

- **Scraper**: 從課程詳細頁面擷取大綱內容
- **Fields**: 解析後的大綱欄位 (5 欄位支援)
- **ChunksForEmbedding**: 為語意搜尋產生分塊內容
- **ComputeContentHash**: SHA256 雜湊用於增量更新偵測

## 擷取欄位 (5 欄位結構)

支援兩種頁面格式：
- **分離格式**: 5 個獨立欄位
- **合併格式**: 中英文合併在 3 個欄位

| 欄位 | 來源 | 用途 |
|------|------|------|
| ObjectivesCN | `教學目標` 標籤 | 回答「這門課學什麼」類查詢 (中文) |
| ObjectivesEN | `Course Objectives` 標籤 | 回答「這門課學什麼」類查詢 (英文) |
| OutlineCN | `內容綱要` 標籤 | 回答主題/內容類查詢 (中文) |
| OutlineEN | `Course Outline` 標籤 | 回答主題/內容類查詢 (英文) |
| Schedule | `教學進度` 表格第3列 | 僅提取「教學預定進度」(Week X: 內容) |

### Schedule 欄位擷取說明

教學進度表格結構：
- 第 1 列 (週別): Week 1, Week 2, ...
- 第 2 列 (日期): 20250911, ...
- **第 3 列 (教學預定進度)**: ← 只擷取這列
- 第 4 列 (教學方法): ← 忽略

**過濾規則**：
- 只擷取含有「週別/Weekly」標題的表格
- 排除「彈性補充教學」等無關內容
- 格式化為 `Week X: 內容`

## 使用

```go
// 建立 scraper
syllabusScraper := syllabus.NewScraper(scraperClient)

// 擷取課程大綱
fields, err := syllabusScraper.ScrapeSyllabus(ctx, &course)

// 產生分塊內容（用於 embedding）
// 自動合併 CN + EN 內容
chunks := fields.ChunksForEmbedding(course.Title)
for _, chunk := range chunks {
    // chunk.Type: objectives, outline, schedule
    // chunk.Content: 包含課程名稱前綴的完整內容
}

// 計算 hash 用於增量更新
contentForHash := fields.ObjectivesCN + "\n" + fields.ObjectivesEN + "\n" +
    fields.OutlineCN + "\n" + fields.OutlineEN + "\n" + fields.Schedule
hash := syllabus.ComputeContentHash(contentForHash)
```

## Chunking 策略

- **語意分塊**: 每個大綱欄位本身就是語意完整的單元
- **CN/EN 合併**: 同類型的中英文內容合併為一個 chunk，提升多語言搜尋效果
- **不截斷內容**: Gemini embedding 支援 2048 tokens (~8000 字元)
- **課程名稱前綴**: 每個 chunk 包含 `【課程名稱】` 前綴，提升檢索準確度
- **Schedule 過濾**: 僅保留教學預定進度內容，排除週次、日期、教學方法等 metadata

## 增量更新

- 使用 `content_hash` 偵測內容變更
- 僅重新產生 embedding 有變更的大綱
- 減少 API 呼叫與運算成本

## 依賴

- `internal/scraper`: HTTP 客戶端
- `internal/storage`: Course 資料模型
