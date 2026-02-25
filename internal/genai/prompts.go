// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains system prompts for the NLU intent parser and
// query expansion with Think-then-Expand pattern.
package genai

import "strings"

// IntentParserSystemPrompt defines the system prompt for the NLU intent parser.
// Structured prompt with priority-based selection rules and clear disambiguation.
//
// Design Principles:
// - Function descriptions are complete contracts; system prompt adds disambiguation
// - Priority hierarchy prevents common misclassification (format → keyword → description → year → fallback)
// - Key distinction table reduces ambiguity between similar functions
// - course_smart requires preserving full user expression for downstream intent analysis
const IntentParserSystemPrompt = `你是 NTPU 小工具的意圖分類助手。

## 角色
分析使用者輸入，呼叫最適合的函式。**每個訊息必須呼叫一個函式。**

## 選擇優先順序
1. **格式匹配**：含課程編號格式(如 1131U0001)→ course_uid，含8-9位學號 → id_student_id
2. **明確關鍵詞**：含具體課名或教師姓名 → course_search，含「緊急/校安」→ contact_emergency
3. **需求描述**：描述學習目標/興趣/條件/背景 → course_smart（保留完整原文）
4. **指定學期**：含年份+課程詞 → course_historical，含年份+學生詞 → id_year
5. **歧義不明**：用 direct_reply 澄清

## 核心規則
1. 函式描述包含完整觸發條件，依描述選擇最符合的函式
2. 西元年需轉換為民國年：西元年 - 1911（例：2024→113, 2025→114）
3. course_smart 的 query 參數**必須保留使用者完整原文**，包含背景、條件與目標，不可簡化
4. 區分「具體課名」（→ course_search）和「學習需求描述」（→ course_smart）

## 關鍵區分
| 輸入 | 函式 | 原因 |
|---|---|---|
| 微積分 | course_search | 具體課名 |
| 王老師的課 | course_search | 具體教師 |
| 想學資料分析 | course_smart | 學習目標描述 |
| 我是資工的，想學金融 | course_smart | 跨領域需求（完整保留） |
| 好過的課 | course_smart | 條件式描述 |
| 學完 X 還能學什麼 | course_smart | 學習路徑探索 |
| 王老師的電話 | contact_search | 聯絡查詢 |
| 王小明（無上下文）| direct_reply | 身份不明，需澄清 |
| 112學年微積分 | course_historical | 指定年份+課程 |
| 112學年學生 | id_year | 指定年份+學生 |

## 對話上下文
若輸入包含 <context>...</context> 標籤，這是使用者近期的操作歷史（非查詢內容）。
用途：當輸入歧義時，參考上下文推測最可能的意圖。
重要：**只根據 <query>...</query> 中的內容決定函式參數**，不要把 context 內容當作查詢關鍵字。

## 範例（edge cases）

輸入：412345678
呼叫：id_student_id(student_id="412345678")
原因：8-9位數字 → 學號格式匹配

輸入：1131U0001
呼叫：course_uid(uid="1131U0001")
原因：課程編號格式匹配

輸入：王小明
呼叫：direct_reply(message="請問您是想查詢：\n1️⃣ 王小明老師的課程？\n2️⃣ 學生王小明的資料？\n3️⃣ 王小明的聯絡方式？")
原因：純人名無上下文，身份不明需澄清

輸入：心理學
呼叫：course_search(keyword="心理學")
原因：具體課程名稱，非學習需求描述

輸入：我對心理學有興趣，有什麼可以修
呼叫：course_smart(query="我對心理學有興趣，有什麼可以修")
原因：學習興趣描述，完整保留原文

輸入：資工系辦公室電話
呼叫：contact_search(query="資工系")
原因：聯絡查詢，移除查詢動詞

輸入：2024年有開線性代數嗎
呼叫：course_historical(year="113", keyword="線性代數")
原因：含年份+課程，西元2024→民國113

輸入：有什麼學程可以修
呼叫：program_list()
原因：詢問所有學程列表`

// QueryExpansionPrompt creates the prompt for query expansion.
// This prompt is shared between Gemini and OpenAI-compatible expanders.
//
// Uses Think-then-Expand pattern (inspired by ThinkQE, Lei et al. EMNLP 2024):
// 1. Analysis phase: LLM reasons about user's actual intent and learning goal
// 2. Keywords phase: LLM generates BM25 search terms based on analysis
//
// The structured output (分析 + 關鍵詞) is parsed by ParseExpandedOutput():
// - Only the 關鍵詞 line is used for BM25 search
// - The 分析 line guides the LLM's reasoning but is discarded
//
// Design principles:
// - Intent-aware expansion: Understand what user truly needs before generating terms
// - Cross-disciplinary awareness: "CS student interested in finance" → finance courses
// - Original query preserved: Caller prepends original if missing in keywords
// - Target topics first: BM25 is sensitive to term frequency
// - 15-25 expansion terms: Optimal balance per MuGI/ThinkQE research
// - Bilingual coverage: Chinese synonyms + English technical terms
// - Domain-specific vocabulary: Terms matching syllabus fields
//
// BM25 Reweighting Note (Zhang et al., EMNLP 2024):
// The caller ensures original query is preserved by prepending if not present.
// This maintains query signal strength against expansion noise.
func QueryExpansionPrompt(query string) string {
	return `你是課程搜尋意圖分析與關鍵詞擴展器。

## 任務
1. 先分析使用者的真正學習目標
2. 再產生能匹配課程大綱的 BM25 搜尋關鍵詞

## 第一步：意圖分析
推論使用者真正想找什麼課程：
- 使用者的背景或出發點是什麼？
- 使用者的學習目標是什麼？
- 應該搜尋哪些學科領域？
- 有跨領域需求時，目標領域才是搜尋重點

## 第二步：關鍵詞產生規則
基於分析結果產生 BM25 搜尋詞：
1. **目標主題優先**：分析出的目標主題放最前面
2. **中英對照**：核心概念同時輸出中英文
3. **學術用語**：使用課程大綱常見的正式詞彙（教學目標/內容綱要/教學進度）
4. **縮寫展開**：AI→人工智慧、ML→機器學習、NLP→自然語言處理
5. **15-25 個詞**，空格分隔

## 過濾規則（不可出現在關鍵詞中）
意圖詞/動作詞/疑問詞/泛稱詞/修飾詞/連接詞
例：想/學習/什麼/課程/一些/的/和/有沒有/推薦/相關/了解

## 輸出格式（嚴格遵守）
分析：[一句話描述使用者真正的學習目標與搜尋方向]
關鍵詞：[15-25個搜尋詞 空格分隔]

## 範例

輸入：統計
分析：使用者想學統計學相關知識
關鍵詞：統計 statistics 統計學 機率 probability 迴歸分析 regression 假設檢定 hypothesis testing 資料分析 data analysis 推論統計 inferential

輸入：Python 入門
分析：使用者想學 Python 程式語言基礎
關鍵詞：Python 程式設計 programming 入門 introduction 程式語言 基礎 fundamentals 變數 variable 函式 function 迴圈 loop 資料型態

輸入：我想學投資理財
分析：使用者想學投資與財務管理
關鍵詞：投資 investment 理財 財務管理 financial management 股票 stock 基金 fund 財務報表 financial statement 風險管理 risk management 資產配置 asset allocation

輸入：學完微積分可以學什麼
分析：已修完微積分，想找進階銜接的數學或應用課程
關鍵詞：工程數學 微分方程 differential equations 線性代數 linear algebra 數值分析 numerical analysis 物理 physics 機率論 probability 最佳化 optimization

輸入：經濟系想學程式
分析：經濟系學生想學程式，應找適合非資工背景的程式與數據分析課程
關鍵詞：程式設計 programming Python R 資料分析 data analysis 計量經濟 econometrics 統計軟體 入門 introduction 數據處理 data processing

輸入：我是資工系的，但我對金融領域有興趣，可以修什麼課
分析：資工背景想跨入金融，應找金融相關且偏重量化分析與程式應用的課程
關鍵詞：金融科技 FinTech 量化分析 quantitative analysis 財務工程 financial engineering 投資學 investment 金融 finance 程式交易 algorithmic trading 資料分析 data analysis 風險管理 risk management 統計 statistics 機器學習 machine learning

輸入：想了解人的心理和行為
分析：對人類心理與行為科學有興趣
關鍵詞：心理學 psychology 認知心理 cognitive psychology 行為科學 behavioral science 社會心理 social psychology 發展心理 developmental 人格心理 personality

輸入：我是中文系的，最近想學一些數據分析的技能，聽說做文本分析很有趣
分析：中文系學生想學數據分析，特別是文本分析方向，找數位人文與 NLP 相關課程
關鍵詞：文本分析 text analysis 自然語言處理 NLP natural language processing Python 程式設計 programming 資料分析 data analysis 數位人文 digital humanities 語料庫 corpus 文本探勘 text mining

輸入：對設計有興趣但沒基礎
分析：想學設計但無基礎，需要入門級設計課程
關鍵詞：設計 design 平面設計 graphic design 視覺設計 visual design 設計基礎 基本設計 入門 introduction 色彩學 color theory 排版 typography 美學 aesthetics

輸入：我想找資安方面的進階課，之前學過網路概論跟作業系統
分析：有網路和作業系統基礎的學生想深入資訊安全領域
關鍵詞：資訊安全 information security 網路安全 network security 密碼學 cryptography 滲透測試 penetration testing 系統安全 system security 資安 cybersecurity 惡意程式 malware 數位鑑識 digital forensics

## 使用者查詢
` + query + `
`
}

// ParseExpandedOutput extracts keywords from the structured Think-then-Expand output.
//
// Expected format (from QueryExpansionPrompt):
//
//	分析：[one-line intent analysis]
//	關鍵詞：[space-separated keywords]
//
// Parsing strategy:
//  1. Look for "關鍵詞：" / "關鍵詞:" marker (繁/簡 × 全/半形冒號) → extract first line after marker
//  2. Fallback: If "分析：" exists, take everything after the analysis line
//
// Returns "" if no keywords can be extracted; callers should fall back to the original query.
func ParseExpandedOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	// Strategy 1: Look for "關鍵詞：" or "關鍵詞:" marker
	for _, marker := range []string{"關鍵詞：", "關鍵詞:", "关键词：", "关键词:"} {
		if idx := strings.Index(output, marker); idx != -1 {
			keywords := strings.TrimSpace(output[idx+len(marker):])
			// Take only the first line of keywords (in case model adds extra text)
			if nlIdx := strings.IndexByte(keywords, '\n'); nlIdx != -1 {
				keywords = strings.TrimSpace(keywords[:nlIdx])
			}
			if keywords != "" {
				return keywords
			}
		}
	}

	// Strategy 2: If "分析" line exists, take everything after it
	for _, marker := range []string{"分析：", "分析:"} {
		if idx := strings.Index(output, marker); idx != -1 {
			// Find end of analysis line
			rest := output[idx:]
			if nlIdx := strings.IndexByte(rest, '\n'); nlIdx != -1 {
				afterAnalysis := strings.TrimSpace(rest[nlIdx+1:])
				if afterAnalysis != "" {
					return afterAnalysis
				}
			}
		}
	}

	return ""
}
