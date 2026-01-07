# Program Module

學程查詢模組 - 提供學程列表、學程搜尋、學程課程查詢等功能，並與課程模組深度整合。

## 功能特性

### 支援的查詢方式

#### 1. **學程列表**
- **關鍵字**：`學程` / `所有學程` / `學程列表`
- **顯示**：所有可修讀學程（依類別分組）
- **來源**：從課程資料解析（`應修系級` 欄位）

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
- **範例**：「有哪些學程」、「人工智慧學程」

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
    db               *storage.DB
    metrics          *metrics.Metrics
    logger           *logger.Logger
    stickerManager   *sticker.Manager
    semesterDetector *course.SemesterDetector  // 共享學期偵測器
    matchers         []PatternMatcher
}
```

### 資料來源

學程資料**不是獨立抓取**，而是從課程資料解析：

```
Course.應修系級 欄位
    ↓ (過濾)
以「學程」結尾的項目
    ↓ (解析)
program_name + course_type (必修/選修)
    ↓ (存入)
course_programs 表
```

**範例**：
```
應修系級：「資訊工程學系3A」、「人工智慧學程」
→ 提取：「人工智慧學程」
→ 存入：program_name="人工智慧學程", course_type="必修"
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
  - 第一列：🎓 學程資訊 標籤（藍色）
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
  - 「查看詳細」按鈕 → 跳轉課程詳情

### 學程列表（Programs for Course）
- **Bubble List**：包含該課程的學程列表
- **按鈕**：每個學程一個「查看課程」按鈕

### Quick Reply
- 使用 `QuickReplyProgramNav()`
- 包含：🎓 學程列表、🎓 學程、📖 說明

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
    ↓ (查看詳細按鈕)
Course Detail (返回)
```

### 共享組件
- **SemesterDetector**：course 模組提供，program 使用
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
Warmup (Daily 3:00 AM)
    ↓
Refresh Courses (course module)
    ↓
Parse 應修系級 → Extract Programs
    ↓
Update course_programs table
    ↓
semesterDetector.Refresh() (shared)
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

## 限制與注意事項

### 資料來源限制
- **依賴課程資料**：學程資料來自課程的 `應修系級` 欄位
- **無獨立抓取**：不從學程網頁抓取
- **解析規則**：只提取以「學程」結尾的項目

### 資料品質
- **完整性**：取決於課程資料的 `應修系級` 欄位品質
- **時效性**：與課程資料同步更新（每日 3:00 AM）
- **覆蓋範圍**：只包含有開課的學程

## 相關文件
- Handler: `internal/modules/program/handler.go`
- Flex: `internal/modules/program/flex.go`
- Tests: `internal/modules/program/handler_test.go`
- Course Module: `internal/modules/course/`

## 依賴關係
- `storage.DB` - 學程/課程資料查詢
- `course.SemesterDetector` - 學期偵測（共享）
- `metrics.Metrics` - 監控指標
- `logger.Logger` - 日誌記錄
- `sticker.Manager` - Sender 頭像
