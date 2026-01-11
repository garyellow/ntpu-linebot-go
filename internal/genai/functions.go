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
			Description: "依課程名稱或教師姓名精確搜尋。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名。範例：「微積分」「程式設計」「王小明」。注意：只提取名稱本身，不要包含「課程」「老師」等前綴。",
					},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "course_smart",
			Description: "依學習興趣或需求描述進行語意搜尋。適用於抽象需求或技術縮寫。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "學習目標描述或技術關鍵詞。範例：「想學 AI」「資料分析」「ESG」",
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
			Description: "在更多歷史學期中搜尋課程。當最近學期找不到想要的課程時使用。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名。範例：「微積分」「程式設計」",
					},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "course_historical",
			Description: "查詢特定學年度的課程。使用者明確指定學年度時使用。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"year": {
						Type:        genai.TypeString,
						Description: "學年度，民國年 3 位數。範例：「110」「112」",
					},
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名。範例：「微積分」「王小明」",
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
						Description: "學生姓名，全名或部分皆可。例如「王小明」或「小明」",
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
			Description: "查詢科系代碼或名稱。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"department": {
						Type:        genai.TypeString,
						Description: "科系代碼（如 85）或名稱（如資工系）。系統會自動辨識數字代碼或文字名稱。",
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
			Description: "查詢校內單位或人員聯絡方式。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "單位或人員名稱。只提取名稱本身，移除「辦公室」「電話」「分機」「email」「怎麼聯絡」「在哪」等查詢詞。範例：「資工系的電話」→「資工系」。",
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
			Description: "依關鍵字搜尋學程。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "學程名稱關鍵字。範例：「智財」「永續」「資訊」",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "program_courses",
			Description: "查詢特定學程包含的課程。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"program_name": {
						Type:        genai.TypeString,
						Description: "學程名稱。範例：「人工智慧學程」「永續發展學程」「智慧財產權學程」",
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
		// 6. Direct Reply (直接回覆)
		// ============================================
		{
			Name:        "direct_reply",
			Description: "直接回覆使用者。當訊息不屬於任何查詢功能（閒聊、問候、感謝、離題詢問），或需要澄清使用者意圖時使用。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"message": {
						Type:        genai.TypeString,
						Description: "要傳送給使用者的回覆內容。",
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
