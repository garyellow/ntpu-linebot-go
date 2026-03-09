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
	"github.com/garyellow/ntpu-linebot-go/internal/modules/course"
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
	semesterCache  *course.SemesterCache // Shared cache (from course module)
	programCache   *ProgramListCache     // Short-TTL cache for GetAllPrograms results

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
	MaxProgramsPerSearch     = 500 // Text-based display limit (increased to cover all programs)
	TextListBatchSize        = 50  // Text-based list batch size
	MaxSearchResultsWithCard = 10  // Flex carousel limit for search results

	MaxCoursesInCarousel     = 40 // Carousel limit (first message is stats, leaves room for 4 carousels)
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
		"學程列表", "所有學程",
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
// semesterCache should be shared from course module for consistent semester data.
// Initializes and sorts matchers by priority during construction.
func NewHandler(
	db *storage.DB,
	metrics *metrics.Metrics,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
	semesterCache *course.SemesterCache,
) *Handler {
	h := &Handler{
		db:             db,
		metrics:        metrics,
		logger:         logger,
		stickerManager: stickerManager,
		semesterCache:  semesterCache,
		programCache:   NewProgramListCache(0),
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
			h.logger.WithModule(ModuleName).
				WithField("intent", IntentList).
				InfoContext(ctx, "Dispatching program intent")
		}
		return h.handleProgramList(ctx), nil

	case IntentSearch:
		query, ok := params["query"]
		if !ok || query == "" {
			return nil, fmt.Errorf("%w: query", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).
				WithField("intent", IntentSearch).
				WithField("query", query).
				InfoContext(ctx, "Dispatching program intent")
		}
		return h.handleProgramSearch(ctx, query), nil

	case IntentCourses:
		programName, ok := params["programName"]
		if !ok || programName == "" {
			return nil, fmt.Errorf("%w: programName", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).
				WithField("intent", IntentCourses).
				WithField("program_name", programName).
				InfoContext(ctx, "Dispatching program intent")
		}
		return h.handleProgramCourses(ctx, programName), nil

	default:
		return nil, fmt.Errorf("%w: %s", domerrors.ErrUnknownIntent, intent)
	}
}

// findMatcher returns the first matching pattern or nil.
// Used by both CanHandle and HandleMessage for consistent routing.
func (h *Handler) findMatcher(text string) *PatternMatcher {
	for i := range h.matchers {
		if h.matchers[i].pattern.MatchString(text) {
			return &h.matchers[i]
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

	log.InfoContext(ctx, "Handling program message")

	// Find matching pattern
	matcher := h.findMatcher(text)
	if matcher == nil {
		return []messaging_api.MessageInterface{}
	}

	// Extract regex match groups
	matches := matcher.pattern.FindStringSubmatch(text)
	// Defensive check: MatchString succeeded but FindStringSubmatch may return empty
	if len(matches) == 0 {
		log.WithField("pattern", matcher.name).
			WarnContext(ctx, "Pattern matched but submatches were empty")
		return []messaging_api.MessageInterface{}
	}

	log.WithField("pattern", matcher.name).
		WithField("priority", matcher.priority).
		DebugContext(ctx, "Pattern matched")

	// Call handler - must return non-empty per PatternHandler contract
	result := matcher.handler(ctx, text, matches)

	// Defensive check: handlers should never return nil/empty when pattern matched
	if len(result) == 0 {
		log.WithField("pattern", matcher.name).
			ErrorContext(ctx, "Pattern handler returned empty result")
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
		log.WarnContext(ctx, "Invalid postback format")
		return nil
	}

	action := parts[0]
	actionData := parts[1]

	log.WithField("action", action).
		DebugContext(ctx, "Processing postback")

	switch action {
	case "courses":
		return h.handleProgramCourses(ctx, actionData)
	case "course_programs":
		// Show all programs for a given course UID
		return h.handleCourseProgramsList(ctx, actionData)
	default:
		log.WithField("action", action).
			WarnContext(ctx, "Unknown postback action")
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
	// Use matches[1] to get the keyword without trailing space
	match := matches[1] // The captured keyword (group 1)
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm == "" {
		// Return help message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		helpText := "🧭 學程查詢說明\n\n" +
			"• 學程列表：查看所有學程\n" +
			"• 學程 關鍵字：搜尋學程\n\n" +
			"例如：\n" +
			"• 學程 資訊\n" +
			"• 學程 管理\n" +
			"• 學程 智慧財產"
		msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	return h.handleProgramSearch(ctx, searchTerm)
}

// handleProgramList retrieves and displays all programs.
func (h *Handler) handleProgramList(ctx context.Context) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.WithField("query_type", "list").
		InfoContext(ctx, "Handling program query")

	// Get recent 2 semesters for filtering statistics (data-driven)
	var years, terms []int
	if h.semesterCache != nil {
		years, terms = h.semesterCache.GetRecentSemesters()
		log.WithField("years", years).
			WithField("terms", terms).
			DebugContext(ctx, "Using semester filter for program statistics")
	}

	// Get all programs from cache (short-TTL) with semester filter
	programs, err := h.programCache.Get(ctx, h.db, years, terms)
	if err != nil {
		log.WithError(err).ErrorContext(ctx, "Failed to get program list")
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
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.WithField("count", len(programs)).
		DebugContext(ctx, "Program list loaded")

	// Limit results
	totalCount := len(programs)
	if totalCount > MaxProgramsPerSearch {
		programs = programs[:MaxProgramsPerSearch]
	}

	title := fmt.Sprintf("🧭 學程列表 (共 %d 個)", totalCount)
	footer := "💡 輸入「學程 關鍵字」搜尋特定學程"

	return h.formatProgramListResponse(programs, title, footer)
}

// handleProgramSearch searches programs by name using 2-tier matching.
func (h *Handler) handleProgramSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.WithField("search_term", searchTerm).
		InfoContext(ctx, "Handling program search")

	// Get recent 2 semesters for filtering statistics (data-driven)
	var years, terms []int
	if h.semesterCache != nil {
		years, terms = h.semesterCache.GetRecentSemesters()
		log.WithField("years", years).
			WithField("terms", terms).
			DebugContext(ctx, "Using semester filter for program search")
	}

	// Search using SQL LIKE + fuzzy matching (2-tier parallel search)
	// Tier 1: SQL LIKE for consecutive substring matches
	programs, err := h.db.SearchPrograms(ctx, searchTerm, years, terms)
	if err != nil {
		log.WithError(err).ErrorContext(ctx, "Failed to search programs")
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 搜尋學程時發生錯誤\n\n請稍後再試。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	// Tier 2: Fuzzy character-set matching
	// Get all programs from cache (short-TTL) and filter by character containment
	allPrograms, err := h.programCache.Get(ctx, h.db, years, terms)
	if err != nil {
		log.WithError(err).WarnContext(ctx, "Failed to get all programs for fuzzy matching")
	} else {
		// Deduplicate by program name (foundNames tracks already matched programs)
		// ContainsAllRunes allows non-contiguous character matching (e.g., "人工" matches "人工智慧")
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
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.WithField("count", len(programs)).
		WithField("search_term", searchTerm).
		DebugContext(ctx, "Program search results loaded")

	// Limit results
	if len(programs) > MaxProgramsPerSearch {
		programs = programs[:MaxProgramsPerSearch]
	}

	// Use Flex Carousel for small number of search results (richer experience)
	if len(programs) <= MaxSearchResultsWithCard {
		return h.formatProgramSearchResponse(programs)
	}

	// Use Text List for larger number of results (easier to scan)
	title := fmt.Sprintf("🔍 搜尋結果 (共 %d 個)", len(programs))
	footer := "💡 搜尋結果過多？請嘗試加入更多關鍵字以減少搜尋結果\n例如：「學程 金融 科技」"

	return h.formatProgramListResponse(programs, title, footer)
}

// handleProgramCourses retrieves and displays courses for a specific program.
// Courses are filtered to the most recent 2 semesters (consistent with smart search).
func (h *Handler) handleProgramCourses(ctx context.Context, programName string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.WithField("program_name", programName).
		InfoContext(ctx, "Handling program courses query")

	// Get recent 2 semesters from semester cache (data-driven)
	var years, terms []int
	if h.semesterCache != nil {
		years, terms = h.semesterCache.GetRecentSemesters()
		log.WithField("years", years).
			WithField("terms", terms).
			DebugContext(ctx, "Using semester filter for program courses")
	} else {
		// No semester cache available - will return all courses
		// This should only happen in tests; in production, semesterCache is always set
		log.DebugContext(ctx, "No semester cache available, returning all program courses")
	}

	// Get program courses from database (filtered by 2 semesters)
	programCourses, err := h.db.GetProgramCourses(ctx, programName, years, terms)
	if err != nil {
		log.WithError(err).ErrorContext(ctx, "Failed to get program courses")
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 取得學程課程時發生錯誤\n\n請稍後再試。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	// 1. If no courses found for exact name, return specific message immediately
	// Do NOT attempt fuzzy search or auto-correction to avoid incorrect program matching
	// e.g., Searching "大數據" should NOT auto-match to "大數據分析學程" without explicit user selection
	if len(programCourses) == 0 {
		h.metrics.RecordCacheMiss(ModuleName)
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("📭 「%s」在近 2 學期沒有課程資料\n\n💡 可能原因：\n• 該學程可能在本學期未開設相關課程\n• 學程名稱可能有誤，請嘗試「學程列表」查看正確名稱", programName),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.WithField("count", len(programCourses)).
		WithField("program_name", programName).
		DebugContext(ctx, "Program courses loaded")

	// Separate required and elective courses
	var requiredCourses, electiveCourses []storage.ProgramCourse
	for _, pc := range programCourses {
		if pc.CourseType == "必" {
			requiredCourses = append(requiredCourses, pc)
		} else {
			electiveCourses = append(electiveCourses, pc)
		}
	}

	// Store original counts before truncation for display
	originalRequiredCount := len(requiredCourses)
	originalElectiveCount := len(electiveCourses)
	totalCourses := originalRequiredCount + originalElectiveCount

	// Decision: Use carousel for ≤40 courses, text list for >40 courses
	if totalCourses <= MaxCoursesInCarousel {
		return h.formatProgramCoursesResponse(programName, requiredCourses, electiveCourses, originalRequiredCount, originalElectiveCount)
	}

	// For >40 courses, use text list format
	log.WithField("total_courses", totalCourses).
		WithField("limit", MaxCoursesInCarousel).
		DebugContext(ctx, "Using text list format for program courses")
	return h.formatProgramCoursesAsTextList(programName, requiredCourses, electiveCourses, originalRequiredCount, originalElectiveCount)
}

// handleCourseProgramsList shows all programs that a course belongs to.
// This is triggered from the "更多學程" button on course detail pages.
func (h *Handler) handleCourseProgramsList(ctx context.Context, courseUID string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.WithField("course_uid", courseUID).
		InfoContext(ctx, "Handling course programs list")

	// Get course info to display course name (not just UID)
	course, err := h.db.GetCourseByUID(ctx, courseUID)
	courseName := courseUID // Fallback to UID if course not found
	if err != nil {
		log.WithError(err).
			WithField("course_uid", courseUID).
			WarnContext(ctx, "Failed to load course info, using UID fallback")
	} else if course != nil {
		courseName = course.Title
	}

	// Get all programs for this course
	programs, err := h.db.GetCoursePrograms(ctx, courseUID)
	if err != nil {
		log.WithError(err).ErrorContext(ctx, "Failed to get course programs")
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 取得相關學程時發生錯誤\n\n請稍後再試。",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	if len(programs) == 0 {
		// Build Flex message with default program list URL
		bubble := h.buildNoProgramsFoundBubble(courseName)
		msg := lineutil.NewFlexMessage("查無相關學程", bubble.FlexBubble)
		msg.Sender = sender
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordCacheHit(ModuleName)
	log.WithField("count", len(programs)).
		WithField("course_uid", courseUID).
		DebugContext(ctx, "Course programs loaded")

	// Always use Flex carousel for related programs (allows unlimited rows via LINE API)
	// This provides a consistent UI experience regardless of program count
	return h.formatCourseProgramsAsCarousel(ctx, programs)
}

// formatCourseProgramsAsCarousel formats course programs as Flex carousel.
func (h *Handler) formatCourseProgramsAsCarousel(ctx context.Context, programs []storage.ProgramRequirement) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Get recent 2 semesters for consistent course count filtering
	var years, terms []int
	if h.semesterCache != nil {
		years, terms = h.semesterCache.GetRecentSemesters()
	}

	// Build program bubbles
	bubbles := make([]messaging_api.FlexBubble, 0, len(programs))
	for _, p := range programs {
		// Get full program info from database using exact name match
		// Pass semester filter to ensure course counts match "查看課程" results
		program, err := h.db.GetProgramByName(ctx, p.ProgramName, years, terms)
		if err != nil || program == nil {
			// Create a minimal program struct if not found
			minimalProgram := storage.Program{
				Name:     p.ProgramName,
				Category: "",
			}
			bubble := h.buildProgramBubble(minimalProgram)
			bubbles = append(bubbles, *bubble.FlexBubble)
			continue
		}
		bubble := h.buildProgramBubble(*program)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Build carousel (no header text - carousel is self-explanatory)
	messages := lineutil.BuildCarouselMessages("相關學程", bubbles, sender)

	// Add quick reply to last message
	if len(messages) > 0 {
		lineutil.AddQuickReplyToMessages(messages, lineutil.QuickReplyProgramNav()...)
	}

	return messages
}

// ================================================
