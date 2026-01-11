// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains function declarations for the NLU intent parser.
//
// Design Principles (Gemini/Groq/Cerebras):
// - functions.go: WHAT the function does (descriptions + parameter formats)
// - prompts.go: WHEN/HOW to use (decision trees, trigger conditions)
//
// This is intentional "Prompt-Primary" design where detailed decision logic lives in
// IntentParserSystemPrompt. Function descriptions here are kept concise but MUST include
// critical parameter extraction rules (e.g., what to remove, what to preserve).
//
// IMPORTANT: Function declarations use genai.Type* constants (e.g., genai.TypeString = "STRING").
// When converting to other provider formats (e.g., Groq), ensure types are lowercased to match
// JSON Schema spec ("string" not "STRING"). See buildGroqTools() in groq_intent.go for example.
//
// Module Order (consistent across all files):
// 1. Course   - 課程查詢
// 2. ID       - 學生查詢
// 3. Contact  - 聯絡資訊
// 4. Program  - 學程查詢
// 5. Help     - 使用說明
// 6. Direct   - 直接回覆
package genai

import "google.golang.org/genai"

// BuildIntentFunctions returns the function declarations for NLU intent parsing.
// These functions represent the available intents the parser can recognize.
//
// Total: 17 functions across 6 modules
func BuildIntentFunctions() []*genai.FunctionDeclaration {
	return []*genai.FunctionDeclaration{
		// ============================================
		// 1. Course Module (課程查詢)
		// ============================================
		{
			Name:        "course_search",
			Description: "依課程名稱或教師姓名搜尋最近 2 個學期的課程。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名。移除修飾詞（課程、老師、教授、的）。例：「微積分」「王小明」。",
					},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "course_smart",
			Description: "依學習需求或技術領域進行智慧搜尋。適合抽象需求、技術縮寫、領域概念的查詢。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "學習目標或技術關鍵字。保留原始表達。例：「想學 AI」「資料分析」「好過的通識」。",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "course_uid",
			Description: "依課程編號查詢詳細資訊。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"uid": {
						Type:        genai.TypeString,
						Description: "課程編號。格式：完整 (1131U0001) 或課號 (U0001)。",
					},
				},
				Required: []string{"uid"},
			},
		},
		{
			Name:        "course_extended",
			Description: "搜尋第 3-4 個學期的課程（較舊學期）。用於「找更多」「舊學期」等時間擴展需求。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名。移除時間修飾詞（更多、舊、歷史）。例：「微積分」。",
					},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "course_historical",
			Description: "查詢指定學年度的課程。需要學年度 + 課程關鍵字兩個參數。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"year": {
						Type:        genai.TypeString,
						Description: "學年度（民國年 3 位數）。移除「學年度」「年」後綴。例：「110」「112」。",
					},
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名。例：「微積分」「王小明」。",
					},
				},
				Required: []string{"year", "keyword"},
			},
		},

		// ============================================
		// 2. ID Module (學生查詢)
		// ============================================
		{
			Name:        "id_search",
			Description: "依姓名搜尋學生資訊。支援完整姓名或部分姓名模糊搜尋。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {
						Type:        genai.TypeString,
						Description: "學生姓名，全名或部分皆可。例：「王小明」「小明」。",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "id_student_id",
			Description: "依學號查詢學生資訊。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"student_id": {
						Type:        genai.TypeString,
						Description: "學號，8-9 位數字。例如「412345678」",
					},
				},
				Required: []string{"student_id"},
			},
		},
		{
			Name:        "id_department",
			Description: "依科系查詢學生。接受系代碼（數字）或系名（文字）。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"department": {
						Type:        genai.TypeString,
						Description: "科系代碼（數字）或名稱（文字）。例：「85」「資工系」「資訊工程」。",
					},
				},
				Required: []string{"department"},
			},
		},
		{
			Name:        "id_year",
			Description: "依學年度查詢學生。僅支援 94-112 學年度完整資料。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"year": {
						Type:        genai.TypeString,
						Description: "學年度，民國年 3 位數。範例：「112」「110」「100」",
					},
				},
				Required: []string{"year"},
			},
		},
		{
			Name:        "id_dept_codes",
			Description: "顯示系代碼對照表。依學制類型分類顯示所有系所代碼。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"degree": {
						Type:        genai.TypeString,
						Description: "學制類型：bachelor（學士班）、master（碩士班）、phd（博士班）。預設 bachelor。",
					},
				},
			},
		},

		// ============================================
		// 3. Contact Module (聯絡資訊)
		// ============================================
		{
			Name:        "contact_search",
			Description: "查詢校內單位或人員聯絡方式（電話、分機、email、地址）。包含行政單位和教職員。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "單位或人員名稱。移除查詢動詞（辦公室、電話、分機、email、怎麼聯絡、在哪）。例：「資工系」「圖書館」「王小明」。",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "contact_emergency",
			Description: "取得校園緊急聯絡電話（保全、校安、救護）。",
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// ============================================
		// 4. Program Module (學程查詢)
		// ============================================
		{
			Name:        "program_list",
			Description: "列出所有可修讀學程。",
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},
		{
			Name:        "program_search",
			Description: "依關鍵字搜尋學程。用於查找特定領域的學程。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "學程名稱關鍵字。移除「學程」後綴。例：「智財」「永續」「人工智慧」。",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "program_courses",
			Description: "列出指定學程包含的課程（必修/選修）。需要完整學程名稱。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"program_name": {
						Type:        genai.TypeString,
						Description: "學程全名（必須包含「學程」二字）。例：「人工智慧學程」「永續發展學程」。",
					},
				},
				Required: []string{"program_name"},
			},
		},

		// ============================================
		// 5. Usage Module (配額查詢)
		// ============================================
		{
			Name:        "usage_query",
			Description: "查詢使用者的功能額度狀態（訊息額度、AI 額度）。",
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// ============================================
		// 6. Help (使用說明)
		// ============================================
		{
			Name:        "help",
			Description: "顯示機器人使用說明與功能介紹。",
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// ============================================
		// 7. Direct Reply (直接回覆)
		// ============================================
		{
			Name:        "direct_reply",
			Description: "直接回覆使用者。用於：社交對話、意圖不明需澄清、離題詢問、無法匹配其他功能的情況。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"message": {
						Type:        genai.TypeString,
						Description: "回覆內容。語氣友善專業。澄清意圖時提供編號選項。例：「請問您是想查詢：\n1️⃣ 王小明老師的課程？\n2️⃣ 學生王小明的資料？」",
					},
				},
				Required: []string{"message"},
			},
		},
	}
}

// IntentModuleMap maps function names to module and intent names.
// Key: function name (from FunctionDeclaration.Name)
// Value: [module, intent] pair
//
// Order: Course → ID → Contact → Program → Usage → Help → Direct
var IntentModuleMap = map[string][2]string{
	// Course Module
	"course_search":     {"course", "search"},
	"course_smart":      {"course", "smart"},
	"course_uid":        {"course", "uid"},
	"course_extended":   {"course", "extended"},
	"course_historical": {"course", "historical"},
	// ID Module
	"id_search":     {"id", "search"},
	"id_student_id": {"id", "student_id"},
	"id_department": {"id", "department"},
	"id_year":       {"id", "year"},
	"id_dept_codes": {"id", "dept_codes"},
	// Contact Module
	"contact_search":    {"contact", "search"},
	"contact_emergency": {"contact", "emergency"},
	// Program Module
	"program_list":    {"program", "list"},
	"program_search":  {"program", "search"},
	"program_courses": {"program", "courses"},
	// Usage Module
	"usage_query": {"usage", "query"},
	// Help
	"help": {"help", ""},
	// Direct Reply
	"direct_reply": {"direct_reply", ""},
}

// ParamKeyMap maps function names to their primary parameter key.
// This is used to extract the parameter value from the function call args.
// Functions without parameters (contact_emergency, program_list, help) are not listed.
//
// Order: Course → ID → Contact → Program → Direct
var ParamKeyMap = map[string]string{
	// Course Module
	"course_search":     "keyword",
	"course_smart":      "query",
	"course_uid":        "uid",
	"course_extended":   "keyword",
	"course_historical": "keyword", // Also has "year" param, but keyword is primary
	// ID Module
	"id_search":     "name",
	"id_student_id": "student_id",
	"id_department": "department",
	"id_year":       "year",
	"id_dept_codes": "degree",
	// Contact Module
	"contact_search": "query",
	// Program Module
	"program_search":  "query",
	"program_courses": "program_name",
	// Direct Reply
	"direct_reply": "message",
}
