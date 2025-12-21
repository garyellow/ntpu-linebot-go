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
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles course-related queries.
// It depends on *storage.DB directly for data access.
type Handler struct {
	db             *storage.DB
	scraper        *scraper.Client
	metrics        *metrics.Metrics
	logger         *logger.Logger
	stickerManager *sticker.Manager
	bm25Index      *rag.BM25Index
	queryExpander  genai.QueryExpander // Interface for multi-provider support
	llmRateLimiter *ratelimit.LLMRateLimiter
}

// Name returns the module name
func (h *Handler) Name() string {
	return ModuleName
}

// Course handler constants.
const (
	ModuleName           = "course" // Module identifier for registration
	senderName           = "課程小幫手"
	MaxCoursesPerSearch  = 40 // Maximum courses to return (40 courses = 4 carousels @ 10 bubbles each), leaving 1 slot for warning (LINE API max: 5 messages)
	MaxTitleDisplayChars = 60 // Maximum characters for course title display before truncation
)

// Valid keywords for course queries
var (
	// Unified course search keywords (includes both course and teacher keywords)
	// All keywords trigger the same unified search that matches both title and teacher
	validCourseKeywords = []string{
		// 中文課程關鍵字
		"課", "課程", "科目",
		"課名", "課程名", "課程名稱",
		"科目名", "科目名稱",
		// 中文教師關鍵字（統一使用課程關鍵字搜尋教師）
		"師", "老師", "教師", "教授",
		"老師名", "教師名", "教授名",
		"老師名稱", "教師名稱", "教授名稱",
		"授課教師", "授課老師", "授課教授",
		// English keywords
		"class", "course",
		"teacher", "professor", "prof", "dr", "doctor",
	}

	// Smart search keywords (direct BM25 smart search)
	// 找課: directly triggers smart search without keyword fallback
	validSmartSearchKeywords = []string{
		"找課", "找課程", "搜課",
	}

	// Extended search keywords (searches 4 semesters instead of 2)
	// Triggered by "📅 更多學期" Quick Reply
	// "歷史課程" kept for backward compatibility
	validExtendedSearchKeywords = []string{
		"更多學期", "歷史課程",
	}

	courseRegex            = bot.BuildKeywordRegex(validCourseKeywords)
	smartSearchCourseRegex = bot.BuildKeywordRegex(validSmartSearchKeywords)
	extendedSearchRegex    = bot.BuildKeywordRegex(validExtendedSearchKeywords)
	// UID format: {year}{term}{no} where:
	// - year: 2-3 digits (e.g., 113, 99)
	// - term: 1 digit (1=上學期, 2=下學期)
	// - no: course number starting with U/M/N/P (case-insensitive) + 4 digits
	// Full UID example: 1131U0001 (year=113, term=1, no=U0001) or 991U0001
	// Regex matches: 3-4 digits (year+term) + U/M/N/P + 4 digits
	uidRegex = regexp.MustCompile(`(?i)\d{3,4}[umnp]\d{4}`)
	// Course number only: {no} (e.g., U0001, M0002)
	// Format: U/M/N/P (education level) + 4 digits
	courseNoRegex = regexp.MustCompile(`(?i)^[umnp]\d{4}$`)
	// Historical course query format: "課程 {year} {keyword}" or "課 {year} {keyword}"
	// e.g., "課程 110 微積分", "課 108 程式設計"
	// Year is in ROC format (e.g., 110 = AD 2021)
	// This pattern is checked BEFORE the regular courseRegex to handle historical queries
	historicalCourseRegex = regexp.MustCompile(`(?i)^(課程?|course|class)\s+(\d{2,3})\s+(.+)$`)
)

// NewHandler creates a new course handler with required dependencies.
// Optional dependencies (bm25Index, queryExpander, llmRateLimiter) can be nil.
func NewHandler(
	db *storage.DB,
	scraper *scraper.Client,
	metrics *metrics.Metrics,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
	bm25Index *rag.BM25Index,
	queryExpander genai.QueryExpander, // Interface for multi-provider support
	llmRateLimiter *ratelimit.LLMRateLimiter,
) *Handler {
	return &Handler{
		db:             db,
		scraper:        scraper,
		metrics:        metrics,
		logger:         logger,
		stickerManager: stickerManager,
		bm25Index:      bm25Index,
		queryExpander:  queryExpander,
		llmRateLimiter: llmRateLimiter,
	}
}

// IsBM25SearchEnabled returns true if BM25 search is enabled.
func (h *Handler) IsBM25SearchEnabled() bool {
	return h.bm25Index != nil && h.bm25Index.IsEnabled()
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

// DispatchIntent handles NLU-parsed intents for the course module.
// It validates required parameters and calls the appropriate handler method.
//
// Supported intents:
//   - "search": requires "keyword" param, calls handleUnifiedCourseSearch
//   - "smart": requires "query" param, calls handleSmartSearch
//   - "uid": requires "uid" param, calls handleCourseUIDQuery
//
// Returns error if intent is unknown or required parameters are missing.
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

// CanHandle checks if the message is for the course module
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)

	// Check for course UID pattern (full: 11312U0001)
	if uidRegex.MatchString(text) {
		return true
	}

	// Check for course number only pattern (e.g., U0001, M0002)
	if courseNoRegex.MatchString(text) {
		return true
	}

	// Check for smart search keywords (找課)
	if smartSearchCourseRegex.MatchString(text) {
		return true
	}

	// Check for course keywords (unified: includes both course and teacher keywords)
	if courseRegex.MatchString(text) {
		return true
	}

	return false
}

// HandleMessage handles text messages for the course module
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	text = strings.TrimSpace(text)

	log.Debugf("Handling course message: %s", text)

	// Check for full course UID first (highest priority, e.g., 11312U0001)
	if match := uidRegex.FindString(text); match != "" {
		return h.handleCourseUIDQuery(ctx, match)
	}

	// Check for course number only (e.g., U0001, M0002)
	// Will search in current and previous semester
	if courseNoRegex.MatchString(text) {
		return h.handleCourseNoQuery(ctx, text)
	}

	// Check for historical course query pattern BEFORE regular course search
	// Format: "課程 {year} {keyword}" e.g., "課程 110 微積分"
	if matches := historicalCourseRegex.FindStringSubmatch(text); len(matches) == 4 {
		yearStr := matches[2]
		keyword := strings.TrimSpace(matches[3])
		year := 0
		if _, err := fmt.Sscanf(yearStr, "%d", &year); err == nil && keyword != "" {
			return h.handleHistoricalCourseSearch(ctx, year, keyword)
		}
	}

	// Check for smart search keywords (找課) - direct smart search
	if smartSearchCourseRegex.MatchString(text) {
		match := smartSearchCourseRegex.FindString(text)
		searchTerm := bot.ExtractSearchTerm(text, match)

		if searchTerm == "" {
			sender := lineutil.GetSender(senderName, h.stickerManager)
			// Check if smart search is actually enabled
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

	// Check for extended search keywords (更多學期) - searches 4 semesters
	// This is triggered by "📅 更多學期" Quick Reply
	if extendedSearchRegex.MatchString(text) {
		match := extendedSearchRegex.FindString(text)
		searchTerm := bot.ExtractSearchTerm(text, match)

		if searchTerm == "" {
			sender := lineutil.GetSender(senderName, h.stickerManager)
			helpText := "📅 更多學期搜尋說明\n\n" +
				"🔍 搜尋範圍：近 4 學期\n" +
				"（一般搜尋僅搜尋近 2 學期）\n\n" +
				"用法範例：\n" +
				"• 更多學期 微積分\n" +
				"• 更多學期 王小明\n\n" +
				"📆 需要指定年份？\n" +
				"使用：「課程 110 微積分」"
			msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				lineutil.QuickReplyCourseAction(),
				lineutil.QuickReplyHelpAction(),
			})
			return []messaging_api.MessageInterface{msg}
		}

		return h.handleExtendedCourseSearch(ctx, searchTerm)
	}

	// Check for course title search - extract term after keyword
	// Support both "keyword term" and "term keyword" patterns
	// Unified search: matches both course title and teacher name
	if courseRegex.MatchString(text) {
		match := courseRegex.FindString(text)
		searchTerm := bot.ExtractSearchTerm(text, match)

		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			var helpText string
			var quickReplyItems []lineutil.QuickReplyItem
			if h.bm25Index != nil && h.bm25Index.IsEnabled() {
				// Smart search enabled - mention it as an option
				helpText = "📚 課程查詢方式\n\n" +
					"🔍 精確搜尋（近 2 學期）\n" +
					"• 課程 微積分\n" +
					"• 課程 王小明\n" +
					"• 課程 線代 王\n\n" +
					"🔮 智慧搜尋（近 2 學期）\n" +
					"• 找課 想學資料分析\n" +
					"• 找課 Python 入門\n\n" +
					"📅 更多學期（近 4 學期）\n" +
					"• 更多學期 微積分\n\n" +
					"📆 指定年份\n" +
					"• 課程 110 微積分\n\n" +
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
					"📅 更多學期（近 4 學期）\n" +
					"• 更多學期 微積分\n\n" +
					"📆 指定年份\n" +
					"• 課程 110 微積分\n\n" +
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

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the course module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	log.Infof("Handling course postback: %s", data)

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
	// Extract the actual UID from data (e.g., "course:1132U2236" -> "1132U2236")
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
		return h.formatCourseResponse(course)
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
	return h.formatCourseResponse(course)
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

	// Get semesters to search based on current date
	searchYears, searchTerms := getSemestersToSearch()

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
			return h.formatCourseResponse(course)
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
			return h.formatCourseResponse(course)
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
// Search range: Recent 2-4 semesters with cache-first strategy.
//
// Search Strategy (2-tier parallel search + scraping fallback):
//
//  1. SQL LIKE (fast path): Search in both title and teachers fields for consecutive substrings.
//     Example: "微積分" matches courses with title containing "微積分"
//     Example: "王" matches courses where any teacher name contains "王"
//
//  2. Fuzzy character-set matching (ALWAYS runs in parallel with SQL LIKE):
//     Loads cached courses and checks if all runes in searchTerm exist in title OR teachers.
//     This catches abbreviations that SQL LIKE misses because characters are scattered.
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
	return h.searchCoursesWithOptions(ctx, searchTerm, false, true)
}

// handleExtendedCourseSearch handles extended course search (4 semesters instead of default 2).
// This is triggered by "課程歷史" or "更多學期" keywords, typically from Quick Reply.
// Search range: 4 semesters (broader historical search).
// Search flow: SQL LIKE → Fuzzy match → Scraping (4 semesters) → No BM25 fallback
// Note: Intentionally skips BM25 fallback as extended search focuses on historical data
func (h *Handler) handleExtendedCourseSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	return h.searchCoursesWithOptions(ctx, searchTerm, true, false)
}

// searchCoursesWithOptions is the core search implementation used by both unified and extended search.
// It consolidates the common search logic to avoid code duplication.
//
// Parameters:
//   - extended: If true, searches 4 semesters instead of 2
//   - enableBM25Fallback: If true, uses BM25 smart search when no keyword results found
//
// Search flow:
//  1. SQL LIKE search (title + teacher) in cache
//  2. Fuzzy character-set matching (parallel with SQL LIKE)
//  3. Web scraping from NTPU website (if cache miss)
//  4. BM25 smart search (optional fallback for unified search only)
func (h *Handler) searchCoursesWithOptions(ctx context.Context, searchTerm string, extended bool, enableBM25Fallback bool) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	semesterType := "近期"
	if extended {
		semesterType = "近 4 個學期"
	}
	log.Infof("Handling course search (%s semesters): %s", semesterType, searchTerm)

	var courses []storage.Course

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

	// Step 2: ALWAYS try fuzzy character-set matching to find additional results
	// This catches cases like "線代" -> "線性代數" that SQL LIKE misses
	// SQL LIKE only finds consecutive substrings, but fuzzy matching finds scattered characters
	allCourses, err := h.db.GetCoursesByRecentSemesters(ctx)
	if err == nil && len(allCourses) > 0 {
		for _, c := range allCourses {
			// Check if searchTerm matches title OR any teacher using fuzzy matching
			titleMatch := bot.ContainsAllRunes(c.Title, searchTerm)
			teacherMatch := false
			for _, teacher := range c.Teachers {
				if bot.ContainsAllRunes(teacher, searchTerm) {
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
		return h.formatCourseListResponseWithOptions(courses, searchTerm, extended)
	}

	// Step 3: Cache miss - Try scraping
	cacheMissMsg := fmt.Sprintf("Cache miss for search term: %s, scraping from %s...", searchTerm, semesterType)
	log.Info(cacheMissMsg)
	h.metrics.RecordCacheMiss(ModuleName)

	// Get semesters to search based on extended flag
	var searchYears, searchTerms []int
	if extended {
		searchYears, searchTerms = getExtendedSemesters()
	} else {
		searchYears, searchTerms = getSemestersToSearch()
	}

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
				titleMatch := bot.ContainsAllRunes(course.Title, searchTerm)
				teacherMatch := false
				for _, teacher := range course.Teachers {
					if bot.ContainsAllRunes(teacher, searchTerm) {
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
		return h.formatCourseListResponseWithOptions(courses, searchTerm, extended)
	}

	// Step 4: No keyword results - try BM25 smart search as last resort (if enabled)
	if enableBM25Fallback && h.bm25Index != nil && h.bm25Index.IsEnabled() {
		log.Infof("No keyword results for %s, trying BM25 search...", searchTerm)

		// Use detached context for smart search operations.
		// PreserveTracing() creates independent context to prevent parent cancellation
		// from aborting LLM API calls (Gemini Query Expansion may take several seconds).
		searchCtx, cancel := context.WithTimeout(ctxutil.PreserveTracing(ctx), config.SmartSearchTimeout)
		defer cancel()
		smartResults, err := h.bm25Index.SearchCourses(searchCtx, searchTerm, 5)

		if err == nil && len(smartResults) > 0 {
			// Convert smart search results to courses
			var smartCourses []storage.Course
			for _, result := range smartResults {
				if course, err := h.db.GetCourseByUID(ctx, result.UID); err == nil && course != nil {
					smartCourses = append(smartCourses, *course)
				}
			}

			if len(smartCourses) > 0 {
				h.metrics.RecordScraperRequest(ModuleName, "smart_fallback", time.Since(startTime).Seconds())
				return h.formatSmartSearchResponse(smartCourses, smartResults)
			}
		}
	}

	// No results found even after scraping and smart search
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
			"🔍 查無「%s」的相關課程\n\n📅 已搜尋範圍：近 2 學期\n\n💡 建議嘗試\n• 「📅 更多學期」搜尋近 4 學期\n• 縮短關鍵字（如「線性」→「線」）\n• 指定年份：「課程 110 %s」",
			searchTerm,
			searchTerm,
		)
	}

	if h.bm25Index != nil && h.bm25Index.IsEnabled() {
		helpText += "\n• 智慧搜尋：「找課 " + searchTerm + "」"
	}

	msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)

	// Build quick reply items
	quickReplyItems := []lineutil.QuickReplyItem{
		lineutil.QuickReplyCourseAction(),
	}

	// Only add "More Semesters" option for regular (non-extended) search
	if !extended {
		quickReplyItems = append(quickReplyItems, lineutil.QuickReplyMoreSemestersAction(searchTerm))
	}

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

	// Check if this is a recent year (within warmup range) - use regular course search
	if year >= currentYear-1 {
		log.Infof("Year %d is within warmup range, redirecting to regular course search", year)
		return h.handleUnifiedCourseSearch(ctx, keyword)
	}

	// Search in historical_courses cache first
	// Search in both terms for the specified year
	var courses []storage.Course
	for _, term := range []int{1, 2} {
		termCourses, err := h.db.GetCoursesByYearTerm(ctx, year, term)
		if err != nil {
			log.WithError(err).Warnf("Failed to get courses for year %d term %d", year, term)
			continue
		}
		// Filter by keyword using fuzzy matching
		for _, c := range termCourses {
			if bot.ContainsAllRunes(c.Title, keyword) {
				courses = append(courses, c)
			}
		}
	}

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(ModuleName)
		log.Infof("Found %d historical courses in cache for year=%d, keyword=%s", len(courses), year, keyword)
		// Limit results
		if len(courses) > MaxCoursesPerSearch {
			courses = courses[:MaxCoursesPerSearch]
		}
		return h.formatCourseListResponse(courses)
	}

	// Cache miss - scrape from historical course system
	h.metrics.RecordCacheMiss(ModuleName)
	log.Infof("Cache miss for historical course: year=%d, keyword=%s, scraping...", year, keyword)

	// Use term=0 to query both semesters at once (more efficient)
	scrapedCourses, err := ntpu.ScrapeCourses(ctx, h.scraper, year, 0, keyword)
	if err != nil {
		log.WithError(err).WithField("year", year).
			Warn("Failed to scrape historical courses")
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

	// Save courses to historical_courses table
	for _, course := range scrapedCourses {
		if err := h.db.SaveCourse(ctx, course); err != nil {
			log.WithError(err).Warn("Failed to save historical course to cache")
		}
	}

	if len(scrapedCourses) > 0 {
		h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
		// Convert []*storage.Course to []storage.Course
		courses := make([]storage.Course, len(scrapedCourses))
		for i, c := range scrapedCourses {
			courses[i] = *c
		}
		return h.formatCourseListResponse(courses)
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

// formatCourseResponse formats a single course as a LINE message
func (h *Handler) formatCourseResponse(course *storage.Course) []messaging_api.MessageInterface {
	// Header: Course label (using standardized component)
	header := lineutil.NewDetailPageLabel("📚", "課程資訊")

	// Hero: Course title with course code in format `{課程名稱} ({課程代碼})`
	heroTitle := lineutil.FormatCourseTitleWithUID(course.Title, course.UID)
	hero := lineutil.NewHeroBox(heroTitle, "")

	// Build body contents using BodyContentBuilder for cleaner code
	body := lineutil.NewBodyContentBuilder()

	// 學期 info - first row
	semesterText := lineutil.FormatSemester(course.Year, course.Term)
	body.AddInfoRow("📅", "開課學期", semesterText, lineutil.DefaultInfoRowStyle())

	// 教師 info
	if len(course.Teachers) > 0 {
		teacherNames := strings.Join(course.Teachers, "、")
		body.AddInfoRow("👨‍🏫", "授課教師", teacherNames, lineutil.DefaultInfoRowStyle())
	}

	// 時間 info - 轉換節次為實際時間
	if len(course.Times) > 0 {
		formattedTimes := lineutil.FormatCourseTimes(course.Times)
		timeStr := strings.Join(formattedTimes, "、")
		body.AddInfoRow("⏰", "上課時間", timeStr, lineutil.DefaultInfoRowStyle())
	}

	// 地點 info
	if len(course.Locations) > 0 {
		locationStr := strings.Join(course.Locations, "、")
		body.AddInfoRow("📍", "上課地點", locationStr, lineutil.DefaultInfoRowStyle())
	}

	// 備註 info
	if course.Note != "" {
		noteStyle := lineutil.DefaultInfoRowStyle()
		noteStyle.ValueSize = "xs"
		noteStyle.ValueColor = lineutil.ColorLabel // Use semantic color constant
		body.AddInfoRow("📝", "備註", course.Note, noteStyle)
	}

	// Add cache time hint (unobtrusive, right-aligned)
	if hint := lineutil.NewCacheTimeHint(course.CachedAt); hint != nil {
		body.AddComponent(hint.FlexText)
	}

	// Build footer actions using button rows for 2-column layout
	var footerRows [][]*lineutil.FlexButton

	// Row 1: 課程大綱 + 查詢系統 (外部連結使用藍色)
	row1 := make([]*lineutil.FlexButton, 0, 2)
	if course.DetailURL != "" {
		row1 = append(row1, lineutil.NewFlexButton(
			lineutil.NewURIAction("📄 課程大綱", course.DetailURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))
	}
	courseQueryURL := fmt.Sprintf("https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.queryByKeyword?qYear=%d&qTerm=%d&courseno=%s&seq1=A&seq2=M",
		course.Year, course.Term, course.No)
	row1 = append(row1, lineutil.NewFlexButton(
		lineutil.NewURIAction("🔍 查詢系統", courseQueryURL),
	).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))
	if len(row1) > 0 {
		footerRows = append(footerRows, row1)
	}

	// Row 2: 教師課表 + 教師課程 (if teachers exist)
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		row2 := make([]*lineutil.FlexButton, 0, 2)

		// Teacher schedule button - opens the teacher's course table webpage (外部連結使用藍色)
		if len(course.TeacherURLs) > 0 && course.TeacherURLs[0] != "" {
			row2 = append(row2, lineutil.NewFlexButton(
				lineutil.NewURIAction("📅 教師課表", course.TeacherURLs[0]),
			).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))
		}

		// Teacher all courses button - searches for all courses taught by this teacher (內部指令使用紫色)
		displayText := lineutil.TruncateRunes(fmt.Sprintf("搜尋 %s 的近期課程", teacherName), 40)
		row2 = append(row2, lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText(
				"👨‍🏫 教師課程",
				displayText,
				fmt.Sprintf("course:授課課程%s%s", bot.PostbackSplitChar, teacherName),
			),
		).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm"))

		if len(row2) > 0 {
			footerRows = append(footerRows, row2)
		}
	}

	// Row 3: Dcard 查詢 + 選課大全
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		row3 := make([]*lineutil.FlexButton, 0, 2)

		// Dcard search button - Google search with site:dcard.tw/f/ntpu (外部連結使用藍色)
		dcardQuery := fmt.Sprintf("%s %s site:dcard.tw/f/ntpu", teacherName, course.Title)
		dcardURL := "https://www.google.com/search?q=" + url.QueryEscape(dcardQuery)
		row3 = append(row3, lineutil.NewFlexButton(
			lineutil.NewURIAction("💬 Dcard", dcardURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))

		// 選課大全 button (外部連結使用藍色)
		courseSelectionQuery := fmt.Sprintf("%s %s", teacherName, course.Title)
		courseSelectionURL := "https://no21.ntpu.org/?s=" + url.QueryEscape(courseSelectionQuery)
		row3 = append(row3, lineutil.NewFlexButton(
			lineutil.NewURIAction("📖 選課大全", courseSelectionURL),
		).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))

		if len(row3) > 0 {
			footerRows = append(footerRows, row3)
		}
	}

	footer := lineutil.NewButtonFooter(footerRows...)

	bubble := lineutil.NewFlexBubble(
		header,
		hero.FlexBox,
		body.Build(),
		footer,
	)

	// Limit altText to 400 chars (LINE API limit, using rune slicing for UTF-8 safety)
	altText := lineutil.TruncateRunes(fmt.Sprintf("課程：%s", course.Title), 400)
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

// extractUniqueSemesters extracts unique semesters from a sorted course list.
// The input courses should be pre-sorted by semester (newest first).
// Returns a slice of SemesterPair in the same order (newest first).
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

	return semesters
}

// formatCourseListResponse formats a list of courses as LINE messages with semester labels.
// Courses are sorted by semester (newest first) and each bubble shows a label indicating
// whether it's from the newest semester in data, previous semester, or older.
func (h *Handler) formatCourseListResponse(courses []storage.Course) []messaging_api.MessageInterface {
	return h.formatCourseListResponseWithOptions(courses, "", false)
}

// formatCourseListResponseWithOptions formats courses with extended options.
// Parameters:
//   - courses: List of courses to display
//   - searchKeyword: Original search keyword (for "more semesters" Quick Reply)
//   - isExtendedSearch: True if this is already an extended (4-semester) search
func (h *Handler) formatCourseListResponseWithOptions(courses []storage.Course, searchKeyword string, isExtendedSearch bool) []messaging_api.MessageInterface {
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
		// Get semester label info based on data position
		labelInfo := lineutil.GetSemesterLabel(course.Year, course.Term, dataSemesters)

		// Colored header with course title
		heroTitle := lineutil.FormatCourseTitleWithUID(course.Title, course.UID)
		header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
			Title: heroTitle,
			Color: labelInfo.Color,
		})

		// Build body contents - first row is semester label
		contents := []messaging_api.FlexComponentInterface{
			lineutil.NewBodyLabel(labelInfo).FlexBox,
		}

		// 學期資訊（完整格式）
		semesterText := lineutil.FormatSemester(course.Year, course.Term)
		contents = append(contents,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("📅 開課學期：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
				lineutil.NewFlexText(semesterText).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).FlexText,
			).WithMargin("sm").WithSpacing("sm").FlexBox,
		)

		// 第二列：授課教師
		if len(course.Teachers) > 0 {
			// Display teachers with truncation if too many (max 5, then "等 N 人")
			carouselTeachers := lineutil.FormatTeachers(course.Teachers, 5)
			contents = append(contents,
				lineutil.NewFlexBox("horizontal",
					lineutil.NewFlexText("👨‍🏫 授課教師：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
					lineutil.NewFlexText(carouselTeachers).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).WithWrap(true).FlexText,
				).WithMargin("sm").WithSpacing("sm").FlexBox,
			)
		}
		// 第三列：上課時間 - 轉換節次為實際時間
		if len(course.Times) > 0 {
			// Format times with actual time ranges, then truncate if too many (max 4, then "等 N 節")
			formattedTimes := lineutil.FormatCourseTimes(course.Times)
			carouselTimes := lineutil.FormatTimes(formattedTimes, 4)
			contents = append(contents,
				lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
				lineutil.NewFlexBox("horizontal",
					lineutil.NewFlexText("⏰ 上課時間：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
					lineutil.NewFlexText(carouselTimes).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).WithWrap(true).FlexText,
				).WithMargin("sm").WithSpacing("sm").FlexBox,
			)
		}
		// Footer with "View Detail" button - displayText shows course title
		displayText := fmt.Sprintf("查詢「%s」課程資訊", lineutil.TruncateRunes(course.Title, 30))
		// Use course: prefix for proper postback routing
		footer := lineutil.NewFlexBox("vertical",
			lineutil.NewFlexButton(
				lineutil.NewPostbackActionWithDisplayText("📝 查看詳細", displayText, "course:"+course.UID),
			).WithStyle("primary").WithColor(lineutil.ColorButtonPrimary).WithHeight("sm").FlexButton,
		).WithSpacing("sm")

		bubble := lineutil.NewFlexBubble(
			header,
			nil, // No hero - title is in colored header
			lineutil.NewFlexBox("vertical", contents...).WithSpacing("sm"),
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
			fmt.Sprintf("⚠️ 搜尋結果超過 %d 門課程，僅顯示前 %d 門\n\n建議使用更精確的搜尋條件以縮小範圍", originalCount, MaxCoursesPerSearch),
			sender,
		)
		messages = append(messages, warningMsg)
	}

	// Build Quick Reply items based on context
	quickReplyItems := []lineutil.QuickReplyItem{
		lineutil.QuickReplyCourseAction(),
	}

	// Add "More Semesters" option if:
	// 1. Not already an extended search
	// 2. Have a search keyword to pass along
	if !isExtendedSearch && searchKeyword != "" {
		quickReplyItems = append(quickReplyItems, lineutil.QuickReplyMoreSemestersAction(searchKeyword))
	}

	// Add smart search option if enabled
	if h.bm25Index != nil && h.bm25Index.IsEnabled() {
		// Preserve original keyword (if any) so users can switch to smart search seamlessly.
		if searchKeyword != "" {
			quickReplyItems = append(quickReplyItems,
				lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("🔮 找課", "找課 "+searchKeyword)},
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
			"🔍 查無相關課程\n\n💡 建議嘗試\n• 換個描述方式\n• 使用精確搜尋：課程 名稱", sender)
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

// formatSmartSearchResponse formats smart search results with confidence labels
func (h *Handler) formatSmartSearchResponse(courses []storage.Course, results []rag.SearchResult) []messaging_api.MessageInterface {
	if len(courses) == 0 {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender("🔍 查無相關課程\n\n💡 建議嘗試\n• 換個描述方式\n• 使用精確搜尋：課程 名稱", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplySmartSearchAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}
	}

	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Create confidence map for lookup
	confidenceMap := make(map[string]float32)
	for _, r := range results {
		confidenceMap[r.UID] = r.Confidence
	}

	// Build bubbles with relevance labels based on confidence
	bubbles := make([]messaging_api.FlexBubble, 0, len(courses))
	for _, course := range courses {
		confidence := confidenceMap[course.UID]
		bubble := h.buildSmartCourseBubble(course, confidence)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Group into carousels
	var messages []messaging_api.MessageInterface

	for i := 0; i < len(bubbles); i += lineutil.MaxBubblesPerCarousel {
		end := i + lineutil.MaxBubblesPerCarousel
		if end > len(bubbles) {
			end = len(bubbles)
		}

		carousel := lineutil.NewFlexCarousel(bubbles[i:end])
		altText := "🔮 智慧搜尋結果"
		if i > 0 {
			altText = fmt.Sprintf("智慧搜尋結果 (%d-%d)", i+1, end)
		}
		msg := lineutil.NewFlexMessage(altText, carousel)
		msg.Sender = sender
		messages = append(messages, msg)
	}

	// Add header message with search guidance
	// Provide tips when results are few to help users refine their queries
	headerText := fmt.Sprintf("🔮 智慧搜尋：找到 %d 門課程", len(courses))
	if len(courses) <= 3 {
		headerText += "\n\n💡 提示：使用更具體的關鍵字可獲得更多結果"
	}
	headerMsg := lineutil.NewTextMessageWithConsistentSender(headerText, sender)
	messages = append([]messaging_api.MessageInterface{headerMsg}, messages...)

	// Add Quick Reply
	lineutil.AddQuickReplyToMessages(messages,
		lineutil.QuickReplySmartSearchAction(),
		lineutil.QuickReplyCourseAction(),
		lineutil.QuickReplyHelpAction(),
	)

	return messages
}

// buildSmartCourseBubble creates a Flex Message bubble for a course with relevance label.
// Uses colored header layout for visual hierarchy.
func (h *Handler) buildSmartCourseBubble(course storage.Course, confidence float32) *lineutil.FlexBubble {
	// Relevance label based on confidence (user-friendly labels)
	labelInfo := getRelevanceLabel(confidence)

	// Colored header with course title
	heroTitle := lineutil.FormatCourseTitleWithUID(course.Title, course.UID)
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: heroTitle,
		Color: labelInfo.Color,
	})

	// Build body contents - first row is relevance label
	contents := []messaging_api.FlexComponentInterface{
		lineutil.NewBodyLabel(labelInfo).FlexBox,
	}

	// 學期資訊（完整格式）
	semesterText := lineutil.FormatSemester(course.Year, course.Term)
	contents = append(contents,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📅 開課學期：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
			lineutil.NewFlexText(semesterText).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).FlexText,
		).WithMargin("sm").WithSpacing("sm").FlexBox,
	)

	// 授課教師
	if len(course.Teachers) > 0 {
		carouselTeachers := lineutil.FormatTeachers(course.Teachers, 5)
		contents = append(contents,
			lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("👨‍🏫 授課教師：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
				lineutil.NewFlexText(carouselTeachers).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).WithWrap(true).FlexText,
			).WithMargin("sm").WithSpacing("sm").FlexBox,
		)
	}

	// 上課時間
	if len(course.Times) > 0 {
		formattedTimes := lineutil.FormatCourseTimes(course.Times)
		carouselTimes := lineutil.FormatTimes(formattedTimes, 4)
		contents = append(contents,
			lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("⏰ 上課時間：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
				lineutil.NewFlexText(carouselTimes).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).WithWrap(true).FlexText,
			).WithMargin("sm").WithSpacing("sm").FlexBox,
		)
	}

	// Footer with "View Detail" button
	displayText := fmt.Sprintf("查詢「%s」課程資訊", lineutil.TruncateRunes(course.Title, 30))
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText("📝 查看詳細", displayText, "course:"+course.UID),
		).WithStyle("primary").WithColor(lineutil.ColorButtonPrimary).WithHeight("sm").FlexButton,
	).WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(
		header,
		nil, // No hero - title is in colored header
		lineutil.NewFlexBox("vertical", contents...).WithSpacing("sm"),
		footer,
	)
	return bubble
}

// getRelevanceLabel returns a user-friendly relevance label info based on relative BM25 score.
//
// Returns: BodyLabelInfo with emoji/label and header background color (ColorHeader*).
//
// Design rationale:
//   - Uses relative score (score / maxScore) from BM25 search
//   - Simple 3-tier system: Clear differentiation without cognitive overload
//   - Relative scoring: Comparable within the same query results
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
		// White label for best matches - highest visibility
		return lineutil.BodyLabelInfo{
			Emoji: "🎯",
			Label: "最佳匹配",
			Color: lineutil.ColorHeaderBest,
		}
	}
	if confidence >= 0.6 {
		// Purple label for highly relevant - attention-grabbing
		return lineutil.BodyLabelInfo{
			Emoji: "✨",
			Label: "高度相關",
			Color: lineutil.ColorHeaderHigh,
		}
	}
	// Amber label for partial relevance - moderate visibility
	return lineutil.BodyLabelInfo{
		Emoji: "📋",
		Label: "部分相關",
		Color: lineutil.ColorHeaderMedium,
	}
}
