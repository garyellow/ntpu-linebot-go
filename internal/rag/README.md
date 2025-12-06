# rag

Retrieval-Augmented Generation (RAG) 模組，提供課程智慧搜尋功能。

> ℹ️ 注意：本模組使用 **BM25 關鍵字搜尋 + LLM Query Expansion**，不是真正的「語意搜尋」(Semantic Search / Embedding-based Vector Search)。

## 功能

- **BM25Index**: BM25 關鍵字搜尋索引 (中文分詞優化)
- **Query Expansion**: LLM 擴展查詢詞彙（同義詞、縮寫、翻譯）
- **Rank-based Confidence**: 基於排名的信心分數

## 架構

```
Search Flow:
  使用者查詢 → Query Expansion (LLM 擴展)
      ↓
  ┌──────────────────────────────────┐
  │ BM25 Search (expanded keywords)  │
  │ - 中文 Unigram 分詞              │
  │ - IDF 加權                       │
  │ - 排名信心分數                   │
  └──────────────────────────────────┘
      ↓
  去重合併 → 排序結果
```

### BM25 實作

使用 [iwilltry42/bm25-go](https://github.com/iwilltry42/bm25-go) 外部函式庫：

- **可靠維護者**：由 [k3d-io/k3d](https://github.com/k3d-io/k3d) (⭐6.1k) 維護者維護
- **已修復 IDF 問題**：解決了常見 Go BM25 庫的負 IDF 值問題
- **BM25Okapi 參數**：k1=1.5, b=0.75（標準值）
- **中文分詞**：CJK 字元使用 Unigram（單字元），非 CJK 保持完整詞彙
- **大小寫不敏感**：所有 token 轉為小寫
- **線程安全**：使用 `sync.RWMutex` 保護索引操作

## 為什麼不用 Embedding？


| 考量 | BM25 + Query Expansion | Embedding (Vector) |
|------|------------------------|-------------------|
| **小型語料庫 (<10K)** | ✅ 足夠有效 | 過度設計 |
| **精確關鍵字匹配** | ✅ 原生支援 | 需要調整 |
| **API 成本** | ✅ 僅查詢擴展 | 每次查詢都需 API |
| **延遲** | ✅ <10ms | 100-500ms |
| **縮寫/同義詞** | ✅ 透過 Query Expansion | 語意匹配但不精確 |
| **複雜度** | ✅ 簡單易維護 | 需要向量資料庫 |

**結論**: 對於 ~2000 門課程的語料庫，BM25 + Query Expansion 已足夠，Embedding 是過度工程。

## 相關度顯示

### 設計原則

BM25 輸出無界分數，不同查詢間無法比較。基於 chatbot UX 研究，我們採用**簡化兩層制**：

| 排名 | 顯示標籤 | 設計理由 |
|------|----------|----------|
| #1-3 | 🎯 最相關 | 前三名，高信心推薦 |
| #4+ | ✨ 相關 | 後續結果，仍符合查詢 |

**為什麼這樣設計？**

1. **移除百分比**：使用者無法跨查詢比較數字，百分比造成虛假精確感
2. **簡化分類**：4 層變 2 層，降低認知負擔
3. **基於排名**：「前三名」比「80% 信心」更直觀
4. **引導改進**：結果少時提示使用更具體關鍵字

### 內部計算 (保留供參考)

```go
// 內部信心分數計算（目前僅用於過濾，不顯示給使用者）
confidence = 1 / (1 + 0.05 * rank)
// rank 1: 95%, rank 5: 80%, rank 10: 67%
```

## 索引策略：Single Document (BM25 最佳實踐)

每門課程 = 一個文檔，不做 chunking。

### 為什麼不像 Embedding 那樣分 Chunk？

| 考量 | BM25 | Embedding |
|------|------|-----------|
| **Token 限制** | ❌ 無限制 | ✅ 512-8192 tokens |
| **長度正規化** | ✅ b=0.75 參數處理 | ❌ 無（需 chunking） |
| **IDF 準確度** | ✅ 1課程=1文檔最準確 | N/A |
| **去重邏輯** | ❌ 不需要 | ✅ 需要（多 chunk 對應同一文檔） |
| **實作複雜度** | ✅ 簡單 | 較複雜 |

### 文檔格式

```
【課程名稱】
教學目標：{ObjectivesCN}
Course Objectives: {ObjectivesEN}
內容綱要：{OutlineCN}
Course Outline: {OutlineEN}
教學進度：{Schedule}
```

**設計原則**:
- 單一文檔策略：每門課程合併所有欄位成一個文檔
- BM25 長度正規化 (b=0.75) 自動處理不同長度的文檔
- 課程名稱前綴提供上下文，改善短查詢的匹配
- 無需去重邏輯：1 UID = 1 document

## 使用

```go
// 初始化 BM25 索引
bm25Index := rag.NewBM25Index(logger)

// 載入資料
syllabi, _ := db.GetAllSyllabi(ctx)
bm25Index.Initialize(syllabi)

// 搜尋課程
results, err := bm25Index.SearchCourses(ctx, "雲端運算 AWS", 10)
for _, r := range results {
    fmt.Printf("%s (%.0f%% 信心)\n", r.Title, r.Confidence*100)
}

// 配合 Query Expansion
expander, _ := genai.NewQueryExpander(ctx, geminiAPIKey)
expanded, _ := expander.Expand(ctx, "AWS")
// "AWS" → "AWS Amazon Web Services 雲端服務 雲端運算"
results, _ = bm25Index.SearchCourses(ctx, expanded, 10)
```

## 儲存

- BM25 索引: 記憶體中，每次啟動時從 SQLite 重建
- 1 UID = 1 Document（單一文檔策略）
- 啟動時自動從 syllabi 表載入資料

## 依賴

- `internal/genai`: Query Expander（可選，需 Gemini API Key）
- `internal/storage`: Syllabus 資料模型
- `internal/syllabus`: Syllabus 欄位處理與內容生成
