// Package genai provides integration with LLM APIs (Gemini and Groq).
// This file contains function declarations for the NLU intent parser.
//
// Design Principles (Gemini/Groq - Best Practices 2025):
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
package genai

import "google.golang.org/genai"

// BuildIntentFunctions returns the function declarations for NLU intent parsing.
// These functions represent the available intents the parser can recognize.
func BuildIntentFunctions() []*genai.FunctionDeclaration {
	return []*genai.FunctionDeclaration{
		// ============================================
		// Course Module
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

		// ============================================
		// ID Module
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

		// ============================================
		// Contact Module
		// ============================================
		{
			Name:        "contact_search",
			Description: "查詢校內單位或人員聯絡方式。只提取名稱，移除「辦公室」「電話」等詞。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "單位或人員名稱。注意：只提取名稱本身，必須移除「辦公室」「電話」「分機」「email」「怎麼聯絡」「在哪」等查詢詞。範例：「資工系的電話」→「資工系」。",
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
		// Help
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
		// Program Module
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
	}
}

// IntentModuleMap maps function names to module and intent names.
// Key: function name (from FunctionDeclaration.Name)
// Value: [module, intent] pair
var IntentModuleMap = map[string][2]string{
	"course_search":     {"course", "search"},
	"course_smart":      {"course", "smart"},
	"course_uid":        {"course", "uid"},
	"id_search":         {"id", "search"},
	"id_student_id":     {"id", "student_id"},
	"id_department":     {"id", "department"},
	"contact_search":    {"contact", "search"},
	"contact_emergency": {"contact", "emergency"},
	"help":              {"help", ""},
	"program_list":      {"program", "list"},
	"program_search":    {"program", "search"},
}

// ParamKeyMap maps function names to their primary parameter key.
// This is used to extract the parameter value from the function call args.
var ParamKeyMap = map[string]string{
	"course_search":  "keyword",
	"course_smart":   "query",
	"course_uid":     "uid",
	"id_search":      "name",
	"id_student_id":  "student_id",
	"id_department":  "department",
	"contact_search": "query",
	"program_search": "query",
	// contact_emergency, program_list and help have no parameters
}
