# rag

Retrieval-Augmented Generation (RAG) 模組，提供課程語意搜尋功能。

## 功能

- **HybridSearcher**: BM25 關鍵字搜尋 + Vector 語意搜尋的混合搜尋器
- **VectorDB**: 封裝 chromem-go 向量資料庫 (Gemini Embedding)
- **BM25Index**: BM25 關鍵字搜尋索引 (中文分詞優化)
- **RRF Fusion**: Reciprocal Rank Fusion 結果融合演算法
- **去重**: 同一課程多個 chunk 只保留最高分結果

## 架構

```
Hybrid Search Flow:
  使用者查詢 → Query Expansion (LLM 擴展)
      ↓
  ┌─────────────────────────────────────────┐
  │         Parallel Search                  │
  │  ┌─────────────┐   ┌──────────────────┐ │
  │  │ BM25 Search │   │ Vector Search    │ │
  │  │ (keyword)   │   │ (semantic)       │ │
  │  └─────────────┘   └──────────────────┘ │
  └─────────────────────────────────────────┘
      ↓
  RRF Fusion (k=60, BM25:0.4, Vector:0.6)
      ↓
  去重合併 → 排序結果
```

## Hybrid Search 策略

**問題**: 純 Vector 搜尋對精確關鍵字匹配效果不佳；純 BM25 對語意相似但詞彙不同的查詢效果差

**解決方案**: Hybrid Search (BM25 + Vector) with RRF Fusion

| 搜尋方法 | 權重 | 優勢 |
|----------|------|------|
| Vector Search | 60% | 語意相似度、跨語言匹配 |
| BM25 Search | 40% | 精確關鍵字匹配、縮寫擴展 |

**RRF 公式**: `score(d) = Σ (w_i / (k + rank_i))`
- k = 60 (標準值，平衡頂部與長尾結果)
- rank_i = 該結果在各搜尋方法中的排名

## Chunking 策略

**解決方案**: 按語意欄位分段，中英文分開

| Chunk | 內容 | 用途 |
|-------|------|------|
| `{UID}_objectives_cn` | 【課程名稱】教學目標：{CN} | 匹配中文查詢 |
| `{UID}_objectives_en` | 【課程名稱】Course Objectives: {EN} | 匹配英文查詢 |
| `{UID}_outline_cn` | 【課程名稱】內容綱要：{CN} | 匹配主題查詢 |
| `{UID}_outline_en` | 【課程名稱】Course Outline: {EN} | 匹配英文主題 |
| `{UID}_schedule` | 【課程名稱】教學進度：... | 匹配週次/進度查詢 |

**設計原則** (2025 RAG 最佳實踐):
- 中英文分開建立獨立 chunk，提升語意匹配清晰度
- Gemini embedding 支援 2048 tokens (~8000 字元)
- 課程名稱前綴提供上下文，改善短查詢的匹配

## 設定

| 常數 | 值 | 說明 |
|------|-----|------|
| `RRFConstant` | 60 | RRF 公式常數 k |
| `DefaultBM25Weight` | 0.4 | BM25 預設權重 (40%) |
| `DefaultVectorWeight` | 0.6 | Vector 預設權重 (60%) |
| `MinSimilarityThreshold` | 0.3 | 最低相似度門檻 (30%) |
| `HighRelevanceThreshold` | 0.7 | 高度相關門檻 (70%) |

## 使用

```go
// 初始化各組件
vectorDB, _ := rag.NewVectorDB(dataDir, geminiAPIKey, logger)
bm25Index := rag.NewBM25Index(logger)
hybridSearcher := rag.NewHybridSearcher(vectorDB, bm25Index, logger)

// 載入資料 (同時初始化 BM25 和 Vector 索引)
syllabi, _ := db.GetAllSyllabi(ctx)
hybridSearcher.Initialize(ctx, syllabi)

// Hybrid 搜尋 (BM25 + Vector with RRF)
results, err := hybridSearcher.Search(ctx, "我想學 AWS", 10)
for _, r := range results {
    fmt.Printf("%s (%.0f%% 相關)\n", r.Title, r.Similarity*100)
}

// 也可以單獨使用 BM25 或 Vector
bm25Results, _ := bm25Index.Search("cloud computing", 10)
vectorResults, _ := vectorDB.Search(ctx, "雲端運算", 10)
```

## Fallback 機制

- **雙引擎可用**: 使用 RRF 融合兩者結果
- **僅 BM25 可用**: 使用 BM25 結果 (無需 API Key)
- **僅 Vector 可用**: 使用 Vector 結果
- **均不可用**: 返回空結果

## 儲存

- Vector 持久化: `data/chromem/syllabi/` (gob 格式)
- BM25 索引: 記憶體中，每次啟動時重建
- Document ID 格式: `{UID}_{chunk_type}`
- 啟動時自動載入已索引資料

## 依賴

- `internal/genai`: Gemini embedding 客戶端、Query Expander
- `internal/storage`: Syllabus 資料模型
- `internal/syllabus`: Chunking 邏輯
- `github.com/crawlab-team/bm25`: BM25 演算法實作
- `github.com/philippgille/chromem-go`: Vector 資料庫
