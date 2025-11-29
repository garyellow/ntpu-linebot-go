# genai

封裝 Google Gemini API 的 embedding 功能，提供課程語意搜尋所需的向量生成。

## 功能

- **EmbeddingClient**: Gemini embedding API 客戶端
- **NewEmbeddingFunc**: chromem-go 相容的嵌入函數

## 技術規格

- 模型: `gemini-embedding-001`
- 向量維度: 768
- API 限流: 1000 RPM (自動處理)

## 錯誤處理

- **指數退避重試**: 針對 429 (RESOURCE_EXHAUSTED) 和 500+ 錯誤自動重試
- **重試配置**: 最多 5 次重試，初始延遲 2 秒，最大延遲 60 秒
- **Jitter**: ±25% 隨機抖動，避免 thundering herd

## 使用

```go
// 建立客戶端
client := genai.NewEmbeddingClient(apiKey)

// 產生 embedding（自動處理重試）
vector, err := client.Embed(ctx, "課程內容文字")

// 或使用 chromem-go 相容的函數
embeddingFunc := genai.NewEmbeddingFunc(apiKey)
```

## 配置

需設定環境變數 `GEMINI_API_KEY`。若未設定，語意搜尋功能將停用。

取得 API Key: [Google AI Studio](https://aistudio.google.com/apikey)
