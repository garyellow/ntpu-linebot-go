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

// Department code mappings (學部 - Undergraduate)
var DepartmentCodes = map[string]string{
	"法律": "71",
	"法學": "712",
	"司法": "714",
	"財法": "716",
	"公行": "72",
	"經濟": "73",
	"社學": "742",
	"社工": "744",
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

// Full department name mappings (學部)
var FullDepartmentCodes = map[string]string{
	"法律學系":       "71",
	"法學組":        "712",
	"司法組":        "714",
	"財經法組":       "716",
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

// Master's program department codes (碩士班)
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

// PhD program department codes (博士班)
var PhDDepartmentCodes = map[string]string{
	"會計學系博士班":         "32",
	"法律學系博士班":         "51",
	"經濟學系博士班":         "61",
	"公共行政暨政策學系博士班":    "71",
	"不動產與城鄉環境學系博士班":   "73",
	"都市計劃研究所博士班":      "74",
	"自然資源與環境管理研究所博士班": "75",
	"電機資訊學院博士班":       "76",
}

// Reverse mappings: code -> name
var DepartmentNames = reverseMap(DepartmentCodes)
var MasterDepartmentNames = reverseMap(MasterDepartmentCodes)
var PhDDepartmentNames = reverseMap(PhDDepartmentCodes)

// reverseMap creates a reverse mapping from code to name
func reverseMap(m map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[v] = k
	}
	return result
}

const (
	// Base URL for student search
	baseURL           = "https://lms.ntpu.edu.tw"
	studentSearchPath = "/portfolio/search.php"
)

// ScrapeStudentsByYear scrapes students by year and department code
// URL: https://lms.ntpu.edu.tw/portfolio/search.php?fmScope=2&page=1&fmKeyword=4{year}{deptCode}
// Returns a list of students matching the criteria
func ScrapeStudentsByYear(ctx context.Context, client *scraper.Client, year int, deptCode string) ([]*storage.Student, error) {
	students := make([]*storage.Student, 0)

	// Build search keyword: 4{year}{deptCode}
	// Example: 411271 for year 112, department 71 (法律)
	keyword := fmt.Sprintf("4%d%s", year, deptCode)

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
	students = append(students, parseStudentPage(doc, year, deptCode)...)

	// Fetch and parse remaining pages
	for page := 2; page <= totalPages; page++ {
		url := fmt.Sprintf("%s%s?fmScope=2&page=%d&fmKeyword=%s", baseURL, studentSearchPath, page, keyword)

		doc, err := client.GetDocument(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch page %d: %w", page, err)
		}

		students = append(students, parseStudentPage(doc, year, deptCode)...)
	}

	return students, nil
}

// parseStudentPage extracts student information from a search result page
func parseStudentPage(doc *goquery.Document, year int, deptCode string) []*storage.Student {
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
// URL: https://lms.ntpu.edu.tw/portfolio/search.php?fmScope=2&page=1&fmKeyword={studentID}
func ScrapeStudentByID(ctx context.Context, client *scraper.Client, studentID string) (*storage.Student, error) {
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

	// Extract department code
	var deptCode string
	if isOver99 {
		deptCode = studentID[4:6] // Positions 4-5
	} else {
		deptCode = studentID[3:5] // Positions 3-4
	}

	// Handle special cases
	if isMaster {
		if name, ok := MasterDepartmentNames[deptCode]; ok {
			return name
		}
		return "未知碩士班"
	}

	if isPhD {
		if name, ok := PhDDepartmentNames[deptCode]; ok {
			return name
		}
		return "未知博士班"
	}

	// Handle 社學 (742) - needs 3rd digit
	if deptCode == "74" {
		if isOver99 && len(studentID) > 6 {
			deptCode += string(studentID[6])
		} else if len(studentID) > 5 {
			deptCode += string(studentID[5])
		}
	}

	// Undergraduate
	if name, ok := DepartmentNames[deptCode]; ok {
		return name + "系"
	}

	return "未知系所"
}
