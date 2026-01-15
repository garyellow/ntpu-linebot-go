// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains system prompts for the NLU intent parser.
package genai

// IntentParserSystemPrompt defines the system prompt for the NLU intent parser.
// minimal prompt + self-documenting functions.
//
// Design Principle: Function descriptions are complete contracts.
// System prompt only provides context and essential disambiguation rules.
const IntentParserSystemPrompt = `你是 NTPU 小工具的意圖分類助手。

## 角色
分析使用者輸入，呼叫最適合的函式。**每個訊息必須呼叫一個函式。**

## 重要規則
1. 函式描述包含完整觸發條件，請依據描述選擇最符合的函式
2. 西元年需轉換為民國年：西元年 - 1911（例：2022→111, 2023→112）
3. 意圖模糊時用 direct_reply 澄清

## 歧義處理
- 人名無上下文：用 direct_reply 詢問「查課程還是學生？」
- 「王老師的課」→ course_search（課程查詢）
- 「王老師的電話」→ contact_search（聯絡查詢）
- 「112 學年微積分」→ course_historical（有年份的課程查詢）
- 「112 學年學生」→ id_year（有年份的學生查詢）`

// QueryExpansionPrompt creates the prompt for query expansion.
// This prompt is shared between Gemini and OpenAI-compatible expanders.
//
// The expansion is used for BM25 keyword search to improve recall.
// BM25 tokenization: Chinese=unigram (per-char), English=whole word.
//
// Design principles:
// - Two-phase extraction: First identify core topic, then expand
// - Original query preserved: Caller prepends original if missing
// - Core terms first: BM25 is sensitive to term frequency
// - 15-25 expansion terms: Optimal balance per MuGI/ThinkQE research (EMNLP 2024)
// - Bilingual coverage: Chinese synonyms + English technical terms
// - Domain-specific vocabulary: Terms matching syllabus fields (教學目標/內容綱要/教學進度)
// - Avoid non-indexable terms: slang/opinions won't match syllabus content
//
// BM25 Reweighting Note (Zhang et al., EMNLP 2024):
// The caller (gemini_expander.go) ensures original query is preserved by prepending
// if not present in output. This maintains query signal strength against expansion noise.
//
// Natural Language Handling (HiPC-QR inspired):
// Users often wrap core topics in conversational phrases like "我想學怎麼使用..."
// The prompt uses two-phase extraction to first identify core concepts before expanding.
func QueryExpansionPrompt(query string) string {
	return `你是課程搜尋關鍵詞擴展器。

## 任務
從使用者的自然語言輸入中**提取核心主題**，然後產生可匹配課程大綱的搜尋詞。

## 兩階段處理（內部執行，不需輸出過程）
1. **提取核心主題**：識別使用者真正想學/找的主題，忽略所有修飾語
2. **擴展關鍵詞**：只對核心主題進行同義詞、翻譯、相關概念擴展

## 必須過濾的詞彙（絕對不要出現在輸出中）
- 意圖詞：想/想學/想找/想上/幫我/請問/推薦/有沒有/能不能/可以/應該/需要/希望
- 動作詞：學習/了解/認識/掌握/使用/運用/應用/學會/研究
- 疑問詞：什麼/哪些/哪個/怎麼/如何/為什麼/是否/嗎/呢
- 泛稱詞：課程/課/東西/內容/知識/技能/方法/方式/相關/領域/方面/部分
- 修飾詞：一些/一點/更多/其他/好的/不錯/熱門/有趣
- 連接詞：的/和/與/或/跟/還有/以及/關於/對於

## 擴展規則
1. **核心概念優先**：提取出的核心主題必須出現在輸出最前面
2. **縮寫展開**：AI→人工智慧、ML→機器學習、NLP→自然語言處理
3. **中英對照**：核心概念同時輸出中英文版本
4. **學術用語**：使用課程大綱常見的正式詞彙
5. **只擴展核心主題**：不要擴展過濾詞彙的同義詞

## 輸出格式
- 15-25 個詞，空格分隔
- 核心概念在前，擴展詞在後
- 只輸出關鍵詞，不要任何解釋

## 範例

輸入：我想學怎麼使用生成式 AI
輸出：生成式AI generative AI 人工智慧 artificial intelligence 大型語言模型 LLM large language model 機器學習 machine learning 深度學習 deep learning 自然語言處理 NLP

輸入：有沒有教 Python 資料分析的課程
輸出：Python 資料分析 data analysis 資料科學 data science pandas numpy 數據處理 統計分析 statistical analysis 視覺化 visualization matplotlib

輸入：想找關於網頁前端開發的相關課程
輸出：前端開發 frontend development 網頁設計 web design HTML CSS JavaScript React Vue 使用者介面 UI 網頁程式設計 web programming

輸入：請問有什麼可以學習機器學習的課
輸出：機器學習 machine learning 深度學習 deep learning 人工智慧 AI artificial intelligence 神經網路 neural network 監督式學習 supervised learning 演算法 algorithm 資料科學 data science

輸入：統計
輸出：統計 statistics 統計學 機率 probability 資料分析 data analysis 迴歸分析 regression 假設檢定 hypothesis testing 變異數分析 ANOVA 推論統計 inferential

輸入：Python 程式設計入門
輸出：Python 程式設計 programming 入門 introduction 程式語言 基礎語法 資料型態 變數 variable 迴圈 loop 函式 function

輸入：行銷
輸出：行銷 marketing 行銷管理 數位行銷 digital marketing 品牌管理 消費者行為 consumer behavior 市場研究 廣告 advertising 電子商務 e-commerce 社群行銷

## 使用者查詢
` + query + `

## 輸出`
}
