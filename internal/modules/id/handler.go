// Package id implements the student ID lookup module for the LINE bot.
// It handles student searches by name, department, and academic year.
package id

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sliceutil"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles student ID related queries.
// It depends on *storage.DB directly for data access.
type Handler struct {
	db             *storage.DB
	scraper        *scraper.Client
	metrics        *metrics.Metrics
	logger         *logger.Logger
	stickerManager *sticker.Manager
}

// Name returns the module name
func (h *Handler) Name() string {
	return ModuleName
}

// ID handler constants.
const (
	ModuleName           = "id" // Module identifier for registration
	senderName           = "學號小幫手"
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

	studentRegex    = bot.BuildKeywordRegex(validStudentKeywords)
	departmentRegex = bot.BuildKeywordRegex(validDepartmentKeywords)
	deptCodeRegex   = bot.BuildKeywordRegex(validDepartmentCodeKeywords)
	yearRegex       = bot.BuildKeywordRegex(validYearKeywords)
	allDeptCodeText = "所有系代碼"
)

// NewHandler creates a new ID handler with required dependencies.
// All parameters are mandatory for proper handler operation.
func NewHandler(
	db *storage.DB,
	scraper *scraper.Client,
	metrics *metrics.Metrics,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
) *Handler {
	return &Handler{
		db:             db,
		scraper:        scraper,
		metrics:        metrics,
		logger:         logger,
		stickerManager: stickerManager,
	}
}

// Intent names for NLU dispatcher
const (
	IntentSearch     = "search"     // Student name search
	IntentStudentID  = "student_id" // Direct student ID lookup
	IntentDepartment = "department" // Department name query
)

// DispatchIntent handles NLU-parsed intents for the ID module.
// It validates required parameters and calls the appropriate handler method.
//
// Supported intents:
//   - "search": requires "name" param, calls handleStudentNameQuery
//   - "student_id": requires "student_id" param, calls handleStudentIDQuery
//   - "department": requires "department" param, calls handleDepartmentNameQuery
//
// Returns error if intent is unknown or required parameters are missing.
func (h *Handler) DispatchIntent(ctx context.Context, intent string, params map[string]string) ([]messaging_api.MessageInterface, error) {
	// Validate parameters first (before logging) to support testing with nil dependencies
	switch intent {
	case IntentSearch:
		name, ok := params["name"]
		if !ok || name == "" {
			return nil, fmt.Errorf("%w: name", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching ID intent: %s, name: %s", intent, name)
		}
		return h.handleStudentNameQuery(ctx, name), nil

	case IntentStudentID:
		studentID, ok := params["student_id"]
		if !ok || studentID == "" {
			return nil, fmt.Errorf("%w: student_id", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching ID intent: %s, student_id: %s", intent, studentID)
		}
		return h.handleStudentIDQuery(ctx, studentID), nil

	case IntentDepartment:
		department, ok := params["department"]
		if !ok || department == "" {
			return nil, fmt.Errorf("%w: department", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching ID intent: %s, department: %s", intent, department)
		}
		return h.handleDepartmentNameQuery(department), nil

	default:
		return nil, fmt.Errorf("%w: %s", domerrors.ErrUnknownIntent, intent)
	}
}

// CanHandle checks if the message is for the ID module
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)

	if text == allDeptCodeText {
		return true
	}

	if len(text) >= 8 && len(text) <= 9 && stringutil.IsNumeric(text) {
		return true
	}

	if studentRegex.MatchString(text) {
		return true
	}

	if departmentRegex.MatchString(text) || deptCodeRegex.MatchString(text) {
		return true
	}

	if yearRegex.MatchString(text) {
		return true
	}

	return false
}

// HandleMessage handles text messages for the ID module
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	text = strings.TrimSpace(text)

	log.Debugf("Handling ID message: %s", text)

	if text == allDeptCodeText {
		return h.handleAllDepartmentCodes()
	}

	if len(text) >= 8 && len(text) <= 9 && stringutil.IsNumeric(text) {
		return h.handleStudentIDQuery(ctx, text)
	}

	// Handle department name query - extract term after keyword
	if match := departmentRegex.FindString(text); match != "" {
		searchTerm := bot.ExtractSearchTerm(text, match)
		if searchTerm != "" {
			return h.handleDepartmentNameQuery(searchTerm)
		}
	}

	// Handle department code query - extract term after keyword
	if match := deptCodeRegex.FindString(text); match != "" {
		searchTerm := bot.ExtractSearchTerm(text, match)
		if searchTerm != "" {
			return h.handleDepartmentCodeQuery(searchTerm)
		}
	}

	// Handle year query - extract year after keyword
	if match := yearRegex.FindString(text); match != "" {
		searchTerm := bot.ExtractSearchTerm(text, match)
		if searchTerm != "" {
			return h.handleYearQuery(searchTerm)
		}
		// No year provided - show guidance message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"📅 按學年度查詢學生\n\n請輸入學年度進行查詢\n例如：學年 112、學年 110\n\n📋 查詢流程：\n1️⃣ 選擇學院群（文法商/公社電資）\n2️⃣ 選擇學院\n3️⃣ 選擇系所\n4️⃣ 查看該系所所有學生\n\n⚠️ 僅提供 94-113 學年度資料",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 112 學年度", "學年 112")},
			{Action: lineutil.NewMessageAction("📅 查詢 111 學年度", "學年 111")},
			{Action: lineutil.NewMessageAction("📅 查詢 110 學年度", "學年 110")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	if loc := studentRegex.FindStringIndex(text); loc != nil {
		match := studentRegex.FindString(text)
		searchTerm := bot.ExtractSearchTerm(text, match)
		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender("🎓 請在關鍵字後輸入查詢內容\n\n例如：\n• 學號 小明\n• 學號 412345678\n\n💡 也可直接輸入 8-9 位學號", sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				lineutil.QuickReplyYearAction(),
				lineutil.QuickReplyHelpAction(),
			})
			return []messaging_api.MessageInterface{msg}
		}

		if stringutil.IsNumeric(searchTerm) && (len(searchTerm) == 8 || len(searchTerm) == 9) {
			return h.handleStudentIDQuery(ctx, searchTerm)
		}
		return h.handleStudentNameQuery(ctx, searchTerm)
	}

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the ID module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	log.Infof("Handling ID postback: %s", data)

	// Handle "兇" (easter egg) - support both with and without prefix
	if data == "兇" || data == "id:兇" {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("泥好兇喔～～(⊙﹏⊙)", sender),
		}
	}

	// Handle year search postback
	if strings.Contains(data, bot.PostbackSplitChar) {
		parts := strings.Split(data, bot.PostbackSplitChar)
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
				msg := lineutil.NewTextMessageWithConsistentSender(
					"❌ 無效的系代碼格式\n\n系代碼應為 1-3 位數字",
					sender,
				)
				msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
					lineutil.QuickReplyYearAction(),
					lineutil.QuickReplyDeptCodeAction(),
					lineutil.QuickReplyHelpAction(),
				})
				return []messaging_api.MessageInterface{msg}
			}

			// Verify department code contains only digits
			if _, err := strconv.Atoi(action); err != nil {
				sender := lineutil.GetSender(senderName, h.stickerManager)
				msg := lineutil.NewTextMessageWithConsistentSender(
					"❌ 無效的系代碼格式\n\n系代碼應為 1-3 位數字",
					sender,
				)
				msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
					lineutil.QuickReplyYearAction(),
					lineutil.QuickReplyDeptCodeAction(),
					lineutil.QuickReplyHelpAction(),
				})
				return []messaging_api.MessageInterface{msg}
			}

			if _, ok := ntpu.DepartmentNames[action]; ok {
				return h.handleDepartmentSelection(ctx, action, year)
			}
		}
	}

	return []messaging_api.MessageInterface{}
}

// handleAllDepartmentCodes returns all department codes organized by college
func (h *Handler) handleAllDepartmentCodes() []messaging_api.MessageInterface {
	var builder strings.Builder
	builder.WriteString("📋 所有系代碼一覽\n")

	// 人文學院
	builder.WriteString("\n📖 人文學院")
	builder.WriteString("\n  中文系 → 81")
	builder.WriteString("\n  應外系 → 82")
	builder.WriteString("\n  歷史系 → 83")

	// 法律學院
	builder.WriteString("\n\n⚖️ 法律學院")
	builder.WriteString("\n  法學組 → 712")
	builder.WriteString("\n  司法組 → 714")
	builder.WriteString("\n  財法組 → 716")

	// 商學院
	builder.WriteString("\n\n💼 商學院")
	builder.WriteString("\n  企管系 → 79")
	builder.WriteString("\n  金融系 → 80")
	builder.WriteString("\n  會計系 → 77")
	builder.WriteString("\n  統計系 → 78")
	builder.WriteString("\n  休運系 → 84")

	// 公共事務學院
	builder.WriteString("\n\n🏛️ 公共事務學院")
	builder.WriteString("\n  公行系 → 72")
	builder.WriteString("\n  財政系 → 75")
	builder.WriteString("\n  不動系 → 76")

	// 社會科學學院
	builder.WriteString("\n\n👥 社會科學學院")
	builder.WriteString("\n  經濟系 → 73")
	builder.WriteString("\n  社學系 → 742")
	builder.WriteString("\n  社工系 → 744")

	// 電機資訊學院
	builder.WriteString("\n\n💻 電機資訊學院")
	builder.WriteString("\n  電機系 → 87")
	builder.WriteString("\n  資工系 → 85")
	builder.WriteString("\n  通訊系 → 86")

	builder.WriteString("\n\n💡 使用方式：學年 112 後選擇科系")

	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyYearAction(),
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// handleDepartmentNameQuery handles department name to code queries with fuzzy matching
// Search Strategy:
//  1. Exact match: Check DepartmentCodes and FullDepartmentCodes maps directly
//  2. Fuzzy match: If no exact match, use ContainsAllRunes to find matching department names
//     Example: "資工" matches "資訊工程學系" because all chars exist in the full name
func (h *Handler) handleDepartmentNameQuery(deptName string) []messaging_api.MessageInterface {
	deptName = strings.TrimSuffix(deptName, "系")
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Step 1: Check regular department codes (exact match)
	if code, ok := ntpu.DepartmentCodes[deptName]; ok {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("%s系的系代碼是：%s", deptName, code), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyDeptCodeAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Step 2: Check full department codes (exact match)
	if code, ok := ntpu.FullDepartmentCodes[deptName]; ok {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("%s的系代碼是：%s", deptName, code), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyDeptCodeAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Step 3: Fuzzy matching - search in FullDepartmentCodes using ContainsAllRunes
	// This enables "資工" to match "資訊工程學系"
	var matches []struct {
		name string
		code string
	}
	for fullName, code := range ntpu.FullDepartmentCodes {
		if bot.ContainsAllRunes(fullName, deptName) {
			matches = append(matches, struct {
				name string
				code string
			}{fullName, code})
		}
	}

	// If exactly one match, return it directly
	if len(matches) == 1 {
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍「%s」→ %s\n\n系代碼是：%s", deptName, matches[0].name, matches[0].code),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyDeptCodeAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// If multiple matches, show all options
	if len(matches) > 1 {
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("🔍「%s」找到多個符合的系所：\n\n", deptName))
		for _, m := range matches {
			builder.WriteString(fmt.Sprintf("• %s → %s\n", m.name, m.code))
		}
		builder.WriteString("\n💡 請輸入更完整的系名以縮小範圍")
		msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyDeptCodeAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	msg := lineutil.NewTextMessageWithConsistentSender("🔍 查無該系所\n\n請輸入正確的系名\n例如：資工、法律、企管", sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyDeptCodeAction(),
		lineutil.QuickReplyHelpAction(),
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
			lineutil.QuickReplyDeptCodeAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	msg := lineutil.NewTextMessageWithConsistentSender("🔍 查無該系代碼\n\n請輸入正確的系代碼\n例如：85（資工系）", sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyDeptCodeAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// handleYearQuery handles year-based search queries
func (h *Handler) handleYearQuery(yearStr string) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Parse year
	year, err := parseYear(yearStr)
	if err != nil {
		msg := lineutil.NewTextMessageWithConsistentSender("📅 年份格式不正確\n\n請輸入 2-4 位數字\n例如：112 或 2023", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 112 學年度", "學年 112")},
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	currentYear := time.Now().Year() - 1911

	// Validate year - order matters for proper responses!
	// 1. Check future year first
	if year > currentYear {
		msg := lineutil.NewTextMessageWithConsistentSender(config.IDYearFutureMessage, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", min(currentYear, config.IDDataYearEnd)), fmt.Sprintf("學年 %d", min(currentYear, config.IDDataYearEnd)))},
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// 2. Check for 2025+ data warning (year >= 114) - LMS 2.0 is deprecated
	if year >= config.IDDataCutoffYear {
		imageURL := "https://raw.githubusercontent.com/garyellow/ntpu-linebot-go/main/assets/rip.png"
		msg := lineutil.NewTextMessageWithConsistentSender(config.IDYear114PlusMessage, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 113 學年度", "學年 113")},
			{Action: lineutil.NewMessageAction("📅 查詢 112 學年度", "學年 112")},
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{
			msg,
			lineutil.NewImageMessage(imageURL, imageURL),
		}
	}

	// 3. Check if year is before NTPU was founded (ROC 89 = 2000)
	if year < config.NTPUFoundedYear {
		msg := lineutil.NewTextMessageWithConsistentSender(config.IDYearBeforeNTPUMessage, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 94 學年度", "學年 94")},
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// 4. Check if year is before LMS has complete data (90-93 have sparse data)
	if year < config.LMSLaunchYear {
		msg := lineutil.NewTextMessageWithConsistentSender(config.IDYearTooOldMessage, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 94 學年度", "學年 94")},
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Create confirmation message with flow explanation + meme buttons
	confirmText := fmt.Sprintf("📅 %d 學年度學生查詢\n\n📋 查詢流程：\n1️⃣ 選擇學院群\n2️⃣ 選擇學院\n3️⃣ 選擇系所\n\n確定要開始查詢？", year)
	confirmMsg := lineutil.NewConfirmTemplate(
		"確認學年度",
		confirmText,
		lineutil.NewPostbackActionWithDisplayText("哪次不是", "哪次不是", fmt.Sprintf("id:搜尋全系%s%d", bot.PostbackSplitChar, year)),
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
		lineutil.NewPostbackActionWithDisplayText("文法商", fmt.Sprintf("搜尋 %s 學年度文法商學院群", yearStr), fmt.Sprintf("id:文法商%s%s", bot.PostbackSplitChar, yearStr)),
		lineutil.NewPostbackActionWithDisplayText("公社電資", fmt.Sprintf("搜尋 %s 學年度公社電資學院群", yearStr), fmt.Sprintf("id:公社電資%s%s", bot.PostbackSplitChar, yearStr)),
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
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Check cache first
	student, err := h.db.GetStudentByID(ctx, studentID)
	if err != nil {
		log.WithError(err).Error("Failed to query cache")
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("查詢學號時發生問題", sender, "學號 "+studentID),
		}
	}

	if student != nil {
		// Cache hit
		h.metrics.RecordCacheHit(ModuleName)
		log.Debugf("Cache hit for student ID: %s", studentID)
		return h.formatStudentResponse(student)
	}

	// Cache miss - scrape from website
	h.metrics.RecordCacheMiss(ModuleName)
	log.Infof("Cache miss for student ID: %s, scraping...", studentID)

	student, err = ntpu.ScrapeStudentByID(ctx, h.scraper, studentID)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape student ID: %s", studentID)
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("🔍 查無此學號\n\n學號：%s\n請確認學號格式是否正確", studentID), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyDeptCodeAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache
	if err := h.db.SaveStudent(ctx, student); err != nil {
		log.WithError(err).Warn("Failed to save student to cache")
	}

	h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
	return h.formatStudentResponse(student)
}

// handleStudentNameQuery handles student name queries with a 2-tier parallel search strategy:
//
// Search Strategy (parallel execution, merged results):
//
//  1. SQL LIKE (fast path): Direct database LIKE query for exact substrings.
//     Example: "小明" matches "王小明" via SQL LIKE '%小明%'
//
//  2. Fuzzy character-set matching (ALWAYS runs in parallel with SQL LIKE):
//     Loads all cached students and checks if all runes in searchTerm exist in name.
//     Example: "王明" matches "王小明" because all chars exist in the name
//
//     Results from both strategies are merged and deduplicated by student ID.
func (h *Handler) handleStudentNameQuery(ctx context.Context, name string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Step 1: Try SQL LIKE search first (fast path for exact substrings)
	students, err := h.db.SearchStudentsByName(ctx, name)
	if err != nil {
		log.WithError(err).Error("Failed to search students by name")
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("搜尋姓名時發生問題", sender, "學號 "+name),
		}
	}

	// Step 2: ALWAYS try fuzzy character-set matching to find additional results
	// This catches cases like "王明" -> "王小明" that SQL LIKE misses
	allStudents, err := h.db.GetAllStudents(ctx)
	if err == nil && len(allStudents) > 0 {
		for _, s := range allStudents {
			if bot.ContainsAllRunes(s.Name, name) {
				students = append(students, s)
			}
		}
	}

	// Deduplicate results by student ID (SQL LIKE and fuzzy may find overlapping results)
	students = sliceutil.Deduplicate(students, func(s storage.Student) string { return s.ID })

	if len(students) == 0 {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf(config.IDNotFoundWithCutoffHint, name), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Sort by student ID (newest first)
	// Database query already limits to 500 students
	// Track if we hit the limit (likely more results available) - warning added at end
	truncated := len(students) >= MaxStudentsPerSearch

	// Format results - split into multiple messages if needed (100 students per message)
	// Reserve 1 message slot for warning if truncated (LINE API: max 5 messages)
	var messages []messaging_api.MessageInterface
	maxMessages := 5
	if truncated {
		maxMessages = 4 // Reserve 1 slot for warning message at the end
	}

	for i := 0; i < len(students); i += 100 {
		// Respect LINE reply limit
		if len(messages) >= maxMessages {
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

	// Add cache time footer to the last message (use oldest CachedAt)
	if len(messages) > 0 && len(students) > 0 {
		// Collect all CachedAt values to find the minimum
		cachedAts := make([]int64, len(students))
		for i, s := range students {
			cachedAts[i] = s.CachedAt
		}
		minCachedAt := lineutil.MinCachedAt(cachedAts...)
		if minCachedAt > 0 {
			if lastMsg, ok := messages[len(messages)-1].(*messaging_api.TextMessage); ok {
				lastMsg.Text += lineutil.FormatCacheTimeFooter(minCachedAt)
			}
		}
	}

	// Append warning message at the end if results were truncated
	if truncated {
		warningMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 搜尋結果達到上限 %d 筆\n\n可能有更多結果未顯示，建議：\n• 輸入更完整的姓名\n• 使用「學年」功能按年度查詢", MaxStudentsPerSearch),
			sender,
		)
		messages = append(messages, warningMsg)
	}

	// Add Quick Reply to the last message
	lineutil.AddQuickReplyToMessages(messages,
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyDeptCodeAction(),
	)

	return messages
}

// formatStudentResponse formats a student record as a LINE message
// Uses Flex Message for modern, card-based UI
func (h *Handler) formatStudentResponse(student *storage.Student) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Header: Student badge (using standardized component)
	header := lineutil.NewHeaderBadge("🎓", "學生資訊")

	// Hero: Name with NTPU green background (using standardized component)
	hero := lineutil.NewHeroBox(student.Name, "國立臺北大學")

	// Body: Student details using BodyContentBuilder for cleaner code
	body := lineutil.NewBodyContentBuilder()
	body.AddInfoRow("🆔", "學號", student.ID, lineutil.BoldInfoRowStyle())
	body.AddInfoRow("🏫", "系所", student.Department, lineutil.BoldInfoRowStyle())
	body.AddInfoRow("📅", "入學學年", fmt.Sprintf("%d 學年度", student.Year), lineutil.BoldInfoRowStyle())

	// Add cache time hint (unobtrusive, right-aligned)
	if hint := lineutil.NewCacheTimeHint(student.CachedAt); hint != nil {
		body.AddComponent(hint.FlexText)
	}

	// Add data source hint (transparency about data limitations - UX best practice)
	if dataHint := lineutil.NewDataRangeHint(); dataHint != nil {
		body.AddComponent(dataHint.FlexText)
	}

	// Footer: Action buttons (內部指令使用紫色)
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(
			lineutil.NewClipboardAction("📋 複製學號", student.ID),
		).WithStyle("primary").WithColor(lineutil.ColorButtonPrimary).WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(
			lineutil.NewMessageAction("🔍 查詢其他學號", "學號"),
		).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm").FlexButton,
	).WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(
		header,
		hero.FlexBox,
		body.Build(),
		footer,
	)

	// Create Flex Message with sender
	msg := lineutil.NewFlexMessage(fmt.Sprintf("學生資訊 - %s", student.Name), bubble.FlexBubble)
	msg.Sender = sender

	// Add Quick Reply for next actions
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyDeptCodeAction(),
		lineutil.QuickReplyYearAction(),
		lineutil.QuickReplyHelpAction(),
	})

	return []messaging_api.MessageInterface{msg}
}

// Helper functions
// Note: isNumeric has been moved to internal/stringutil package

// parseYear parses a year string (2-4 digits) to ROC year
// Only validates format, not range (range validation is done in handleYearQuery for proper error messages)
func parseYear(yearStr string) (int, error) {
	if len(yearStr) < 2 || len(yearStr) > 4 {
		return 0, errors.New("invalid year length")
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
			lineutil.NewPostbackActionWithDisplayText("📖 人文學院", fmt.Sprintf("搜尋 %s 學年度人文學院", year), fmt.Sprintf("id:人文學院%s%s", bot.PostbackSplitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("⚖️ 法律學院", fmt.Sprintf("搜尋 %s 學年度法律學院", year), fmt.Sprintf("id:法律學院%s%s", bot.PostbackSplitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("💼 商學院", fmt.Sprintf("搜尋 %s 學年度商學院", year), fmt.Sprintf("id:商學院%s%s", bot.PostbackSplitChar, year)),
		}
	} else { // 公社電資
		collegeList = "🏛️ 公共事務：公行、不動、財政\n👥 社科：經濟、社學、社工\n💻 電資：電機、資工、通訊"
		actions = []messaging_api.ActionInterface{
			lineutil.NewPostbackActionWithDisplayText("🏛️ 公共事務學院", fmt.Sprintf("搜尋 %s 學年度公共事務學院", year), fmt.Sprintf("id:公共事務學院%s%s", bot.PostbackSplitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("👥 社會科學學院", fmt.Sprintf("搜尋 %s 學年度社會科學學院", year), fmt.Sprintf("id:社會科學學院%s%s", bot.PostbackSplitChar, year)),
			lineutil.NewPostbackActionWithDisplayText("💻 電機資訊學院", fmt.Sprintf("搜尋 %s 學年度電機資訊學院", year), fmt.Sprintf("id:電機資訊學院%s%s", bot.PostbackSplitChar, year)),
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
		msg := lineutil.NewTextMessageWithConsistentSender("❌ 無效的學院選擇\n\n請重新選擇學年度後操作", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
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
			fmt.Sprintf("id:%s%s%s", deptCode, bot.PostbackSplitChar, year),
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
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		msg := lineutil.NewTextMessageWithConsistentSender("❌ 無效的年份格式\n\n請重新選擇學年度", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	deptName, ok := ntpu.DepartmentNames[deptCode]
	if !ok {
		msg := lineutil.NewTextMessageWithConsistentSender("❌ 無效的系代碼\n\n請重新選擇學年度後操作", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyDeptCodeAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Query students from cache using department name that matches determineDepartment logic
	// determineDepartment returns "法律系" for all 71x codes, and "XX系" for others
	// So we should query using "法律系", "資工系", "社學系", "社工系", etc.
	var queryDeptName string
	if ntpu.IsLawDepartment(deptCode) {
		// All law school departments (712/714/716) are stored as "法律系"
		queryDeptName = "法律系"
	} else {
		// For other departments, add "系" suffix
		queryDeptName = deptName + "系"
	}

	students, err := h.db.GetStudentsByDepartment(ctx, queryDeptName, year)
	if err != nil {
		log.WithError(err).Error("Failed to search students by year and department")
		msg := lineutil.ErrorMessageWithDetailAndSender("查詢學生名單時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				lineutil.QuickReplyYearAction(),
				lineutil.QuickReplyDeptCodeAction(),
				lineutil.QuickReplyHelpAction(),
			})
		}
		return []messaging_api.MessageInterface{msg}
	}

	// If not found in cache, try scraping
	if len(students) == 0 {
		log.Infof("Cache miss for department selection: %d %s, scraping...", year, deptCode)
		h.metrics.RecordCacheMiss(ModuleName)
		startTime := time.Now()

		scrapedStudents, err := ntpu.ScrapeStudentsByYear(ctx, h.scraper, year, deptCode)
		if err != nil {
			log.WithError(err).Errorf("Failed to scrape students for year %d dept %s", year, deptCode)
			h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
			msg := lineutil.ErrorMessageWithDetailAndSender("查詢學生名單時發生問題，可能是學校網站暫時無法存取", sender)
			if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
				textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
					lineutil.QuickReplyRetryAction(fmt.Sprintf("學年 %d", year)),
					lineutil.QuickReplyYearAction(),
					lineutil.QuickReplyHelpAction(),
				})
			}
			return []messaging_api.MessageInterface{msg}
		}

		if len(scrapedStudents) > 0 {
			h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
			// Save to cache and convert to value slice
			for _, s := range scrapedStudents {
				if err := h.db.SaveStudent(ctx, s); err != nil {
					log.WithError(err).Warn("Failed to save student to cache")
				}
				students = append(students, *s)
			}
		} else {
			h.metrics.RecordScraperRequest(ModuleName, "not_found", time.Since(startTime).Seconds())
		}
	} else {
		h.metrics.RecordCacheHit(ModuleName)
	}

	if len(students) == 0 {
		departmentType := "系"
		if ntpu.IsLawDepartment(deptCode) {
			departmentType = "組"
		}
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("🤔 %d 學年度%s%s好像沒有人耶", year, deptName, departmentType), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Format student list
	var builder strings.Builder
	departmentType := "系"
	displayName := deptName
	if ntpu.IsLawDepartment(deptCode) {
		departmentType = "組"
		// For law, use "法律系XX組" format
		displayName = "法律系" + deptName
	}

	builder.WriteString(fmt.Sprintf("%d學年度%s%s學生名單：\n\n", year, displayName, departmentType))

	// Collect CachedAt values for time footer
	cachedAts := make([]int64, len(students))
	for i, student := range students {
		builder.WriteString(fmt.Sprintf("%s  %s\n", student.ID, student.Name))
		cachedAts[i] = student.CachedAt
	}

	builder.WriteString(fmt.Sprintf("\n%d學年度%s%s共有%d位學生", year, displayName, departmentType, len(students)))

	// Add cache time footer
	minCachedAt := lineutil.MinCachedAt(cachedAts...)
	builder.WriteString(lineutil.FormatCacheTimeFooter(minCachedAt))

	// Note: sender was already created at the start of handleDepartmentSelection, reuse it
	msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyYearAction(),
		lineutil.QuickReplyDeptCodeAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}
