// Package genai provides integration with Google's Generative AI APIs.
// This file contains function declarations for the NLU intent parser.
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
		// Course module functions
		{
			Name:        "course_search",
			Description: "【精確搜尋】當使用者**已知**課程名稱或教師姓名時使用。觸發條件：(1) 提到具體課名如「微積分」「資料結構」「會計學」；(2) 提到教師姓名如「王小明」「陳教授」；(3) 詢問特定課程的資訊（時間、教室、學分）。這是快速查詢模式，直接比對課名或教師名，不消耗 LLM 配額。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名關鍵字。應為具體名稱而非抽象描述。範例：「微積分」（課程名）、「線性代數」（課程名）、「王小明」（教師名）、「程式設計」（課程名）。",
					},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "course_smart",
			Description: "【智慧搜尋】當使用者**不確定**課程名稱，只能描述學習需求或興趣時使用。觸發條件：(1) 使用「想學」「想要」「有興趣」等描述詞；(2) 描述技能或主題而非課名（如「學 Python」「做網站」）；(3) 抽象需求如「輕鬆過的通識」「實用的程式課」；(4) 短縮寫但非課名（如「AI」「NLP」「ESG」）。此功能使用語意搜尋分析課程大綱，消耗 LLM 配額。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "自然語言描述使用者的學習目標或需求。**重要**：若輸入較短（<5字）或為技術縮寫，必須擴展為完整描述。擴展範例：「AI」→「人工智慧 AI artificial intelligence 機器學習 深度學習」、「想學資料分析」→「資料分析 data analysis 統計 視覺化 Python」、「好過的通識」→「通識課程 好過 輕鬆 學分」、「ESG」→「ESG 永續發展 企業社會責任 sustainability」。",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "course_uid",
			Description: "依課程編號查詢課程詳細資訊。支援兩種格式：(1) 完整編號：年度(2-3碼)+學期(1碼)+課號，如 1131U0001、1132M0002；(2) 僅課號：字母+4碼，如 U0001、M0002（自動搜尋最近兩學期）。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"uid": {
						Type:        genai.TypeString,
						Description: "課程編號或課號。完整編號如 1131U0001、1132M0002；僅課號如 U0001、M0002",
					},
				},
				Required: []string{"uid"},
			},
		},

		// ID module functions
		{
			Name:        "id_search",
			Description: "依姓名搜尋學生資訊。**重要限制**：僅有 94-113 學年度（2005-2024）的學生資料。原因：數位學苑 2.0 已於 114 學年度停用，114 學年度以後入學的學生無法查詢。可使用全名或部分姓名搜尋。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {
						Type:        genai.TypeString,
						Description: "學生姓名，可以是全名或部分姓名，如「王小明」「小明」「王」",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "id_student_id",
			Description: "依學號查詢學生資訊。學號為 8-9 位數字，開頭通常代表入學年度。**重要限制**：僅有 94-113 學年度（2005-2024）的學生資料。原因：數位學苑 2.0 已於 114 學年度停用。114 開頭或更新的學號無法查詢。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"student_id": {
						Type:        genai.TypeString,
						Description: "學號，8-9 位數字，如 412345678、41234567",
					},
				},
				Required: []string{"student_id"},
			},
		},
		{
			Name:        "id_department",
			Description: "查詢科系代碼或科系資訊。可輸入科系名稱查代碼，或輸入代碼查科系名稱。常見代碼：資工系(85)、企管系(35)、法律系(25)。注意：學生資料僅涵蓋 94-113 學年度。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"department": {
						Type:        genai.TypeString,
						Description: "科系名稱或代碼，如「資工系」「資訊工程學系」「85」「企管」",
					},
				},
				Required: []string{"department"},
			},
		},

		// Contact module functions
		{
			Name:        "contact_search",
			Description: "查詢校內單位或人員的聯絡方式，包含電話、分機、email 等資訊。可查詢行政單位（教務處、學務處）、學術單位（資工系、圖書館）或特定人員。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "要查詢的單位或人員名稱，如「資工系」「圖書館」「教務處」「學務處」「總務處」",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "contact_emergency",
			Description: "取得校園緊急聯絡電話清單，包含保全、校安中心、緊急救護等聯絡資訊。適用於緊急情況或需要立即聯繫校方的場合。不需要任何參數。",
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// Help function
		{
			Name:        "help",
			Description: "顯示機器人使用說明和功能介紹。當使用者詢問如何使用、需要幫助、輸入「使用說明」「help」「？」或不知道可以查什麼時呼叫。",
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
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
	// contact_emergency and help have no parameters
}
