// Package course implements the course query module for the LINE bot.
// It handles course searches by title, teacher, or UID from NTPU's course system.
package course

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sliceutil"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles course-related queries using Pattern-Action Table architecture.
// Both CanHandle() and HandleMessage() share the same matchers list, which structurally
// guarantees routing consistency and eliminates the possibility of divergence.
//
// Pattern priority (1=highest): UID → CourseNo → Historical → Smart → Extended → Regular
type Handler struct {
	db               *storage.DB
	scraper          *scraper.Client
	metrics          *metrics.Metrics
	logger           *logger.Logger
	stickerManager   *sticker.Manager
	bm25Index        *rag.BM25Index
	queryExpander    genai.QueryExpander // Interface for multi-provider support
	llmRateLimiter   *ratelimit.KeyedLimiter
	semesterDetector *SemesterDetector // Data-driven semester detection

	// matchers contains all pattern-handler pairs sorted by priority.
	// Shared by CanHandle and HandleMessage for consistent routing.
	matchers []PatternMatcher
}

// Name returns the module name
func (h *Handler) Name() string {
	return ModuleName
}

// Module constants for course handler.
const (
	ModuleName          = "course" // Module identifier for registration
	senderName          = "課程小幫手"
	MaxCoursesPerSearch = 40 // 4 carousels @ 10 bubbles, +1 slot for warning (LINE max: 5 messages)

)

// Pattern priorities (lower = higher).
const (
	PriorityUID        = 1 // Full UID (e.g., 1131U0001)
	PriorityCourseNo   = 2 // Course number (e.g., U0001)
	PriorityHistorical = 3 // Historical (課程 110 微積分)
	PrioritySmart      = 4 // Smart (找課)
	PriorityExtended   = 5 // Extended (更多學期)
	PriorityRegular    = 6 // Regular (課程/老師)
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
	// validCourseKeywords: unified search (course + teacher), semesters 1-2.
	validCourseKeywords = []string{
		// 中文課程關鍵字
		"課", "課程", "科目",
		"課名", "課程名", "課程名稱",
		"科目名", "科目名稱",
		// 中文教師關鍵字
		"師", "老師", "教師", "教授",
		"老師名", "教師名", "教授名",
		"老師名稱", "教師名稱", "教授名稱",
		"授課教師", "授課老師", "授課教授",
		// English keywords
		"class", "course",
		"teacher", "professor", "prof", "dr", "doctor",
	}

	// validSmartSearchKeywords: semantic search (BM25 + LLM expansion), semesters 1-2.
	validSmartSearchKeywords = []string{
		"找課", "找課程", "搜課",
	}

	// validExtendedSearchKeywords: extended time range, semesters 3-4.
	validExtendedSearchKeywords = []string{
		"更多學期", "更多課程", "歷史課程",
	}

	courseRegex            = bot.BuildKeywordRegex(validCourseKeywords)
	smartSearchCourseRegex = bot.BuildKeywordRegex(validSmartSearchKeywords)
	extendedSearchRegex    = bot.BuildKeywordRegex(validExtendedSearchKeywords)
	// Full UID: {year}{term}{no} = 3-4 digits + [UMNP] + 4 digits (e.g., 1131U0001, 991U0001)
	uidRegex = regexp.MustCompile(`(?i)\d{3,4}[umnp]\d{4}`)
	// Course number: [UMNP] + 4 digits (e.g., U0001, M0002)
	courseNoRegex = regexp.MustCompile(`(?i)^[umnp]\d{4}$`)
	// Historical: "課程 {year} {keyword}" where year = ROC (2-3 digits) or Western (4 digits)
	// Examples: 課程 110 微積分 (ROC), 課程 2021 微積分 (Western)
	historicalCourseRegex = regexp.MustCompile(`(?i)^(課程?|course|class)\s+(\d{2,4})\s+(.+)$`)
)

// NewHandler creates a new course handler.
// Optional: bm25Index, queryExpander, llmRateLimiter (pass nil if unused).
// Initializes and sorts matchers by priority during construction.
// The semesterDetector is initialized with db.CountCoursesBySemester for data-driven semester detection.
func NewHandler(
	db *storage.DB,
	scraper *scraper.Client,
	metrics *metrics.Metrics,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
	bm25Index *rag.BM25Index,
	queryExpander genai.QueryExpander, // Interface for multi-provider support
	llmRateLimiter *ratelimit.KeyedLimiter,
) *Handler {
	// Create semester detector with database count function
	semesterDetector := NewSemesterDetector(db.CountCoursesBySemester)

	h := &Handler{
		db:               db,
		scraper:          scraper,
		metrics:          metrics,
		logger:           logger,
		stickerManager:   stickerManager,
		bm25Index:        bm25Index,
		queryExpander:    queryExpander,
		llmRateLimiter:   llmRateLimiter,
		semesterDetector: semesterDetector,
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
			pattern:  uidRegex,
			priority: PriorityUID,
			handler:  h.handleUIDPattern,
			name:     "UID",
		},
		{
			pattern:  courseNoRegex,
			priority: PriorityCourseNo,
			handler:  h.handleCourseNoPattern,
			name:     "CourseNumber",
		},
		{
			pattern:  historicalCourseRegex,
			priority: PriorityHistorical,
			handler:  h.handleHistoricalPattern,
			name:     "Historical",
		},
		{
			pattern:  smartSearchCourseRegex,
			priority: PrioritySmart,
			handler:  h.handleSmartPattern,
			name:     "Smart",
		},
		{
			pattern:  extendedSearchRegex,
			priority: PriorityExtended,
			handler:  h.handleExtendedPattern,
			name:     "Extended",
		},
		{
			pattern:  courseRegex,
			priority: PriorityRegular,
			handler:  h.handleRegularPattern,
			name:     "Regular",
		},
	}

	// Sort by priority (lower number = higher priority)
	slices.SortFunc(h.matchers, func(a, b PatternMatcher) int {
		return a.priority - b.priority
	})
}

// IsBM25SearchEnabled returns true if BM25 search is enabled.
func (h *Handler) IsBM25SearchEnabled() bool {
	return h.bm25Index != nil && h.bm25Index.IsEnabled()
}

// GetSemesterDetector returns the semester detector for sharing with other modules.
// This enables consistent 2-semester filtering across course and program modules.
func (h *Handler) GetSemesterDetector() *SemesterDetector {
	return h.semesterDetector
}

// RefreshSemesters updates the cached semester data from database.
// This should be called after warmup completes to ensure user queries
// use data-driven semester detection based on actual course availability.
func (h *Handler) RefreshSemesters(ctx context.Context) {
	if h.semesterDetector == nil {
		return
	}
	h.semesterDetector.RefreshSemesters(ctx)
	if h.semesterDetector.HasData() {
		years, terms := h.semesterDetector.GetAllSemesters()
		h.logger.WithModule(ModuleName).
			WithField("semesters", formatSemesterList(years, terms)).
			Info("Refreshed semester data (data-driven)")
	}
}

// formatSemesterList formats semester pairs for logging (e.g., "113-2, 113-1, 112-2, 112-1")
func formatSemesterList(years, terms []int) string {
	if len(years) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%d-%d", years[0], terms[0]))
	for i := 1; i < len(years); i++ {
		builder.WriteString(fmt.Sprintf(", %d-%d", years[i], terms[i]))
	}
	return builder.String()
}

// hasQueryExpander returns true if query expander is available.
func (h *Handler) hasQueryExpander() bool {
	return h.queryExpander != nil
}

// Intent names for NLU dispatcher
const (
	IntentSearch = "search" // Unified course/teacher search
	IntentSmart  = "smart"  // Smart search via BM25 + Query Expansion
	IntentUID    = "uid"    // Direct course UID lookup
)

// DispatchIntent handles NLU-parsed intents.
// Intents: "search" (keyword), "smart" (query), "uid" (uid).
// Returns error if intent unknown or required params missing.
func (h *Handler) DispatchIntent(ctx context.Context, intent string, params map[string]string) ([]messaging_api.MessageInterface, error) {
	// Validate parameters first (before logging) to support testing with nil dependencies
	switch intent {
	case IntentSearch:
		keyword, ok := params["keyword"]
		if !ok || keyword == "" {
			return nil, fmt.Errorf("%w: keyword", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching course intent: %s, keyword: %s", intent, keyword)
		}
		return h.handleUnifiedCourseSearch(ctx, keyword), nil

	case IntentSmart:
		query, ok := params["query"]
		if !ok || query == "" {
			return nil, fmt.Errorf("%w: query", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching course intent: %s, query: %s", intent, query)
		}
		return h.handleSmartSearch(ctx, query), nil

	case IntentUID:
		uid, ok := params["uid"]
		if !ok || uid == "" {
			return nil, fmt.Errorf("%w: uid", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching course intent: %s, uid: %s", intent, uid)
		}
		return h.handleCourseUIDQuery(ctx, uid), nil

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

	log.Debugf("Handling course message: %s", text)

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

// Pattern handler adapters - implement PatternHandler contract.
// Must return non-empty messages when invoked (pattern matched).

// handleUIDPattern extracts UID and queries course.
func (h *Handler) handleUIDPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	uid := matches[0] // Full UID match
	return h.handleCourseUIDQuery(ctx, uid)
}

// handleCourseNoPattern processes course number queries.
func (h *Handler) handleCourseNoPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	return h.handleCourseNoQuery(ctx, text)
}

// handleHistoricalPattern parses year and keyword from historical query.
// Regex groups: [0]=fullMatch, [1]=keywordPrefix, [2]=year, [3]=searchTerm
func (h *Handler) handleHistoricalPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	// Defensive validation (should not happen if regex is correct)
	if len(matches) < 4 {
		log := h.logger.WithModule(ModuleName)
		log.Errorf("Historical pattern matched but got %d groups (expected 4)", len(matches))
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 查詢格式有誤\n\n正確格式：課程 110 微積分\n（年份可使用民國年或西元年，如 110、2021）",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	yearStr := matches[2]                    // Year (ROC or Western)
	keyword := strings.TrimSpace(matches[3]) // Search keyword
	year := 0

	if _, err := fmt.Sscanf(yearStr, "%d", &year); err != nil || keyword == "" {
		// Invalid year format or empty keyword
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⚠️ 查詢格式有誤\n\n正確格式：課程 110 微積分\n（年份可使用民國年或西元年，如 110、2021）",
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Convert Western year to ROC year if needed
	// ROC year 0 = 1911 AD, so 2021 AD = 110 ROC
	if year >= 1911 {
		year = year - 1911
		log := h.logger.WithModule(ModuleName)
		log.Debugf("Converted Western year to ROC: %s -> %d", yearStr, year)
	}

	// Validate year is within reasonable range
	if year < config.CourseSystemLaunchYear {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 年份過早\n\n課程系統於民國 %d 年才啟用\n請輸入 %d 年（西元 %d 年）之後的課程",
				config.CourseSystemLaunchYear,
				config.CourseSystemLaunchYear,
				config.CourseSystemLaunchYear+1911),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	return h.handleHistoricalCourseSearch(ctx, year, keyword)
}

// handleSmartPattern processes smart search with help message fallback.
func (h *Handler) handleSmartPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	match := matches[0] // The matched keyword
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm == "" {
		// Return help message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		var helpText string
		if h.bm25Index != nil && h.bm25Index.IsEnabled() {
			helpText = "🔮 智慧搜尋說明\n\n" +
				"請描述您想找的課程內容：\n" +
				"• 找課 想學資料分析\n" +
				"• 找課 Python 機器學習\n" +
				"• 找課 商業管理相關\n\n" +
				"💡 提示\n" +
				"• 根據課程大綱內容智慧匹配\n" +
				"• 若知道課名，建議用「課程 名稱」"
		} else {
			helpText = "⚠️ 智慧搜尋目前未啟用\n\n" +
				"請使用精確搜尋：\n" +
				"• 課程 微積分\n" +
				"• 課程 王小明"
		}
		msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	return h.handleSmartSearch(ctx, searchTerm)
}

// handleExtendedPattern processes extended search queries (e.g., 更多學期 微積分).
func (h *Handler) handleExtendedPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	match := matches[0] // The matched keyword
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm == "" {
		// Return help message
		sender := lineutil.GetSender(senderName, h.stickerManager)
		helpText := "📅 更多學期搜尋說明\n\n" +
			"🔍 搜尋範圍：額外 2 個歷史學期（第 3-4 學期）\n" +
			"（精確搜尋僅搜尋近 2 學期＝最新第 1-2 學期）\n\n" +
			"用法範例：\n" +
			"• 更多學期 微積分\n" +
			"• 更多學期 王小明\n\n" +
			"📆 需要指定年份？\n" +
			"使用：「課程 110 微積分」或「課程 2021 微積分」"
		msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	return h.handleExtendedCourseSearch(ctx, searchTerm)
}

// handleRegularPattern processes regular course/teacher queries (e.g., 課程 微積分).
func (h *Handler) handleRegularPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	match := matches[0] // The matched keyword
	searchTerm := bot.ExtractSearchTerm(text, match)

	if searchTerm == "" {
		// Return help message with all options
		sender := lineutil.GetSender(senderName, h.stickerManager)
		var helpText string
		var quickReplyItems []lineutil.QuickReplyItem
		if h.bm25Index != nil && h.bm25Index.IsEnabled() {
			helpText = "📚 課程查詢方式\n\n" +
				"🔍 精確搜尋（近 2 學期）\n" +
				"• 課程 微積分\n" +
				"• 課程 王小明\n" +
				"• 課程 線代 王\n\n" +
				"🔮 智慧搜尋（近 2 學期）\n" +
				"• 找課 想學資料分析\n" +
				"• 找課 Python 入門\n\n" +
				"📅 更多學期（第 3-4 學期）\n" +
				"• 更多學期 微積分\n\n" +
				"📆 指定年份\n" +
				"• 課程 110 微積分（民國年）\n" +
				"• 課程 2021 微積分（西元年）\n\n" +
				"💡 直接輸入課號（如 U0001）\n" +
				"   或完整編號（如 1131U0001）"
			quickReplyItems = []lineutil.QuickReplyItem{
				lineutil.QuickReplySmartSearchAction(),
				lineutil.QuickReplyHelpAction(),
			}
		} else {
			helpText = "📚 課程查詢方式\n\n" +
				"🔍 精確搜尋（近 2 學期）\n" +
				"• 課程 微積分\n" +
				"• 課程 王小明\n" +
				"• 課程 線代 王\n\n" +
				"📅 更多學期（第 3-4 學期）\n" +
				"• 更多學期 微積分\n\n" +
				"📆 指定年份\n" +
				"• 課程 110 微積分（民國年）\n" +
				"• 課程 2021 微積分（西元年）\n\n" +
				"💡 直接輸入課號（如 U0001）\n" +
				"   或完整編號（如 1131U0001）"
			quickReplyItems = []lineutil.QuickReplyItem{
				lineutil.QuickReplyHelpAction(),
			}
		}
		msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
		msg.QuickReply = lineutil.NewQuickReply(quickReplyItems)
		return []messaging_api.MessageInterface{msg}
	}

	return h.handleUnifiedCourseSearch(ctx, searchTerm)
}

// HandlePostback handles postback events for the course module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	log.Infof("Handling course postback: %s", data)

	// Strip module prefix if present (registry passes original data)
	data = strings.TrimPrefix(data, "course:")

	// Handle "授課課程" postback FIRST (before UID check, since teacher name might contain numbers)
	if strings.HasPrefix(data, "授課課程") {
		parts := strings.Split(data, bot.PostbackSplitChar)
		if len(parts) >= 2 {
			teacherName := parts[1]
			log.Infof("Handling teacher courses postback for: %s", teacherName)
			return h.handleUnifiedCourseSearch(ctx, teacherName)
		}
	}

	// Check for course UID in postback (with or without prefix)
	// Extract the actual UID from data (e.g., "1132U2236")
	if uidRegex.MatchString(data) {
		uid := uidRegex.FindString(data)
		return h.handleCourseUIDQuery(ctx, uid)
	}

	return []messaging_api.MessageInterface{}
}

// handleCourseUIDQuery handles course UID queries
func (h *Handler) handleCourseUIDQuery(ctx context.Context, uid string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Normalize UID to uppercase
	uid = strings.ToUpper(uid)

	// Check cache first
	course, err := h.db.GetCourseByUID(ctx, uid)
	if err != nil {
		log.WithError(err).Error("Failed to query cache")
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("查詢課程時發生問題", sender, uid),
		}
	}

	if course != nil {
		// Cache hit
		h.metrics.RecordCacheHit(ModuleName)
		log.Debugf("Cache hit for course UID: %s", uid)
		return h.formatCourseResponseWithContext(ctx, course)
	}

	// Cache miss - scrape from website
	h.metrics.RecordCacheMiss(ModuleName)
	log.Infof("Cache miss for course UID: %s, scraping...", uid)

	course, err = ntpu.ScrapeCourseByUID(ctx, h.scraper, uid)
	if err != nil {
		// Check if it's a context error (timeout/cancellation)
		if ctx.Err() != nil {
			log.WithError(err).Warnf("Context error while scraping course UID %s: %v", uid, ctx.Err())
			h.metrics.RecordScraperRequest(ModuleName, "timeout", time.Since(startTime).Seconds())
		} else {
			log.WithError(err).Errorf("Failed to scrape course UID: %s (error type: %T)", uid, err)
			h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		}
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("🔍 查無此課程編號\n\n課程編號：%s\n💡 請確認編號格式是否正確", uid), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Check if course was found (prevent nil pointer dereference)
	if course == nil {
		log.Warnf("Course UID %s not found after scraping", uid)
		h.metrics.RecordScraperRequest(ModuleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無課程編號 %s\n\n💡 建議\n• 確認課程編號是否正確\n• 該課程是否有開設", uid),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache
	if err := h.db.SaveCourse(ctx, course); err != nil {
		log.WithError(err).Warn("Failed to save course to cache")
	}

	h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
	return h.formatCourseResponseWithContext(ctx, course)
}

// handleCourseNoQuery handles course number only queries (e.g., U0001, M0002)
// It searches in current and previous semester to find the course
func (h *Handler) handleCourseNoQuery(ctx context.Context, courseNo string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Normalize course number to uppercase
	courseNo = strings.ToUpper(courseNo)

	log.Infof("Handling course number query: %s", courseNo)

	// Get semesters to search from data-driven semester detection
	searchYears, searchTerms := h.semesterDetector.GetRecentSemesters()

	// Search in cache first
	for i := range searchYears {
		year := searchYears[i]
		term := searchTerms[i]
		uid := fmt.Sprintf("%d%d%s", year, term, courseNo)

		course, err := h.db.GetCourseByUID(ctx, uid)
		if err != nil {
			log.WithError(err).Warnf("Failed to query cache for UID: %s", uid)
			continue
		}

		if course != nil {
			h.metrics.RecordCacheHit(ModuleName)
			log.Debugf("Cache hit for course UID: %s (from course no: %s)", uid, courseNo)
			return h.formatCourseResponseWithContext(ctx, course)
		}
	}

	// Cache miss - try scraping from each semester
	h.metrics.RecordCacheMiss(ModuleName)
	log.Infof("Cache miss for course number: %s, scraping...", courseNo)

	for i := range searchYears {
		year := searchYears[i]
		term := searchTerms[i]
		uid := fmt.Sprintf("%d%d%s", year, term, courseNo)

		course, err := ntpu.ScrapeCourseByUID(ctx, h.scraper, uid)
		if err != nil {
			log.WithError(err).Debugf("Course not found for UID: %s", uid)
			continue
		}

		if course != nil {
			// Save to cache
			if err := h.db.SaveCourse(ctx, course); err != nil {
				log.WithError(err).Warn("Failed to save course to cache")
			}

			h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
			log.Infof("Found course for UID: %s (from course no: %s)", uid, courseNo)
			return h.formatCourseResponseWithContext(ctx, course)
		}
	}

	// No results found
	h.metrics.RecordScraperRequest(ModuleName, "not_found", time.Since(startTime).Seconds())

	// Build helpful message
	msg := lineutil.NewTextMessageWithConsistentSender(
		fmt.Sprintf("🔍 查無課程編號 %s\n\n💡 建議\n• 確認課程編號是否正確（如 U0001）\n• 該課程是否有開設\n• 或使用「課程 課名」搜尋", courseNo),
		sender,
	)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyCourseAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// handleUnifiedCourseSearch handles unified course search queries with fuzzy matching.
// It searches both course titles and teacher names simultaneously.
// Search range: Recent 2 semesters with cache-first strategy.
//
// Search Strategy (2-tier parallel search + scraping fallback):
//
//  1. SQL LIKE (fast path): Search in both title and teachers fields for consecutive substrings.
//     Example: "微積分" matches courses with title containing "微積分"
//     Example: "王" matches courses where any teacher name contains "王"
//
//  2. Fuzzy character-set matching (ALWAYS runs in parallel with SQL LIKE):
//     Loads cached courses for the target semesters and checks if all runes in searchTerm
//     exist in title OR teachers. This catches abbreviations that SQL LIKE misses.
//     Example: "線代" matches "線性代數" (all chars exist in title but not consecutive)
//     Example: "王明" matches teacher "王小明" (all chars exist)
//
//     Results from both strategies are merged and deduplicated by UID.
//
//  3. Web scraping (external fallback): If cache has no results, scrape from website.
//
// Multi-word search: "微積分 王" will find courses where title contains "微積分王"
// OR where all characters exist in title+teachers combined.
func (h *Handler) handleUnifiedCourseSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	return h.searchCoursesWithOptions(ctx, searchTerm, false)
}

// handleExtendedCourseSearch handles extended course search (3rd and 4th semesters).
// This is triggered by "課程歷史" or "更多學期" keywords, typically from Quick Reply.
// Search range: 2 additional historical semesters (excludes the 2 most recent).
// Search flow: SQL LIKE → Fuzzy match (2 historical semesters) → Scraping (2 historical semesters)
func (h *Handler) handleExtendedCourseSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	return h.searchCoursesWithOptions(ctx, searchTerm, true)
}

// searchCoursesWithOptions is the core search implementation used by both unified and extended search.
// It consolidates the common search logic to avoid code duplication.
//
// Parameters:
//   - extended: If true, searches 2 historical semesters (3rd-4th); if false, searches 2 recent semesters (1st-2nd)
//
// Search flow:
//  1. SQL LIKE search (title + teacher) in cache
//  2. Fuzzy character-set matching (respects extended flag for semester range)
//  3. Web scraping from NTPU website (if cache miss)
//
// Note: Smart search (BM25) is completely separate and triggered by "找課" keyword only.
func (h *Handler) searchCoursesWithOptions(ctx context.Context, searchTerm string, extended bool) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	semesterType := "近期"
	if extended {
		semesterType = "過去 2 學期"
	}
	log.Infof("Handling course search (%s semesters): %s", semesterType, searchTerm)

	var courses []storage.Course

	// Get courses based on search range (2 or 4 semesters) - data-driven
	var searchYears, searchTerms []int
	if extended {
		searchYears, searchTerms = h.semesterDetector.GetExtendedSemesters()
	} else {
		searchYears, searchTerms = h.semesterDetector.GetRecentSemesters()
	}

	// Step 1: Try SQL LIKE search for title first
	titleCourses, err := h.db.SearchCoursesByTitle(ctx, searchTerm)
	if err != nil {
		log.WithError(err).Error("Failed to search courses by title in cache")
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())

		// Build retry text based on extended flag
		retryText := "課程 " + searchTerm
		if extended {
			retryText = "更多學期 " + searchTerm
		}

		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("搜尋課程時發生問題", sender, retryText),
		}
	}
	courses = append(courses, titleCourses...)

	// Step 1b: Also try SQL LIKE search for teacher
	teacherCourses, err := h.db.SearchCoursesByTeacher(ctx, searchTerm)
	if err != nil {
		log.WithError(err).Warn("Failed to search courses by teacher in cache")
		// Don't return error, continue with title results
	} else {
		// Merge results, avoiding duplicates
		courses = append(courses, teacherCourses...)
	}

	// Filter SQL results by semester scope to ensure consistency
	courses = filterCoursesBySemesters(courses, searchYears, searchTerms)

	// Step 2: ALWAYS try fuzzy character-set matching to find additional results
	// This catches cases like "線代" -> "線性代數" that SQL LIKE misses
	// SQL LIKE only finds consecutive substrings, but fuzzy matching finds scattered characters

	// Get all courses for the specified semesters from cache
	for i := range searchYears {
		year := searchYears[i]
		term := searchTerms[i]
		semesterCourses, err := h.db.GetCoursesByYearTerm(ctx, year, term)
		if err != nil {
			log.WithError(err).Warnf("Failed to get courses for year %d term %d", year, term)
			continue
		}

		// Fuzzy match against all courses in this semester
		for _, c := range semesterCourses {
			// Check if searchTerm matches title OR any teacher using fuzzy matching
			titleMatch := stringutil.ContainsAllRunes(c.Title, searchTerm)
			teacherMatch := false
			for _, teacher := range c.Teachers {
				if stringutil.ContainsAllRunes(teacher, searchTerm) {
					teacherMatch = true
					break
				}
			}
			if titleMatch || teacherMatch {
				courses = append(courses, c)
			}
		}
	}

	// Deduplicate results by UID (SQL LIKE and fuzzy may find overlapping results)
	courses = sliceutil.Deduplicate(courses, func(c storage.Course) string { return c.UID })

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(ModuleName)
		log.Infof("Found %d courses in cache for search term: %s", len(courses), searchTerm)
		return h.formatCourseListResponseWithOptions(courses, FormatOptions{
			SearchKeyword:    searchTerm,
			IsExtendedSearch: extended,
		})
	}

	// Step 3: Cache miss - Try scraping
	cacheMissMsg := fmt.Sprintf("Cache miss for search term: %s, scraping from %s...", searchTerm, semesterType)
	log.Info(cacheMissMsg)
	h.metrics.RecordCacheMiss(ModuleName)

	// Search courses from multiple semesters
	foundCourses := make([]*storage.Course, 0)
	existingUIDs := make(map[string]bool)

	for i := range searchYears {
		year := searchYears[i]
		term := searchTerms[i]

		// Scrape courses (this will search by title on the school website)
		scrapedCourses, err := ntpu.ScrapeCourses(ctx, h.scraper, year, term, searchTerm)
		if err != nil {
			log.WithError(err).WithField("year", year).WithField("term", term).
				Debug("Failed to scrape courses for year/term")
			continue
		}

		// Save courses to cache and collect results
		for _, course := range scrapedCourses {
			if err := h.db.SaveCourse(ctx, course); err != nil {
				log.WithError(err).Warn("Failed to save course to cache")
			}
			if !existingUIDs[course.UID] {
				foundCourses = append(foundCourses, course)
				existingUIDs[course.UID] = true
			}
		}
	}

	// Also scrape all courses to find by teacher name (if no results yet)
	// WARNING: This is a heavy operation that scrapes all courses for each semester.
	// It iterates through all education codes (U/M/N/P) since the school system
	// doesn't support direct teacher search via URL parameters.
	// This may take significant time and could approach the 60s webhook deadline.
	if len(foundCourses) == 0 {
		for i := range searchYears {
			year := searchYears[i]
			term := searchTerms[i]

			// Scrape all courses for this semester (empty search term)
			scrapedCourses, err := ntpu.ScrapeCourses(ctx, h.scraper, year, term, "")
			if err != nil {
				log.WithError(err).WithField("year", year).WithField("term", term).
					Debug("Failed to scrape all courses for year/term")
				continue
			}

			// Filter by searchTerm (title or teacher) using fuzzy matching
			for _, course := range scrapedCourses {
				// Save all courses for future queries
				if err := h.db.SaveCourse(ctx, course); err != nil {
					log.WithError(err).Warn("Failed to save course to cache")
				}

				// Check if matches title or teacher
				titleMatch := stringutil.ContainsAllRunes(course.Title, searchTerm)
				teacherMatch := false
				for _, teacher := range course.Teachers {
					if stringutil.ContainsAllRunes(teacher, searchTerm) {
						teacherMatch = true
						break
					}
				}

				if (titleMatch || teacherMatch) && !existingUIDs[course.UID] {
					foundCourses = append(foundCourses, course)
					existingUIDs[course.UID] = true
				}
			}
		}
	}

	if len(foundCourses) > 0 {
		h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
		// Convert []*storage.Course to []storage.Course
		courses := make([]storage.Course, len(foundCourses))
		for i, c := range foundCourses {
			courses[i] = *c
		}
		return h.formatCourseListResponseWithOptions(courses, FormatOptions{
			SearchKeyword:    searchTerm,
			IsExtendedSearch: extended,
		})
	}

	// No results found even after scraping
	h.metrics.RecordScraperRequest(ModuleName, "not_found", time.Since(startTime).Seconds())

	// Build help message with suggestions (different for extended vs regular search)
	var helpText string
	if extended {
		helpText = fmt.Sprintf(
			"🔍 查無相關課程\n\n搜尋內容：%s\n📅 搜尋範圍：%s\n\n💡 建議嘗試\n• 縮短關鍵字（如「線性」→「線」）\n• 只輸入教師姓氏\n• 指定年份：「課程 110 %s」",
			searchTerm,
			semesterType,
			searchTerm,
		)
	} else {
		helpText = fmt.Sprintf(
			"🔍 查無「%s」的相關課程\n\n📅 已搜尋範圍：近 2 學期\n\n💡 建議嘗試\n• 使用「📅 更多學期」搜尋第 3-4 學期\n• 縮短關鍵字（如「線性」→「線」）\n• 指定年份：「課程 110 %s」",
			searchTerm,
			searchTerm,
		)
	}

	if h.bm25Index != nil && h.bm25Index.IsEnabled() {
		helpText += "\n• 智慧搜尋：「找課 " + searchTerm + "」"
	}

	msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)

	// Build quick reply items (consistent order as search results)
	var quickReplyItems []lineutil.QuickReplyItem

	// Add "更多" button FIRST for visibility (only for non-extended search)
	if !extended {
		quickReplyItems = append(quickReplyItems, lineutil.QuickReplyMoreCoursesCompact(searchTerm))
	}
	quickReplyItems = append(quickReplyItems, lineutil.QuickReplyCourseAction())

	if h.bm25Index != nil && h.bm25Index.IsEnabled() {
		quickReplyItems = append(quickReplyItems,
			lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("🔮 找課", "找課 "+searchTerm)},
		)
	}
	quickReplyItems = append(quickReplyItems, lineutil.QuickReplyHelpAction())
	msg.QuickReply = lineutil.NewQuickReply(quickReplyItems)
	return []messaging_api.MessageInterface{msg}
}

// handleHistoricalCourseSearch handles historical course queries using "課程 {year} {keyword}" syntax
// Uses separate historical_courses table with 7-day TTL for on-demand caching
// This function is called for courses older than the regular warmup range (4 semesters)
// Supports real-time scraping for any academic year since NTPU was founded
func (h *Handler) handleHistoricalCourseSearch(ctx context.Context, year int, keyword string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Validate year range: Course system launch year to current year
	// Course system supports real-time scraping from year 90 onwards
	currentYear := time.Now().Year() - 1911
	if year < config.CourseSystemLaunchYear || year > currentYear {
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("❌ 無效的學年度：%d\n\n📅 可搜尋範圍：%d-%d 學年度\n（民國 %d-%d 年 = 西元 %d-%d 年）\n\n範例：\n• 課程 110 微積分\n• 課 108 線性代數", year, config.CourseSystemLaunchYear, currentYear, config.CourseSystemLaunchYear, currentYear, config.CourseSystemLaunchYear+1911, currentYear+1911),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyCourseNav(h.bm25Index != nil && h.bm25Index.IsEnabled()))
		return []messaging_api.MessageInterface{msg}
	}

	log.Infof("Handling historical course search: year=%d, keyword=%s", year, keyword)

	// Check if the requested year is in the recent/active semesters (Hot Data).
	// If so, we query the 'courses' table instead of 'historical_courses' to use the pre-warmed cache.
	isRecent := false
	if h.semesterDetector != nil {
		recentYears, _ := h.semesterDetector.GetRecentSemesters()
		for _, recentYear := range recentYears {
			if year == recentYear {
				isRecent = true
				break
			}
		}
		// Also check extended semesters (3rd and 4th) as they are also in 'courses' table
		if !isRecent {
			extendedYears, _ := h.semesterDetector.GetExtendedSemesters()
			for _, extendedYear := range extendedYears {
				if year == extendedYear {
					isRecent = true
					break
				}
			}
		}
	}

	// If it's a recent year, redirect to the hot path logic
	if isRecent {
		log.Infof("Requested year %d is recent, using hot cache (courses table)", year)
		// Reuse the logic from handleRegularPattern but filtered by year
		var courses []storage.Course
		for _, term := range []int{1, 2} {
			termCourses, err := h.db.GetCoursesByYearTerm(ctx, year, term)
			if err != nil {
				log.WithError(err).Warnf("Failed to get courses for year %d term %d", year, term)
				continue
			}
			// Filter by keyword using fuzzy matching (Title or Teacher)
			for _, c := range termCourses {
				matched := stringutil.ContainsAllRunes(c.Title, keyword)
				if !matched {
					// Check teachers
					for _, teacher := range c.Teachers {
						if stringutil.ContainsAllRunes(teacher, keyword) {
							matched = true
							break
						}
					}
				}

				if matched {
					courses = append(courses, c)
				}
			}
		}

		if len(courses) > 0 {
			h.metrics.RecordCacheHit(ModuleName)
			// Limit results
			if len(courses) > MaxCoursesPerSearch {
				courses = courses[:MaxCoursesPerSearch]
			}
			return h.formatCourseListResponseForHistorical(courses)
		}
		// Cache miss for recent year: fall through to historical search/scraper path.
		// Data will be saved to the appropriate table (courses for recent, historical_courses for old)
		// based on the logic later in this function.
	}

	// Search in historical_courses cache first
	// Search by year (returns both semesters) from historical_courses table
	cachedCourses, err := h.db.SearchHistoricalCoursesByYear(ctx, year)
	if err != nil {
		log.WithError(err).Warnf("Failed to get historical courses for year %d", year)
	}

	var courses []storage.Course
	// Filter by keyword using fuzzy matching (Title OR Teacher)
	for _, c := range cachedCourses {
		// Check title
		if stringutil.ContainsAllRunes(c.Title, keyword) {
			courses = append(courses, c)
			continue
		}

		// Check teachers
		teacherMatch := false
		for _, teacher := range c.Teachers {
			if stringutil.ContainsAllRunes(teacher, keyword) {
				teacherMatch = true
				break
			}
		}
		if teacherMatch {
			courses = append(courses, c)
		}
	}

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(ModuleName)
		log.Infof("Found %d historical courses in cache for year=%d, keyword=%s", len(courses), year, keyword)
		// Limit results
		if len(courses) > MaxCoursesPerSearch {
			courses = courses[:MaxCoursesPerSearch]
		}
		return h.formatCourseListResponseForHistorical(courses)
	}

	// Cache miss - scrape from historical course system
	h.metrics.RecordCacheMiss(ModuleName)
	log.Infof("Cache miss for historical course: year=%d, keyword=%s, scraping...", year, keyword)

	// Use term=0 to query both semesters at once (more efficient)
	// Strategy: Dual scrape (Parallel-ish) to catch both Course Title and Teacher Name matches
	// 1. Scrape by Course Title (original logic)
	scrapedCoursesTitle, errTitle := ntpu.ScrapeCourses(ctx, h.scraper, year, 0, keyword)
	if errTitle != nil {
		log.WithError(errTitle).WithField("year", year).Warn("Failed to scrape historical courses by title")
	}

	// 2. Scrape by Teacher Name (new specific logic)
	scrapedCoursesTeacher, errTeacher := ntpu.ScrapeCoursesByTeacher(ctx, h.scraper, year, 0, keyword)
	if errTeacher != nil {
		log.WithError(errTeacher).WithField("year", year).Warn("Failed to scrape historical courses by teacher")
	}

	// Merge results (Deduplicate by UID)
	courseMap := make(map[string]*storage.Course)
	for _, c := range scrapedCoursesTitle {
		courseMap[c.UID] = c
	}
	for _, c := range scrapedCoursesTeacher {
		courseMap[c.UID] = c
	}

	// Convert back to slice
	scrapedCourses := make([]*storage.Course, 0, len(courseMap))
	for _, c := range courseMap {
		scrapedCourses = append(scrapedCourses, c)
	}

	// If both failed, treat as error
	if errTitle != nil && errTeacher != nil {
		log.Warn("Both title and teacher scraping failed")
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無 %d 學年度「%s」的課程\n\n請確認\n• 學年度和課程名稱是否正確\n• 該課程是否有開設", year, keyword),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📚 搜尋近期課程", "課程 "+keyword)},
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}
	log.Infof("Scraped %d historical courses for year=%d", len(scrapedCourses), year)

	// Save courses to correct table based on recency (Hot vs Cold)
	for _, course := range scrapedCourses {
		var err error
		if isRecent {
			err = h.db.SaveCourse(ctx, course)
		} else {
			err = h.db.SaveHistoricalCourse(ctx, course)
		}

		if err != nil {
			log.WithError(err).WithField("is_recent", isRecent).Warn("Failed to save course to cache")
		}
	}

	if len(scrapedCourses) > 0 {
		h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
		// Convert []*storage.Course to []storage.Course
		courses := make([]storage.Course, len(scrapedCourses))
		for i, c := range scrapedCourses {
			courses[i] = *c
		}
		return h.formatCourseListResponseForHistorical(courses)
	}

	// No results found
	h.metrics.RecordScraperRequest(ModuleName, "not_found", time.Since(startTime).Seconds())
	msg := lineutil.NewTextMessageWithConsistentSender(
		fmt.Sprintf("🔍 查無 %d 學年度「%s」的課程\n\n請確認\n• 學年度和課程名稱是否正確\n• 該課程是否有開設", year, keyword),
		sender,
	)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📚 搜尋近期課程", "課程 "+keyword)},
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// formatCourseResponseWithContext formats a single course as a LINE message with context for database queries.
// This allows querying related programs for the course.
func (h *Handler) formatCourseResponseWithContext(ctx context.Context, course *storage.Course) []messaging_api.MessageInterface {
	// Header: Course title with colored background (detail page style)
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: lineutil.FormatCourseTitleWithUID(course.Title, course.UID),
		Color: lineutil.ColorHeaderCourse,
	})

	// Build body contents using BodyContentBuilder for cleaner code
	body := lineutil.NewBodyContentBuilder()

	// Body label for detail page context (consistent with design guide)
	body.AddComponent(lineutil.NewBodyLabel(lineutil.BodyLabelInfo{
		Emoji: "📚",
		Label: "課程資訊",
		Color: lineutil.ColorHeaderCourse,
	}).FlexBox)

	// 學期 info - first row (no separator between label and first row)
	semesterText := lineutil.FormatSemester(course.Year, course.Term)
	firstInfoRow := lineutil.NewInfoRow("📅", "開課學期", semesterText, lineutil.DefaultInfoRowStyle())
	body.AddComponent(firstInfoRow.FlexBox)

	// 教師 info
	if len(course.Teachers) > 0 {
		teacherNames := strings.Join(course.Teachers, "、")
		body.AddInfoRow("👨‍🏫", "授課教師", teacherNames, lineutil.DefaultInfoRowStyle())
	}

	// 時間 info - 轉換節次為實際時間 (課程詳細使用 wrap=true 以完整顯示所有時間)
	if len(course.Times) > 0 {
		formattedTimes := lineutil.FormatCourseTimes(course.Times)
		timeStr := strings.Join(formattedTimes, "、")
		timeStyle := lineutil.DefaultInfoRowStyle()
		timeStyle.Wrap = true // Full display in course detail page
		body.AddInfoRow("⏰", "上課時間", timeStr, timeStyle)
	}

	// 地點 info
	if len(course.Locations) > 0 {
		locationStr := strings.Join(course.Locations, "、")
		body.AddInfoRow("📍", "上課地點", locationStr, lineutil.DefaultInfoRowStyle())
	}

	// 備註 info (課程詳細使用 wrap=true 允許較長備註顯示)
	if course.Note != "" {
		noteStyle := lineutil.DefaultInfoRowStyle()
		noteStyle.ValueSize = "xs"
		noteStyle.ValueColor = lineutil.ColorLabel // Use semantic color constant
		noteStyle.Wrap = true                      // Allow note to wrap in detail page
		body.AddInfoRow("📝", "備註", course.Note, noteStyle)
	}

	// Add cache time hint (unobtrusive, right-aligned)
	if hint := lineutil.NewCacheTimeHint(course.CachedAt); hint != nil {
		body.AddComponent(hint.FlexText)
	}

	// Build footer actions using button rows for 2-column layout
	var footerRows [][]*lineutil.FlexButton

	// Query course programs first to determine layout
	programs, err := h.db.GetCoursePrograms(ctx, course.UID)
	if err != nil {
		h.logger.WithModule(ModuleName).WithError(err).Warnf("Failed to get programs for course %s", course.UID)
	}

	// Build course query URL (used in different rows depending on whether programs exist)
	courseQueryURL := fmt.Sprintf("https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.queryByKeyword?qYear=%d&qTerm=%d&courseno=%s&seq1=A&seq2=M",
		course.Year, course.Term, course.No)

	// Row 1 layout depends on whether course has programs
	row1 := make([]*lineutil.FlexButton, 0, 2)
	if course.DetailURL != "" {
		row1 = append(row1, lineutil.NewFlexButton(
			lineutil.NewURIAction("📄 課程大綱", course.DetailURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))
	}

	// If no programs, add query system to row 1; otherwise query system goes to row 2
	if len(programs) == 0 {
		row1 = append(row1, lineutil.NewFlexButton(
			lineutil.NewURIAction("🔍 查詢系統", courseQueryURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))
	}

	if len(row1) > 0 {
		footerRows = append(footerRows, row1)
	}

	// Row 2: 查詢系統 + 學程 (if course has programs)
	if len(programs) > 0 {
		row2 := make([]*lineutil.FlexButton, 0, 2)

		// Add query system button
		row2 = append(row2, lineutil.NewFlexButton(
			lineutil.NewURIAction("🔍 查詢系統", courseQueryURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))

		// Add program button
		if len(programs) == 1 {
			// Single program: show program info (same as multiple programs)
			firstProgram := programs[0]
			displayText := lineutil.FormatLabel("查看學程", firstProgram.ProgramName, 40)
			row2 = append(row2, lineutil.NewFlexButton(
				lineutil.NewPostbackActionWithDisplayText(
					"🎓 相關學程",
					displayText,
					fmt.Sprintf("program:course_programs%s%s", bot.PostbackSplitChar, course.UID),
				),
			).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm"))
		} else {
			// Multiple programs: show count and link to list
			moreText := fmt.Sprintf("查看 %d 個相關學程", len(programs))
			row2 = append(row2, lineutil.NewFlexButton(
				lineutil.NewPostbackActionWithDisplayText(
					"🎓 相關學程",
					moreText,
					fmt.Sprintf("program:course_programs%s%s", bot.PostbackSplitChar, course.UID),
				),
			).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm"))
		}

		if len(row2) > 0 {
			footerRows = append(footerRows, row2)
		}
	}

	// Row 3: 教師課表 + 教師課程 (if teachers exist)
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		row3 := make([]*lineutil.FlexButton, 0, 2)

		// Teacher schedule button - opens the teacher's course table webpage (外部連結使用藍色)
		if len(course.TeacherURLs) > 0 && course.TeacherURLs[0] != "" {
			row3 = append(row3, lineutil.NewFlexButton(
				lineutil.NewURIAction("📅 教師課表", course.TeacherURLs[0]),
			).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))
		}

		// Teacher all courses button - searches for all courses taught by this teacher (內部指令使用紫色)
		displayText := lineutil.FormatLabel("搜尋近期課程", teacherName, 40)
		row3 = append(row3, lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText(
				"👨‍🏫 教師課程",
				displayText,
				fmt.Sprintf("course:授課課程%s%s", bot.PostbackSplitChar, teacherName),
			),
		).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm"))

		if len(row3) > 0 {
			footerRows = append(footerRows, row3)
		}
	}

	// Row 4: Dcard 查詢 + 選課大全
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		row4 := make([]*lineutil.FlexButton, 0, 2)

		// Dcard search button - Google search with site:dcard.tw/f/ntpu (外部連結使用藍色)
		dcardQuery := fmt.Sprintf("%s %s site:dcard.tw/f/ntpu", teacherName, course.Title)
		dcardURL := "https://www.google.com/search?q=" + url.QueryEscape(dcardQuery)
		row4 = append(row4, lineutil.NewFlexButton(
			lineutil.NewURIAction("💬 Dcard", dcardURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))

		// 選課大全 button (外部連結使用藍色)
		courseSelectionQuery := fmt.Sprintf("%s %s", teacherName, course.Title)
		courseSelectionURL := "https://no21.ntpu.org/?s=" + url.QueryEscape(courseSelectionQuery)
		row4 = append(row4, lineutil.NewFlexButton(
			lineutil.NewURIAction("📖 選課大全", courseSelectionURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))

		if len(row4) > 0 {
			footerRows = append(footerRows, row4)
		}
	}

	footer := lineutil.NewButtonFooter(footerRows...)

	bubble := lineutil.NewFlexBubble(
		header,
		nil, // No hero - colored header already contains title
		body.Build(),
		footer,
	)

	// Limit altText to 400 chars (LINE API limit, using rune slicing for UTF-8 safety)
	altText := lineutil.FormatLabel("課程", course.Title, 400)
	msg := lineutil.NewFlexMessage(altText, bubble.FlexBubble)
	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg.Sender = sender

	// Add Quick Reply for related actions
	// Include teacher-specific search if teacher info is available
	quickReplyItems := []lineutil.QuickReplyItem{
		lineutil.QuickReplyCourseAction(),
	}
	if len(course.Teachers) > 0 {
		// Add option to search for more courses by the same teacher
		teacherName := course.Teachers[0]
		quickReplyItems = append(quickReplyItems,
			lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("👨‍🏫 "+teacherName+"的課程", "課程 "+teacherName)},
		)
	}
	quickReplyItems = append(quickReplyItems, lineutil.QuickReplyHelpAction())
	msg.QuickReply = lineutil.NewQuickReply(quickReplyItems)

	return []messaging_api.MessageInterface{msg}
}

// filterCoursesBySemesters filters a list of courses to include only those matching the specified years and terms.
// years and terms slices must be of equal length and correspond positionally.
func filterCoursesBySemesters(courses []storage.Course, years, terms []int) []storage.Course {
	if len(years) == 0 || len(terms) == 0 || len(years) != len(terms) {
		return courses
	}

	// Create a map for O(1) lookup: "year-term" -> bool
	allowed := make(map[string]bool)
	for i := range years {
		key := fmt.Sprintf("%d-%d", years[i], terms[i])
		allowed[key] = true
	}

	filtered := make([]storage.Course, 0, len(courses))
	for _, c := range courses {
		key := fmt.Sprintf("%d-%d", c.Year, c.Term)
		if allowed[key] {
			filtered = append(filtered, c)
		}
	}

	return filtered
}

// extractUniqueSemesters extracts unique semesters from a course list.
// Returns a slice of SemesterPair sorted by semester (newest first).
// The function internally sorts the result, so input order doesn't matter.
//
// This is used for data-driven label calculation:
// - Index 0: 最新學期 (newest semester with data)
// - Index 1: 上個學期 (second newest)
// - Index 2+: 過去學期 (older semesters)
func extractUniqueSemesters(courses []storage.Course) []lineutil.SemesterPair {
	seen := make(map[string]bool)
	var semesters []lineutil.SemesterPair

	for _, c := range courses {
		key := fmt.Sprintf("%d-%d", c.Year, c.Term)
		if !seen[key] {
			seen[key] = true
			semesters = append(semesters, lineutil.SemesterPair{
				Year: c.Year,
				Term: c.Term,
			})
		}
	}

	// Sort by year descending, then term descending (newest first)
	slices.SortFunc(semesters, func(a, b lineutil.SemesterPair) int {
		if a.Year != b.Year {
			return b.Year - a.Year // Descending
		}
		return b.Term - a.Term // Descending
	})

	return semesters
}

// FormatOptions configures course list formatting behavior.
type FormatOptions struct {
	SearchKeyword    string // Original search keyword (for "more semesters" Quick Reply)
	IsExtendedSearch bool   // True if this is already an extended (4-semester) search
	IsHistorical     bool   // True for historical queries (uses semester text as label instead of "最新學期")
}

// formatCourseListResponse formats a list of courses as LINE messages with semester labels.
// Courses are sorted by semester (newest first) and each bubble shows a label indicating
// whether it's from the newest semester in data, previous semester, or older.
func (h *Handler) formatCourseListResponse(courses []storage.Course) []messaging_api.MessageInterface {
	return h.formatCourseListResponseWithOptions(courses, FormatOptions{})
}

// formatCourseListResponseForHistorical formats courses with semester-only labels.
// Used for historical course searches where relative labels ("最新學期") are misleading.
func (h *Handler) formatCourseListResponseForHistorical(courses []storage.Course) []messaging_api.MessageInterface {
	return h.formatCourseListResponseWithOptions(courses, FormatOptions{IsHistorical: true})
}

// formatCourseListResponseWithOptions formats courses with extended options.
// Parameters:
//   - courses: List of courses to display
//   - opts: Formatting options (search keyword, extended/historical flags)
func (h *Handler) formatCourseListResponseWithOptions(courses []storage.Course, opts FormatOptions) []messaging_api.MessageInterface {
	if len(courses) == 0 {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender("🔍 查無課程資料", sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyCourseNav(h.bm25Index != nil && h.bm25Index.IsEnabled()))
		return []messaging_api.MessageInterface{msg}
	}

	// Sort courses: year descending (recent first), then term descending (term 2 before term 1)
	slices.SortFunc(courses, func(a, b storage.Course) int {
		if a.Year != b.Year {
			return b.Year - a.Year // Year: recent first
		}
		return b.Term - a.Term // Term: 2 (下學期) before 1 (上學期)
	})

	// Extract unique semesters from sorted courses (data-driven, not calendar-based)
	// This ensures label is based on actual data availability:
	// - Index 0: 最新學期 (newest semester with data)
	// - Index 1: 上個學期 (second newest)
	// - Index 2+: 過去學期 (older semesters)
	dataSemesters := extractUniqueSemesters(courses)

	sender := lineutil.GetSender(senderName, h.stickerManager)
	var messages []messaging_api.MessageInterface

	// Limit to 40 courses - track if truncated for warning message
	originalCount := len(courses)
	truncated := len(courses) > MaxCoursesPerSearch
	if truncated {
		courses = courses[:MaxCoursesPerSearch]
	}

	// Create bubbles for carousel (LINE API limit: max 10 bubbles per Flex Carousel)
	bubbles := make([]messaging_api.FlexBubble, 0, len(courses))
	for _, course := range courses {
		// Get label info based on mode:
		// - Historical mode: Use semester text directly as label (e.g., "📅 113 學年度 下學期")
		// - Regular mode: Use relative labels (最新學期/上個學期/過去學期)
		var labelInfo lineutil.BodyLabelInfo
		if opts.IsHistorical {
			// Historical: show explicit semester (no "開課學期" info row needed)
			semesterText := lineutil.FormatSemester(course.Year, course.Term)
			labelInfo = lineutil.BodyLabelInfo{
				Emoji: "📅",
				Label: semesterText,
				Color: lineutil.ColorHeaderHistorical, // Use historical color for all
			}
		} else {
			// Regular: use relative labels based on data position
			labelInfo = lineutil.GetSemesterLabel(course.Year, course.Term, dataSemesters)
		}

		// Colored header with course title
		heroTitle := lineutil.FormatCourseTitleWithUID(course.Title, course.UID)
		header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
			Title: heroTitle,
			Color: labelInfo.Color,
		})

		// Build body contents using BodyContentBuilder for cleaner code
		body := lineutil.NewBodyContentBuilder()

		// First row is semester label
		body.AddComponent(lineutil.NewBodyLabel(labelInfo).FlexBox)

		// For regular mode, add semester info row; for historical mode, skip (already in label)
		if !opts.IsHistorical {
			semesterText := lineutil.FormatSemester(course.Year, course.Term)
			firstInfoRow := lineutil.NewInfoRow("📅", "開課學期", semesterText, lineutil.DefaultInfoRowStyle())
			body.AddComponent(firstInfoRow.FlexBox)
		}

		// 第二列：授課教師 - use shrink-to-fit for maximum content display
		if len(course.Teachers) > 0 {
			teacherNames := strings.Join(course.Teachers, "、")
			body.AddInfoRow("👨‍🏫", "授課教師", teacherNames, lineutil.CarouselInfoRowStyle())
		}

		// 第三列：上課時間 - use shrink-to-fit for maximum content display
		if len(course.Times) > 0 {
			formattedTimes := lineutil.FormatCourseTimes(course.Times)
			timeStr := strings.Join(formattedTimes, "、")
			body.AddInfoRow("⏰", "上課時間", timeStr, lineutil.CarouselInfoRowStyle())
		}

		// Footer with "View Detail" button - displayText shows course title
		// Button color syncs with header for visual harmony
		displayText := lineutil.FormatLabel("查詢課程資訊", course.Title, 40)
		// Use course: prefix for proper postback routing
		footer := lineutil.NewFlexBox("vertical",
			lineutil.NewFlexButton(
				lineutil.NewPostbackActionWithDisplayText("📝 查看詳細", displayText, "course:"+course.UID),
			).WithStyle("primary").WithColor(labelInfo.Color).WithHeight("sm").FlexButton,
		).WithSpacing("sm")

		bubble := lineutil.NewFlexBubble(
			header,
			nil, // No hero - title is in colored header
			body.Build(),
			footer,
		)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Build carousel messages with automatic splitting (max 10 bubbles per carousel)
	// Limit to 4 carousels to leave room for warning message (LINE API: max 5 messages per reply)
	for i := 0; i < len(bubbles); i += lineutil.MaxBubblesPerCarousel {
		// Respect LINE reply limit (max 5 messages, reserve 1 for warning if truncated)
		maxCarousels := 5
		if truncated {
			maxCarousels = 4 // Reserve 1 message slot for warning
		}
		if len(messages) >= maxCarousels {
			break
		}

		end := i + lineutil.MaxBubblesPerCarousel
		if end > len(bubbles) {
			end = len(bubbles)
		}

		carousel := lineutil.NewFlexCarousel(bubbles[i:end])
		altText := "課程列表"
		if i > 0 {
			altText = fmt.Sprintf("課程列表 (%d-%d)", i+1, end)
		}
		msg := lineutil.NewFlexMessage(altText, carousel)
		msg.Sender = sender
		messages = append(messages, msg)
	}

	// Append warning message at the end if results were truncated
	if truncated {
		warningMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 搜尋結果有 %d 門課程，僅顯示前 %d 門\n\n建議使用更精確的搜尋條件以縮小範圍", originalCount, MaxCoursesPerSearch),
			sender,
		)
		messages = append(messages, warningMsg)
	}

	// Build Quick Reply items based on context
	var quickReplyItems []lineutil.QuickReplyItem

	// Add "更多" (More) button FIRST for visibility when search keyword exists
	// Uses compact label "📅 更多" for cleaner UX, but outputs "更多學期 {keyword}"
	if !opts.IsExtendedSearch && opts.SearchKeyword != "" {
		quickReplyItems = append(quickReplyItems, lineutil.QuickReplyMoreCoursesCompact(opts.SearchKeyword))
	}

	quickReplyItems = append(quickReplyItems, lineutil.QuickReplyCourseAction())

	// Add smart search option if enabled
	if h.bm25Index != nil && h.bm25Index.IsEnabled() {
		// Preserve original keyword (if any) so users can switch to smart search seamlessly.
		if opts.SearchKeyword != "" {
			quickReplyItems = append(quickReplyItems,
				lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("🔮 找課", "找課 "+opts.SearchKeyword)},
			)
		} else {
			quickReplyItems = append(quickReplyItems, lineutil.QuickReplySmartSearchAction())
		}
	}

	quickReplyItems = append(quickReplyItems, lineutil.QuickReplyHelpAction())

	// Add Quick Reply to the last message
	lineutil.AddQuickReplyToMessages(messages, quickReplyItems...)

	return messages
}

// handleSmartSearch performs smart search using BM25 + Query Expansion.
// This is triggered by "找課" keywords and searches course syllabi content.
// Search range: Newest semester only (ensures current/most recent course offerings).
//
// Timeout hierarchy (nested within 60s webhook processing timeout):
//   - SmartSearchTimeout: 30s total (detached context from HTTP request)
//   - QueryExpander: 8s nested timeout within search context
//   - Actual search: remainder of 30s after expansion completes
//
// Total operation is bounded by SmartSearchTimeout (30s), well within
// the 60s webhook limit. Reply token remains valid for ~20 minutes.
func (h *Handler) handleSmartSearch(ctx context.Context, query string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()

	// Check if BM25 search is enabled
	bm25Enabled := h.bm25Index != nil && h.bm25Index.IsEnabled()

	if !bm25Enabled {
		log.Info("Smart search not enabled")
		h.metrics.RecordSearch("disabled", "skipped", time.Since(startTime).Seconds(), 0)
		sender := lineutil.GetSender(senderName, h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply(
				"智慧搜尋目前未啟用\n\n建議使用精確搜尋\n• 課程 微積分\n• 課程 王小明",
				sender,
				"課程 "+query,
				lineutil.QuickReplyCourseNav(false)...,
			),
		}
	}

	searchType := "bm25"

	// Use detached context for API calls (Query Expansion LLM + BM25 search).
	// PreserveTracing() preserves tracing values (request ID, user ID, chat ID)
	// for observability while preventing cancellation from parent timeout.
	// This ensures LLM API calls complete even if HTTP request is canceled.
	// Safer than WithoutCancel (avoids memory leaks from parent references).
	searchCtx, cancel := context.WithTimeout(ctxutil.PreserveTracing(ctx), config.SmartSearchTimeout)
	defer cancel()

	// Expand query for better search results (adds synonyms, translations, related terms)
	// Examples: "AWS" → "AWS Amazon Web Services 雲端服務 雲端運算 cloud computing"
	//
	// LLM Rate Limiting Strategy:
	// - NLU-routed requests: Already checked at webhook layer before reaching here
	// - Keyword-triggered requests: Check here using chatID from context
	//
	// This design maintains low coupling - the course handler doesn't need to know
	// about webhook sources or user sessions, it just uses the chatID from context.
	expandedQuery := query
	if h.hasQueryExpander() {
		// Check LLM rate limit if limiter is available and we have a chatID in context
		// The chatID is injected by webhook handler via ctxutil.WithChatID
		chatID := ctxutil.GetChatID(ctx)
		if h.llmRateLimiter != nil && chatID != "" {
			if !h.llmRateLimiter.Allow(chatID) {
				log.WithField("chat_id", chatID[:min(8, len(chatID))]+"...").Debug("LLM rate limit exceeded for query expansion, using original query")
				// Graceful degradation: continue with original query instead of failing
			} else {
				// Rate limit OK, proceed with expansion
				expanded, err := h.queryExpander.Expand(searchCtx, query)
				if err != nil {
					log.WithError(err).Debug("Query expansion failed, using original query")
				} else if expanded != query {
					expandedQuery = expanded
					log.WithFields(map[string]any{
						"original": query,
						"expanded": expandedQuery,
					}).Debug("Query expanded")
				}
			}
		} else {
			// No rate limiting configured, proceed with expansion
			expanded, err := h.queryExpander.Expand(searchCtx, query)
			if err != nil {
				log.WithError(err).Debug("Query expansion failed, using original query")
			} else if expanded != query {
				expandedQuery = expanded
				log.WithFields(map[string]any{
					"original": query,
					"expanded": expandedQuery,
				}).Debug("Query expanded")
			}
		}
	}

	log.WithFields(map[string]any{
		"type":     searchType,
		"original": query,
		"expanded": expandedQuery,
	}).Infof("Performing smart search")

	// Perform BM25 search
	results, err := h.bm25Index.SearchCourses(searchCtx, expandedQuery, 10)

	if err != nil {
		log.WithError(err).Warn("Smart search failed")
		h.metrics.RecordSearch(searchType, "error", time.Since(startTime).Seconds(), 0)
		sender := lineutil.GetSender(senderName, h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply(
				"智慧搜尋暫時無法使用\n\n建議稍後再試，或使用精確搜尋",
				sender,
				"找課 "+query,
				lineutil.QuickReplyCourseNav(h.IsBM25SearchEnabled())...,
			),
		}
	}

	if len(results) == 0 {
		log.Info("No smart search results found")
		h.metrics.RecordSearch(searchType, "no_results", time.Since(startTime).Seconds(), 0)
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"🔍 未找到相關課程\n\n💡 建議嘗試\n• 換個描述方式或關鍵字\n• 使用精確搜尋：「課程 課名」", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplySmartSearchAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Convert search results to Course objects for display
	var courses []storage.Course
	for _, result := range results {
		// Get full course data from DB
		course, err := h.db.GetCourseByUID(ctx, result.UID)
		if err != nil || course == nil {
			// Use data from search result if course not in DB
			courses = append(courses, storage.Course{
				UID:      result.UID,
				Title:    result.Title,
				Teachers: result.Teachers,
				Year:     result.Year,
				Term:     result.Term,
			})
		} else {
			courses = append(courses, *course)
		}
	}

	// Record successful smart search metrics
	h.metrics.RecordSearch(searchType, "success", time.Since(startTime).Seconds(), len(results))

	// Format response with confidence labels
	return h.formatSmartSearchResponse(courses, results)
}

// formatSmartSearchResponse formats smart search results grouped by semester.
// Results are separated into newest and previous semester groups (10 each max).
// Each semester gets its own carousel row for clear visual separation.
func (h *Handler) formatSmartSearchResponse(courses []storage.Course, results []rag.SearchResult) []messaging_api.MessageInterface {
	if len(courses) == 0 {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender("🔍 未找到相關課程\n\n💡 建議嘗試\n• 換個描述方式或關鍵字\n• 使用精確搜尋：「課程 課名」", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplySmartSearchAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Create confidence map for sorting within each semester
	confidenceMap := make(map[string]float32)
	for _, r := range results {
		confidenceMap[r.UID] = r.Confidence
	}

	// Extract unique semesters from courses (sorted newest first)
	dataSemesters := extractUniqueSemesters(courses)

	// Group courses by semester
	semesterCourses := make(map[lineutil.SemesterPair][]storage.Course)
	for _, course := range courses {
		sem := lineutil.SemesterPair{Year: course.Year, Term: course.Term}
		semesterCourses[sem] = append(semesterCourses[sem], course)
	}

	// Sort each semester's courses by confidence (best first)
	for sem := range semesterCourses {
		slices.SortFunc(semesterCourses[sem], func(a, b storage.Course) int {
			confA, confB := confidenceMap[a.UID], confidenceMap[b.UID]
			if confA > confB {
				return -1
			}
			if confA < confB {
				return 1
			}
			return 0
		})
	}

	// Build one carousel per semester (each semester = one row)
	// Pre-allocate for max 2 semesters × 2 messages (text header + carousel) = 4
	const maxPerSemester = 10
	messages := make([]messaging_api.MessageInterface, 0, 4)

	for i, sem := range dataSemesters {
		// Only show top 2 semesters (最新學期 + 上個學期)
		if i >= 2 {
			break
		}

		semCourses := semesterCourses[sem]
		if len(semCourses) == 0 {
			continue
		}

		// Limit to 10 per semester
		if len(semCourses) > maxPerSemester {
			semCourses = semCourses[:maxPerSemester]
		}

		// Build bubbles for this semester
		var bubbles []messaging_api.FlexBubble
		for _, course := range semCourses {
			confidence := confidenceMap[course.UID]
			bubble := h.buildSmartCourseBubble(course, confidence)
			bubbles = append(bubbles, *bubble.FlexBubble)
		}

		// Create header text message for this semester
		// Use human-friendly format: "113 學年度 下學期" instead of "113-2"
		semLabel := lineutil.FormatSemester(sem.Year, sem.Term)
		headerText := fmt.Sprintf("📚 %s 相關課程", semLabel)
		headerMsg := lineutil.NewTextMessageWithConsistentSender(headerText, sender)
		messages = append(messages, headerMsg)

		// Create carousel for this semester
		carousel := lineutil.NewFlexCarousel(bubbles)
		msg := lineutil.NewFlexMessage("🔮 智慧搜尋結果", carousel)
		msg.Sender = sender
		messages = append(messages, msg)
	}

	// Add Quick Reply to the last message
	lineutil.AddQuickReplyToMessages(messages,
		lineutil.QuickReplySmartSearchAction(),
		lineutil.QuickReplyCourseAction(),
		lineutil.QuickReplyHelpAction(),
	)

	return messages
}

// buildSmartCourseBubble creates a Flex Message bubble for smart search with relevance labels.
// Uses getRelevanceLabel for confidence-based tags (green/teal gradient for relevance).
func (h *Handler) buildSmartCourseBubble(course storage.Course, confidence float32) *lineutil.FlexBubble {
	// Get relevance label info (based on BM25 confidence)
	labelInfo := getRelevanceLabel(confidence)

	// Colored header with course title
	heroTitle := lineutil.FormatCourseTitleWithUID(course.Title, course.UID)
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: heroTitle,
		Color: labelInfo.Color,
	})

	// Build body contents using BodyContentBuilder
	body := lineutil.NewBodyContentBuilder()

	// First row is relevance label (🎯最佳匹配/✨高度相關/📋部分相關)
	// Note: Semester info is already in the header text message, so we don't repeat it here
	body.AddComponent(lineutil.NewBodyLabel(labelInfo).FlexBox)

	// 授課教師 - use shrink-to-fit for maximum content display
	if len(course.Teachers) > 0 {
		teacherNames := strings.Join(course.Teachers, "、")
		body.AddInfoRow("👨‍🏫", "授課教師", teacherNames, lineutil.CarouselInfoRowStyle())
	}

	// 上課時間 - use shrink-to-fit for maximum content display
	if len(course.Times) > 0 {
		formattedTimes := lineutil.FormatCourseTimes(course.Times)
		timeStr := strings.Join(formattedTimes, "、")
		body.AddInfoRow("⏰", "上課時間", timeStr, lineutil.CarouselInfoRowStyle())
	}

	// Footer with "View Detail" button
	// Button color syncs with header for visual harmony
	displayText := lineutil.FormatLabel("查詢課程資訊", course.Title, 40)
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText("📝 查看詳細", displayText, "course:"+course.UID),
		).WithStyle("primary").WithColor(labelInfo.Color).WithHeight("sm").FlexButton,
	).WithSpacing("sm")

	return lineutil.NewFlexBubble(
		header,
		nil, // No hero - title is in colored header
		body.Build(),
		footer,
	)
}

// getRelevanceLabel returns a user-friendly relevance label info based on relative BM25 score.
//
// Returns: BodyLabelInfo with:
//   - Emoji: Visual relevance indicator ("🎯" best, "✨" high, "📋" medium)
//   - Label: User-friendly text ("最佳匹配", "高度相關", "部分相關")
//   - Color: Used for both header background and body label text for visual coordination
//
// Design rationale:
//   - Uses relative score (score / maxScore) from BM25 search
//   - Simple 3-tier system: Clear differentiation without cognitive overload
//   - Relative scoring: Comparable within the same query results
//   - Color coordination: Same color used for header background and body label text
//   - Sequential green/teal gradient: Deep teal (best) → Teal → Emerald (partial)
//     All colors meet WCAG AA (≥4.5:1) for accessibility
//     Aligns with universal "green = good/relevant" intuition
//     Distinguishes clearly from semester labels (blue gradient system)
//
// Academic foundation (Arampatzis et al., 2009):
//   - BM25 follows Normal-Exponential mixture distribution
//   - Relevant docs: Normal distribution (high scores)
//   - Non-relevant docs: Exponential distribution (low scores)
//   - Relative thresholds work better than absolute ones
//
// Categories (based on confidence = score / maxScore):
//   - Confidence >= 0.8: "最佳匹配" (Best Match) - Normal distribution core
//   - Confidence >= 0.6: "高度相關" (Highly Relevant) - Mixed region
//   - Confidence < 0.6: "部分相關" (Partially Relevant) - Exponential tail
func getRelevanceLabel(confidence float32) lineutil.BodyLabelInfo {
	if confidence >= 0.8 {
		// Deep teal - strongest relevance, deep and distinct
		return lineutil.BodyLabelInfo{
			Emoji: "🎯",
			Label: "最佳匹配",
			Color: lineutil.ColorHeaderBest,
		}
	}
	if confidence >= 0.6 {
		// Teal - high relevance, stable and trustworthy
		return lineutil.BodyLabelInfo{
			Emoji: "✨",
			Label: "高度相關",
			Color: lineutil.ColorHeaderHigh,
		}
	}
	// Emerald - moderate relevance, softer appearance
	return lineutil.BodyLabelInfo{
		Emoji: "📋",
		Label: "部分相關",
		Color: lineutil.ColorHeaderMedium,
	}
}
