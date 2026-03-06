# Course Module

課程查詢模組 - 提供多種課程搜尋方式，包括精確搜尋、智慧搜尋、課號查詢等功能。

## 功能特性

### 支援的查詢方式

#### 1. **精確搜尋**（最近 2 學期）
- **關鍵字**：`課程 [關鍵字]`
- **SQL LIKE 搜尋** + **模糊搜尋**（2-tier search）
- **範圍**：最近 2 個學期（semester 1-2）
- **排序**：最新學期優先

#### 2. **擴展搜尋**（歷史學期）
- **關鍵字**：`更多學期 [關鍵字]`
- **範圍**：接下來 2 個歷史學期（semester 3-4）
- **使用情境**：精確搜尋無結果時的延伸查詢
- **Quick Reply**：📅 更多 按鈕（compact display）

#### 3. **智慧搜尋**（BM25 + Query Expansion）
- **關鍵字**：`找課 [描述]`
- **技術**：BM25 索引 + LLM Query Expansion
- **特色**：
  - 語意搜尋（理解自然語言需求）
  - 相關性評分（0-1，首筆永遠 1.0）
  - 中文分詞（unigram tokenization）
  - 支援縮寫和專業術語
- **範例**：「找課 我想學程式語言」、「找課 AI 機器學習」

#### 4. **課號查詢**
- **格式**：
  - 完整 UID：`1131U0001`（年度+學期+課號）
  - 課號：`U0001`（自動搜尋最近學期）
- **回應**：課程詳情 Flex Message
- **額外資訊**：課程大綱、相關學程

#### 5. **NLU 自然語言查詢**（需要 LLM API Key）
- **Intent Functions**：
  - `course_search` - 精確搜尋（課名/教師）
  - `course_extended` - 延伸搜尋（更多學期）
  - `course_historical` - 歷史搜尋（指定學年）
  - `course_smart` - 智慧搜尋（語意需求）
  - `course_uid` - 課號查詢
- **範例**：「微積分的課有哪些」、「找更多學期的微積分」、「110 學年度的程式設計」、「想學 AI」、「U0001 是什麼課」

### 搜尋限制
- **最大結果數**：40 筆（`MaxCoursesPerSearch`）
  - 4 個輪播（carousel）× 10 個泡泡（bubbles）
  - 預留 1 個訊息位置給警告訊息
  - LINE API 限制：最多 5 個訊息/回應

## 架構設計

### Pattern-Action Table

使用 **Pattern-Action Table** 架構確保 `CanHandle()` 和 `HandleMessage()` 的路由一致性：

```go
type PatternMatcher struct {
    pattern  *regexp.Regexp
    priority int            // 優先級（越小越優先）
    handler  PatternHandler // 處理函數
    name     string         // 用於日誌
}
```

**優先級順序**（1=最高）：
1. **UID** - 完整 UID (e.g., `1131U0001`)
2. **CourseNo** - 課號 (e.g., `U0001`)
3. **Historical** - 歷史查詢 (`課程 110 微積分`)
4. **Smart** - 智慧搜尋 (`找課`)
5. **Extended** - 擴展搜尋 (`更多學期`)
6. **Regular** - 精確搜尋 (`課程`)

### 核心組件

#### Handler 結構
```go
type Handler struct {
    db               *storage.DB
    scraper          *scraper.Client
    metrics          *metrics.Metrics
    logger           *logger.Logger
    stickerManager   *sticker.Manager
    bm25Index        *rag.BM25Index           // 智慧搜尋
    queryExpander    genai.QueryExpander      // LLM Query Expansion
    llmRateLimiter   *ratelimit.KeyedLimiter  // LLM 額度控制
    semesterCache    *SemesterCache           // 共享學期快取
    matchers         []PatternMatcher         // Pattern-Action Table
}
```

#### SemesterCache
- **資料驅動設計**：由 refresh 探測實際資料源更新
- **方法**：
  - `GetRecentSemesters()` - 取得最近 2 個學期
  - `GetExtendedSemesters()` - 取得第 3-4 個學期
  - `GetAllSemesters()` - 取得全部快取的學期
  - `Update(semesters)` - 更新快取（refresh 呼叫）
- **使用情境**：
  - 精確搜尋：`GetRecentSemesters()` → 1-2
  - 擴展搜尋：`GetExtendedSemesters()` → 3-4
  - 避免硬編碼學期範圍

### 搜尋策略

#### 2-Tier Search（精確/擴展搜尋）
1. **SQL LIKE**：`WHERE title LIKE ? OR teachers LIKE ?`
2. **SQL Fuzzy**：`ContainsAllRunes()` - 字元集合匹配（非連續）
3. **排序**：學期由新到舊（semester_sort_key）

#### Smart Search（智慧搜尋）
1. **Query Expansion**：LLM 擴展原始查詢
   - 添加同義詞、相關術語
   - 處理縮寫和專業詞彙
2. **BM25 搜尋**：語意相似度排序
   - k1=1.2, b=0.75（BM25 業界標準預設值）
   - 中文 unigram tokenization
3. **相關性標籤**：
   - 🎯 最佳匹配（深青綠）- 首筆永遠 1.0
   - ✨ 高度相關（青綠）- 相對分數 > 0.6
   - 📋 部分相關（翠綠）- 其他

## Flex Message 設計

### 輪播卡片（Course Carousel）
- **Colored Header**：學期/相關性標籤
  - 藍色系（Data-driven 前四學期）：最新學期 → 上個學期 → 上上學期 → 上上上學期；第 5 個學期（含）後為「過去學期」
  - 青綠色漸層：最佳匹配（深青綠）→ 高度相關（青綠）→ 部分相關（翠綠）
- **Body**：
  - 第一列：`NewBodyLabel()` 學期/相關性標籤（文字色與 header 一致）
  - 課程資訊：課號、教師、時間、地點
  - 重要欄位（教師、時間）使用 `CarouselInfoRowStyleMultiLine()`：`maxLines: 2` + `shrink-to-fit`
- **Footer**：
  - 「詳細資訊」按鈕（顏色與 header 同步）

### 詳情頁（Course Detail）
- **Colored Header**（藍色）：課程名稱
- **Body**：
  - 第一列：📚 課程資訊 標籤（明亮藍色）
  - 完整資訊：課號、學期、教師、必選修、學分、時間、地點、備註
  - 文字使用 `wrap: true` 完整顯示
- **Footer**：
  - 課程大綱按鈕（外部連結）
  - 教師課程按鈕（內部 Postback）
  - 相關學程按鈕（如有）

### Quick Reply
- 使用 `QuickReplyCourseNav(smartSearchEnabled)`
- 包含：📚 課程、🔮 找課（智慧搜尋）、📖 說明

## 資料流程

### 查詢流程
```
User Input
    ↓
Pattern Matching (priority order)
    ↓
┌─ Regular/Extended ─┐   ┌─ Smart Search ─┐   ┌─ UID/CourseNo ─┐
│ GetRecentSemesters│   │ Query Expansion│   │ Parse UID      │
│ SQL LIKE + Fuzzy  │   │ BM25 Search    │   │ GetCourse()    │
│ Sort by semester  │   │ Score + Label  │   │ Detail Flex    │
└───────────────────┘   └────────────────┘   └────────────────┘
    ↓                       ↓                       ↓
Carousel (max 40)       Carousel + Score        Single Bubble
```

### 資料時效策略

> 完整的資料時效策略請參考 [架構說明文件](/.github/copilot-instructions.md#data-layer-cache-first-strategy)

- **TTL**：7 天（依 `NTPU_MAINTENANCE_REFRESH_INTERVAL` 自動更新）
- **範圍**：最近 4 個學期的課程資料

### Syllabus 整合
- **更新時機**：Refresh only（非即時查詢，僅最近 2 個學期）
- **範圍**：最近 2 個有資料的學期
- **用途**：智慧搜尋的語意索引

## 測試覆蓋

### 單元測試
- Pattern matching 測試
- Semester detection 測試
- UID parsing 測試
- Search result formatting 測試

### 整合測試（`-short` flag 跳過）
- Database integration
- Scraper integration
- BM25 search

## 效能考量

### 搜尋優化
- **SQL 索引**：year, term, title, teachers
- **BM25 in-memory**：避免每次重建索引
- **Rate limiting**：LLM API 調用限制
- **結果截斷**：最多 40 筆避免訊息過載

### Memory 管理
- BM25 索引：~10-20MB（取決於課程數量）
- Query Expansion：每次調用 ~1KB
- 結果集：限制 40 筆避免過度消耗

## 相關文件
- Handler: `internal/modules/course/handler.go`
- Semester: `internal/modules/course/semester.go`
- Tests: `internal/modules/course/handler_test.go`
- BM25: `internal/rag/bm25.go`
- Syllabus: `internal/syllabus/scraper.go`
- Query Expansion: `internal/genai/`

## 依賴關係
- `storage.DB` - 課程資料查詢
- `scraper.Client` - 即時抓取
- `rag.BM25Index` - 智慧搜尋（可選）
- `genai.QueryExpander` - 查詢擴展（可選）
- `ratelimit.KeyedLimiter` - LLM 額度控制（可選）
