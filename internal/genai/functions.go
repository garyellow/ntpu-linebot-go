// Package genai provides integration with Google's Generative AI APIs.
// This file contains function declarations for the NLU intent parser.
package genai

import "google.golang.org/genai"

// BuildIntentFunctions returns the function declarations for NLU intent parsing.
// These functions represent the available intents the parser can recognize.
func BuildIntentFunctions() []*genai.FunctionDeclaration {
	return []*genai.FunctionDeclaration{
		// Course module functions
		{
			Name:        "course_search",
			Description: "搜尋課程或教師。使用者想依課程名稱或教師姓名查詢課程時呼叫此函數。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"keyword": {
						Type:        genai.TypeString,
						Description: "課程名稱或教師姓名關鍵字，例如「微積分」、「王小明」、「程式設計」",
					},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "course_semantic",
			Description: "用自然語言描述想要的課程類型進行語意搜尋。適用於使用者不知道確切課程名稱，但能描述想學什麼的情況。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "自然語言描述，例如「想學習資料分析」、「Python 相關課程」、「商業管理入門」",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "course_uid",
			Description: "依課程編號直接查詢課程詳細資訊。課程編號格式為：年度(3-4碼)+課號(字母+4碼)，例如 1131U0001。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"uid": {
						Type:        genai.TypeString,
						Description: "課程編號，格式如 1131U0001、1132M0002",
					},
				},
				Required: []string{"uid"},
			},
		},

		// ID module functions
		{
			Name:        "id_search",
			Description: "依姓名搜尋學生資訊。注意：僅支援 112 學年度以前的學生資料。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {
						Type:        genai.TypeString,
						Description: "學生姓名，可以是全名或部分姓名，例如「王小明」、「小明」",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "id_student_id",
			Description: "依學號查詢學生資訊。學號為 8-9 位數字。注意：僅支援 112 學年度以前的學生資料。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"student_id": {
						Type:        genai.TypeString,
						Description: "學號，8-9 位數字，例如 412345678",
					},
				},
				Required: []string{"student_id"},
			},
		},
		{
			Name:        "id_department",
			Description: "查詢科系代碼或科系資訊。可以輸入科系名稱查代碼，或輸入代碼查科系名稱。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"department": {
						Type:        genai.TypeString,
						Description: "科系名稱或代碼，例如「資工系」、「資訊工程學系」、「85」",
					},
				},
				Required: []string{"department"},
			},
		},

		// Contact module functions
		{
			Name:        "contact_search",
			Description: "查詢校內單位或人員的聯絡方式，包含電話、分機、email 等資訊。",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "要查詢的單位或人員名稱，例如「資工系」、「圖書館」、「學務處」",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "contact_emergency",
			Description: "取得校園緊急聯絡電話清單，包含保全、校安中心等緊急聯絡資訊。不需要任何參數。",
			Parameters: &genai.Schema{
				Type:       genai.TypeObject,
				Properties: map[string]*genai.Schema{},
			},
		},

		// Help function
		{
			Name:        "help",
			Description: "顯示機器人使用說明。當使用者詢問如何使用、需要幫助、或輸入「使用說明」時呼叫。",
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
	"course_semantic":   {"course", "semantic"},
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
	"course_search":   "keyword",
	"course_semantic": "query",
	"course_uid":      "uid",
	"id_search":       "name",
	"id_student_id":   "student_id",
	"id_department":   "department",
	"contact_search":  "query",
	// contact_emergency and help have no parameters
}
