# rag

Retrieval-Augmented Generation (RAG) 模組，使用 chromem-go 進行課程大綱的向量搜尋。

## 功能

- **VectorDB**: 封裝 chromem-go 向量資料庫
- **SearchResult**: 語意搜尋結果結構
- **Chunking**: 語意分段提升搜尋準確度

## 架構

```
RAG Flow (with Chunking):
  課程大綱 → 語意分段 (教學目標/內容綱要/教學進度)
      ↓
  各段落 → Gemini Embedding → chromem-go Vector Store
      ↓
  使用者查詢 → Embedding → 餘弦相似度搜尋 → 去重合併 → 過濾低相似度 → 排序結果
```

## Chunking 策略 (2025 最佳實踐)

**問題**: Asymmetric semantic search（短查詢 vs 長文檔）會導致相似度分數偏低

**解決方案**: 按語意欄位分段，每段完整保留

| Chunk | 內容 | 用途 |
|-------|------|------|
| `{UID}_objectives` | 【課程名稱】教學目標：... | 匹配「想學什麼」類查詢 |
| `{UID}_outline` | 【課程名稱】內容綱要：... | 匹配主題/內容查詢 |
| `{UID}_schedule` | 【課程名稱】教學進度：... | 匹配週次/進度查詢 |

**設計原則** (參考 2025 RAG Chunking 研究):
- 每個欄位本身是語意完整的單元，不需額外截斷
- Gemini embedding 支援 2048 tokens (~8000 字元)，遠超單欄位長度
- 完整內容確保搜尋準確度最大化
- 課程名稱前綴提供上下文，改善短查詢的匹配

**優點**:
- 每個 chunk 長度更接近查詢長度
- 相似度分數更準確
- 搜尋結果自動去重（同課程只顯示最高分）

## 設定

| 常數 | 值 | 說明 |
|------|-----|------|
| `DefaultSearchResults` | 10 | 預設返回筆數 |
| `MaxSearchResults` | 20 | 最大返回筆數 |
| `MinSimilarityThreshold` | 0.3 | 最低相似度門檻 (30%) |

## 使用

```go
// 初始化
vectorDB, err := rag.NewVectorDB(dataDir, geminiAPIKey, logger)

// 載入現有資料 (自動分段)
syllabi, _ := db.GetAllSyllabi(ctx)
vectorDB.Initialize(ctx, syllabi)

// 搜尋 (自動去重)
results, err := vectorDB.Search(ctx, "想學機器學習", 10)
for _, r := range results {
    fmt.Printf("%s (%.0f%% 相關)\n", r.Title, r.Similarity*100)
}
```

## 儲存

- 資料持久化: `data/chromem/syllabi/` (gob 格式)
- Document ID 格式: `{UID}_{chunk_type}`
- 啟動時自動載入已索引資料

## 依賴

- `internal/genai`: Gemini embedding 客戶端
- `internal/storage`: Syllabus 資料模型
- `internal/syllabus`: Chunking 邏輯
