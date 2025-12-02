# rag

Retrieval-Augmented Generation (RAG) 模組，使用 chromem-go 進行課程大綱的向量搜尋。

## 功能

- **VectorDB**: 封裝 chromem-go 向量資料庫
- **SearchResult**: 語意搜尋結果結構
- **Chunking**: 語意分段提升搜尋準確度
- **去重**: 同一課程多個 chunk 只保留最高相似度

## 架構

```
RAG Flow (with Chunking):
  課程大綱 → 語意分段 (教學目標/內容綱要/教學進度)
      ↓
  CN + EN 合併 → Gemini Embedding → chromem-go Vector Store
      ↓
  使用者查詢 → Embedding → 餘弦相似度搜尋 → 去重合併 → 過濾低相似度 → 排序結果
```

## Chunking 策略

**問題**: Asymmetric semantic search（短查詢 vs 長文檔）會導致相似度分數偏低

**解決方案**: 按語意欄位分段，中英文合併

| Chunk | 內容 | 用途 |
|-------|------|------|
| `{UID}_objectives` | 【課程名稱】教學目標：{CN} {EN} | 匹配「想學什麼」類查詢 |
| `{UID}_outline` | 【課程名稱】內容綱要：{CN} {EN} | 匹配主題/內容查詢 |
| `{UID}_schedule` | 【課程名稱】教學進度：... | 匹配週次/進度查詢 |

**設計原則** (參考 2025 RAG Chunking 研究):
- 每個欄位本身是語意完整的單元，不需額外截斷
- 中英文內容合併提升多語言搜尋效果
- Gemini embedding 支援 2048 tokens (~8000 字元)
- 課程名稱前綴提供上下文，改善短查詢的匹配

## 設定

| 常數 | 值 | 說明 |
|------|-----|------|
| `DefaultSearchResults` | 10 | 預設返回筆數 |
| `MaxSearchResults` | 100 | 最大返回筆數 |
| `MinSimilarityThreshold` | 0.3 | 最低相似度門檻 (30%) |
| `HighRelevanceThreshold` | 0.7 | 高度相關門檻 (70%) |

## 搜尋結果邏輯

1. **高度相關 (≥70%)**: 全部返回
2. **結果補足**: 向上取整到 10 的倍數 (例如 13 個 ≥70% → 返回 20 個)
3. **最少保證**: 至少返回 10 個結果 (如果有足夠資料)
4. **去重**: 同課程只保留最高相似度的結果

## 使用

```go
// 初始化
vectorDB, err := rag.NewVectorDB(dataDir, geminiAPIKey, logger)

// 載入現有資料 (自動分段)
syllabi, _ := db.GetAllSyllabi(ctx)
vectorDB.Initialize(ctx, syllabi)

// 搜尋 (自動去重，70% 高度相關全部返回)
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
