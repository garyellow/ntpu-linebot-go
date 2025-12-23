// Package program implements the academic program/certificate module for the LINE bot.
// It handles queries for academic programs (學程) including listing all programs,
// searching programs by name, and viewing program courses.
package program

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles program-related queries using Pattern-Action Table architecture.
// Both CanHandle() and HandleMessage() share the same matchers list, which structurally
// guarantees routing consistency and eliminates the possibility of divergence.
//
// Pattern priority (1=highest): PostbackViewCourses → List → Search
type Handler struct {
	db             *storage.DB
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

// Module constants for program handler.
const (
	ModuleName               = "program" // Module identifier for registration
	senderName               = "學程小幫手"
	MaxProgramsPerSearch     = 30 // 3 carousels @ 10 bubbles (LINE max: 5 messages)
	MaxCoursesPerProgram     = 40 // 4 carousels @ 10 bubbles
	MaxTitleDisplayChars     = 50 // Truncation limit for program titles
	PostbackPrefix           = "program:"
	PostbackViewCoursesLabel = "查看課程"
)

// Pattern priorities (lower = higher).
const (
	PriorityList   = 1 // List all programs (學程列表)
	PrioritySearch = 2 // Search program (學程 XX)
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
	pattern  *regexp.Regexp
	priority int
	handler  PatternHandler
	name     string // For logging
}

// Keyword definitions for bot.BuildKeywordRegex (case-insensitive, ^-anchored).
var (
	// validListProgramKeywords: list all programs
	validListProgramKeywords = []string{
		"學程列表", "學程清單", "所有學程", "全部學程",
		"program list", "programs",
	}

	// validSearchProgramKeywords: search program by name
	validSearchProgramKeywords = []string{
		"學程",
		"program",
	}

	listProgramRegex   = bot.BuildKeywordRegex(validListProgramKeywords)
	searchProgramRegex = bot.BuildKeywordRegex(validSearchProgramKeywords)
)

// NewHandler creates a new program handler.
// Initializes and sorts matchers by priority during construction.
func NewHandler(
	db *storage.DB,
	metrics *metrics.Metrics,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
) *Handler {
	h := &Handler{
		db:             db,
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
			pattern:  listProgramRegex,
			priority: PriorityList,
			handler:  h.handleListPattern,
			name:     "List",
		},
		{
			pattern:  searchProgramRegex,
			priority: PrioritySearch,
			handler:  h.handleSearchPattern,
			name:     "Search",
		},
	}

	// Sort by priority (lower number = higher priority)
	slices.SortFunc(h.matchers, func(a, b PatternMatcher) int {
		return a.priority - b.priority
	})
}

// Intent names for NLU dispatcher
const (
	IntentList    = "list"    // List all programs
	IntentSearch  = "search"  // Search program by name
	IntentCourses = "courses" // Get courses for a program
)

// DispatchIntent handles NLU-parsed intents.
// Intents: "list" (no params), "search" (query), "courses" (programName).
// Returns error if intent unknown or required params missing.
func (h *Handler) DispatchIntent(ctx context.Context, intent string, params map[string]string) ([]messaging_api.MessageInterface, error) {
	// Validate parameters first (before logging) to support testing with nil dependencies
	switch intent {
	case IntentList:
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debug("Dispatching program intent: list")
		}
		return h.handleProgramList(ctx), nil

	case IntentSearch:
		query, ok := params["query"]
		if !ok || query == "" {
			return nil, fmt.Errorf("%w: query", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching program intent: search, query: %s", query)
		}
		return h.handleProgramSearch(ctx, query), nil

	case IntentCourses:
		programName, ok := params["programName"]
		if !ok || programName == "" {
			return nil, fmt.Errorf("%w: programName", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching program intent: courses, programName: %s", programName)
		}
		return h.handleProgramCourses(ctx, programName), nil

	default:
		return nil, fmt.Errorf("%w: %s", domerrors.ErrUnknownIntent, intent)
	}
}

// findMatcher returns the first matching pattern or nil.
// Used by both CanHandle and HandleMessage for consistent routing.
func (h *Handler) findMatcher(text string) *PatternMatcher {
	text = strings.TrimSpace(text)
	for i := range h.matchers {
		if h.matchers[i].pattern.MatchString(text) {
			return &h.matchers[i]
		}
	}
	return nil
}

// CanHandle returns true if any pattern matches (consistent with HandleMessage).
func (h *Handler) CanHandle(text string) bool {
	return h.findMatcher(text) != nil
}

// HandleMessage finds the matching pattern and executes its handler.
// Returns empty slice if no pattern matches (fallback to NLU).
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	text = strings.TrimSpace(text)

	log.Debugf("Handling program message: %s", text)

	// Find matching pattern
	matcher := h.findMatcher(text)
	if matcher == nil {
		return []messaging_api.MessageInterface{}
	}

	// Extract regex match groups
	matches := matcher.pattern.FindStringSubmatch(text)
	// Defensive check: MatchString succeeded but FindStringSubmatch may return empty
	if len(matches) == 0 {
		log.Warnf("Pattern %s matched but FindStringSubmatch returned empty", matcher.name)
		return []messaging_api.MessageInterface{}
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

// HandlePostback processes postback events from the program module.
// Postback format: "program:{action}:{data}" where action is "courses".
// Returns nil if postback is not for this module.
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)

	// Check if postback is for this module
	if !strings.HasPrefix(data, PostbackPrefix) {
		return nil
	}

	// Extract action and data
	parts := strings.SplitN(data[len(PostbackPrefix):], bot.PostbackSplitChar, 2)
	if len(parts) < 2 {
		log.Warnf("Invalid postback format: %s", data)
		return nil
	}

	action := parts[0]
	actionData := parts[1]

	log.Debugf("Processing postback: action=%s, data=%s", action, actionData)

	switch action {
	case "courses":
		return h.handleProgramCourses(ctx, actionData)
	case "course_programs":
		// Show all programs for a given course UID
		return h.handleCourseProgramsList(ctx, actionData)
	default:
		log.Warnf("Unknown postback action: %s", action)
		return nil
	}
}

// CanHandlePostback checks if the postback data is for this module.
func (h *Handler) CanHandlePostback(data string) bool {
	return strings.HasPrefix(data, PostbackPrefix)
}

// Pattern handler adapters - implement PatternHandler contract.
// Must return non-empty messages when invoked (pattern matched).

// handleListPattern handles program list queries.
func (h *Handler) handleListPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	return h.handleProgramList(ctx)
}

// handleSearchPattern extracts search term and queries programs.
func (h *Handler) handleSearchPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	match := matches[0] // The matched keyword
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm == "" {
		// Return help message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		helpText := "🎓 學程查詢說明\n\n" +
			"• 學程列表：查看所有學程\n" +
			"• 學程 關鍵字：搜尋學程\n\n" +
			"例如：\n" +
			"• 學程 資訊\n" +
			"• 學程 管理\n" +
			"• 學程 智慧財產"
		msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
		msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	return h.handleProgramSearch(ctx, searchTerm)
}

// handleProgramList retrieves and displays all programs.
func (h *Handler) handleProgramList(ctx context.Context) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.Info("Handling program list query")

	// Get all programs from database
	programs, err := h.db.GetAllPrograms(ctx)
	if err != nil {
		log.WithError(err).Error("Failed to get program list")
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 取得學程列表時發生錯誤\n\n請稍後再試。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
		return []messaging_api.MessageInterface{msg}
	}

	if len(programs) == 0 {
		msg := lineutil.NewTextMessageWithConsistentSender(
			"📭 目前沒有學程資料\n\n請稍後再試，系統會定期更新學程資訊。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.Infof("Found %d programs", len(programs))

	// Limit results
	if len(programs) > MaxProgramsPerSearch {
		programs = programs[:MaxProgramsPerSearch]
	}

	return h.formatProgramListResponse(programs, len(programs))
}

// handleProgramSearch searches programs by name using 2-tier matching.
func (h *Handler) handleProgramSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.Infof("Handling program search: %s", searchTerm)

	// Search using SQL LIKE + fuzzy matching (2-tier parallel search)
	// Tier 1: SQL LIKE for consecutive substring matches
	programs, err := h.db.SearchPrograms(ctx, searchTerm)
	if err != nil {
		log.WithError(err).Error("Failed to search programs")
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 搜尋學程時發生錯誤\n\n請稍後再試。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	// Tier 2: Fuzzy character-set matching
	// Get all programs and filter by character containment
	allPrograms, err := h.db.GetAllPrograms(ctx)
	if err != nil {
		log.WithError(err).Warn("Failed to get all programs for fuzzy matching")
	} else {
		// Create a set of already found program names
		foundNames := make(map[string]bool)
		for _, p := range programs {
			foundNames[p.Name] = true
		}

		// Add fuzzy matches that weren't found by SQL LIKE
		for _, p := range allPrograms {
			if !foundNames[p.Name] && stringutil.ContainsAllRunes(p.Name, searchTerm) {
				programs = append(programs, p)
			}
		}
	}

	if len(programs) == 0 {
		h.metrics.RecordCacheMiss(ModuleName)
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無「%s」相關學程\n\n💡 建議\n• 使用「學程列表」查看所有學程\n• 嘗試其他關鍵字", searchTerm),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			QuickReplyProgramListAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.Infof("Found %d programs for search: %s", len(programs), searchTerm)

	// Limit results
	if len(programs) > MaxProgramsPerSearch {
		programs = programs[:MaxProgramsPerSearch]
	}

	return h.formatProgramListResponse(programs, len(programs))
}

// handleProgramCourses retrieves and displays courses for a specific program.
func (h *Handler) handleProgramCourses(ctx context.Context, programName string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.Infof("Handling program courses query: %s", programName)

	// Get program courses from database
	programCourses, err := h.db.GetProgramCourses(ctx, programName)
	if err != nil {
		log.WithError(err).Error("Failed to get program courses")
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 取得學程課程時發生錯誤\n\n請稍後再試。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	if len(programCourses) == 0 {
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("📭 「%s」目前沒有課程資料\n\n請稍後再試，系統會定期更新課程資訊。", programName),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.Infof("Found %d courses for program: %s", len(programCourses), programName)

	// Separate required and elective courses
	var requiredCourses, electiveCourses []storage.ProgramCourse
	for _, pc := range programCourses {
		if pc.CourseType == "必" {
			requiredCourses = append(requiredCourses, pc)
		} else {
			electiveCourses = append(electiveCourses, pc)
		}
	}

	// Limit total results
	totalLimit := MaxCoursesPerProgram
	if len(requiredCourses)+len(electiveCourses) > totalLimit {
		// Prioritize required courses
		if len(requiredCourses) >= totalLimit {
			requiredCourses = requiredCourses[:totalLimit]
			electiveCourses = nil
		} else {
			remainingSlots := totalLimit - len(requiredCourses)
			if len(electiveCourses) > remainingSlots {
				electiveCourses = electiveCourses[:remainingSlots]
			}
		}
	}

	return h.formatProgramCoursesResponse(programName, requiredCourses, electiveCourses)
}

// handleCourseProgramsList shows all programs that a course belongs to.
// This is triggered from the "更多學程" button on course detail pages.
func (h *Handler) handleCourseProgramsList(ctx context.Context, courseUID string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.Infof("Handling course programs list for: %s", courseUID)

	// Get all programs for this course
	programs, err := h.db.GetCoursePrograms(ctx, courseUID)
	if err != nil {
		log.WithError(err).Error("Failed to get course programs")
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 取得相關學程時發生錯誤\n\n請稍後再試。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	if len(programs) == 0 {
		msg := lineutil.NewTextMessageWithConsistentSender(
			"📭 這門課程目前沒有相關學程資料",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.Infof("Found %d programs for course: %s", len(programs), courseUID)

	// Build text message listing all programs
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎓 課程 %s 的相關學程\n\n", courseUID))

	// Separate required and elective
	var required, elective []storage.ProgramRequirement
	for _, p := range programs {
		if p.CourseType == "必" {
			required = append(required, p)
		} else {
			elective = append(elective, p)
		}
	}

	if len(required) > 0 {
		sb.WriteString("✅ 必修學程：\n")
		for _, p := range required {
			sb.WriteString(fmt.Sprintf("• %s\n", p.ProgramName))
		}
		sb.WriteString("\n")
	}

	if len(elective) > 0 {
		sb.WriteString("📝 選修學程：\n")
		for _, p := range elective {
			sb.WriteString(fmt.Sprintf("• %s\n", p.ProgramName))
		}
	}

	sb.WriteString("\n💡 點擊下方按鈕查看學程的所有課程")

	msg := lineutil.NewTextMessageWithConsistentSender(sb.String(), sender)

	// Add quick reply items for each program
	quickReplyItems := make([]lineutil.QuickReplyItem, 0, len(programs)+2)
	for _, p := range programs {
		if len(quickReplyItems) >= 11 { // Leave room for list and help
			break
		}
		quickReplyItems = append(quickReplyItems, lineutil.QuickReplyItem{
			Action: lineutil.NewPostbackActionWithDisplayText(
				lineutil.TruncateRunes("🎓 "+p.ProgramName, 20),
				lineutil.TruncateRunes(fmt.Sprintf("查看「%s」課程", p.ProgramName), 40),
				PostbackPrefix+"courses"+bot.PostbackSplitChar+p.ProgramName,
			),
		})
	}
	quickReplyItems = append(quickReplyItems, QuickReplyProgramListAction())
	quickReplyItems = append(quickReplyItems, lineutil.QuickReplyHelpAction())

	msg.QuickReply = lineutil.NewQuickReply(quickReplyItems)

	return []messaging_api.MessageInterface{msg}
}

// ================================================
// Quick Reply Actions
// ================================================

// QuickReplyProgramListAction returns a "學程列表" quick reply item.
func QuickReplyProgramListAction() lineutil.QuickReplyItem {
	return lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("🎓 學程列表", "學程列表")}
}

// QuickReplyProgramSearchAction returns a "學程" quick reply item.
func QuickReplyProgramSearchAction() lineutil.QuickReplyItem {
	return lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("🎓 學程", "學程")}
}

// QuickReplyProgramNav returns quick reply items for program module navigation.
// Order: 🎓 學程列表 → 🎓 學程 → 📖 說明
func QuickReplyProgramNav() []lineutil.QuickReplyItem {
	return []lineutil.QuickReplyItem{
		QuickReplyProgramListAction(),
		QuickReplyProgramSearchAction(),
		lineutil.QuickReplyHelpAction(),
	}
}
