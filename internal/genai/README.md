# genai

封裝 Google Gemini API 功能，提供課程語意搜尋所需的向量生成，以及 NLU 意圖解析。

## 功能

- **EmbeddingClient**: Gemini embedding API 客戶端（向量生成）
- **IntentParser**: NLU 意圖解析器（Function Calling）
- **NewEmbeddingFunc**: chromem-go 相容的嵌入函數

## 檔案結構

```
internal/genai/
├── types.go          # 共享類型定義 (ParseResult, IntentParserInterface)
├── intent.go         # IntentParser 實作
├── functions.go      # Function Calling 函數定義
├── prompts.go        # 系統提示詞
├── embedding.go      # Embedding 客戶端
└── README.md
```

## NLU Intent Parser

使用 Gemini Function Calling (AUTO mode) 解析使用者自然語言意圖。

### 支援的意圖

| 函數名稱 | 模組 | 說明 |
|---------|------|------|
| `course_search` | course | 課程/教師名稱搜尋 |
| `course_semantic` | course | 課程語意搜尋 |
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

- 模型: `gemini-2.5-flash-lite`
- 超時: 10 秒
- Temperature: 0.1 (低變異性)
- Max Tokens: 256

## Embedding Client

### 技術規格

- 模型: `gemini-embedding-001`
- 向量維度: 768
- API 限流: 1000 RPM (自動處理)

### 使用方式

```go
// 建立客戶端
client := genai.NewEmbeddingClient(apiKey)

// 產生 embedding（自動處理重試）
vector, err := client.Embed(ctx, "課程內容文字")

// 或使用 chromem-go 相容的函數
embeddingFunc := genai.NewEmbeddingFunc(apiKey)
```

## 錯誤處理

- **指數退避重試**: 針對 429 (RESOURCE_EXHAUSTED) 和 500+ 錯誤自動重試
- **重試配置**: 最多 5 次重試，初始延遲 2 秒，退避因子 2.0
- **Jitter**: ±25% 隨機抖動，避免 thundering herd

## 配置

需設定環境變數 `GEMINI_API_KEY`。若未設定：
- 語意搜尋功能停用
- NLU 意圖解析停用（fallback 到關鍵字匹配）

取得 API Key: [Google AI Studio](https://aistudio.google.com/apikey)
