# genai

封裝 Google Gemini API 功能，提供 NLU 意圖解析和查詢擴展。

## 功能

- **GeminiIntentParser**: NLU 意圖解析器（Gemini Function Calling 實作）
- **QueryExpander**: 查詢擴展器（同義詞、縮寫、翻譯）

## 檔案結構

```
internal/genai/
├── types.go          # 共享類型定義 (ParseResult)
├── intent.go         # GeminiIntentParser 實作 (Gemini Function Calling)
├── functions.go      # Function Calling 函數定義
├── prompts.go        # 系統提示詞
├── expander.go       # Query Expander
└── README.md
```

## NLU Intent Parser

使用 Gemini Function Calling (AUTO mode) 解析使用者自然語言意圖。

### 支援的意圖

| 函數名稱 | 模組 | 說明 |
|---------|------|------|
| `course_search` | course | 課程/教師名稱搜尋 |
| `course_smart` | course | 課程智慧搜尋 |
| `course_uid` | course | 課號查詢 |
| `id_search` | id | 學生姓名搜尋 |
| `id_student_id` | id | 學號查詢 |
| `id_department` | id | 科系代碼查詢 |
| `contact_search` | contact | 聯絡資訊搜尋 |
| `contact_emergency` | contact | 緊急電話 |
| `help` | help | 使用說明 |

### 使用方式

```go
// 建立 Intent Parser
parser, err := genai.NewIntentParser(ctx, apiKey)
if err != nil {
    // 處理錯誤
}

// 解析使用者輸入
result, err := parser.Parse(ctx, "我想找微積分的課")
if err != nil {
    // 處理錯誤
}

// 結果包含模組、意圖、參數
// result.Module = "course"
// result.Intent = "search"
// result.Params["keyword"] = "微積分"
```

### 技術規格

- 模型: `gemma-3-27b-it`
- 超時: 10 秒
- Temperature: 0.1 (低變異性)
- Max Tokens: 256

## Query Expander (查詢擴展)

獨立的查詢擴展模組，用於增強 BM25 搜尋效果。

### 功能說明

當使用者輸入智慧搜尋查詢時，`QueryExpander` 會自動擴展查詢，添加：

1. **英文縮寫的全稱**: AWS → Amazon Web Services
2. **跨語言翻譯**: AI → 人工智慧, 程式設計 → programming
3. **相關概念和同義詞**: 雲端 → cloud computing, 雲端服務

### 使用範例

| 原始查詢 | 擴展後 |
|---------|--------|
| `AWS` | `AWS Amazon Web Services 雲端服務 雲端運算 cloud computing EC2 S3` |
| `我想學 AI` | `AI 人工智慧 artificial intelligence 機器學習 machine learning 深度學習` |
| `程式設計` | `程式設計 programming 軟體開發 coding 程式語言 software development` |
| `資料分析` | `資料分析 data analysis 數據分析 統計 statistics 資料科學 data science` |

### 觸發條件

- 查詢長度 ≤ 15 字（runes）
- 或包含技術縮寫（AWS, AI, ML, SQL, API 等）

### 技術規格

- 模型: `gemma-3-27b-it`
- 超時: 8 秒
- Temperature: 0.3 (適度變異性)
- Max Tokens: 200

### 使用方式

```go
// 建立 Query Expander
expander, err := genai.NewQueryExpander(ctx, apiKey)
if err != nil {
    // 處理錯誤
}

// 擴展查詢
expanded, err := expander.Expand(ctx, "我想學 AWS")
// expanded = "我想學 AWS Amazon Web Services 雲端服務 雲端運算 cloud computing..."
```

### 整合方式

課程模組透過 `SetQueryExpander()` 注入：

```go
handler.SetQueryExpander(expander)
```

智慧搜尋時自動使用擴展後的查詢進行 BM25 搜尋。

## 錯誤處理

- **指數退避重試**: 針對 429 (RESOURCE_EXHAUSTED) 和 500+ 錯誤自動重試
- **重試配置**: 最多 5 次重試，初始延遲 2 秒，退避因子 2.0
- **Jitter**: ±25% 隨機抖動，避免 thundering herd

## 配置

### 環境變數

**`GEMINI_API_KEY`** (必填)：
- 若未設定：NLU 意圖解析停用（fallback 到關鍵字匹配）、查詢擴展停用
- 取得方式: [Google AI Studio](https://aistudio.google.com/apikey)

**`LLM_RATE_LIMIT_PER_HOUR`** (可選，預設 50)：
- 每位使用者每小時的 LLM API 請求數上限
- 適用於 NLU 意圖解析 (IntentParser) 和課程智慧搜尋的 Query Expansion (QueryExpander)
- 超過限制時顯示友善提示並記錄 `ntpu_rate_limiter_dropped_total{limiter="llm"}` 指標
