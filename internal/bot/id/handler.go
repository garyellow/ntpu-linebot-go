package id

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles student ID related queries
type Handler struct {
	db             *storage.DB
	scraper        *scraper.Client
	metrics        *metrics.Metrics
	logger         *logger.Logger
	stickerManager *sticker.Manager
}

const (
	moduleName           = "id"
	splitChar            = "$"
	senderName           = "學號魔法師"
	MaxStudentsPerSearch = 500 // Maximum students to return in name search results
)

// Valid keywords for student ID queries
var (
	validStudentKeywords = []string{
		"學號", "學生", "姓名", "學生姓名", "學生編號",
		"student", "id", // English keywords
	}
	validDepartmentKeywords = []string{
		"系", "所", "系所", "科系", "系名", "系所名", "科系名", "系所名稱", "科系名稱",
		"dep", "department", // English keywords
	}
	validDepartmentCodeKeywords = []string{
		"系代碼", "系所代碼", "科系代碼", "系編號", "系所編號", "科系編號",
		"depCode", "departmentCode", // English keywords
	}
	validYearKeywords = []string{
		"學年", "年份", "年度", "學年度", "入學年", "入學學年", "入學年度",
		"year", // English keyword
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
func NewHandler(db *storage.DB, scraper *scraper.Client, metrics *metrics.Metrics, logger *logger.Logger, stickerManager *sticker.Manager) *Handler {
	return &Handler{
		db:             db,
		scraper:        scraper,
		metrics:        metrics,
		logger:         logger,
		stickerManager: stickerManager,
	}
}

// CanHandle checks if the message is for the ID module
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)

	// Check for "所有系代碼"
	if text == allDeptCodeText {
		return true
	}

	// Check for student ID (8-9 digits) at the start of text
	// This handles direct ID input like "412345678"
	if len(text) >= 8 && len(text) <= 9 && isNumeric(text) {
		return true
	}

	// Check for student name search with keyword
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

	// Check for direct student ID input (8-9 digits without keyword)
	if len(text) >= 8 && len(text) <= 9 && isNumeric(text) {
		return h.handleStudentIDQuery(ctx, text)
	}

	// Handle department name query - extract term after keyword
	if match := departmentRegex.FindString(text); match != "" {
		// Extract what comes after the keyword
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))
		if searchTerm != "" {
			return h.handleDepartmentNameQuery(searchTerm)
		}
	}

	// Handle department code query - extract term after keyword
	if match := deptCodeRegex.FindString(text); match != "" {
		// Extract what comes after the keyword
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))
		if searchTerm != "" {
			return h.handleDepartmentCodeQuery(searchTerm)
		}
	}

	// Handle year query - extract year after keyword
	if match := yearRegex.FindString(text); match != "" {
		// Extract what comes after the keyword
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))
		if searchTerm != "" {
			return h.handleYearQuery(searchTerm)
		}
		// No year provided - show guidance message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"📅 按學年查詢學生\n\n請輸入要查詢的學年度\n例如：學年 112、學年 110\n\n📋 查詢流程：\n1️⃣ 選擇學院群（文法商/公社電資）\n2️⃣ 選擇學院\n3️⃣ 選擇科系\n4️⃣ 查看該年度該科系所有學生\n\n⚠️ 僅提供 95-112 學年度資料",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("查詢 112 學年度", "學年 112")},
			{Action: lineutil.NewMessageAction("查詢 111 學年度", "學年 111")},
			{Action: lineutil.NewMessageAction("查詢 110 學年度", "學年 110")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Handle student ID or name query
	if loc := studentRegex.FindStringIndex(text); loc != nil {
		// Extract the search term after the keyword
		searchTerm := strings.TrimSpace(text[loc[1]:])
		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender("🔢 請在關鍵字後輸入查詢內容\n\n例如：\n• 學號 小明\n• 學號 412345678\n\n💡 也可直接輸入 8-9 位學號", sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("📅 按學年查詢", "學年")},
				{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
			})
			return []messaging_api.MessageInterface{msg}
		}

		// Check if it's a student ID (8-9 digits)
		if isNumeric(searchTerm) && (len(searchTerm) == 8 || len(searchTerm) == 9) {
			return h.handleStudentIDQuery(ctx, searchTerm)
		}
		// Otherwise, it's a name search
		return h.handleStudentNameQuery(ctx, searchTerm)
	}

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the ID module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	log.Infof("Handling ID postback: %s", data)

	// Handle "兇" (easter egg) - support both with and without prefix
	if data == "兇" || data == "id:兇" {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("泥好兇喔～～(⊙﹏⊙)", sender),
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

		switch action {
		case "搜尋全系":
			return h.handleYearSearchConfirm(year)
		case "文法商", "公社電資":
			return h.handleCollegeGroupSelection(action, year)
		case "人文學院", "法律學院", "商學院", "公共事務學院", "社會科學學院", "電機資訊學院":
			return h.handleCollegeSelection(action, year)
		default:
			// Validate department code format (1-3 digits) before lookup
			if len(action) > 3 || len(action) == 0 {
				sender := lineutil.GetSender(senderName, h.stickerManager)
				return []messaging_api.MessageInterface{
					lineutil.NewTextMessageWithConsistentSender(
						"❌ 無效的系代碼格式\n\n系代碼應為 1-3 位數字",
						sender,
					),
				}
			}

			// Verify department code contains only digits
			if _, err := strconv.Atoi(action); err != nil {
				sender := lineutil.GetSender(senderName, h.stickerManager)
				return []messaging_api.MessageInterface{
					lineutil.NewTextMessageWithConsistentSender(
						"❌ 無效的系代碼格式\n\n系代碼應為 1-3 位數字",
						sender,
					),
				}
			}

			// Check if it's a department code
			if _, ok := ntpu.DepartmentNames[action]; ok {
				return h.handleDepartmentSelection(ctx, action, year)
			}
		}
	}

	return []messaging_api.MessageInterface{}
}

// handleAllDepartmentCodes returns all department codes
func (h *Handler) handleAllDepartmentCodes() []messaging_api.MessageInterface {
	var builder strings.Builder
	builder.WriteString("📚 所有系代碼：\n")

	// Group by department
	for name, code := range ntpu.DepartmentCodes {
		builder.WriteString(fmt.Sprintf("\n%s系 → %s", name, code))
	}

	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("查詢學號", "學號")},
		{Action: lineutil.NewMessageAction("按學年查詢", "學年")},
	})
	return []messaging_api.MessageInterface{msg}
}

// handleDepartmentNameQuery handles department name to code queries
func (h *Handler) handleDepartmentNameQuery(deptName string) []messaging_api.MessageInterface {
	deptName = strings.TrimSuffix(deptName, "系")
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Check regular department codes
	if code, ok := ntpu.DepartmentCodes[deptName]; ok {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("%s系的系代碼是：%s", deptName, code), sender)
		// Add quick reply for all department codes
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(allDeptCodeText, allDeptCodeText)},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Check full department codes
	if code, ok := ntpu.FullDepartmentCodes[deptName]; ok {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("%s的系代碼是：%s", deptName, code), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(allDeptCodeText, allDeptCodeText)},
		})
		return []messaging_api.MessageInterface{msg}
	}

	msg := lineutil.NewTextMessageWithConsistentSender("🔍 找不到該系所\n\n請輸入正確的系名，例如：資工、法律、企管", sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📋 "+allDeptCodeText, allDeptCodeText)},
		{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
	})
	return []messaging_api.MessageInterface{msg}
}

// handleDepartmentCodeQuery handles department code to name queries
func (h *Handler) handleDepartmentCodeQuery(code string) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Check department names
	if name, ok := ntpu.DepartmentNames[code]; ok {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("系代碼 %s 是：%s系", code, name), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(allDeptCodeText, allDeptCodeText)},
		})
		return []messaging_api.MessageInterface{msg}
	}

	msg := lineutil.NewTextMessageWithConsistentSender("🔍 找不到該系代碼\n\n請輸入正確的系代碼，例如：85（資工系）", sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📋 "+allDeptCodeText, allDeptCodeText)},
		{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
	})
	return []messaging_api.MessageInterface{msg}
}

// handleYearQuery handles year-based search queries
func (h *Handler) handleYearQuery(yearStr string) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Parse year
	year, err := parseYear(yearStr)
	if err != nil {
		msg := lineutil.NewTextMessageWithConsistentSender("📅 無效的年份格式\n\n請輸入 2-4 位數字\n例如：112 或 2023", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 112 學年度", "學年 112")},
			{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	currentYear := time.Now().Year() - 1911

	// Validate year - order matters for proper responses!
	// 1. Check future year first
	if year > currentYear {
		msg := lineutil.NewTextMessageWithConsistentSender("🔮 哎呀～你是未來人嗎？", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", currentYear), fmt.Sprintf("學年 %d", currentYear))},
			{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// 2. Check for 2024+ data warning (year >= 113) - LMS 2.0 is deprecated
	if year >= 113 {
		imageURL := "https://raw.githubusercontent.com/garyellow/ntpu-linebot-go/main/assets/rip.png"
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("數位學苑 2.0 已停止使用，無法取得資料", sender),
			lineutil.NewImageMessage(imageURL, imageURL),
		}
	}

	// 3. Check if year is before NTPU was founded (ROC 89 = 2000)
	if year < 90 {
		msg := lineutil.NewTextMessageWithConsistentSender("🏫 學校都還沒蓋好啦\n\n臺北大學於民國 89 年成立", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 95 學年度", "學年 95")},
			{Action: lineutil.NewMessageAction("🔢 查詢學號", "學號")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// 4. Check if year is before LMS was launched (ROC 95)
	if year >= 90 && year < 95 {
		msg := lineutil.NewTextMessageWithConsistentSender("📒 數位學苑還沒出生喔\n\n請輸入 95 學年度以後的年份", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 95 學年度", "學年 95")},
			{Action: lineutil.NewMessageAction("🔢 查詢學號", "學號")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Create confirmation message with flow explanation + Python-style meme buttons
	confirmText := fmt.Sprintf("📅 %d 學年度學生查詢\n\n📋 查詢流程：\n1️⃣ 選擇學院群\n2️⃣ 選擇學院\n3️⃣ 選擇科系\n\n確定要查詢嗎？", year)
	confirmMsg := lineutil.NewConfirmTemplate(
		"確認學年度",
		confirmText,
		lineutil.NewPostbackActionWithDisplayText("哪次不是", "哪次不是", fmt.Sprintf("id:搜尋全系%s%d", splitChar, year)),
		lineutil.NewPostbackActionWithDisplayText("我在想想", "再啦乾ಠ_ಠ", "id:兇"),
	)
	return []messaging_api.MessageInterface{
		lineutil.SetSender(confirmMsg, sender),
	}
}

// handleYearSearchConfirm handles the year search confirmation - shows college group selection
func (h *Handler) handleYearSearchConfirm(yearStr string) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Create college group selection template with clear guidance
	actions := []messaging_api.ActionInterface{
		lineutil.NewPostbackActionWithDisplayText("文法商", fmt.Sprintf("搜尋 %s 學年度文法商學院群", yearStr), fmt.Sprintf("id:文法商%s%s", splitChar, yearStr)),
		lineutil.NewPostbackActionWithDisplayText("公社電資", fmt.Sprintf("搜尋 %s 學年度公社電資學院群", yearStr), fmt.Sprintf("id:公社電資%s%s", splitChar, yearStr)),
	}

	msg := lineutil.NewButtonsTemplateWithImage(
		fmt.Sprintf("%s 學年度學生查詢", yearStr),
		fmt.Sprintf("%s 學年度", yearStr),
		"請選擇科系所屬學院群\n\n📚 文法商：人文、法律、商學院\n🏛️ 公社電資：公共、社科、電資學院",
		"https://new.ntpu.edu.tw/assets/logo/ntpu_logo.png",
		actions,
	)

	return []messaging_api.MessageInterface{
		lineutil.SetSender(msg, sender),
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
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.ErrorMessageWithDetailAndSender("查詢學號時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重試", "學號 "+studentID)},
				{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
			})
		}
		return []messaging_api.MessageInterface{msg}
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

	// Get consistent sender for all messages in this reply
	sender := lineutil.GetSender(senderName, h.stickerManager)

	student, err = ntpu.ScrapeStudentByID(ctx, h.scraper, studentID)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape student ID: %s", studentID)
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("🔍 學號 %s 不存在喔\n\n請確認學號是否正確", studentID), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("🔢 查詢其他學號", "學號")},
			{Action: lineutil.NewMessageAction("🏛️ 查詢系所代碼", allDeptCodeText)},
		})
		return []messaging_api.MessageInterface{msg}
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

	// Get consistent sender for all messages in this reply
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Search in cache
	students, err := h.db.SearchStudentsByName(name)
	if err != nil {
		log.WithError(err).Error("Failed to search students by name")
		msg := lineutil.ErrorMessageWithDetailAndSender("搜尋姓名時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重新搜尋", "學號")},
				{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
			})
		}
		return []messaging_api.MessageInterface{msg}
	}

	if len(students) == 0 {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf(
			"🔍 查無姓名包含「%s」的學生\n\n💡 請注意：\n• 僅提供 101-112 學年度資料\n• 請確認姓名拼寫是否正確\n• 可嘗試輸入完整姓名或部分姓名",
			name,
		), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("🔄 重新搜尋", "學號")},
			{Action: lineutil.NewMessageAction("📅 按學年查詢", "學年")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Sort by student ID (newest first)
	// Database query already limits to 500 students
	// Add warning if we hit the limit (likely more results available)
	var messages []messaging_api.MessageInterface
	if len(students) >= MaxStudentsPerSearch {
		warningMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 搜尋結果達到上限 %d 筆\n\n可能有更多結果未顯示，建議：\n• 輸入更完整的姓名\n• 使用「學年」功能按年度查詢", MaxStudentsPerSearch),
			sender,
		)
		messages = append(messages, warningMsg)
	}

	// Format results - split into multiple messages if needed (100 students per message)
	for i := 0; i < len(students); i += 100 {
		// Respect LINE reply limit (max 5 messages)
		if len(messages) >= 5 {
			break
		}

		end := i + 100
		if end > len(students) {
			end = len(students)
		}

		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("📋 搜尋結果（第 %d-%d 筆，共 %d 筆）\n\n", i+1, end, len(students)))

		for j := i; j < end; j++ {
			student := students[j]
			builder.WriteString(fmt.Sprintf("%s  %s  %d  %s\n",
				student.ID, student.Name, student.Year, student.Department))
		}

		messages = append(messages, lineutil.NewTextMessageWithConsistentSender(builder.String(), sender))
	}

	// Add Quick Reply to the last message
	if len(messages) > 0 {
		if lastMsg, ok := messages[len(messages)-1].(*messaging_api.TextMessage); ok {
			lastMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("🔄 重新搜尋", "學號")},
				{Action: lineutil.NewMessageAction("🏛️ 查詢系所代碼", allDeptCodeText)},
			})
		}
	}

	return messages
}

// formatStudentResponse formats a student record as a LINE message
// Uses Flex Message for modern, card-based UI (improved from Python simple text version)
func (h *Handler) formatStudentResponse(student *storage.Student, fromCache bool) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Header: Student badge (using standardized component)
	header := lineutil.NewHeaderBadge("🎓", "學生資訊")

	// Hero: Name with NTPU green background (using standardized component)
	hero := lineutil.NewHeroBox(student.Name, "國立臺北大學")

	// Body: Student details with improved vertical layout to prevent truncation
	// Each info row uses vertical stacking: icon+label on top, value below
	contents := []messaging_api.FlexComponentInterface{
		// 學號 row
		lineutil.NewInfoRowWithMargin("🆔", "學號", student.ID, lineutil.BoldInfoRowStyle(), "md"),
		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
		// 系所 row
		lineutil.NewInfoRowWithMargin("🏫", "系所", student.Department, lineutil.BoldInfoRowStyle(), "md"),
		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
		// 學年度 row
		lineutil.NewInfoRowWithMargin("📅", "入學學年", fmt.Sprintf("%d 學年度", student.Year), lineutil.BoldInfoRowStyle(), "md"),
	}

	if fromCache {
		contents = append(contents,
			lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
			lineutil.NewFlexText("📌 資料來自快取").WithSize("xs").WithColor(lineutil.ColorGray400).WithMargin("md").FlexText,
		)
	}

	// Footer: Action buttons
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(
			lineutil.NewClipboardAction("📋 複製學號", student.ID),
		).WithStyle("primary").WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(
			lineutil.NewMessageAction("🔍 查詢其他學號", "學號"),
		).WithStyle("secondary").WithHeight("sm").FlexButton,
	).WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(
		header,
		hero.FlexBox,
		lineutil.NewFlexBox("vertical", contents...).WithSpacing("sm"),
		footer,
	)

	// Create Flex Message with sender
	msg := lineutil.NewFlexMessage(fmt.Sprintf("學生資訊 - %s", student.Name), bubble.FlexBubble)
	msg.Sender = sender

	// Add Quick Reply for next actions
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📚 查看所有系代碼", "所有系代碼")},
		{Action: lineutil.NewMessageAction("📅 按學年查詢", "學年")},
		{Action: lineutil.NewMessageAction("📌 使用說明", "使用說明")},
	})

	return []messaging_api.MessageInterface{msg}
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
// Only validates format, not range (range validation is done in handleYearQuery for proper error messages)
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

// handleCollegeGroupSelection handles college group selection (文法商 or 公社電資)
func (h *Handler) handleCollegeGroupSelection(group, year string) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)
	var actions []messaging_api.ActionInterface
	var collegeList string

	if group == "文法商" {
		collegeList = "📖 人文：中文、應外、歷史\n⚖️ 法律：法學、司法、財法\n💼 商學：企管、金融、會計、統計、休運"
		actions = []messaging_api.ActionInterface{
			lineutil.NewPostbackActionWithDisplayText("📖 人文學院", fmt.Sprintf("搜尋 %s 學年度人文學院", year), fmt.Sprintf("id:人文學院%s%s", splitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("⚖️ 法律學院", fmt.Sprintf("搜尋 %s 學年度法律學院", year), fmt.Sprintf("id:法律學院%s%s", splitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("💼 商學院", fmt.Sprintf("搜尋 %s 學年度商學院", year), fmt.Sprintf("id:商學院%s%s", splitChar, year)),
		}
	} else { // 公社電資
		collegeList = "🏛️ 公共事務：公行、不動、財政\n👥 社科：經濟、社學、社工\n💻 電資：電機、資工、通訊"
		actions = []messaging_api.ActionInterface{
			lineutil.NewPostbackActionWithDisplayText("🏛️ 公共事務學院", fmt.Sprintf("搜尋 %s 學年度公共事務學院", year), fmt.Sprintf("id:公共事務學院%s%s", splitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("👥 社會科學學院", fmt.Sprintf("搜尋 %s 學年度社會科學學院", year), fmt.Sprintf("id:社會科學學院%s%s", splitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("💻 電機資訊學院", fmt.Sprintf("搜尋 %s 學年度電機資訊學院", year), fmt.Sprintf("id:電機資訊學院%s%s", splitChar, year)),
		}
	}

	msg := lineutil.NewButtonsTemplate(
		fmt.Sprintf("%s 學年度 %s", year, group),
		fmt.Sprintf("%s 學年度・%s", year, group),
		fmt.Sprintf("請選擇學院\n\n%s", collegeList),
		actions,
	)

	return []messaging_api.MessageInterface{
		lineutil.SetSender(msg, sender),
	}
}

// handleCollegeSelection handles specific college selection
func (h *Handler) handleCollegeSelection(college, year string) []messaging_api.MessageInterface {
	// College to departments mapping
	collegeMap := map[string]struct {
		imageURL    string
		departments []string
		isLaw       bool
	}{
		"人文學院": {
			imageURL:    "https://walkinto.in/upload/-192z7YDP8-JlchfXtDvI.JPG",
			departments: []string{"中文", "應外", "歷史"},
			isLaw:       false,
		},
		"法律學院": {
			imageURL:    "https://walkinto.in/upload/byupdk9PvIZyxupOy9Dw8.JPG",
			departments: []string{"法學", "司法", "財法"},
			isLaw:       true,
		},
		"商學院": {
			imageURL:    "https://walkinto.in/upload/ZJum7EYwPUZkedmXNtvPL.JPG",
			departments: []string{"企管", "金融", "會計", "統計", "休運"},
		},
		"公共事務學院": {
			imageURL:    "https://walkinto.in/upload/ZJhs4wEaDIWklhiVwV6DI.jpg",
			departments: []string{"公行", "不動", "財政"},
		},
		"社會科學學院": {
			imageURL:    "https://walkinto.in/upload/WyPbshN6DIZ1gvZo2NTvU.JPG",
			departments: []string{"經濟", "社學", "社工"},
			isLaw:       false,
		},
		"電機資訊學院": {
			imageURL:    "https://walkinto.in/upload/bJ9zWWHaPLWJg9fW-STD8.png",
			departments: []string{"電機", "資工", "通訊"},
		},
	}

	info, ok := collegeMap[college]
	if !ok {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("❌ 無效的學院選擇", sender),
		}
	}

	return h.buildDepartmentSelectionTemplate(year, info.imageURL, info.departments, info.isLaw)
}

// buildDepartmentSelectionTemplate creates department selection template
func (h *Handler) buildDepartmentSelectionTemplate(year, imageURL string, departments []string, isLaw bool) []messaging_api.MessageInterface {
	departmentClass := "科系"
	if isLaw {
		departmentClass = "組別"
	}

	// Build actions
	actions := make([]messaging_api.ActionInterface, 0, len(departments))
	for _, deptName := range departments {
		deptCode, ok := ntpu.DepartmentCodes[deptName]
		if !ok {
			continue
		}

		displayText := fmt.Sprintf("搜尋%s學年度", year)
		if isLaw {
			displayText += "法律系"
		}
		displayText += ntpu.DepartmentNames[deptCode]
		if isLaw {
			displayText += "組"
		} else {
			displayText += "系"
		}

		label := deptName
		if isLaw {
			// For law, use full name from DepartmentNames
			if fullName, ok := ntpu.DepartmentNames[deptCode]; ok {
				label = fullName
			}
		}

		actions = append(actions, lineutil.NewPostbackActionWithDisplayText(
			label,
			displayText,
			fmt.Sprintf("id:%s%s%s", deptCode, splitChar, year),
		))
	}

	// If actions <= 4, use ButtonsTemplate; otherwise use CarouselTemplate
	// LINE API limits: ButtonsTemplate max 4 actions, CarouselTemplate max 10 columns
	sender := lineutil.GetSender(senderName, h.stickerManager)

	if len(actions) <= 4 {
		msg := lineutil.NewButtonsTemplateWithImage(
			fmt.Sprintf("選擇%s", departmentClass),
			fmt.Sprintf("選擇%s", departmentClass),
			fmt.Sprintf("請選擇要查詢的%s", departmentClass),
			imageURL,
			actions,
		)
		return []messaging_api.MessageInterface{
			lineutil.SetSender(msg, sender),
		}
	}

	// Use carousel for more than 4 actions (split into groups of 3)
	columns := make([]lineutil.CarouselColumn, 0)
	for i := 0; i < len(actions); i += 3 {
		end := i + 3
		if end > len(actions) {
			end = len(actions)
		}
		columnActions := actions[i:end]

		// Pad to 3 actions if needed
		for len(columnActions) < 3 {
			columnActions = append(columnActions, lineutil.NewPostbackAction("　", "　"))
		}

		columns = append(columns, lineutil.CarouselColumn{
			ThumbnailImageURL: imageURL,
			Title:             fmt.Sprintf("選擇%s", departmentClass),
			Text:              fmt.Sprintf("請選擇要查詢的%s", departmentClass),
			Actions:           columnActions,
		})
	}

	msg := lineutil.NewCarouselTemplate(fmt.Sprintf("選擇%s", departmentClass), columns)
	return []messaging_api.MessageInterface{
		lineutil.SetSender(msg, sender),
	}
}

// handleDepartmentSelection handles final department selection and queries the database
func (h *Handler) handleDepartmentSelection(ctx context.Context, deptCode, yearStr string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("❌ 無效的年份格式", sender),
		}
	}

	deptName, ok := ntpu.DepartmentNames[deptCode]
	if !ok {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("❌ 無效的系代碼", sender),
		}
	}

	// Query students from cache using department name that matches determineDepartment logic
	// determineDepartment returns "法律系" for all 71x codes, and "XX系" for others
	// So we should query using "法律系", "資工系", "社學系", "社工系", etc.
	var queryDeptName string
	if strings.HasPrefix(deptCode, "71") {
		// All law school departments (712/714/716) are stored as "法律系"
		queryDeptName = "法律系"
	} else {
		// For other departments, add "系" suffix
		queryDeptName = deptName + "系"
	}

	students, err := h.db.GetStudentsByYearDept(year, queryDeptName)
	if err != nil {
		log.WithError(err).Error("Failed to search students by year and department")
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithDetailAndSender("查詢學生名單時發生問題", sender),
		}
	}

	// If not found in cache, try scraping
	if len(students) == 0 {
		log.Infof("Cache miss for department selection: %d %s, scraping...", year, deptCode)
		h.metrics.RecordCacheMiss(moduleName)
		startTime := time.Now()

		scrapedStudents, err := ntpu.ScrapeStudentsByYear(ctx, h.scraper, year, deptCode)
		if err != nil {
			log.WithError(err).Errorf("Failed to scrape students for year %d dept %s", year, deptCode)
			h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
			return []messaging_api.MessageInterface{
				lineutil.ErrorMessageWithDetailAndSender("查詢學生名單時發生問題，可能是學校網站暫時無法存取", sender),
			}
		}

		if len(scrapedStudents) > 0 {
			h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
			// Save to cache and convert to value slice
			for _, s := range scrapedStudents {
				if err := h.db.SaveStudent(s); err != nil {
					log.WithError(err).Warn("Failed to save student to cache")
				}
				students = append(students, *s)
			}
		} else {
			h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
		}
	} else {
		h.metrics.RecordCacheHit(moduleName)
	}

	if len(students) == 0 {
		departmentType := "系"
		if strings.HasPrefix(deptCode, "71") {
			departmentType = "組"
		}
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("🤔 %d 學年度%s%s好像沒有人耶", year, deptName, departmentType), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("🔄 重新選擇", fmt.Sprintf("學年 %d", year))},
			{Action: lineutil.NewMessageAction("🔢 查詢學號", "學號")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Format student list
	var builder strings.Builder
	departmentType := "系"
	displayName := deptName
	if strings.HasPrefix(deptCode, "71") {
		departmentType = "組"
		// For law, use "法律系XX組" format
		displayName = "法律系" + deptName
	}

	builder.WriteString(fmt.Sprintf("%d學年度%s%s學生名單：\n\n", year, displayName, departmentType))

	for _, student := range students {
		builder.WriteString(fmt.Sprintf("%s  %s\n", student.ID, student.Name))
	}

	builder.WriteString(fmt.Sprintf("\n%d學年度%s%s共有%d位學生", year, displayName, departmentType, len(students)))

	// Note: sender was already created at the start of handleDepartmentSelection, reuse it
	msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📅 查詢其他學年", "學年")},
		{Action: lineutil.NewMessageAction("🏛️ 查詢系所代碼", allDeptCodeText)},
	})
	return []messaging_api.MessageInterface{msg}
}
