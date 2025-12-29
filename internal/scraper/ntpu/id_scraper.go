package ntpu

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// DepartmentCodes maps department names to codes (大學部).
// Note: "法律" and base codes for 社學/社工 require 3rd digit from student ID
var DepartmentCodes = map[string]string{
	"法律": "71", // Base code, requires 3rd digit: 712(法學)/714(司法)/716(財法)
	"法學": "712",
	"司法": "714",
	"財法": "716",
	"公行": "72",
	"經濟": "73",
	"社學": "742", // Full 3-digit code
	"社工": "744", // Full 3-digit code
	"財政": "75",
	"不動": "76",
	"會計": "77",
	"統計": "78",
	"企管": "79",
	"金融": "80",
	"中文": "81",
	"應外": "82",
	"歷史": "83",
	"休運": "84",
	"資工": "85",
	"通訊": "86",
	"電機": "87",
}

// FullDepartmentCodes maps full department names to codes (大學部)
var FullDepartmentCodes = map[string]string{
	"法律學系":       "71",
	"法律學系法學組":    "712",
	"法律學系司法組":    "714",
	"法律學系財經法組":   "716",
	"公共行政暨政策學系":  "72",
	"經濟學系":       "73",
	"社會學系":       "742",
	"社會工作學系":     "744",
	"財政學系":       "75",
	"不動產與城鄉環境學系": "76",
	"會計學系":       "77",
	"統計學系":       "78",
	"企業管理學系":     "79",
	"金融與合作經營學系":  "80",
	"中國文學系":      "81",
	"應用外語學系":     "82",
	"歷史學系":       "83",
	"休閒運動管理學系":   "84",
	"資訊工程學系":     "85",
	"通訊工程學系":     "86",
	"電機工程學系":     "87",
}

// MasterDepartmentCodes maps master's program department names to codes (碩士班)
var MasterDepartmentCodes = map[string]string{
	"企業管理學系碩士班":       "31",
	"會計學系碩士班":         "32",
	"統計學系碩士班":         "33",
	"金融與合作經營學系碩士班":    "34",
	"國際企業研究所碩士班":      "35",
	"資訊管理研究所":         "36",
	"財務金融英語碩士學位學程":    "37",
	"民俗藝術與文化資產研究所":    "41",
	"古典文獻學研究所":        "42",
	"中國文學系碩士班":        "43",
	"歷史學系碩士班":         "44",
	"法律學系碩士班一般生組":     "51",
	"法律學系碩士班法律專業組":    "52",
	"經濟學系碩士班":         "61",
	"社會學系碩士班":         "62",
	"社會工作學系碩士班":       "63",
	"犯罪學研究所":          "64",
	"公共行政暨政策學系碩士班":    "71",
	"財政學系碩士班":         "72",
	"不動產與城鄉環境學系碩士班":   "73",
	"都市計劃研究所碩士班":      "74",
	"自然資源與環境管理研究所碩士班": "75",
	"城市治理英語碩士學位學程":    "76",
	"會計學系碩士在職專班":      "77",
	"統計學系碩士在職專班":      "78",
	"企業管理學系碩士在職專班":    "79",
	"通訊工程學系碩士班":       "81",
	"電機工程學系碩士班":       "82",
	"資訊工程學系碩士班":       "83",
	"智慧醫療管理英語碩士學位學程":  "91",
}

// PhDDepartmentCodes maps PhD program department names to codes (博士班)
var PhDDepartmentCodes = map[string]string{
	"企業管理學系博士班":       "31",
	"會計學系博士班":         "32",
	"法律學系博士班":         "51",
	"經濟學系博士班":         "61",
	"公共行政暨政策學系博士班":    "71",
	"不動產與城鄉環境學系博士班":   "73",
	"都市計劃研究所博士班":      "74",
	"自然資源與環境管理研究所博士班": "75",
	"電機資訊學院博士班":       "76",
}

// DepartmentNames provides reverse mappings: code -> name
var DepartmentNames = reverseMap(DepartmentCodes)

// MasterDepartmentNames provides reverse mappings for master degree programs: code -> name.
var MasterDepartmentNames = reverseMap(MasterDepartmentCodes)

// PhDDepartmentNames provides reverse mappings for PhD programs: code -> name.
var PhDDepartmentNames = reverseMap(PhDDepartmentCodes)

// reverseMap creates a reverse mapping from code to name
func reverseMap(m map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[v] = k
	}
	return result
}

// IsLawDepartment returns true if the department code belongs to Law School (71x).
// Used to determine if "組" should be used instead of "系" in display text.
func IsLawDepartment(deptCode string) bool {
	return strings.HasPrefix(deptCode, "71")
}

// Student type prefixes for scraping
const (
	StudentTypeUndergrad = "4" // 大學部
	StudentTypeMaster    = "7" // 碩士班
	StudentTypePhD       = "8" // 博士班
)

const (
	studentSearchPath = "/portfolio/search.php"
)

// UndergradDeptCodes contains undergraduate department codes for scraping.
var UndergradDeptCodes = []string{
	"71", "712", "714", "716", // 法律學系 (法學/司法/財法)
	"72",         // 公共行政暨政策學系
	"73",         // 經濟學系
	"742", "744", // 社會學系/社會工作學系
	"75", // 財政學系
	"76", // 不動產與城鄉環境學系
	"77", // 會計學系
	"78", // 統計學系
	"79", // 企業管理學系
	"80", // 金融與合作經營學系
	"81", // 中國文學系
	"82", // 應用外語學系
	"83", // 歷史學系
	"84", // 休閒運動管理學系
	"85", // 資訊工程學系
	"86", // 通訊工程學系
	"87", // 電機工程學系
}

// MasterDeptCodes contains master's program department codes for scraping.
var MasterDeptCodes = []string{
	"31", // 企業管理學系碩士班
	"32", // 會計學系碩士班
	"33", // 統計學系碩士班
	"34", // 金融與合作經營學系碩士班
	"35", // 國際企業研究所碩士班
	"36", // 資訊管理研究所
	"37", // 財務金融英語碩士學位學程
	"41", // 民俗藝術與文化資產研究所
	"42", // 古典文獻學研究所
	"43", // 中國文學系碩士班
	"44", // 歷史學系碩士班
	"51", // 法律學系碩士班一般生組
	"52", // 法律學系碩士班法律專業組
	"61", // 經濟學系碩士班
	"62", // 社會學系碩士班
	"63", // 社會工作學系碩士班
	"64", // 犯罪學研究所
	"71", // 公共行政暨政策學系碩士班
	"72", // 財政學系碩士班
	"73", // 不動產與城鄉環境學系碩士班
	"74", // 都市計劃研究所碩士班
	"75", // 自然資源與環境管理研究所碩士班
	"76", // 城市治理英語碩士學位學程
	"77", // 會計學系碩士在職專班
	"78", // 統計學系碩士在職專班
	"79", // 企業管理學系碩士在職專班
	"81", // 通訊工程學系碩士班
	"82", // 電機工程學系碩士班
	"83", // 資訊工程學系碩士班
	"91", // 智慧醫療管理英語碩士學位學程
}

// PhDDeptCodes contains PhD program department codes for scraping.
var PhDDeptCodes = []string{
	"31", // 企業管理學系博士班
	"32", // 會計學系博士班
	"51", // 法律學系博士班
	"61", // 經濟學系博士班
	"71", // 公共行政暨政策學系博士班
	"73", // 不動產與城鄉環境學系博士班
	"74", // 都市計劃研究所博士班
	"75", // 自然資源與環境管理研究所博士班
	"76", // 電機資訊學院博士班
}

// lmsCache is a package-level helper for LMS URL caching.
// Returns the cached working URL or detects a new one.
func lmsCache(ctx context.Context, client *scraper.Client) (string, error) {
	return scraper.NewURLCache(client, "lms").Get(ctx)
}

// ScrapeStudentsByYear scrapes students by year, department code and student type.
// URL: {baseURL}/portfolio/search.php?fmScope=2&page=1&fmKeyword={studentType}{year}{deptCode}
// studentType: "4" for undergrad, "7" for master's, "8" for PhD
// Returns a list of students matching the criteria.
// Supports automatic URL failover across multiple LMS endpoints.
func ScrapeStudentsByYear(ctx context.Context, client *scraper.Client, year int, deptCode, studentType string) ([]*storage.Student, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping students: %w", err)
	}

	var students []*storage.Student

	// Get working base URL with failover support
	baseURL, err := lmsCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working LMS URL: %w", err)
	}

	// Build search keyword: {studentType}{year}{deptCode}
	// Example: 411271 for undergrad year 112, department 71 (法律)
	// Example: 711271 for master's year 112, department 71 (公行碩)
	keyword := fmt.Sprintf("%s%d%s", studentType, year, deptCode)

	// First request to get total pages
	url := fmt.Sprintf("%s%s?fmScope=2&page=1&fmKeyword=%s", baseURL, studentSearchPath, keyword)

	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch first page: %w", err)
	}

	// Count total pages by finding pagination elements
	// Look for <span class="item"> elements
	totalPages := 1
	doc.Find("span.item").Each(func(i int, s *goquery.Selection) {
		if pageNum, err := strconv.Atoi(strings.TrimSpace(s.Text())); err == nil {
			if pageNum > totalPages {
				totalPages = pageNum
			}
		}
	})

	// Parse first page
	students = append(students, parseStudentPage(doc, year)...)

	// Fetch and parse remaining pages
	for page := 2; page <= totalPages; page++ {
		// Check context before each page request
		if err := ctx.Err(); err != nil {
			return students, fmt.Errorf("context canceled during student scraping (partial results): %w", err)
		}

		url := fmt.Sprintf("%s%s?fmScope=2&page=%d&fmKeyword=%s", baseURL, studentSearchPath, page, keyword)

		doc, err := client.GetDocument(ctx, url)
		if err != nil {
			return students, fmt.Errorf("failed to fetch page %d: %w", page, err)
		}

		students = append(students, parseStudentPage(doc, year)...)
	}

	return students, nil
}

// parseStudentPage extracts student information from a search result page
func parseStudentPage(doc *goquery.Document, year int) []*storage.Student {
	students := make([]*storage.Student, 0)
	cachedAt := time.Now().Unix()

	// Find all student entries: <div class="bloglistTitle">
	doc.Find("div.bloglistTitle").Each(func(i int, s *goquery.Selection) {
		// Get student name from <a> tag
		name := strings.TrimSpace(s.Find("a").Text())

		// Get student ID from href attribute
		// Example: /portfolio/410571074
		href, exists := s.Find("a").Attr("href")
		if !exists {
			return
		}

		// Extract student ID from href (last part after /)
		parts := strings.Split(href, "/")
		if len(parts) == 0 {
			return
		}
		studentID := parts[len(parts)-1]

		// Determine department from student ID
		department := determineDepartment(studentID)

		students = append(students, &storage.Student{
			ID:         studentID,
			Name:       name,
			Year:       year,
			Department: department,
			CachedAt:   cachedAt,
		})
	})

	return students
}

// ScrapeStudentByID scrapes a specific student by their student ID
// URL: {baseURL}/portfolio/search.php?fmScope=2&page=1&fmKeyword={studentID}
// Supports automatic URL failover across multiple LMS endpoints
func ScrapeStudentByID(ctx context.Context, client *scraper.Client, studentID string) (*storage.Student, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping student: %w", err)
	}

	// Get working base URL with failover support
	baseURL, err := lmsCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working LMS URL: %w", err)
	}

	url := fmt.Sprintf("%s%s?fmScope=2&page=1&fmKeyword=%s", baseURL, studentSearchPath, studentID)

	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch student: %w", err)
	}

	// Find the student entry
	var student *storage.Student
	doc.Find("div.bloglistTitle").Each(func(i int, s *goquery.Selection) {
		if i > 0 {
			return // Only take the first match
		}

		name := strings.TrimSpace(s.Find("a").Text())
		if name == "" {
			return
		}

		// Extract year from student ID
		year := extractYear(studentID)
		department := determineDepartment(studentID)

		student = &storage.Student{
			ID:         studentID,
			Name:       name,
			Year:       year,
			Department: department,
			CachedAt:   time.Now().Unix(),
		}
	})

	if student == nil {
		return nil, fmt.Errorf("student not found: %s", studentID)
	}

	return student, nil
}

// extractYear extracts the academic year from a student ID
// Example: 410571074 -> 105, 41121074 -> 112
func extractYear(studentID string) int {
	if len(studentID) < 5 {
		return 0
	}

	// Check if year is 3 digits (>= 100) or 2 digits
	isOver99 := len(studentID) == 9

	var yearStr string
	if isOver99 {
		yearStr = studentID[1:4] // Extract 3 digits
	} else {
		yearStr = studentID[1:3] // Extract 2 digits
	}

	year, _ := strconv.Atoi(yearStr)
	return year
}

// determineDepartment determines the department name from a student ID
func determineDepartment(studentID string) string {
	if len(studentID) < 7 {
		return "未知"
	}

	// Check student type and year format
	isOver99 := len(studentID) == 9
	isMaster := studentID[0] == '7'
	isPhD := studentID[0] == '8'

	// Extract department code based on ID length
	// Both 8-digit and 9-digit use 2-digit base department code
	// 8-digit (year<=99): Type(1) + Year(2) + Dept(2) + Group?(1) + Serial(2)
	//   Example: 4 + 10 + 71 + 2 + 01 → need to check if 71/74 then extract position [5]
	// 9-digit (year>=100): Type(1) + Year(3) + Dept(2) + Group?(1) + Serial(3)
	//   Example: 4 + 107 + 71 + 2 + 001 → need to check if 71/74 then extract position [6]
	var deptCode string
	if isOver99 {
		// 9-digit: dept code is 2-digit at positions [4:6]
		deptCode = studentID[4:6] // e.g., "71", "74", "79", "87"
	} else {
		// 8-digit: dept code is 2-digit at positions [3:5]
		deptCode = studentID[3:5] // e.g., "71", "74", "79"
	}

	// Handle Master's and PhD programs (always use first 2 digits of dept code)
	if isMaster || isPhD {
		// Graduate programs only use 2-digit department codes
		baseDept := deptCode
		if len(deptCode) > 2 {
			baseDept = deptCode[:2]
		}

		if isMaster {
			if name, ok := MasterDepartmentNames[baseDept]; ok {
				return name
			}
			return "未知碩士班"
		}

		if name, ok := PhDDepartmentNames[baseDept]; ok {
			return name
		}
		return "未知博士班"
	}

	// Undergraduate: For dept 71/74, need to extract 3rd digit
	// Both 8-digit and 9-digit formats require this
	if deptCode == "71" || deptCode == "74" {
		// Extract the 3rd digit (group identifier)
		// 8-digit: position [5], 9-digit: position [6]
		thirdDigitPos := 5
		if isOver99 {
			thirdDigitPos = 6
		}
		if len(studentID) > thirdDigitPos {
			deptCode += string(studentID[thirdDigitPos])
		}
		// Now deptCode is 3-digit: 712(法學), 714(司法), 716(財法), 742(社學), 744(社工)
	}

	// Undergraduate lookup with 2 or 3-digit code
	// For 3-digit codes (71x, 74x), try exact match first, then fall back to 2-digit base
	name, ok := DepartmentNames[deptCode]
	if !ok && len(deptCode) == 3 {
		// Try 2-digit base code if 3-digit lookup fails
		// e.g., "712" → "71", "742" → "74", "790" → "79"
		baseDept := deptCode[:2]
		name, ok = DepartmentNames[baseDept]
	}

	if ok {
		// All 71x departments (712/714/716) return unified "法律系"
		if IsLawDepartment(deptCode) {
			return "法律系"
		}
		// 742/744 have specific names in DepartmentNames (社學/社工)
		return name + "系"
	}

	return "未知系所"
}
