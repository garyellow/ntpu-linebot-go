package id

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/pkg/lineutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles student ID related queries
type Handler struct {
	db      *storage.DB
	scraper *scraper.Client
	metrics *metrics.Metrics
	logger  *logger.Logger
}

const (
	moduleName = "id"
	splitChar  = "$"
)

// Valid keywords for student ID queries
var (
	validStudentKeywords = []string{
		"學號", "student", "name", "學生", "姓名", "學生姓名", "學生編號",
	}
	validDepartmentKeywords = []string{
		"dep", "department", "系", "所", "系所", "科系", "系名", "系所名", "科系名",
		"系所名稱", "科系名稱",
	}
	validDepartmentCodeKeywords = []string{
		"depCode", "departmentCode", "系代碼", "系所代碼", "科系代碼",
		"系編號", "系所編號", "科系編號",
	}
	validYearKeywords = []string{
		"year", "年份", "學年", "年度", "學年度", "入學年", "入學學年", "入學年度",
	}

	studentRegex    = buildRegex(validStudentKeywords)
	departmentRegex = buildRegex(validDepartmentKeywords)
	deptCodeRegex   = buildRegex(validDepartmentCodeKeywords)
	yearRegex       = buildRegex(validYearKeywords)
	allDeptCodeText = "所有系代碼"
)

// buildRegex creates a regex pattern from keywords
func buildRegex(keywords []string) *regexp.Regexp {
	pattern := "(?i)" + strings.Join(keywords, "|")
	return regexp.MustCompile(pattern)
}

// NewHandler creates a new ID handler
func NewHandler(db *storage.DB, scraper *scraper.Client, metrics *metrics.Metrics, logger *logger.Logger) *Handler {
	return &Handler{
		db:      db,
		scraper: scraper,
		metrics: metrics,
		logger:  logger,
	}
}

// CanHandle checks if the message is for the ID module
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)

	// Check for "所有系代碼"
	if text == allDeptCodeText {
		return true
	}

	// Check for student ID (8-9 digits)
	if match := studentRegex.FindString(text); match != "" {
		if isNumeric(match) && (len(match) == 8 || len(match) == 9) {
			return true
		}
	}

	// Check for student name search
	if studentRegex.MatchString(text) {
		return true
	}

	// Check for department queries
	if departmentRegex.MatchString(text) || deptCodeRegex.MatchString(text) {
		return true
	}

	// Check for year queries
	if yearRegex.MatchString(text) {
		return true
	}

	return false
}

// HandleMessage handles text messages for the ID module
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	text = strings.TrimSpace(text)

	log.Infof("Handling ID message: %s", text)

	// Handle "所有系代碼"
	if text == allDeptCodeText {
		return h.handleAllDepartmentCodes()
	}

	// Handle department name query
	if match := departmentRegex.FindString(text); match != "" {
		return h.handleDepartmentNameQuery(match)
	}

	// Handle department code query
	if match := deptCodeRegex.FindString(text); match != "" {
		return h.handleDepartmentCodeQuery(match)
	}

	// Handle year query
	if match := yearRegex.FindString(text); match != "" {
		return h.handleYearQuery(match)
	}

	// Handle student ID or name query
	if match := studentRegex.FindString(text); match != "" {
		// Check if it's a student ID (8-9 digits)
		if isNumeric(match) && (len(match) == 8 || len(match) == 9) {
			return h.handleStudentIDQuery(ctx, match)
		}
		// Otherwise, it's a name search
		return h.handleStudentNameQuery(ctx, match)
	}

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the ID module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	log.Infof("Handling ID postback: %s", data)

	// Handle "兇" (easter egg)
	if data == "兇" {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage("泥好兇喔~~இ௰இ"),
		}
	}

	// Handle year search postback
	if strings.Contains(data, splitChar) {
		parts := strings.Split(data, splitChar)
		if len(parts) != 2 {
			return []messaging_api.MessageInterface{}
		}

		action := parts[0]
		year := parts[1]

		if action == "搜尋全系" {
			return h.handleYearSearchConfirm(year)
		}
	}

	return []messaging_api.MessageInterface{}
}

// handleAllDepartmentCodes returns all department codes
func (h *Handler) handleAllDepartmentCodes() []messaging_api.MessageInterface {
	var builder strings.Builder
	builder.WriteString("📚 所有系代碼：\n\n")

	// Group by department
	for name, code := range ntpu.DepartmentCodes {
		builder.WriteString(fmt.Sprintf("%s系 → %s\n", name, code))
	}

	return []messaging_api.MessageInterface{
		lineutil.NewTextMessage(builder.String()),
	}
}

// handleDepartmentNameQuery handles department name to code queries
func (h *Handler) handleDepartmentNameQuery(deptName string) []messaging_api.MessageInterface {
	deptName = strings.TrimSuffix(deptName, "系")

	// Check regular department codes
	if code, ok := ntpu.DepartmentCodes[deptName]; ok {
		msg := lineutil.NewTextMessage(fmt.Sprintf("%s系的系代碼是：%s", deptName, code))
		// Add quick reply for all department codes
		msg.(*messaging_api.TextMessage).QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(allDeptCodeText, allDeptCodeText)},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Check full department codes
	if code, ok := ntpu.FullDepartmentCodes[deptName]; ok {
		msg := lineutil.NewTextMessage(fmt.Sprintf("%s的系代碼是：%s", deptName, code))
		msg.(*messaging_api.TextMessage).QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(allDeptCodeText, allDeptCodeText)},
		})
		return []messaging_api.MessageInterface{msg}
	}

	return []messaging_api.MessageInterface{
		lineutil.NewTextMessage("❌ 找不到該系所\n\n請輸入正確的系名，例如：資工、法律、企管"),
	}
}

// handleDepartmentCodeQuery handles department code to name queries
func (h *Handler) handleDepartmentCodeQuery(code string) []messaging_api.MessageInterface {
	// Check department names
	if name, ok := ntpu.DepartmentNames[code]; ok {
		msg := lineutil.NewTextMessage(fmt.Sprintf("系代碼 %s 是：%s系", code, name))
		msg.(*messaging_api.TextMessage).QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(allDeptCodeText, allDeptCodeText)},
		})
		return []messaging_api.MessageInterface{msg}
	}

	return []messaging_api.MessageInterface{
		lineutil.NewTextMessage("❌ 找不到該系代碼\n\n請輸入正確的系代碼，例如：85（資工系）"),
	}
}

// handleYearQuery handles year-based search queries
func (h *Handler) handleYearQuery(yearStr string) []messaging_api.MessageInterface {
	// Parse year
	year, err := parseYear(yearStr)
	if err != nil {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage("❌ 無效的年份格式\n\n請輸入 2-4 位數字，例如：112 或 2023"),
		}
	}

	currentYear := time.Now().Year() - 1911

	// Validate year
	if year > currentYear {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage("你未來人？(⊙ˍ⊙)"),
		}
	}

	// Check for 2024+ data warning (year >= 113)
	if year >= 113 {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage("⚠️ 數位學苑 2.0 已停止使用\n\n無法取得 113 學年度（2024年）之後的資料。\n\n舊版系統已不再維護，建議洽詢學校相關單位。"),
		}
	}

	if year < 90 {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage("學校都還沒蓋好(￣▽￣)"),
		}
	}

	if year >= 90 && year < 95 {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage("數位學苑還沒出生喔~~"),
		}
	}

	// Create confirmation message
	confirmText := fmt.Sprintf("是否要搜尋 %d 學年度的學生？", year)
	return []messaging_api.MessageInterface{
		lineutil.NewConfirmTemplate(
			"確認學年度",
			confirmText,
			lineutil.NewPostbackAction("哪次不是", fmt.Sprintf("搜尋全系%s%d", splitChar, year)),
			lineutil.NewPostbackAction("我再想想", "兇"),
		),
	}
}

// handleYearSearchConfirm handles the year search confirmation
func (h *Handler) handleYearSearchConfirm(yearStr string) []messaging_api.MessageInterface {
	return []messaging_api.MessageInterface{
		lineutil.NewTextMessage(fmt.Sprintf("🔍 搜尋功能開發中\n\n%s 學年度的全系學生搜尋功能將在未來版本中實現。", yearStr)),
	}
}

// handleStudentIDQuery handles student ID queries
func (h *Handler) handleStudentIDQuery(ctx context.Context, studentID string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()

	// Check cache first
	student, err := h.db.GetStudentByID(studentID)
	if err != nil {
		log.WithError(err).Error("Failed to query cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessage(fmt.Errorf("資料庫查詢失敗")),
		}
	}

	if student != nil {
		// Cache hit
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Cache hit for student ID: %s", studentID)
		return h.formatStudentResponse(student, true)
	}

	// Cache miss - scrape from website
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("Cache miss for student ID: %s, scraping...", studentID)

	student, err = ntpu.ScrapeStudentByID(ctx, h.scraper, studentID)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape student ID: %s", studentID)
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage(fmt.Sprintf("❌ 學號 %s 不存在或無法取得資料\n\n請確認學號是否正確", studentID)),
		}
	}

	// Save to cache
	if err := h.db.SaveStudent(student); err != nil {
		log.WithError(err).Warn("Failed to save student to cache")
	}

	h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
	return h.formatStudentResponse(student, false)
}

// handleStudentNameQuery handles student name queries
func (h *Handler) handleStudentNameQuery(ctx context.Context, name string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)

	// Search in cache
	students, err := h.db.SearchStudentsByName(name)
	if err != nil {
		log.WithError(err).Error("Failed to search students by name")
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessage(fmt.Errorf("資料庫查詢失敗")),
		}
	}

	if len(students) == 0 {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessage(fmt.Sprintf("🔍 查無姓名包含「%s」的學生\n\n請確認姓名是否正確，或嘗試其他關鍵字", name)),
		}
	}

	// Sort by student ID (newest first)
	// Take only last 500 students
	if len(students) > 500 {
		students = students[len(students)-500:]
	}

	// Format results - split into multiple messages if needed (100 students per message)
	messages := make([]messaging_api.MessageInterface, 0)
	for i := 0; i < len(students); i += 100 {
		end := i + 100
		if end > len(students) {
			end = len(students)
		}

		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("🔍 搜尋結果 (第 %d-%d 筆，共 %d 筆)：\n\n", i+1, end, len(students)))

		for j := i; j < end; j++ {
			student := students[j]
			builder.WriteString(fmt.Sprintf("%s  %s  %d  %s\n",
				student.ID, student.Name, student.Year, student.Department))
		}

		messages = append(messages, lineutil.NewTextMessage(builder.String()))
	}

	return messages
}

// formatStudentResponse formats a student record as a LINE message
func (h *Handler) formatStudentResponse(student *storage.Student, fromCache bool) []messaging_api.MessageInterface {
	// Format student information
	text := fmt.Sprintf("👤 學生資訊\n\n學號：%s\n姓名：%s\n學年：%d\n系所：%s",
		student.ID, student.Name, student.Year, student.Department)

	if fromCache {
		text += "\n\n📌 資料來自快取"
	}

	// Add 2024+ warning for recent years
	messages := []messaging_api.MessageInterface{
		lineutil.NewTextMessage(text),
	}

	if student.Year >= 113 {
		messages = append(messages, lineutil.NewTextMessage(
			"⚠️ 資料提醒\n\n113 學年度（2024年）後的資料可能不完整或已過期。\n數位學苑 2.0 已停止使用。"))
	}

	return messages
}

// Helper functions

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// parseYear parses a year string (2-4 digits) to ROC year
func parseYear(yearStr string) (int, error) {
	if len(yearStr) < 2 || len(yearStr) > 4 {
		return 0, fmt.Errorf("invalid year length")
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return 0, err
	}

	// Convert to ROC year if AD year
	if year >= 1911 {
		year = year - 1911
	}

	return year, nil
}
