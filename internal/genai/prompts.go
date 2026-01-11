// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains system prompts for the NLU intent parser.
package genai

// IntentParserSystemPrompt defines the system prompt for the NLU intent parser.
// Follows 2025-2026 best practices: minimal prompt + self-documenting functions.
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
// This prompt is shared between Gemini and Groq expanders.
//
// The expansion is used for BM25 keyword search to improve recall by:
// 1. Expanding abbreviations (AWS→Amazon Web Services)
// 2. Adding bilingual translations (Chinese↔English)
// 3. Including related academic/technical concepts
// 4. Cleaning up verbose queries to extract key concepts
func QueryExpansionPrompt(query string) string {
	return `你是大學課程搜尋查詢擴展助手。將使用者查詢擴展為 **10-25 個** 搜尋關鍵詞組合。

## 核心任務
為 BM25 關鍵字搜尋系統生成豐富的查詢擴展詞彙，最大化召回率 (Recall)。

## 擴展規則（嚴格遵守）
1. **保留原始查詢**：第一個詞必須是原始查詢。
2. **強制中英雙語**：
   - 中文概念 → 添加英文翻譯（含正式名稱 + 常用縮寫）
   - 英文概念 → 添加中文翻譯（含正式名稱 + 口語說法）
   - 縮寫 → 展開完整全稱（AWS → Amazon Web Services）
3. **廣泛同義詞**：學術名詞、技術術語、口語說法、應用場景。
4. **相關領域擴展**：包含上下游概念、工具、框架、子領域（目標 10-25 個詞）。
5. **格式要求**：僅輸出關鍵詞，用空格分隔，**絕對不要**標點符號、清單符號或解釋文字。

## 領域擴展範例（10-25 個詞的擴展）

### 資訊科技類
| 輸入 | 輸出 |
|-----|------|
| AI | AI 人工智慧 artificial intelligence 機器學習 machine learning 深度學習 deep learning 神經網路 neural networks 類神經網路 資料科學 data science 演算法 algorithms 智慧系統 intelligent systems 電腦視覺 computer vision 影像辨識 image recognition 自然語言處理 NLP natural language processing 強化學習 reinforcement learning 機器人 robotics 自動化 automation 預測模型 predictive modeling 大數據 big data 資料探勘 data mining TensorFlow PyTorch Keras 深度神經網路 DNN 卷積神經網路 CNN 遞迴神經網路 RNN 生成式 AI generative AI ChatGPT 語言模型 LLM |
| Python | Python 程式設計 programming 程式語言 programming language 軟體開發 software development coding 資料分析 data analysis 數據分析 資料科學 data science 自動化 automation 腳本 scripting 網頁爬蟲 web scraping 爬蟲 crawler 數據視覺化 data visualization 視覺化 visualization 後端開發 backend development 全端開發 full stack 科學計算 scientific computing 機器學習 machine learning NumPy Pandas Matplotlib PyTorch TensorFlow Django Flask FastAPI 網頁開發 web development 資料處理 data processing 演算法 algorithms 物件導向 OOP object oriented |
| 資安 | 資安 資訊安全 information security cybersecurity 網路安全 network security 系統安全 system security 密碼學 cryptography 加密 encryption 滲透測試 penetration testing 白帽駭客 white hat 倫理駭客 ethical hacking 惡意軟體 malware 病毒 virus 木馬 trojan 防火牆 firewall 入侵偵測 intrusion detection IDS IPS 數位鑑識 digital forensics 資安鑑識 風險管理 risk management 資料保護 data protection 隱私保護 privacy 個資保護 PDPA GDPR 資安攻防 攻防演練 漏洞掃描 vulnerability 弱點分析 威脅分析 threat analysis 存取控制 access control |

### 商管法律類
| 輸入 | 輸出 |
|-----|------|
| marketing | marketing 行銷 市場行銷 行銷學 行銷管理 marketing management 行銷策略 marketing strategy 數位行銷 digital marketing 網路行銷 online marketing 社群行銷 social media marketing 社群媒體 品牌管理 brand management 品牌經營 branding 消費者行為 consumer behavior 消費心理 市場調查 market research 市調 廣告 advertising 廣告學 公共關係 PR public relations 內容行銷 content marketing 電子商務 e-commerce 電商 網路商店 online store 銷售 sales 通路 channel 行銷企劃 SEO SEM 搜尋引擎優化 關鍵字廣告 整合行銷傳播 IMC |
| ESG | ESG 永續發展 sustainability 環境保護 environment environmental 社會責任 social responsibility 公司治理 governance corporate governance 企業社會責任 CSR corporate social responsibility 永續經營 sustainable 綠色金融 green finance 綠色投資 碳中和 carbon neutrality 淨零排放 net zero 氣候變遷 climate change 全球暖化 global warming 聯合國永續發展目標 SDGs sustainable development goals 綠色能源 green energy 再生能源 renewable energy 碳足跡 carbon footprint 碳排放 環境影響評估 EIA 循環經濟 circular economy 社會創新 social innovation 影響力投資 impact investing |

### 自然語言描述
| 輸入 | 輸出 |
|-----|------|
| 想學資料分析 | 資料分析 data analysis 數據分析 數據科學 data science 統計學 statistics 商業分析 business analytics BA 商業智慧 BI business intelligence 資料探勘 data mining 機器學習 machine learning 預測分析 predictive analytics 視覺化 visualization 資料視覺化 data visualization Tableau PowerBI Python R語言 SQL 資料庫 database Excel 報表 reporting dashboard 儀表板 大數據 big data 預測模型 prediction 決策支援 decision support 數據驅動 data driven KPI 關鍵績效指標 分析工具 analytics tools 統計軟體 |
| 好過的通識 | 通識 general education 通識課程 通識教育 營養學分 easy pass 輕鬆 easy 甜課 涼課 high grades 好過 簡單 博雅 liberal arts 核心通識 core curriculum 選修 elective 通識選修 興趣課程 interest 電影賞析 film appreciation 電影欣賞 音樂賞析 music appreciation 音樂欣賞 藝術鑑賞 art appreciation 藝術欣賞 歷史 history 文學 literature 哲學 philosophy 人文 humanities 社會科學 social science 自然科學 natural science 生活科學 通識學分 涼爽 容易過 |

## 查詢
` + query + `

## 輸出
請直接輸出擴展後的關鍵詞，用空格分隔，不要有任何其他內容。`
}
