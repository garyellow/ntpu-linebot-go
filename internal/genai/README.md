# genai

封裝 LLM API 功能，提供 NLU 意圖解析和查詢擴展，支援多提供者 (Gemini + Groq) 自動故障轉移。

## 功能

- **IntentParser**: NLU 意圖解析器（Function Calling 實作）
- **QueryExpander**: 查詢擴展器（同義詞、縮寫、翻譯）
- **Multi-Provider Fallback**: 自動故障轉移和重試機制

## 支援的 LLM 提供者

| 提供者 | 用途 | 預設模型鏈（逗號分隔 fallback）| 特色 |
|--------|------|-------------------------------|------|
| **Gemini** | Intent | `gemini-2.5-flash, gemini-2.5-flash-lite` | 高品質、多模態 |
| **Gemini** | Expander | `gemini-2.5-flash, gemini-2.5-flash-lite` | 高品質、多模態 |
| **Groq** | Intent | `llama-4-maverick-17b-128e-instruct, llama-3.3-70b-versatile` | 極速推論 (~900 TPS) |
| **Groq** | Expander | `llama-4-scout-17b-16e-instruct, llama-3.1-8b-instant` | 極速推論 (~750 TPS) |

## 檔案結構

```
internal/genai/
├── types.go              # 共享類型定義 (IntentParser, QueryExpander interfaces)
├── errors.go             # 錯誤分類和重試判斷
├── retry.go              # AWS Full Jitter 重試邏輯
├── gemini_intent.go      # Gemini IntentParser 實作
├── groq_intent.go        # Groq IntentParser 實作
├── gemini_expander.go    # Gemini QueryExpander 實作
├── groq_expander.go      # Groq QueryExpander 實作
├── provider_fallback.go  # 跨提供者故障轉移
├── factory.go            # 工廠函式
├── functions.go          # Function Calling 函式定義
├── prompts.go            # 系統提示詞
└── README.md
```

## 故障轉移架構

```
使用者請求
    ↓
┌─────────────────────────────────────────────────┐
│  FallbackIntentParser / FallbackQueryExpander   │
├─────────────────────────────────────────────────┤
│  1. 主要提供者重試 (Full Jitter Backoff)        │
│     - 429/5xx → 重試 (最多 2 次)                │
│     - 400/401/403 → 直接失敗                    │
│                                                  │
│  2. 提供者故障轉移                               │
│     - 主要提供者失敗 → 備援提供者               │
│     - 記錄 metrics (ntpu_llm_fallback_total)    │
│                                                  │
│  3. 優雅降級 (QueryExpander only)               │
│     - 全部失敗 → 返回原始查詢                   │
└─────────────────────────────────────────────────┘
```

## NLU Intent Parser

使用 Gemini Function Calling (ANY mode) 和 Groq (required mode) 解析使用者自然語言意圖。
強制 function calling 確保穩定性，透過 `direct_reply` function 處理閒聊、澄清等非查詢情境。

## Intent Parser (意圖解析)

### 支援的意圖

| 函式名稱 | 模組 | 說明 |
|---------|------|------|
| `course_search` | course | 課程/教師名稱搜尋 |
| `course_smart` | course | 課程智慧搜尋 |
| `course_uid` | course | 課號查詢 |
| `id_search` | id | 學生姓名搜尋 |
| `id_student_id` | id | 學號查詢 |
| `id_department` | id | 科系代碼查詢 |
| `contact_search` | contact | 聯絡資訊搜尋 |
| `contact_emergency` | contact | 緊急電話 |
| `program_list` | program | 列出所有學程 |
| `program_search` | program | 搜尋學程 |
| `help` | help | 使用說明 |
| `direct_reply` | direct_reply | 直接回覆（閒聊、澄清、離題詢問）|

### 使用方式

```go
import "ntpu-linebot/internal/genai"

// 透過工廠函數建立 (自動配置故障轉移)
llmConfig := genai.DefaultLLMConfig(geminiKey, groqKey)
parser, err := genai.CreateIntentParser(ctx, llmConfig)
if err != nil {
    return err
}
defer parser.Close()

// 解析使用者輸入
result, err := parser.Parse(ctx, "我想找微積分的課")
if err != nil {
    return err
}

// 結果包含模組、意圖、參數
// result.Module = "course"
// result.Intent = "search"
// result.Params["keyword"] = "微積分"
// result.FunctionName = "course_search"
```

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

- 所有智慧搜尋查詢都會自動觸發 Query Expansion（若 `queryExpander` 已配置）
- 提供 graceful degradation：擴展失敗時使用原始查詢，不影響搜尋功能
- 受 LLM Rate Limiter 限制（預設：40 burst, 20/hr refill, 100/day cap）

### 使用方式

```go
import "ntpu-linebot/internal/genai"

// 透過工廠函數建立 (自動配置故障轉移)
llmConfig := genai.DefaultLLMConfig(geminiKey, groqKey)
expander, err := genai.CreateQueryExpander(ctx, llmConfig)
if err != nil {
    return err
}
defer expander.Close()

// 擴展查詢
expanded, err := expander.Expand(ctx, "我想學 AWS")
// expanded = "我想學 AWS Amazon Web Services 雲端服務 雲端運算 cloud computing..."
```

## 錯誤處理與重試

### 重試策略 (AWS Full Jitter)

```
重試延遲 = random(0, min(cap, base * 2^attempt))

- base: 2 秒
- cap: 60 秒
- MaxRetries: 2
```

### 錯誤分類

| 錯誤類型 | 可重試 | 說明 |
|---------|--------|------|
| 429 RESOURCE_EXHAUSTED | ✅ | Rate limit，重試後可能成功 |
| 500+ Server Error | ✅ | 暫時性錯誤 |
| 400 Bad Request | ❌ | 請求格式錯誤 |
| 401/403 Auth Error | ❌ | 認證失敗 |
| Context Canceled | ❌ | 請求取消 |

### 故障轉移流程

1. **Primary Provider Retry**: 主要提供者內部重試 (最多 2 次)
2. **Cross-Provider Fallback**: 主要失敗後切換到備援提供者
3. **Graceful Degradation**: 全部失敗時，QueryExpander 返回原始查詢

## 配置

### 環境變數

#### LLM Provider Keys

| 變數名稱 | 必填 | 說明 |
|---------|------|------|
| `GEMINI_API_KEY` | 任一 | Google AI Studio API Key |
| `GROQ_API_KEY` | 任一 | Groq API Key |

> **注意**: 至少需要設定其中一個 API Key 才能啟用 LLM 功能

#### Provider Selection

| 變數名稱 | 預設值 | 說明 |
|---------|--------|------|
| `LLM_PRIMARY_PROVIDER` | gemini | 主要提供者 (gemini/groq) |
| `LLM_FALLBACK_PROVIDER` | groq | 備援提供者 (gemini/groq) |

#### Model Configuration

模型配置使用逗號分隔的 fallback chain 格式。第一個模型為主要模型，其餘為備援模型（依序嘗試）。

| 變數名稱 | 預設值 | 說明 |
|---------|--------|------|
| `GEMINI_INTENT_MODELS` | gemini-2.5-flash,gemini-2.5-flash-lite | Gemini 意圖解析模型鏈 |
| `GEMINI_EXPANDER_MODELS` | gemini-2.5-flash,gemini-2.5-flash-lite | Gemini 查詢擴展模型鏈 |
| `GROQ_INTENT_MODELS` | llama-4-maverick...,llama-3.3-70b-versatile | Groq 意圖解析模型鏈 |
| `GROQ_EXPANDER_MODELS` | llama-4-scout...,llama-3.1-8b-instant | Groq 查詢擴展模型鏈 |

> **💡 提示**：可添加更多 fallback 模型，例如：
> `GEMINI_INTENT_MODELS=gemini-2.5-flash,gemini-2.5-flash-lite,gemini-2.0-flash`

#### Rate Limiting

| 變數名稱 | 預設值 | 說明 |
|---------|--------|------|
| `LLM_BURST_TOKENS` | 40 | LLM 請求 burst 上限 |
| `LLM_REFILL_PER_HOUR` | 20 | 每小時恢復 token 數 |
| `LLM_DAILY_LIMIT` | 100 | 每日 LLM 請求上限 (0=無限制) |

### 獲取 API Key

- **Gemini**: [Google AI Studio](https://aistudio.google.com/apikey)
- **Groq**: [Groq Console](https://console.groq.com/keys)

## Metrics

| 指標名稱 | 類型 | 標籤 | 說明 |
|---------|------|------|------|
| `ntpu_llm_total` | Counter | provider, operation, status | LLM 請求總數 |
| `ntpu_llm_duration_seconds` | Histogram | provider, operation | LLM 請求延遲 |
| `ntpu_llm_fallback_total` | Counter | from_provider, to_provider, operation | 故障轉移次數 |
| `ntpu_rate_limiter_dropped_total` | Counter | limiter="llm" | 限流丟棄請求數 |

### 常用查詢

```promql
# Gemini vs Groq 成功率比較
sum(rate(ntpu_llm_total{status="success"}[5m])) by (provider)
/ sum(rate(ntpu_llm_total[5m])) by (provider)

# 故障轉移頻率
sum(rate(ntpu_llm_fallback_total[1h])) by (from_provider, to_provider)

# P95 延遲 (by provider)
histogram_quantile(0.95, sum(rate(ntpu_llm_duration_seconds_bucket[5m])) by (le, provider))
```
