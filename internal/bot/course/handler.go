// Package course implements the course query module for the LINE bot.
// It handles course searches by title, teacher, or UID from NTPU's course system.
package course

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles course-related queries
type Handler struct {
	db             *storage.DB
	scraper        *scraper.Client
	metrics        *metrics.Metrics
	logger         *logger.Logger
	stickerManager *sticker.Manager
}

// Course handler constants.
const (
	moduleName           = "course"
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

	courseRegex = bot.BuildKeywordRegex(validCourseKeywords)
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

// NewHandler creates a new course handler
func NewHandler(db *storage.DB, scraper *scraper.Client, metrics *metrics.Metrics, logger *logger.Logger, stickerManager *sticker.Manager) *Handler {
	return &Handler{
		db:             db,
		scraper:        scraper,
		metrics:        metrics,
		logger:         logger,
		stickerManager: stickerManager,
	}
}

// CanHandle checks if the message is for the course module
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)

	// Check for course UID pattern (full: 11312U0001)
	if uidRegex.MatchString(text) {
		return true
	}

	// Check for course number only pattern (e.g., U0001, 1U0001, 2U0001)
	if courseNoRegex.MatchString(text) {
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
	log := h.logger.WithModule(moduleName)
	text = strings.TrimSpace(text)

	log.Infof("Handling course message: %s", text)

	// Check for full course UID first (highest priority, e.g., 11312U0001)
	if match := uidRegex.FindString(text); match != "" {
		return h.handleCourseUIDQuery(ctx, match)
	}

	// Check for course number only (e.g., U0001, 1U0001, 2U0001)
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

	// Check for course title search - extract term after keyword
	// Support both "keyword term" and "term keyword" patterns
	// Unified search: matches both course title and teacher name
	if courseRegex.MatchString(text) {
		match := courseRegex.FindString(text)
		searchTerm := bot.ExtractSearchTerm(text, match)

		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender("📚 請輸入課程關鍵字\n\n例如：\n• 課 程式設計\n• 課程 微積分\n• 課 王小明（搜尋教師）\n• 課 線代 王（搜尋課名+教師）\n\n💡 也可直接輸入課程編號（如：3141U0001）", sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
			})
			return []messaging_api.MessageInterface{msg}
		}
		return h.handleUnifiedCourseSearch(ctx, searchTerm)
	}

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the course module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
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
	if uidRegex.MatchString(data) {
		return h.handleCourseUIDQuery(ctx, data)
	}

	return []messaging_api.MessageInterface{}
}

// handleCourseUIDQuery handles course UID queries
func (h *Handler) handleCourseUIDQuery(ctx context.Context, uid string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Normalize UID to uppercase
	uid = strings.ToUpper(uid)

	// Check cache first
	course, err := h.db.GetCourseByUID(ctx, uid)
	if err != nil {
		log.WithError(err).Error("Failed to query cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("查詢課程時發生問題", sender, uid),
		}
	}

	if course != nil {
		// Cache hit
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Cache hit for course UID: %s", uid)
		return h.formatCourseResponse(course)
	}

	// Cache miss - scrape from website
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("Cache miss for course UID: %s, scraping...", uid)

	course, err = ntpu.ScrapeCourseByUID(ctx, h.scraper, uid)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape course UID: %s", uid)
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("🔍 查無課程編號 %s\n\n請確認課程編號是否正確", uid), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📚 搜尋課程", "課程")},
			{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Check if course was found (prevent nil pointer dereference)
	if course == nil {
		log.Warnf("Course UID %s not found after scraping", uid)
		h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無課程編號 %s\n\n💡 請確認：\n• 課程編號拼寫是否正確\n• 該課程是否在近兩學年度開設", uid),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📚 搜尋課程", "課程")},
			{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache
	if err := h.db.SaveCourse(ctx, course); err != nil {
		log.WithError(err).Warn("Failed to save course to cache")
	}

	h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
	return h.formatCourseResponse(course)
}

// handleCourseNoQuery handles course number only queries (e.g., U0001, M0002)
// It searches in current and previous semester to find the course
func (h *Handler) handleCourseNoQuery(ctx context.Context, courseNo string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
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
			h.metrics.RecordCacheHit(moduleName)
			log.Infof("Cache hit for course UID: %s (from course no: %s)", uid, courseNo)
			return h.formatCourseResponse(course)
		}
	}

	// Cache miss - try scraping from each semester
	h.metrics.RecordCacheMiss(moduleName)
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

			h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
			log.Infof("Found course for UID: %s (from course no: %s)", uid, courseNo)
			return h.formatCourseResponse(course)
		}
	}

	// No results found
	h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())

	// Build helpful message with examples
	exampleUID := fmt.Sprintf("%d1%s", searchYears[0], courseNo)
	msg := lineutil.NewTextMessageWithConsistentSender(
		fmt.Sprintf("🔍 查無課程編號 %s\n\n💡 請確認：\n• 課程編號拼寫是否正確\n• 該課程是否在近兩學年度開設\n\n📝 若已知完整課號，可直接輸入：\n   例如：%s", courseNo, exampleUID),
		sender,
	)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📚 搜尋課程", "課程")},
		{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
	})
	return []messaging_api.MessageInterface{msg}
}

// handleUnifiedCourseSearch handles unified course search queries with fuzzy matching.
// It searches both course titles and teacher names simultaneously.
//
// Search Strategy (3-tier cascade):
//
//  1. SQL LIKE (fast path): Search in both title and teachers fields
//     Example: "微積分" matches courses with title containing "微積分"
//     Example: "王" matches courses where any teacher name contains "王"
//
//  2. Fuzzy character-set matching (cache fallback): If SQL LIKE returns no results,
//     loads cached courses and checks if all runes in searchTerm exist in title OR teachers.
//     Example: "線代" matches "線性代數" (all chars exist in title)
//     Example: "王明" matches teacher "王小明" (all chars exist)
//
//  3. Web scraping (external fallback): If cache has no results, scrape from website.
//
// Multi-word search: "微積分 王" will find courses where title contains "微積分王"
// OR where all characters exist in title+teachers combined.
func (h *Handler) handleUnifiedCourseSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	var courses []storage.Course

	// Step 1: Try SQL LIKE search for title first
	titleCourses, err := h.db.SearchCoursesByTitle(ctx, searchTerm)
	if err != nil {
		log.WithError(err).Error("Failed to search courses by title in cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("搜尋課程時發生問題", sender, "課程 "+searchTerm),
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
		existingUIDs := make(map[string]bool)
		for _, c := range courses {
			existingUIDs[c.UID] = true
		}
		for _, c := range teacherCourses {
			if !existingUIDs[c.UID] {
				courses = append(courses, c)
				existingUIDs[c.UID] = true
			}
		}
	}

	// Step 2: If SQL LIKE didn't find results, try fuzzy character-set matching
	if len(courses) == 0 {
		allCourses, err := h.db.GetCoursesByRecentSemesters(ctx)
		if err == nil && len(allCourses) > 0 {
			existingUIDs := make(map[string]bool)
			for _, c := range allCourses {
				// Check if searchTerm matches title OR any teacher
				titleMatch := lineutil.ContainsAllRunes(c.Title, searchTerm)
				teacherMatch := false
				for _, teacher := range c.Teachers {
					if lineutil.ContainsAllRunes(teacher, searchTerm) {
						teacherMatch = true
						break
					}
				}
				if (titleMatch || teacherMatch) && !existingUIDs[c.UID] {
					courses = append(courses, c)
					existingUIDs[c.UID] = true
				}
			}
		}
	}

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Found %d courses in cache for search term: %s", len(courses), searchTerm)
		return h.formatCourseListResponse(courses)
	}

	// Step 3: Cache miss - Try scraping from current and previous semester
	log.Infof("Cache miss for search term: %s, scraping from recent semesters...", searchTerm)
	h.metrics.RecordCacheMiss(moduleName)

	// Get semesters to search based on current date
	searchYears, searchTerms := getSemestersToSearch()

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
	// This may take significant time and could approach the 25s webhook deadline.
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
				titleMatch := lineutil.ContainsAllRunes(course.Title, searchTerm)
				teacherMatch := false
				for _, teacher := range course.Teachers {
					if lineutil.ContainsAllRunes(teacher, searchTerm) {
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
		h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
		// Convert []*storage.Course to []storage.Course
		courses := make([]storage.Course, len(foundCourses))
		for i, c := range foundCourses {
			courses[i] = *c
		}
		return h.formatCourseListResponse(courses)
	}

	// No results found even after scraping
	h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
	msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf(
		"🔍 查無包含「%s」的課程或教師\n\n💡 請確認：\n• 課程名稱或教師姓名是否正確\n• 該課程是否在近兩學年度開設\n• 可嘗試只輸入部分關鍵字（如姓氏）",
		searchTerm,
	), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("🔄 重新查詢", "課程")},
		{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
	})
	return []messaging_api.MessageInterface{msg}
}

// handleHistoricalCourseSearch handles historical course queries using "課程 {year} {keyword}" syntax
// Uses separate historical_courses table with 7-day TTL for on-demand caching
// This function is called for courses older than the regular warmup range (2 years)
func (h *Handler) handleHistoricalCourseSearch(ctx context.Context, year int, keyword string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Validate year range (ROC year: 89 = AD 2000 to current year)
	currentYear := time.Now().Year() - 1911
	if year < 89 || year > currentYear {
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("❌ 無效的學年度：%d\n\n💡 請輸入 89-%d 之間的學年度\n範例：課程 110 微積分", year, currentYear),
			sender,
		)
		return []messaging_api.MessageInterface{msg}
	}

	log.Infof("Handling historical course search: year=%d, keyword=%s", year, keyword)

	// Check if this is a recent year (within warmup range) - use regular course search
	if year >= currentYear-1 {
		log.Infof("Year %d is within warmup range, redirecting to regular course search", year)
		return h.handleUnifiedCourseSearch(ctx, keyword)
	}

	// Search in historical_courses cache first
	courses, err := h.db.SearchHistoricalCoursesByYearAndTitle(ctx, year, keyword)
	if err != nil {
		log.WithError(err).Error("Failed to search historical courses in cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetailAndSender("搜尋歷史課程時發生問題", sender)
		return []messaging_api.MessageInterface{msg}
	}

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Found %d historical courses in cache for year=%d, keyword=%s", len(courses), year, keyword)
		return h.formatCourseListResponse(courses)
	}

	// Cache miss - scrape from historical course system
	log.Infof("Cache miss for historical course: year=%d, keyword=%s, scraping...", year, keyword)
	h.metrics.RecordCacheMiss(moduleName)

	// Use term=0 to query both semesters at once (more efficient)
	scrapedCourses, err := ntpu.ScrapeCourses(ctx, h.scraper, year, 0, keyword)
	if err != nil {
		log.WithError(err).WithField("year", year).
			Warn("Failed to scrape historical courses")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無 %d 學年度包含「%s」的課程\n\n💡 請確認：\n• 學年度和課程名稱是否正確\n• 該課程是否在該學年度開設", year, keyword),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📚 查詢近期課程", "課程 "+keyword)},
			{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
		})
		return []messaging_api.MessageInterface{msg}
	}
	log.Infof("Scraped %d historical courses for year=%d", len(scrapedCourses), year)

	// Save courses to historical_courses table
	for _, course := range scrapedCourses {
		if err := h.db.SaveHistoricalCourse(ctx, course); err != nil {
			log.WithError(err).Warn("Failed to save historical course to cache")
		}
	}

	if len(scrapedCourses) > 0 {
		h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
		// Convert []*storage.Course to []storage.Course
		courses := make([]storage.Course, len(scrapedCourses))
		for i, c := range scrapedCourses {
			courses[i] = *c
		}
		return h.formatCourseListResponse(courses)
	}

	// No results found
	h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
	msg := lineutil.NewTextMessageWithConsistentSender(
		fmt.Sprintf("🔍 查無 %d 學年度包含「%s」的課程\n\n💡 請確認：\n• 學年度和課程名稱是否正確\n• 該課程是否在該學年度開設", year, keyword),
		sender,
	)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📚 查詢近期課程", "課程 "+keyword)},
		{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
	})
	return []messaging_api.MessageInterface{msg}
}

// formatCourseResponse formats a single course as a LINE message
func (h *Handler) formatCourseResponse(course *storage.Course) []messaging_api.MessageInterface {
	// Header: Course badge (using standardized component)
	header := lineutil.NewHeaderBadge("📚", "課程資訊")

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
		noteStyle.ValueColor = "#666666"
		body.AddInfoRow("📝", "備註", course.Note, noteStyle)
	}

	// Add cache time hint (unobtrusive, right-aligned)
	if hint := lineutil.NewCacheTimeHint(course.CachedAt); hint != nil {
		body.AddComponent(hint.FlexText)
	}

	// Build footer actions using button rows for 2-column layout
	var footerRows [][]*lineutil.FlexButton

	// Row 1: 課程大綱 + 查詢系統
	row1 := make([]*lineutil.FlexButton, 0, 2)
	if course.DetailURL != "" {
		row1 = append(row1, lineutil.NewFlexButton(
			lineutil.NewURIAction("📄 課程大綱", course.DetailURL),
		).WithStyle("primary").WithHeight("sm"))
	}
	courseQueryURL := fmt.Sprintf("https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.queryByKeyword?qYear=%d&qTerm=%d&courseno=%s&seq1=A&seq2=M",
		course.Year, course.Term, course.No)
	row1 = append(row1, lineutil.NewFlexButton(
		lineutil.NewURIAction("🔍 查詢系統", courseQueryURL),
	).WithStyle("secondary").WithHeight("sm"))
	if len(row1) > 0 {
		footerRows = append(footerRows, row1)
	}

	// Row 2: 教師課表 + 教師課程 (if teachers exist)
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		row2 := make([]*lineutil.FlexButton, 0, 2)

		// Teacher schedule button - opens the teacher's course table webpage
		if len(course.TeacherURLs) > 0 && course.TeacherURLs[0] != "" {
			row2 = append(row2, lineutil.NewFlexButton(
				lineutil.NewURIAction("📅 教師課表", course.TeacherURLs[0]),
			).WithStyle("secondary").WithHeight("sm"))
		}

		// Teacher all courses button - searches for all courses taught by this teacher
		displayText := lineutil.TruncateRunes(fmt.Sprintf("搜尋 %s 的近期課程", teacherName), 40)
		row2 = append(row2, lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText(
				"👤 教師課程",
				displayText,
				fmt.Sprintf("course:授課課程%s%s", bot.PostbackSplitChar, teacherName),
			),
		).WithStyle("secondary").WithHeight("sm"))

		if len(row2) > 0 {
			footerRows = append(footerRows, row2)
		}
	}

	// Row 3: Dcard 查詢 + 選課大全
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		row3 := make([]*lineutil.FlexButton, 0, 2)

		// Dcard search button - Google search with site:dcard.tw/f/ntpu
		dcardQuery := fmt.Sprintf("%s %s site:dcard.tw/f/ntpu", teacherName, course.Title)
		dcardURL := "https://www.google.com/search?q=" + url.QueryEscape(dcardQuery)
		row3 = append(row3, lineutil.NewFlexButton(
			lineutil.NewURIAction("💬 Dcard", dcardURL),
		).WithStyle("secondary").WithHeight("sm"))

		// 選課大全 button
		courseSelectionQuery := fmt.Sprintf("%s %s", teacherName, course.Title)
		courseSelectionURL := "https://no21.ntpu.org/?s=" + url.QueryEscape(courseSelectionQuery)
		row3 = append(row3, lineutil.NewFlexButton(
			lineutil.NewURIAction("📖 選課大全", courseSelectionURL),
		).WithStyle("secondary").WithHeight("sm"))

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
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📚 查詢其他課程", "課程")},
		{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
	})

	return []messaging_api.MessageInterface{msg}
}

// formatCourseListResponse formats a list of courses as LINE messages
func (h *Handler) formatCourseListResponse(courses []storage.Course) []messaging_api.MessageInterface {
	if len(courses) == 0 {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("🔍 查無課程資料", sender),
		}
	}

	// Sort courses: year descending (recent first), then term descending (term 2 before term 1)
	sort.Slice(courses, func(i, j int) bool {
		if courses[i].Year != courses[j].Year {
			return courses[i].Year > courses[j].Year // Year: recent first
		}
		return courses[i].Term > courses[j].Term // Term: 2 (下學期) before 1 (上學期)
	})

	sender := lineutil.GetSender(senderName, h.stickerManager)
	var messages []messaging_api.MessageInterface

	// Limit to 50 courses - track if truncated for warning message
	originalCount := len(courses)
	truncated := len(courses) > MaxCoursesPerSearch
	if truncated {
		courses = courses[:MaxCoursesPerSearch]
	}

	// Create bubbles for carousel (LINE API limit: max 10 bubbles per Flex Carousel)
	var bubbles []messaging_api.FlexBubble
	for _, course := range courses {
		// Hero: Course title with course code in format `{課程名稱} ({課程代碼})`
		heroTitle := lineutil.FormatCourseTitleWithUID(course.Title, course.UID)
		hero := lineutil.NewCompactHeroBox(heroTitle)

		// Build body contents with improved layout
		// 第一列：學期資訊
		semesterText := lineutil.FormatSemester(course.Year, course.Term)
		contents := []messaging_api.FlexComponentInterface{
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("📅 開課學期：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
				lineutil.NewFlexText(semesterText).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).FlexText,
			).WithMargin("sm").WithSpacing("sm").FlexBox,
			lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		}

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
			).WithStyle("primary").WithHeight("sm").FlexButton,
		).WithSpacing("sm")

		bubble := lineutil.NewFlexBubble(
			nil,
			hero.FlexBox,
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

	// Prepend warning message if results were truncated (and we have room)
	if truncated && len(messages) < 5 {
		warningMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 搜尋結果超過 %d 門課程，僅顯示前 %d 門。\n\n建議使用更精確的搜尋條件以縮小範圍。", originalCount, MaxCoursesPerSearch),
			sender,
		)
		messages = append([]messaging_api.MessageInterface{warningMsg}, messages...)
	}

	// Add Quick Reply to the last message
	lineutil.AddQuickReplyToMessages(messages,
		lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("🔄 重新查詢", "課程")},
		lineutil.QuickReplyHelpAction(),
	)

	return messages
}
