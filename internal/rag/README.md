# rag

Retrieval-Augmented Generation (RAG) 模組，提供課程智慧搜尋功能。

> ℹ️ 注意：本模組使用 **BM25 關鍵字搜尋 + LLM Query Expansion**，不是真正的「語意搜尋」(Semantic Search / Embedding-based Vector Search)。

## 功能

- **BM25Index**: BM25 關鍵字搜尋索引 (中文分詞優化)
- **Query Expansion**: LLM 擴展查詢詞彙（同義詞、縮寫、翻譯）
- **Per-Semester Indexing**: 每學期獨立索引，獨立計算 IDF 和信心度
- **Newest 2 Semesters**: 搜尋僅返回最新 2 學期課程
- **Token Cache**: SQLite 持久化分詞結果，跨重啟重用，避免重複呼叫 gse

## 架構

```
Per-Semester Index Architecture:
┌─────────────────────────────────────────────────────────────┐
│ BM25Index                                                    │
├─────────────────────────────────────────────────────────────┤
│ semesterIndexes:                                             │
│   ├── 114-1 → semesterIndex (獨立 BM25 engine, 獨立 IDF)    │
│   ├── 113-2 → semesterIndex (獨立 BM25 engine, 獨立 IDF)    │
│   └── 113-1 → semesterIndex (獨立 BM25 engine, 獨立 IDF)    │
│                                                              │
│ allSemesters: [114-1, 113-2, 113-1] (排序後，最新在前)       │
└─────────────────────────────────────────────────────────────┘

Search Flow:
  使用者查詢 → Query Expansion (LLM 擴展)
      ↓
  ┌──────────────────────────────────┐
  │ 選擇最新 2 學期                   │
  │ (從 allSemesters 取前 2 個)       │
  └──────────────────────────────────┘
      ↓
  ┌──────────────────────────────────┐
  │ 114-1 搜尋 → 取 Top-10           │  ← 獨立 IDF
  │ 計算信心度 (best = 1.0)          │
  └──────────────────────────────────┘
      ↓
  ┌──────────────────────────────────┐
  │ 113-2 搜尋 → 取 Top-10           │  ← 獨立 IDF
  │ 計算信心度 (best = 1.0)          │
  └──────────────────────────────────┘
      ↓
  合併結果（最多 20 門課）
```

### 為什麼 Per-Semester Indexing？

| 舊架構（單一索引） | 新架構（Per-Semester） |
|------------------|----------------------|
| 所有學期共用 IDF | 每學期獨立 IDF |
| 「雲端」重要性受所有學期影響 | 「雲端」重要性只與同學期課程比較 |
| 大學期可能佔據所有結果 | 每學期公平取 Top-10 |

**核心優勢**：課程相關度只與同學期課程比較，不受其他學期影響。

### BM25 實作

使用 **自行實作的 BM25 Okapi 引擎**（`internal/rag/engine.go`）：

- **倒排索引**：建立階段一次分詞所有文件，查詢時零 tokenizer 呼叫
- **預計算 IDF**：索引建立時計算，採用 Lucene 風格 `log(1 + (N-df+0.5)/(df+0.5))` 公式，永遠非負（無 min-IDF 參數）
- **BM25Okapi 參數**：k1=1.2, b=0.75（業界標準預設值，Lucene/Elasticsearch/Azure 共識）
- **分詞策略**：文件索引用 `CutSearchAll`（保留重複 token，計入 TF 和文件長度），查詢用 `CutSearch`（去重，同一 term 出現兩次無額外信號）
- **並行分詞**：Cache miss 時以 GOMAXPROCS 大小的 goroutine pool 並行分詞，gse Segmenter 初始化後為唯讀、並發安全
- **大小寫不敏感**：所有 token 轉為小寫
- **線程安全**：`Initialize` 在無鎖下完成所有 CPU 密集工作，最後以 O(1) 原子指標交換完成上線

### Token Cache（分詞快取）

每次 `Initialize` 將 gse 分詞結果持久化到 `storage.syllabus_tokens` 表，下次啟動可直接取用，省去重複分詞開銷。

```
Initialize(ctx, db)
  ↓  GetDistinctSemesters                       (讀 syllabi)
  ↓  GetSyllabiByYearTerm (per semester)       (讀 syllabi)
  ↓  GetSyllabusTokensBatch (JOIN uid+hash)    ← 快取讀取
      ┌── 命中 (content_hash 一致)  → 直接使用已存 tokens
      └── 未命中 (hash 已變更或初次) → gse 重新分詞
  ↓  buildSemesterIndex (CPU 並行)
  ↓  原子指標交換 (O(1) 加鎖)
  ↓  SaveSyllabusTokensBatch                   ← 寫入未命中的新 tokens
```

**快取一致性**：`GetSyllabusTokensBatch` 使用 `JOIN syllabi ON content_hash = content_hash`，確保只返回與當前 syllabi 版本一致的 tokens；更換後的舊 tokens 自動因 JOIN 不匹配而被過濾，無需主動失效。

**孤兒清理**：`DeleteStaleSyllabusTokens` （`NOT EXISTS` 子查詢）移除 content_hash 已不存在於 syllabi 的過期 token 行，由 `runDataCleanup` 定期觸發。

**序列化格式**：tokens 以空白分隔整行字串儲存（`strings.Join / strings.Fields`），避免 JSON 解析開銷；gse 產生的 token 本身不含空白，序列化是無損的。

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

1. **相對分數**：使用 `score / maxScore` 計算，同學期內可比較（Azure、Elasticsearch 官方建議）
2. **Per-Semester 計算**：每學期獨立計算，每學期的最佳結果 = 1.0
3. **三層分類**：0.8、0.6 分界點基於 Normal-Exponential 混合模型（Arampatzis et al., 2009）
4. **Per-Semester Top-K**：每學期獨立取 Top-10，確保兩學期公平展示
5. **不使用 log**：BM25 的 IDF 已包含 log 轉換，無需額外處理

### 內部計算

```go
// 相對分數計算（學術最佳實踐）
confidence = score / maxScore
// 每學期的第一名永遠是 1.0

// 分類閾值（基於 Normal-Exponential 混合模型）
// >= 0.8: 最佳匹配 (Normal 分佈核心)
// >= 0.6: 高度相關 (混合區域)
// < 0.6: 部分相關 (Exponential 尾部)

// Per-Semester Top-K 策略
MaxSearchResults = 10  // 每學期最多 10 門課，總共最多 20 門
```

**學術依據**：
- BM25 分數遵循 Normal-Exponential 混合分佈（相關文件=Normal，非相關=Exponential）
- 相對閾值優於絕對閾值（Arampatzis et al., 2009）
- Per-Semester indexing 確保 IDF 反映「在這個學期中該 term 的重要性」

## 索引策略：Single Document + Per-Semester

每門課程 = 一個文檔，每個學期 = 一個獨立索引。

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
- 每學期獨立索引：IDF 只反映該學期的 term 分佈
- BM25 長度正規化 (b=0.75) 自動處理不同長度的文檔
- 課程名稱前綴提供上下文，改善短查詢的匹配

## 使用

```go
// 初始化 BM25 索引（使用共享中文分詞器）
seg := stringutil.NewSegmenter()
bm25Index := rag.NewBM25Index(logger, seg)

// 從資料庫載入資料（自動按學期分組）
err := bm25Index.Initialize(ctx, db)

// 搜尋課程（返回最新 2 學期，各取 Top-10）
results, err := bm25Index.SearchCourses(ctx, "雲端運算 AWS", 10)
for _, r := range results {
    fmt.Printf("[%d-%d] %s (%.0f%% 信心)\n", r.Year, r.Term, r.Title, r.Confidence*100)
}

// 配合 Query Expansion
llmCfg := genai.LLMConfig{
    Gemini: genai.ProviderConfig{APIKey: geminiAPIKey},
    Providers: []genai.Provider{genai.ProviderGemini},
    RetryConfig: genai.RetryConfig{MaxAttempts: 2},
}
expander, _ := genai.CreateQueryExpander(ctx, llmCfg)
expanded, _ := expander.Expand(ctx, "AWS")
// "AWS" → "AWS Amazon Web Services 雲端服務 雲端運算"
results, _ = bm25Index.SearchCourses(ctx, expanded, 10)
```

## 儲存

- BM25 索引: 記憶體中，每次啟動時從 SQLite 重建
- Per-Semester: 每個學期有獨立的 BM25 engine
- 啟動時自動從 syllabi 表載入並按學期分組

## 依賴

- `internal/genai`: Query Expander（可選，需 Gemini、Groq 或 Cerebras API Key）
- `internal/storage`: Syllabus 資料模型
- `internal/syllabus`: Syllabus 欄位處理與內容生成
