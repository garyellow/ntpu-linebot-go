// Package id implements the student ID lookup module for the LINE bot.
// It handles student searches by name, department, and academic year.
package id

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
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
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles student ID related queries using Pattern-Action Table architecture.
// Both CanHandle() and HandleMessage() share the same matchers list, which structurally
// guarantees routing consistency and eliminates the possibility of divergence.
//
// Pattern priority (1=highest): AllDeptCode → StudentID → DeptCode → DeptName → Year → Student
type Handler struct {
	db             *storage.DB
	scraper        *scraper.Client
	metrics        *metrics.Metrics
	logger         *logger.Logger
	stickerManager *sticker.Manager

	// matchers contains all pattern-handler pairs sorted by priority.
	// Shared by CanHandle and HandleMessage for consistent routing.
	matchers []PatternMatcher
}

// Name returns the module name
func (h *Handler) Name() string {
	return ModuleName
}

// ID handler constants.
const (
	ModuleName = "id" // Module identifier for registration
	senderName = "學號小幫手"
)

// Pattern priorities (lower = higher priority).
// IMPORTANT: More specific patterns (e.g., "系代碼") must have higher priority
// than less specific ones (e.g., "系") to prevent incorrect matches.
const (
	PriorityAllDeptCode = 1 // Exact match: "所有系代碼"
	PriorityStudentID   = 2 // 8-9 digit numeric student ID
	PriorityDepartment  = 3 // Department query (name or code) - Higher than Year
	PriorityYear        = 4 // Year query (學年)
	PriorityStudent     = 5 // Student name/ID query (學號, 學生)
)

// PatternHandler processes a matched pattern and returns LINE messages.
// Parameters: context, original text, regex match groups (matches[0] = full match).
//
// Contract: When invoked (pattern matched), MUST return at least one user-facing message.
// Even if processing fails or validation errors occur, return error/help messages instead
// of nil/empty slice to preserve CanHandle/HandleMessage consistency guarantee.
type PatternHandler func(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface

// PatternMatcher represents a pattern-action pair sorted by priority.
type PatternMatcher struct {
	pattern   *regexp.Regexp
	priority  int
	handler   PatternHandler
	name      string            // For logging
	matchFunc func(string) bool // Optional custom match function (for non-regex patterns)
}

// Keyword definitions for bot.BuildKeywordRegex (case-insensitive, ^-anchored).
var (
	validStudentKeywords = []string{
		"學號", "學生", "姓名",
		"student", "id", // English keywords
	}
	validDepartmentKeywords = []string{
		"系代碼", "系所代碼", "科系代碼", "系編號", "系所編號", "科系編號",
		"系所", "科系", "系名", "系所名", "科系名", "系所名稱", "科系名稱",
		"系", "所", // standalone, highly natural department keywords (rely on matcher priority to reduce false positives)
		"dep", "department", "depCode", "departmentCode", // English keywords
	}
	validYearKeywords = []string{
		"學年", "年份", "年度", "學年度", "入學年", "入學學年", "入學年度",
		"year", // English keyword
	}

	studentRegex    = bot.BuildKeywordRegex(validStudentKeywords)
	departmentRegex = bot.BuildKeywordRegex(validDepartmentKeywords)
	yearRegex       = bot.BuildKeywordRegex(validYearKeywords)
	allDeptCodeText = "所有系代碼"
)

// NewHandler creates a new ID handler with required dependencies.
// All parameters are mandatory for proper handler operation.
// Initializes and sorts matchers by priority during construction.
func NewHandler(
	db *storage.DB,
	scraper *scraper.Client,
	metrics *metrics.Metrics,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
) *Handler {
	h := &Handler{
		db:             db,
		scraper:        scraper,
		metrics:        metrics,
		logger:         logger,
		stickerManager: stickerManager,
	}

	// Initialize Pattern-Action Table
	h.initializeMatchers()

	return h
}

// initializeMatchers sets up the Pattern-Action Table.
// All pattern matching logic is defined here in one place.
// Matchers are automatically sorted by priority after initialization.
func (h *Handler) initializeMatchers() {
	h.matchers = []PatternMatcher{
		{
			// Exact match: "所有系代碼"
			pattern:  nil, // Uses matchFunc instead
			priority: PriorityAllDeptCode,
			handler:  h.handleAllDeptCodePattern,
			name:     "AllDeptCode",
			matchFunc: func(text string) bool {
				return text == allDeptCodeText
			},
		},
		{
			// 8-9 digit numeric student ID
			pattern:  nil, // Uses matchFunc instead
			priority: PriorityStudentID,
			handler:  h.handleStudentIDPattern,
			name:     "StudentID",
			matchFunc: func(text string) bool {
				return len(text) >= 8 && len(text) <= 9 && stringutil.IsNumeric(text)
			},
		},
		{
			// Department query (name or code)
			pattern:  departmentRegex,
			priority: PriorityDepartment,
			handler:  h.handleDepartmentPattern,
			name:     "Department",
		},
		{
			// Year query (學年)
			pattern:  yearRegex,
			priority: PriorityYear,
			handler:  h.handleYearPattern,
			name:     "Year",
		},
		{
			// Student name/ID query (學號, 學生)
			pattern:  studentRegex,
			priority: PriorityStudent,
			handler:  h.handleStudentPattern,
			name:     "Student",
		},
	}

	// Sort by priority (lower number = higher priority)
	slices.SortFunc(h.matchers, func(a, b PatternMatcher) int {
		return a.priority - b.priority
	})
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
//   - "department": requires "department" param, calls handleUnifiedDepartmentQuery
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

		return h.handleUnifiedDepartmentQuery(department), nil

	default:
		return nil, fmt.Errorf("%w: %s", domerrors.ErrUnknownIntent, intent)
	}
}

// findMatcher returns the first matching pattern or nil.
// Used by both CanHandle and HandleMessage for consistent routing.
func (h *Handler) findMatcher(text string) *PatternMatcher {
	for i := range h.matchers {
		m := &h.matchers[i]
		// Use custom match function if provided, otherwise use regex
		if m.matchFunc != nil {
			if m.matchFunc(text) {
				return m
			}
		} else if m.pattern != nil && m.pattern.MatchString(text) {
			return m
		}
	}
	return nil
}

// CanHandle returns true if any pattern matches (consistent with HandleMessage).
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)
	return h.findMatcher(text) != nil
}

// HandleMessage finds the matching pattern and executes its handler.
// Returns empty slice if no pattern matches (fallback to NLU).
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	text = strings.TrimSpace(text)

	log.Debugf("Handling ID message: %s", text)

	// Find matching pattern
	matcher := h.findMatcher(text)
	if matcher == nil {
		return []messaging_api.MessageInterface{}
	}

	// Extract regex match groups (empty for non-regex matchers)
	var matches []string
	if matcher.pattern != nil {
		matches = matcher.pattern.FindStringSubmatch(text)
	} else {
		matches = []string{text} // For custom matchers, just pass the text
	}

	log.Debugf("Pattern matched: %s (priority %d)", matcher.name, matcher.priority)

	// Call handler - must return non-empty per PatternHandler contract
	result := matcher.handler(ctx, text, matches)

	// Defensive check: handlers should never return nil/empty when pattern matched
	if len(result) == 0 {
		log.Errorf("Handler %s violated contract: returned empty for matched pattern", matcher.name)
		// Return generic error to user
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 抱歉，處理您的查詢時發生問題\n\n請稍後再試或輸入「說明」查看使用方式。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
		return []messaging_api.MessageInterface{msg}
	}

	return result
}

// ================================================
// Pattern handler adapters - implement PatternHandler contract.
// Must return non-empty messages when invoked (pattern matched).
// ================================================

// handleAllDeptCodePattern handles "所有系代碼" exact match.
func (h *Handler) handleAllDeptCodePattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	return h.handleAllDepartmentCodes()
}

// handleStudentIDPattern handles 8-9 digit numeric student ID.
func (h *Handler) handleStudentIDPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	return h.handleStudentIDQuery(ctx, text)
}

// handleDepartmentPattern handles all department-related queries (name or code).
func (h *Handler) handleDepartmentPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	match := matches[0] // The matched keyword
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm == "" {
		// Provide guidance message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"🔍 查詢系所資訊\n\n請輸入系名或系代碼：\n例如：「系 資工」或「系代碼 85」\n\n💡 提示：輸入「所有系代碼」查看完整對照表",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyDeptCodeAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	return h.handleUnifiedDepartmentQuery(searchTerm)
}

// handleYearPattern handles year query (學年 XXX).
func (h *Handler) handleYearPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	match := matches[0] // The matched keyword
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm != "" {
		return h.handleYearQuery(searchTerm)
	}

	// No year provided - show guidance message
	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender(
		"📅 按學年度查詢學生\n\n請輸入學年度進行查詢\n例如：學年 112、學年 110\n\n📋 查詢流程：\n1️⃣ 選擇學院群（文法商/公社電資）\n2️⃣ 選擇學院\n3️⃣ 選擇系所\n4️⃣ 查看該系所所有學生\n\n⚠️ 僅提供 94-112 學年度完整資料（113 年極不完整、114 年起無資料）",
		sender,
	)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		// Use IDDataYearEnd from config to ensure we don't suggest years that have no data
		{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", config.IDDataYearEnd), fmt.Sprintf("學年 %d", config.IDDataYearEnd))},
		{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", config.IDDataYearEnd-1), fmt.Sprintf("學年 %d", config.IDDataYearEnd-1))},
		{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", config.IDDataYearEnd-2), fmt.Sprintf("學年 %d", config.IDDataYearEnd-2))},
	})
	return []messaging_api.MessageInterface{msg}
}

// handleStudentPattern handles student name/ID query (學號 XXX).
func (h *Handler) handleStudentPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	match := matches[0] // The matched keyword
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm == "" {
		// If no search term provided, give helpful message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"🎓 請在關鍵字後輸入查詢內容\n\n例如：\n• 學號 小明\n• 學號 412345678\n\n💡 提示：也可直接輸入 8-9 位學號",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// If search term is numeric 8-9 digits, treat as student ID
	if stringutil.IsNumeric(searchTerm) && (len(searchTerm) == 8 || len(searchTerm) == 9) {
		return h.handleStudentIDQuery(ctx, searchTerm)
	}

	return h.handleStudentNameQuery(ctx, searchTerm)
}

// HandlePostback handles postback events for the ID module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	log.Infof("Handling ID postback: %s", data)

	// Strip module prefix if present (registry passes original data)
	data = strings.TrimPrefix(data, "id:")

	// Handle "兇" (easter egg)
	if data == "兇" {
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
				msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyStudentNav())
				return []messaging_api.MessageInterface{msg}
			}

			// Verify department code contains only digits
			if _, err := strconv.Atoi(action); err != nil {
				sender := lineutil.GetSender(senderName, h.stickerManager)
				msg := lineutil.NewTextMessageWithConsistentSender(
					"❌ 無效的系代碼格式\n\n系代碼應為 1-3 位數字",
					sender,
				)
				msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyStudentNav())
				return []messaging_api.MessageInterface{msg}
			}

			if _, ok := ntpu.DepartmentNames[action]; ok {
				return h.handleDepartmentSelection(ctx, action, year)
			}
		}
	}

	return []messaging_api.MessageInterface{}
}

// handleAllDepartmentCodes returns all undergraduate department codes organized by college.
// Includes a tip for searching graduate program codes.
func (h *Handler) handleAllDepartmentCodes() []messaging_api.MessageInterface {
	var builder strings.Builder
	builder.WriteString("📋 大學部系代碼一覽\n")

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

	builder.WriteString("\n\n🎓 查詢碩博士班\n輸入「系名 XXX」（如：系名 法律）可搜尋所有學制")

	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyYearAction(),
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// RIP image URL for LMS 2.0 deprecation (year 114+)
const lmsDeprecatedImageURL = "https://raw.githubusercontent.com/garyellow/ntpu-linebot-go/main/assets/rip.png"

// buildLMSDeprecatedResponse builds a response for year 114+ (NO data at all).
// Returns text message + RIP image with quick reply.
func (h *Handler) buildLMSDeprecatedResponse(message string, sender *messaging_api.Sender, quickReplyItems []lineutil.QuickReplyItem) []messaging_api.MessageInterface {
	textMsg := lineutil.NewTextMessageWithConsistentSender(message, sender)

	// Image message with quick reply (must be on last message)
	imgMsg := &messaging_api.ImageMessage{
		OriginalContentUrl: lmsDeprecatedImageURL,
		PreviewImageUrl:    lmsDeprecatedImageURL,
	}
	imgMsg.Sender = sender
	imgMsg.QuickReply = lineutil.NewQuickReply(quickReplyItems)

	return []messaging_api.MessageInterface{textMsg, imgMsg}
}

// handleDepartmentNameQuery handles department name to code queries with fuzzy matching.
// Searches across all degree types: undergraduate, master's, and PhD programs.
// Uses ContainsAllRunes for character-set matching:
//
//	Example: "資工" matches "資訊工程學系" and "資訊工程學系碩士班"
func (h *Handler) handleDepartmentNameQuery(deptName string) []messaging_api.MessageInterface {
	deptName = strings.TrimSuffix(deptName, "系")
	deptName = strings.TrimSuffix(deptName, "班")
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Define search sources with degree labels
	type deptMatch struct {
		name   string
		code   string
		degree string // 大學部, 碩士班, 博士班
	}
	var matches []deptMatch

	// Helper to add matches from a map using character-set matching
	addMatches := func(m map[string]string, degree string) {
		for fullName, code := range m {
			if stringutil.ContainsAllRunes(fullName, deptName) {
				matches = append(matches, deptMatch{fullName, code, degree})
			}
		}
	}

	// Fuzzy search across all degree types
	addMatches(ntpu.FullDepartmentCodes, "大學部")
	addMatches(ntpu.MasterDepartmentCodes, "碩士班")
	addMatches(ntpu.PhDDepartmentCodes, "博士班")

	// If exactly one match, return it directly
	if len(matches) == 1 {
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍「%s」→ %s（%s）\n\n系代碼是：%s", deptName, matches[0].name, matches[0].degree, matches[0].code),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyStudentNav())
		return []messaging_api.MessageInterface{msg}
	}

	// If multiple matches, group by degree and show all
	if len(matches) > 1 {
		var builder strings.Builder
		fmt.Fprintf(&builder, "🔍「%s」找到 %d 個符合的系所：\n", deptName, len(matches))

		// Group by degree for clearer display
		degreeOrder := []string{"大學部", "碩士班", "博士班"}
		for _, deg := range degreeOrder {
			var degMatches []deptMatch
			for _, m := range matches {
				if m.degree == deg {
					degMatches = append(degMatches, m)
				}
			}
			if len(degMatches) > 0 {
				fmt.Fprintf(&builder, "\n🎓 %s\n", deg)
				for _, m := range degMatches {
					builder.WriteString(fmt.Sprintf("  • %s → %s\n", m.name, m.code))
				}
			}
		}
		msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyStudentNav())
		return []messaging_api.MessageInterface{msg}
	}

	msg := lineutil.NewTextMessageWithConsistentSender("🔍 查無該系所\n\n請輸入正確的系名\n例如：資工、法律、企管", sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyDeptCodeAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// handleUnifiedDepartmentQuery handles both code (numeric) and name (text) queries for departments.
// It acts as a smart router to unify the search logic.
func (h *Handler) handleUnifiedDepartmentQuery(query string) []messaging_api.MessageInterface {
	if stringutil.IsNumeric(query) {
		return h.handleDepartmentCodeQuery(query)
	}
	return h.handleDepartmentNameQuery(query)
}

// handleDepartmentCodeQuery handles department code to name queries.
// Searches across all degree types: undergraduate, master's, and PhD programs.
func (h *Handler) handleDepartmentCodeQuery(code string) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Collect all matches across degree types
	type codeMatch struct {
		name   string
		degree string
	}
	var matches []codeMatch

	// Check undergraduate names
	if name, ok := ntpu.DepartmentNames[code]; ok {
		matches = append(matches, codeMatch{name + "系", "大學部"})
	}

	// Check master's program names
	if name, ok := ntpu.MasterDepartmentNames[code]; ok {
		matches = append(matches, codeMatch{name, "碩士班"})
	}

	// Check PhD program names
	if name, ok := ntpu.PhDDepartmentNames[code]; ok {
		matches = append(matches, codeMatch{name, "博士班"})
	}

	// If exactly one match, return it directly
	if len(matches) == 1 {
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🎓 系代碼 %s 是：%s（%s）", code, matches[0].name, matches[0].degree),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyStudentNav())
		return []messaging_api.MessageInterface{msg}
	}

	// If multiple matches (same code used across degrees), show all
	if len(matches) > 1 {
		var builder strings.Builder
		fmt.Fprintf(&builder, "🔍 系代碼 %s 對應多個系所：\n", code)
		for _, m := range matches {
			builder.WriteString(fmt.Sprintf("\n• %s（%s）", m.name, m.degree))
		}
		msg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyStudentNav())
		return []messaging_api.MessageInterface{msg}
	}

	msg := lineutil.NewTextMessageWithConsistentSender("🔍 查無該系代碼\n\n請輸入正確的系代碼\n例如：85（資工系）、31（企管碩/博）", sender)
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
			{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", config.IDDataYearEnd), fmt.Sprintf("學年 %d", config.IDDataYearEnd))},
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

	// 2. Check for year 114+ (NO data at all) - reject with RIP image
	if year > config.IDDataYearEnd+1 {
		return h.buildLMSDeprecatedResponse(
			config.IDLMSDeprecatedMessage,
			sender,
			[]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", config.IDDataYearEnd), fmt.Sprintf("學年 %d", config.IDDataYearEnd))},
				lineutil.QuickReplyYearAction(),
				lineutil.QuickReplyHelpAction(),
			},
		)
	}

	// 3. Check for year 113 (sparse data) - warn but allow query
	if year == config.IDDataYearEnd+1 {
		// Show warning but allow user to proceed
		msg := lineutil.NewTextMessageWithConsistentSender(config.ID113YearWarningMessage, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("繼續查詢 ➡️", fmt.Sprintf("搜尋全系%s%d", bot.PostbackSplitChar, year))},
			{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 改查 %d 學年度", config.IDDataYearEnd), fmt.Sprintf("學年 %d", config.IDDataYearEnd))},
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// 4. Check if year is before NTPU was founded
	if year < config.NTPUFoundedYear {
		msg := lineutil.NewTextMessageWithConsistentSender(config.IDYearBeforeNTPUMessage, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📅 查詢 94 學年度", "學年 94")},
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// 5. Check if year is before LMS has complete data (90-93 have sparse data)
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

	// Validate student ID format (8-9 digits)
	if len(studentID) < 8 || len(studentID) > 9 || !stringutil.IsNumeric(studentID) {
		msg := lineutil.NewTextMessageWithConsistentSender(
			"🔍 學號格式不正確\n\n學號應為 8-9 位數字\n例如：412345678",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Extract year for validation (keep in scope for later error handling)
	year := ntpu.ExtractYear(studentID)

	// Check year before querying - reject 114+ immediately
	if year > config.IDDataYearEnd+1 {
		return h.buildLMSDeprecatedResponse(
			config.IDLMSDeprecatedMessage,
			sender,
			[]lineutil.QuickReplyItem{
				lineutil.QuickReplyYearAction(),
				lineutil.QuickReplyStudentAction(),
				lineutil.QuickReplyHelpAction(),
			},
		)
	}

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

		// Check if the student ID belongs to year 113 (incomplete data)
		// Year 114+ would have been rejected earlier, so this is only for 113
		if year == config.IDDataYearEnd+1 {
			msg := lineutil.NewTextMessageWithConsistentSender(
				fmt.Sprintf("🔍 查無學號 %s 的資料\n\n"+
					"⚠️ 113 學年度資料極不完整\n"+
					"僅極少數手動建立 LMS 2.0 帳號的學生有資料。\n\n"+
					"📅 完整資料範圍：94-112 學年度",
					studentID),
				sender,
			)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				lineutil.QuickReplyYearAction(),
				lineutil.QuickReplyStudentAction(),
				lineutil.QuickReplyHelpAction(),
			})
			return []messaging_api.MessageInterface{msg}
		}

		// Regular not found message
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

// handleStudentNameQuery handles student name queries with application-layer character-set matching.
//
// Search Strategy:
//
//  1. Loads all cached students from SQLite (fast with WAL mode).
//  2. Filters using stringutil.ContainsAllRunes() for non-contiguous character matching.
//     Example: "王明" and "明王" both match "王小明" because all characters exist in the name.
//  3. Returns both the total count of matches and the first 400 results.
//
// This approach supports flexible matching that SQL LIKE cannot provide, such as:
// - Non-contiguous characters: "王明" → "王小明"
// - Reversed order: "明王" → "王小明"
// - Character-set membership: "資工" → "資訊工程"
func (h *Handler) handleStudentNameQuery(ctx context.Context, name string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Search using character-set matching (application layer)
	// Supports non-contiguous character matching: "王明" matches "王小明"
	// Returns total count and limited results (up to 400)
	result, err := h.db.SearchStudentsByName(ctx, name)
	if err != nil {
		log.WithError(err).Error("Failed to search students by name")
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("搜尋姓名時發生問題", sender, "學號 "+name),
		}
	}
	students := result.Students
	totalCount := result.TotalCount

	if len(students) == 0 {
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf(config.IDNotFoundWithCutoffHint, name), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyYearAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Character-set matching strategy (application layer):
	// 1. Search all students using ContainsAllRunes (supports "王明" → "王小明")
	// 2. Get accurate total count of all matches
	// 3. Return first 400 students (sorted by year DESC, id DESC)
	// 4. Display all returned students (4 messages × 100 students), reserve 5th message for meta info

	// Format student list - up to 4 messages (100 students per message)
	// 5th message is always reserved for disclaimer and optional warning
	var messages []messaging_api.MessageInterface
	const maxListMessages = 4                                       // Max messages for student list
	const studentsPerMessage = 100                                  // Students per message
	const maxDisplayStudents = maxListMessages * studentsPerMessage // 400 students max

	displayCount := min(len(students), maxDisplayStudents)

	for i := 0; i < displayCount; i += studentsPerMessage {
		end := min(i+studentsPerMessage, displayCount)

		var builder strings.Builder
		fmt.Fprintf(&builder, "📋 搜尋結果（第 %d-%d 筆，共 %d 筆）\n\n", i+1, end, totalCount)

		for j := i; j < end; j++ {
			student := students[j]
			fmt.Fprintf(&builder, "%s  %s  %d  %s\n",
				student.ID, student.Name, student.Year, student.Department)
		}

		listMsg := lineutil.NewTextMessageWithConsistentSender(builder.String(), sender)
		messages = append(messages, listMsg)
	}

	// Add cache time footer to the last student list message
	if len(messages) > 0 && displayCount > 0 {
		// Collect CachedAt values from displayed students only
		cachedAts := make([]int64, displayCount)
		for i := range displayCount {
			cachedAts[i] = students[i].CachedAt
		}
		minCachedAt := lineutil.MinCachedAt(cachedAts...)
		if minCachedAt > 0 {
			if lastMsg, ok := messages[len(messages)-1].(*messaging_api.TextMessage); ok {
				lastMsg.Text += lineutil.FormatCacheTimeFooter(minCachedAt)
			}
		}
	}

	// 5th message: Always add disclaimer, with optional warning if results exceed display limit
	var infoBuilder strings.Builder

	// Add warning if we have more results than displayed
	if totalCount > maxDisplayStudents {
		infoBuilder.WriteString("⚠️ 搜尋結果達到顯示上限\n\n")
		fmt.Fprintf(&infoBuilder, "已顯示前 %d 筆結果（共找到 %d 筆），建議：\n", maxDisplayStudents, totalCount)
		infoBuilder.WriteString("• 輸入更完整的姓名\n")
		infoBuilder.WriteString("• 使用「學年」功能按年度查詢\n\n")
		infoBuilder.WriteString("────────────────\n\n")
	}

	// Always add department inference disclaimer
	infoBuilder.WriteString("ℹ️ 系所資訊說明\n")
	infoBuilder.WriteString("系所資訊由學號推測，若有轉系之類的情況可能與實際不符。\n\n")
	infoBuilder.WriteString("📊 姓名查詢範圍\n")
	infoBuilder.WriteString("• 大學部/碩博士班：101-112 學年度（完整）\n")
	infoBuilder.WriteString("• 113 學年度：資料極不完整\n\n")
	infoBuilder.WriteString("💡 若找不到學生，可使用「學年」功能按年度查詢")

	infoMsg := lineutil.NewTextMessageWithConsistentSender(infoBuilder.String(), sender)
	messages = append(messages, infoMsg)

	// Add Quick Reply to the last message (5th message)
	lineutil.AddQuickReplyToMessages(messages,
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyDeptCodeAction(),
	)

	return messages
}

// formatStudentResponse formats a student record as a LINE message
// Uses Flex Message for modern, card-based UI with colored header (consistent with Course/Contact modules)
func (h *Handler) formatStudentResponse(student *storage.Student) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Header: Student name with colored background (using standardized colored header component)
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: student.Name,
		Color: lineutil.ColorHeaderStudent, // Purple color for student module
	})

	// Body: Student details using BodyContentBuilder for cleaner code
	body := lineutil.NewBodyContentBuilder()

	// First row: NTPU label (consistent with course/contact modules)
	body.AddComponent(lineutil.NewBodyLabel(lineutil.BodyLabelInfo{
		Emoji: "🎓",
		Label: "國立臺北大學",
		Color: lineutil.ColorHeaderStudent, // Purple color matching header
	}).FlexBox)

	// 學號 info - first row (no separator so it flows directly after the label)
	firstInfoRow := lineutil.NewInfoRow("🆔", "學號", student.ID, lineutil.BoldInfoRowStyle())
	body.AddComponent(firstInfoRow.FlexBox)
	body.AddInfoRow("🏫", "系所", student.Department, lineutil.BoldInfoRowStyle())
	body.AddInfoRow("📅", "入學學年", fmt.Sprintf("%d 學年度", student.Year), lineutil.BoldInfoRowStyle())

	// Add department inference note (transparency about data limitations)
	body.AddComponent(lineutil.NewFlexText("⚠️ 系所資訊由學號推測，若有轉系之類的情況可能與實際不符").
		WithSize("xs").
		WithColor(lineutil.ColorNote).
		WithWrap(true).
		WithMargin("md").FlexText)

	// Add name search scope note (姓名查詢限制說明)
	body.AddComponent(lineutil.NewFlexText(
		"📊 姓名查詢涵蓋大學部/碩博士班 101-112 學年度（完整）、113 學年度（極不完整）。").
		WithSize("xs").
		WithColor(lineutil.ColorNote).
		WithWrap(true).
		WithMargin("sm").FlexText)

	// Add cache time hint (unobtrusive, right-aligned)
	if hint := lineutil.NewCacheTimeHint(student.CachedAt); hint != nil {
		body.AddComponent(hint.FlexText)
	}

	// Add data source hint (transparency about data limitations)
	if dataHint := lineutil.NewDataRangeHint(); dataHint != nil {
		body.AddComponent(dataHint.FlexText)
	}

	// Footer: Action buttons (內部指令使用紫色)
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(
			lineutil.NewClipboardAction("📋 複製學號", student.ID),
		).WithStyle("primary").WithColor(lineutil.ColorButtonAction).WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(
			lineutil.NewMessageAction("🔍 查詢其他學號", "學號"),
		).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm").FlexButton,
	).WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(
		header,
		nil, // No hero - title is in colored header
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

		scrapedStudents, err := ntpu.ScrapeStudentsByYear(ctx, h.scraper, year, deptCode, ntpu.StudentTypeUndergrad)
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
		// Special message for year 113 (incomplete data)
		// Year 114+ would have been rejected in handleYearQuery
		if year == config.IDDataYearEnd+1 {
			msg := lineutil.NewTextMessageWithConsistentSender(
				fmt.Sprintf(config.ID113YearEmptyMessage, deptName+departmentType),
				sender,
			)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction(fmt.Sprintf("📅 查詢 %d 學年度", config.IDDataYearEnd), fmt.Sprintf("學年 %d", config.IDDataYearEnd))},
				lineutil.QuickReplyYearAction(),
				lineutil.QuickReplyHelpAction(),
			})
			return []messaging_api.MessageInterface{msg}
		}
		// Regular "no students" message for other years
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
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyStudentNav())
	return []messaging_api.MessageInterface{msg}
}
