# Program Module

學程查詢模組 - 提供學程列表、學程搜尋、學程課程查詢等功能，並與課程模組深度整合。

## 功能特性

### 支援的查詢方式

#### 1. **學程列表**
- **關鍵字**：`學程` / `所有學程` / `學程列表`
- **顯示**：所有可修讀學程（依類別分組）
- **來源**：課程列表 + 課程大綱雙來源融合（名稱準確 + 必/選修正確）

#### 2. **學程搜尋**
- **關鍵字**：`學程 [關鍵字]`
- **搜尋策略**：
  - SQL LIKE 搜尋
  - 模糊搜尋 `ContainsAllRunes()`（字元集合匹配）
- **範例**：`學程 人工智慧`、`學程 永續`

#### 3. **學程課程查詢**
- **觸發**：Postback `program:courses$[學程名稱]`
- **顯示**：該學程的所有必修/選修課程
- **排序**：
  - 必修課程優先
  - 學期由新到舊

#### 4. **課程關聯查詢**
- **觸發**：課程詳情頁「相關學程」按鈕
- **Postback**：`program:course_programs$[課程 UID]`
- **顯示**：包含該課程的所有學程

#### 5. **NLU 自然語言查詢**（需要 LLM API Key）
- **Intent Functions**：
  - `program_list` - 列出所有學程
  - `program_search` - 搜尋特定學程
  - `program_courses` - 查詢學程課程
- **範例**：「有哪些學程」、「人工智慧學程」、「人工智慧學程有哪些課」

## 架構設計

### Pattern-Action Table

使用與 course 模組一致的 **Pattern-Action Table** 架構：

```go
type PatternMatcher struct {
    pattern  *regexp.Regexp
    priority int            // 1=list, 2=search
    handler  PatternHandler
    name     string
}
```

**優先級順序**：
1. **List** - 學程列表（無參數）
2. **Search** - 學程搜尋（提取關鍵字）

### Handler 結構

```go
type Handler struct {
    db             *storage.DB
    metrics        *metrics.Metrics
    logger         *logger.Logger
    stickerManager *sticker.Manager
    semesterCache  *course.SemesterCache  // 共享學期快取
    programCache   *ProgramListCache      // 短 TTL 快取：GetAllPrograms 結果
    matchers       []PatternMatcher
}
```

### 資料來源（雙來源融合）

學程資料採 **課程列表 + 課程大綱** 的雙來源融合，於刷新任務時同步：

```
Course Refresh (interval-based)
    ↓
ScrapeCourses() - 課程列表頁 (queryByKeyword)
    ↓
parseMajorAndTypeFields() - 擷取「應修系級」+「必選修別」(原始名稱 + 必/選)
    ↓
RawProgramReqs (UID → []{name,type})

Syllabus Refresh (interval-based, after course refresh)
    ↓
ScrapeCourseDetail() - 課程大綱頁面 (queryguide)
    ↓
parseProgramNamesFromDetailPage() - 完整學程名稱 (Major 欄位)
    ↓
MatchProgramTypes() - 模糊比對，將「必/選」對齊到完整名稱
    ↓
SaveCoursePrograms() → course_programs 表
```

**為何使用雙來源**：
- 課程列表頁 (queryByKeyword) 的應修系級欄位 **有必/選修資訊**，但名稱可能縮寫或不完整
- 課程大綱頁 (queryguide) 的 Major 欄位 **名稱完整且準確**，但不提供學程必/選修

**範例**（課程大綱頁 HTML）：
```html
應修系級 Major:<b class="font-c15">統計學系3, 商業智慧與大數據分析學士學分學程, ...</b>
```

## 資料庫設計

### course_programs 表
```sql
CREATE TABLE course_programs (
    course_uid   TEXT NOT NULL,  -- 課程 UID (e.g., 1131U0001)
    program_name TEXT NOT NULL,  -- 學程名稱
    course_type  TEXT NOT NULL,  -- 必修/選修
    cached_at    INTEGER NOT NULL,
    PRIMARY KEY (course_uid, program_name)
);
```

**索引**：
- `course_uid` - 快速查詢課程的學程
- `program_name` - 快速查詢學程的課程

## Flex Message 設計

### 學程輪播（Program Carousel）
- **Colored Header**（藍色）：學程名稱
- **Body**：
    - 第一列：🎓/📚/📌 學程類別標籤（依學程類型）
    - 學程類別（如有）
  - 課程數量統計
- **Footer**：
  - 「查看課程」按鈕 → Postback: `program:courses$[學程名稱]`

### 課程輪播（Courses in Program）
- **Colored Header**：課程類型標籤
  - 綠色：必修課程
  - 青色：選修課程
- **Body**：
  - 第一列：課程類型標籤（文字色與 header 一致）
  - 課程資訊：課號、教師、學期、時間
- **Footer**：
  - 「詳細資訊」按鈕 → 跳轉課程詳情

### 學程列表（Programs for Course）
- **Bubble List**：包含該課程的學程列表
- **按鈕**：每個學程一個「查看課程」按鈕

### Quick Reply
- **統一設計**：所有 Quick Reply 函數定義在 `internal/lineutil/builder.go`
- **Actions**：
    - `lineutil.QuickReplyProgramListAction()` - 🗂️ 學程列表
    - `lineutil.QuickReplyProgramAction()` - 🧭 學程
  - `lineutil.QuickReplyHelpAction()` - 📖 說明
- **Navigation**：`lineutil.QuickReplyProgramNav()` 組合上述動作
- **一致性**：與其他模組（course, id, contact, usage）保持相同模式

## 搜尋策略

### 2-Tier Search
1. **SQL LIKE**：`WHERE name LIKE ?`
2. **SQL Fuzzy**：`ContainsAllRunes()` 字元匹配

### 排序邏輯
- **學程列表**：依學程名稱排序
- **課程列表**：
  1. 必修課程優先（`course_type='必修'`）
  2. 學期由新到舊（semester_sort_key）

## 與 Course 模組整合

### 雙向關聯
```
Course Detail
    ↓ (相關學程按鈕)
Program List (for this course)
    ↓ (查看課程按鈕)
Program Courses
    ↓ (詳細資訊按鈕)
Course Detail (返回)
```

### 共享組件
- **SemesterCache**：course 模組提供，refresh 更新，program 使用
- **ProgramListCache**：program 模組內部，短 TTL（30s），降低 `GetAllPrograms` 重複 JOIN 查詢
- **Flex Message Builders**：共用 lineutil 工具

### Postback 路由
- `program:courses$[學程名稱]` - 查看學程課程
- `program:course_programs$[課程 UID]` - 查看課程學程

## 資料流程

### 查詢流程
```
User Input
    ↓
Pattern Matching (list > search)
    ↓
┌─ List ──────────┐   ┌─ Search ────────┐
│ GetAllPrograms()│   │ SearchPrograms()│
│ Group by type   │   │ SQL LIKE + Fuzzy│
│ Count courses   │   │ Sort by name    │
└─────────────────┘   └─────────────────┘
    ↓                       ↓
Program Carousel        Program Carousel
```

### Postback 流程
```
Postback: program:courses$AI學程
    ↓
GetProgramCourses("AI學程")
    ↓
Group by type (必修/選修)
    ↓
Sort by semester (newest first)
    ↓
Build Course Carousel (colored by type)
```

### 資料同步
```
Refresh (interval-based)
    ↓
Probe Semesters (scraper)
    ↓
Refresh Courses (course module)
    ↓
Syllabus Refresh (most recent 2 semesters)
    ↓
ScrapeCourseDetail() → Extract Syllabus + Programs
    ↓
SaveCoursePrograms() → course_programs table
    ↓
semesterCache.Update() (shared)
```

## 測試覆蓋

### 單元測試
- Pattern matching 測試
- Program search 測試
- Course grouping 測試
- Postback parsing 測試

### 整合測試
- Database queries
- Course module integration

## 效能考量

### 查詢優化
- **索引**：course_uid, program_name
- **結果限制**：最多 40 筆課程
- **快取**：7-day TTL

### Memory 使用
- 學程列表：輕量級查詢（< 100 筆）
- 課程列表：可能較大（限制 40 筆）
- **`ProgramListCache`**：短 TTL（30s）記憶體快取，避免相同學期條件的重複 JOIN 查詢

## 限制與注意事項

### 資料來源限制
- **雙來源融合**：學程名稱來自課程大綱頁，必/選修來自課程列表頁
- **Refresh 依賴**：需先完成 course refresh 才能帶入必/選修，再由 syllabus refresh 做匹配
- **啟用條件**：syllabus refresh 需設定 LLM API Key 才會啟用
- **解析規則**：只提取以「學程」結尾的項目（排除系所）

### 資料品質
- **完整性**：課程大綱頁面提供完整且準確的學程名稱
- **正確性**：必/選修以課程列表頁為準（避免大綱缺欄位）
- **時效性**：依 `NTPU_MAINTENANCE_REFRESH_INTERVAL` 同步更新（最近 2 學期）
- **覆蓋範圍**：只包含有開課的學程

## 相關文件
- Handler: `internal/modules/program/handler.go`
- Flex: `internal/modules/program/flex.go`
- Tests: `internal/modules/program/handler_test.go`
- Course Module: `internal/modules/course/`

## 依賴關係
- `storage.DB` - 學程/課程資料查詢
- `course.SemesterCache` - 學期快取（共享）
- `ProgramListCache` - 短 TTL 快取（模組內部）
- `metrics.Metrics` - 監控指標
- `logger.Logger` - 日誌記錄
- `sticker.Manager` - Sender 頭像
