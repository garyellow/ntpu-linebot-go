// Package genai provides integration with LLM APIs (Gemini, Groq, and Cerebras).
// This file contains function declarations for the NLU intent parser.
//
// Design Principles (2025-2026 Best Practices):
//
// 1. SELF-DOCUMENTING: Each function description is a complete contract
//   - Trigger conditions embedded in descriptions
//   - Clear parameter specifications with examples
//   - Model selects function based on description match
//
// 2. STRONG TYPING: Use enums where possible to reduce ambiguity
//
// 3. MINIMAL PROMPT: System prompt is concise; functions are self-explanatory
//
// IMPORTANT: Function declarations use genai.Type* constants (e.g., genai.TypeString = "STRING").
// When converting to other provider formats (e.g., Groq), ensure types are lowercased to match
// JSON Schema spec ("string" not "STRING"). See buildGroqTools() in groq_intent.go for example.
//
// Module Organization:
// - Course Module: course_search, course_smart, course_uid, course_extended, course_historical
// - ID Module: id_search, id_student_id, id_department, id_year, id_dept_codes
// - Contact Module: contact_search, contact_emergency
// - Program Module: program_list, program_search, program_courses
// - Usage Module: usage_query
// - Help: help
// - Direct Reply: direct_reply
package genai

import "google.golang.org/genai"

// BuildIntentFunctions returns the function declarations for NLU intent parsing.
// Model selects the appropriate function based on description match.
//
// Total: 18 functions across 7 modules
func BuildIntentFunctions() []*genai.FunctionDeclaration {
	return []*genai.FunctionDeclaration{
		// ============================================
		// 1. Course Module (課程查詢)
		// ============================================

		// Search by course name or teacher name
		{
			Name: "course_search",
			Description: `依課程名稱或教師姓名搜尋最近學期課程。

觸發條件：輸入包含明確的課程名稱或教師姓名
範例：微積分、資料結構、王小明老師、陳教授的課`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名",
					},
				},
				Required: []string{"keyword"},
			},
		},

		// Smart search by learning needs
		{
			Name: "course_smart",
			Description: `依學習需求智慧搜尋課程。

觸發條件：描述學習目標、興趣或需求，而非具體課名
特徵詞：想學、有興趣、好過的、輕鬆的、XX相關
範例：想學資料分析、好過的通識、AI相關課程`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "學習需求描述（保留原始表達）",
					},
				},
				Required: []string{"query"},
			},
		},

		// Search by course UID
		{
			Name: "course_uid",
			Description: `依課程編號查詢課程詳細資訊。

觸發條件：輸入包含課程編號格式
格式：完整編號(1131U0001)或簡短編號(U0001)
範例：1131U0001、U2345、1132M0002`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"uid": {
						Type:        genai.TypeString,
						Description: "課程編號",
					},
				},
				Required: []string{"uid"},
			},
		},

		// Extended search in older semesters
		{
			Name: "course_extended",
			Description: `搜尋較舊學期的課程（3-4學期前）。

觸發條件：要求搜尋更多學期或舊學期的課程
關鍵詞：更多學期、舊學期、之前開過、歷史課程、找更多
範例：找更多學期的微積分、舊學期有沒有開資料庫`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名（不含時間修飾詞）",
					},
				},
				Required: []string{"keyword"},
			},
		},

		// Historical search by specific year
		{
			Name: "course_historical",
			Description: `查詢指定學年度的課程。

觸發條件：輸入包含明確年份數字（2-4位）+ 課程或教師相關詞
年份格式：民國年(110,112)或西元年(2022,2023)
西元轉民國：西元年-1911（2022→111, 2023→112）

範例：
• 110學年微積分 → year=110, keyword=微積分
• 2022年資料結構 → year=111, keyword=資料結構
• 幫我找2023年王老師的課 → year=112, keyword=王老師`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"year": {
						Type:        genai.TypeString,
						Description: "民國年學年度（3位數）。西元年需轉換：2022→111, 2023→112",
					},
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名（不含年份詞）",
					},
				},
				Required: []string{"year", "keyword"},
			},
		},

		// ============================================
		// 2. ID Module (學生查詢)
		// ============================================

		// Search student by name
		{
			Name: "id_search",
			Description: `依姓名搜尋學生資料。

觸發條件：明確要查學生資料，包含「學生」「學號」「同學」等詞 + 人名
範例：學號查詢王小明、找學生小明、查同學張三`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {
						Type:        genai.TypeString,
						Description: "學生姓名（全名或部分）",
					},
				},
				Required: []string{"name"},
			},
		},

		// Search student by student ID
		{
			Name: "id_student_id",
			Description: `依學號查詢學生資料。

觸發條件：輸入包含8-9位連續數字
範例：412345678、41234567`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"student_id": {
						Type:        genai.TypeString,
						Description: "學號（8-9位數字）",
					},
				},
				Required: []string{"student_id"},
			},
		},

		// Search students by academic year
		{
			Name: "id_year",
			Description: `依學年度查詢學生名單。

觸發條件：查詢特定學年度的學生（包含「學生」相關詞）
範例：112學年度學生、110年入學的學生
支援範圍：94-112學年度`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"year": {
						Type:        genai.TypeString,
						Description: "民國年學年度（3位數）",
					},
				},
				Required: []string{"year"},
			},
		},

		// Search students by department
		{
			Name: "id_department",
			Description: `依科系查詢學生。

觸發條件：查詢特定科系的學生
範例：資工系學生、85系的學生`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"department": {
						Type:        genai.TypeString,
						Description: "科系代碼或名稱",
					},
				},
				Required: []string{"department"},
			},
		},

		// Department code reference table
		{
			Name: "id_dept_codes",
			Description: `顯示系代碼對照表。

觸發條件：詢問系所代碼列表
範例：系代碼、碩士班代碼、所有系所代碼`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"degree": {
						Type:        genai.TypeString,
						Description: "學制類型：bachelor（學士班）、master（碩士班）、phd（博士班）",
						Enum:        []string{"bachelor", "master", "phd"},
					},
				},
			},
		},

		// ============================================
		// 3. Contact Module (聯絡資訊)
		// ============================================

		// Search contact information
		{
			Name: "contact_search",
			Description: `查詢校內單位或人員聯絡方式。

觸發條件：詢問聯絡資訊（電話、分機、email、地址）
範例：資工系電話、圖書館分機、王小明怎麼聯絡`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "單位或人員名稱（移除查詢動詞如電話、分機、email）",
					},
				},
				Required: []string{"query"},
			},
		},

		// Emergency contact
		{
			Name: "contact_emergency",
			Description: `取得校園緊急聯絡電話。

觸發條件：詢問緊急聯絡資訊
關鍵詞：緊急、校安、保全、救護`,
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// ============================================
		// 4. Program Module (學程查詢)
		// ============================================

		// List all programs
		{
			Name: "program_list",
			Description: `列出所有可修讀學程。

觸發條件：詢問所有學程列表
範例：有哪些學程、學程列表`,
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// Search programs by keyword
		{
			Name: "program_search",
			Description: `依關鍵字搜尋學程。

觸發條件：搜尋特定領域的學程
範例：人工智慧學程、永續相關學程`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "學程名稱關鍵字（可省略「學程」後綴）",
					},
				},
				Required: []string{"query"},
			},
		},

		// List courses in a program
		{
			Name: "program_courses",
			Description: `列出指定學程包含的課程（必修/選修）。

觸發條件：詢問特定學程有哪些課程
範例：人工智慧學程有哪些課、永續發展學程的課程`,
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"programName": {
						Type:        genai.TypeString,
						Description: "學程全名（必須包含「學程」二字）。例：「人工智慧學程」「永續發展學程」。",
					},
				},
				Required: []string{"programName"},
			},
		},

		// ============================================
		// 5. Usage Module (配額查詢)
		// ============================================

		// Query usage quota
		{
			Name: "usage_query",
			Description: `查詢使用者的功能額度狀態。

觸發條件：詢問使用額度或配額
範例：我的額度、還剩多少配額`,
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// ============================================
		// 6. Help (使用說明)
		// ============================================

		// Show help
		{
			Name: "help",
			Description: `顯示機器人使用說明與功能介紹。

觸發條件：詢問如何使用或功能說明
範例：怎麼用、說明、help`,
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// ============================================
		// 7. Direct Reply (直接回覆)
		// ============================================

		// Direct reply for conversation
		{
			Name: "direct_reply",
			Description: `直接回覆使用者。

觸發條件：社交對話、意圖不明需澄清、離題詢問
範例：你好、謝謝、今天天氣如何`,
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

// ParamKeysMap maps function names to their parameter keys.
// All parameter keys in the slice will be extracted from the function call args.
// Functions without parameters (contact_emergency, program_list, help) are not listed.
//
// Best Practice:
//   - LLM function calling returns all args in FunctionCall.Args (Gemini) or Function.Arguments (OpenAI)
//   - We iterate through all specified keys to build the params map
//   - Handler receives the full params map and validates required parameters
//
// Order: Course → ID → Contact → Program → Direct
var ParamKeysMap = map[string][]string{
	// Course Module
	"course_search":     {"keyword"},
	"course_smart":      {"query"},
	"course_uid":        {"uid"},
	"course_extended":   {"keyword"},
	"course_historical": {"year", "keyword"}, // Multi-param: both are required
	// ID Module
	"id_search":     {"name"},
	"id_student_id": {"student_id"},
	"id_department": {"department"},
	"id_year":       {"year"},
	"id_dept_codes": {"degree"}, // Optional param, handler has default value
	// Contact Module
	"contact_search": {"query"},
	// Program Module
	"program_search":  {"query"},
	"program_courses": {"programName"},
	// Direct Reply
	"direct_reply": {"message"},
}
