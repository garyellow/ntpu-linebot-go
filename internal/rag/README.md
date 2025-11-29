# rag

Retrieval-Augmented Generation (RAG) 模組，使用 chromem-go 進行課程大綱的向量搜尋。

## 功能

- **VectorDB**: 封裝 chromem-go 向量資料庫
- **SearchResult**: 語意搜尋結果結構

## 架構

```
RAG Flow:
  課程大綱 → Gemini Embedding → chromem-go Vector Store
      ↓
  使用者查詢 → Embedding → 餘弦相似度搜尋 → 排序結果
```

## 使用

```go
// 初始化
vectorDB, err := rag.NewVectorDB(dataDir, geminiAPIKey, logger)

// 載入現有資料
syllabi, _ := db.GetAllSyllabi(ctx)
vectorDB.Initialize(ctx, syllabi)

// 搜尋
results, err := vectorDB.Search(ctx, "想學機器學習", 10)
for _, r := range results {
    fmt.Printf("%s (%.0f%% 相關)\n", r.Title, r.Similarity*100)
}
```

## 儲存

- 資料持久化: `data/chromem/syllabi/` (gob 格式)
- 啟動時自動載入已索引資料

## 依賴

- `internal/genai`: Gemini embedding 客戶端
- `internal/storage`: Syllabus 資料模型
