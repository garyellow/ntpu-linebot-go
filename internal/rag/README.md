# rag

Retrieval-Augmented Generation (RAG) 模組，提供課程智慧搜尋功能。

> ℹ️ 注意：本模組使用 **BM25 關鍵字搜尋 + LLM Query Expansion**，不是真正的「語意搜尋」(Semantic Search / Embedding-based Vector Search)。

## 功能

- **BM25Index**: BM25 關鍵字搜尋索引 (中文分詞優化)
- **Query Expansion**: LLM 擴展查詢詞彙（同義詞、縮寫、翻譯）
- **Relative Confidence**: 基於相對 BM25 分數的信心度 (score / maxScore)
- **Newest Semester Filter**: 智慧搜尋僅返回最新學期課程（data-driven）

## 架構

```
Search Flow:
  使用者查詢 → Query Expansion (LLM 擴展)
      ↓
  ┌──────────────────────────────────┐
  │ BM25 Search (expanded keywords)  │
  │ - 中文 Unigram 分詞              │
  │ - IDF 加權                       │
  │ - 僅最新學期過濾                 │
  │ - 相對信心分數 (score/maxScore) │
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
- **學期過濾**：SearchCourses() 自動過濾至最新學期（data-driven）

## 為什麼不用 Embedding?


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

BM25 輸出無界分數，不同查詢間無法比較。我們使用**相對分數** (score / maxScore) 進行分類：

| 條件 | 顯示標籤 | 設計理由 |
|------|----------|----------|
| Confidence ≥ 0.8 | 🎯 最佳匹配 | Normal 分佈核心區域（學術建議） |
| Confidence ≥ 0.6 | ✨ 高度相關 | Normal-Exponential 混合區域 |
| Confidence < 0.6 | 📋 部分相關 | Exponential 分佈尾部 |

**為什麼這樣設計？**

1. **相對分數**：使用 `score / maxScore` 計算，同一查詢內可比較（Azure、Elasticsearch 官方建議）
2. **無過濾門檻**：不預先過濾結果，交由 UI 層分類顯示
3. **三層分類**：0.8、0.6 分界點基於 Normal-Exponential 混合模型（Arampatzis et al., 2009）
4. **Top-K 優先**：主要使用 Top-10 截斷，相對分數僅用於結果分類
5. **不使用 log**：BM25 的 IDF 已包含 log 轉換，無需額外處理

### 內部計算

```go
// 相對分數計算（學術最佳實踐）
confidence = score / maxScore
// 第一名永遠是 1.0

// 分類閾值（基於 Normal-Exponential 混合模型）
// >= 0.8: 最佳匹配 (Normal 分佈核心)
// >= 0.6: 高度相關 (混合區域)
// < 0.6: 部分相關 (Exponential 尾部)

// Top-K 截斷（主要過濾方法）
MaxSearchResults = 10
```

**學術依據**：
- BM25 分數遵循 Normal-Exponential 混合分佈（相關文件=Normal，非相關=Exponential）
- 相對閾值優於絕對閾值（Arampatzis et al., 2009）
- Top-K 是 BM25 的標準過濾方法（Azure AI Search、Elasticsearch 推薦）

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
教學目標：本課程介紹雲端運算基礎概念，包含 AWS EC2, S3, Lambda 等服務
Introduction to cloud computing with AWS services
內容綱要：1. 雲端運算概論 2. AWS 架構 3. EC2 虛擬機器 4. S3 儲存服務
1. Cloud Computing Overview 2. AWS Architecture 3. EC2 4. S3
教學進度：Week 1: 課程介紹
Week 2: AWS Academy
```

**設計原則**:
- 單一文檔策略：每門課程合併所有欄位成一個文檔
- BM25 長度正規化 (b=0.75) 自動處理不同長度的文檔
- 課程名稱前綴提供上下文，改善短查詢的匹配
- 無需去重邏輯：1 UID = 1 document
- 使用 show_info=all 格式，所有欄位已包含中英文合併內容

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
llmCfg := genai.DefaultLLMConfig()
llmCfg.Gemini.APIKey = geminiAPIKey
expander, _ := genai.CreateQueryExpander(ctx, llmCfg)
expanded, _ := expander.Expand(ctx, "AWS")
// "AWS" → "AWS Amazon Web Services 雲端服務 雲端運算"
results, _ = bm25Index.SearchCourses(ctx, expanded, 10)
```

## 儲存

- BM25 索引: 記憶體中，每次啟動時從 SQLite 重建
- 1 UID = 1 Document（單一文檔策略）
- 啟動時自動從 syllabi 表載入資料

## 依賴

- `internal/genai`: Query Expander（可選，需 Gemini 或 Groq API Key）
- `internal/storage`: Syllabus 資料模型
- `internal/syllabus`: Syllabus 欄位處理與內容生成
