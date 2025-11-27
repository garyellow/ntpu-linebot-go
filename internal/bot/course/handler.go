package course

import (
	"context"
	"fmt"
	"regexp"
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

const (
	moduleName           = "course"
	senderName           = "課程魔法師"
	MaxCoursesPerSearch  = 50 // Maximum courses to return in search results
	MaxTitleDisplayChars = 60 // Maximum characters for course title display before truncation
)

// Valid keywords for course queries
var (
	validCourseKeywords = []string{
		// 中文關鍵字
		"課", "課程", "科目",
		"課名", "課程名", "課程名稱",
		"科目名", "科目名稱",
		// English keywords
		"class", "course",
	}
	validTeacherKeywords = []string{
		// 中文關鍵字（基本）
		"師", "老師", "教師", "教授",
		// 中文關鍵字（完整）
		"老師名", "教師名", "教授名",
		"老師名稱", "教師名稱", "教授名稱",
		// 中文關鍵字（授課相關）
		"授課教師", "授課老師", "授課教授",
		// English keywords
		"teacher", "professor", "prof", "dr", "doctor",
	}

	courseRegex  = bot.BuildKeywordRegex(validCourseKeywords)
	teacherRegex = bot.BuildKeywordRegex(validTeacherKeywords)
	// UID format: {year}{term}{no} where:
	// - year: 2-3 digits (e.g., 113, 12)
	// - term: 1 digit (1=上學期, 2=下學期)
	// - no: course number starting with U/M/N/P (case-insensitive) + 4 digits
	// Full UID example: 11312U0001 (year=113, term=1, no=2U0001) or 9921U0001
	// User input format: just the course_no part with term prefix, e.g., 1U0001, 2M0002
	// So regex matches: 3-4 digits (year+term) + U/M/N/P + 4 digits
	uidRegex = regexp.MustCompile(`(?i)\d{3,4}[umnp]\d{4}`)
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

	// Check for course UID pattern
	if uidRegex.MatchString(text) {
		return true
	}

	// Check for course keywords
	if courseRegex.MatchString(text) {
		return true
	}

	// Check for teacher keywords
	if teacherRegex.MatchString(text) {
		return true
	}

	return false
}

// HandleMessage handles text messages for the course module
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	text = strings.TrimSpace(text)

	log.Infof("Handling course message: %s", text)

	// Check for course UID first (highest priority)
	if match := uidRegex.FindString(text); match != "" {
		return h.handleCourseUIDQuery(ctx, match)
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
	if courseRegex.MatchString(text) {
		match := courseRegex.FindString(text)
		searchTerm := bot.ExtractSearchTerm(text, match)

		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender("📚 請輸入課程名稱\n\n例如：\n• 課 程式設計\n• 課程 微積分\n• 微積分課\n\n💡 也可直接輸入課程編號（如：3141U0001）", sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("👨‍🏫 按教師查詢", "老師")},
				{Action: lineutil.NewMessageAction("📌 使用說明", "使用說明")},
			})
			return []messaging_api.MessageInterface{msg}
		}
		return h.handleCourseTitleSearch(ctx, searchTerm)
	}

	// Check for teacher search - extract term after keyword
	// Support both "keyword term" and "term keyword" patterns
	if teacherRegex.MatchString(text) {
		match := teacherRegex.FindString(text)
		searchTerm := bot.ExtractSearchTerm(text, match)

		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender("👨‍🏫 請輸入教師姓名\n\n例如：\n• 老師 王小明\n• 教師 李大華\n• 王小明老師\n\n💡 只輸入姓氏也可以（如：老師 王）", sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("📚 按課程查詢", "課程")},
				{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
			})
			return []messaging_api.MessageInterface{msg}
		}
		return h.handleTeacherSearch(ctx, searchTerm)
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
			return h.handleTeacherSearch(ctx, teacherName)
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
	course, err := h.db.GetCourseByUID(uid)
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
			{Action: lineutil.NewMessageAction("📚 按課名查詢", "課程")},
			{Action: lineutil.NewMessageAction("👨‍🏫 按教師查詢", "老師")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Check if course was found (prevent nil pointer dereference)
	if course == nil {
		log.Warnf("Course UID %s not found after scraping", uid)
		h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無課程編號 %s\n\n💡 請確認：\n• 課程編號拼寫是否正確\n• 該課程是否在本學期或上學期開設", uid),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📚 按課名查詢", "課程")},
			{Action: lineutil.NewMessageAction("👨‍🏫 按教師查詢", "老師")},
			{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache
	if err := h.db.SaveCourse(course); err != nil {
		log.WithError(err).Warn("Failed to save course to cache")
	}

	h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
	return h.formatCourseResponse(course)
}

// handleCourseTitleSearch handles course title search queries
func (h *Handler) handleCourseTitleSearch(ctx context.Context, title string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Search in cache first
	courses, err := h.db.SearchCoursesByTitle(title)
	if err != nil {
		log.WithError(err).Error("Failed to search courses in cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("搜尋課程時發生問題", sender, "課程 "+title),
		}
	}

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Found %d courses in cache for title: %s", len(courses), title)
		return h.formatCourseListResponse(courses)
	}

	// Cache miss - Try scraping from current and previous semester
	log.Infof("Cache miss for course title: %s, scraping from recent semesters...", title)
	h.metrics.RecordCacheMiss(moduleName)

	// Get semesters to search based on current date
	searchYears, searchTerms := getSemestersToSearch()

	// Search courses from multiple semesters
	foundCourses := make([]*storage.Course, 0)
	for i := range searchYears {
		year := searchYears[i]
		term := searchTerms[i]

		scrapedCourses, err := ntpu.ScrapeCourses(ctx, h.scraper, year, term, title)
		if err != nil {
			log.WithError(err).WithField("year", year).WithField("term", term).
				Debug("Failed to scrape courses for year/term")
			continue
		}

		// Save courses to cache
		for _, course := range scrapedCourses {
			if err := h.db.SaveCourse(course); err != nil {
				log.WithError(err).Warn("Failed to save course to cache")
			}
		}

		foundCourses = append(foundCourses, scrapedCourses...)
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
		"🔍 查無包含「%s」的課程\n\n💡 請確認：\n• 課程名稱是否正確\n• 該課程是否在本學期或上學期開設\n• 或使用課程編號直接查詢（如：3141U0001）",
		title,
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
		return h.handleCourseTitleSearch(ctx, keyword)
	}

	// Search in historical_courses cache first
	courses, err := h.db.SearchHistoricalCoursesByYearAndTitle(year, keyword)
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
		if err := h.db.SaveHistoricalCourse(course); err != nil {
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

// handleTeacherSearch handles teacher search queries with a 2-tier search strategy:
//
// Search Strategy:
//
//  1. SQL LIKE (fast path): Direct database LIKE query for teacher name substrings.
//     Example: "王教授" matches courses where any teacher contains "王教授"
//
//  2. Fuzzy character-set matching (cache fallback): If SQL LIKE returns no results,
//     loads up to 2000 recent courses and checks if all runes in teacherName exist in each teacher.
//     Example: "王" matches "王小明" because all chars exist in the teacher name
//     This enables single-character surname search.
//
// Note: Unlike contact search, teacher search does NOT use search variants for scraping.
// If cache miss occurs, it triggers a full semester scrape (heavy operation).
// Future optimization: Add "semester fully scraped" flag to avoid repeated scrapes.
//
// Performance notes:
//   - SQL LIKE is indexed and fast; most queries resolve here
//   - Fuzzy matching iterates O(n*m) where n=courses, m=teachers per course
//   - Current limit of 2000 courses is acceptable within 25s webhook timeout
func (h *Handler) handleTeacherSearch(ctx context.Context, teacherName string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Search in cache using SQL LIKE first
	courses, err := h.db.SearchCoursesByTeacher(teacherName)
	if err != nil {
		log.WithError(err).Error("Failed to search courses by teacher")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("搜尋教師課程時發生問題", sender, "老師 "+teacherName),
		}
	}

	// If SQL LIKE didn't find results, try fuzzy character-set matching
	// This enables "王" to match "王小明" teacher names
	if len(courses) == 0 {
		allCourses, err := h.db.GetCoursesByRecentSemesters()
		if err == nil && len(allCourses) > 0 {
			for _, c := range allCourses {
				for _, teacher := range c.Teachers {
					if lineutil.ContainsAllRunes(teacher, teacherName) {
						courses = append(courses, c)
						break
					}
				}
			}
		}
	}

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Found %d courses for teacher: %s", len(courses), teacherName)
		return h.formatCourseListResponse(courses)
	}

	// Cache miss - Try scraping recent semesters
	// Note: This triggers a full scrape of all courses for the semester if the teacher is not found in cache.
	// This is a heavy operation (iterates through all education codes U/M/N/P) but necessary as the
	// school system doesn't support direct teacher search via URL parameters.
	// Future optimization: Implement a "semester fully scraped" flag to avoid repeated scrapes for non-existent teachers.
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("Cache miss for teacher: %s, trying to scrape...", teacherName)

	// Get semesters to search based on current date
	searchYears, searchTerms := getSemestersToSearch()

	// Search and save courses
	foundCourses := make([]*storage.Course, 0)
	for i := range searchYears {
		year := searchYears[i]
		term := searchTerms[i]

		// Scrape all courses for this semester
		scrapedCourses, err := ntpu.ScrapeCourses(ctx, h.scraper, year, term, "")
		if err != nil {
			log.WithError(err).WithField("year", year).WithField("term", term).
				Debug("Failed to scrape courses for year/term")
			continue
		}

		// Filter by teacher and save to cache
		for _, course := range scrapedCourses {
			// Save all courses for future queries
			if err := h.db.SaveCourse(course); err != nil {
				log.WithError(err).Warn("Failed to save course to cache")
			}

			// Check if teacher matches using fuzzy matching
			for _, teacher := range course.Teachers {
				if lineutil.ContainsAllRunes(teacher, teacherName) {
					foundCourses = append(foundCourses, course)
					break
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

	// No results found
	h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
	msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf(
		"🔍 查無教師「%s」的授課課程\n\n💡 請確認：\n• 教師姓名是否正確（可嘗試只輸入姓氏）\n• 該教師本學期或上學期是否有開課\n• 若為兼任或新進教師，資料可能尚未更新",
		teacherName,
	), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("🔄 重試", "老師 "+teacherName)},
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

	// 時間 info
	if len(course.Times) > 0 {
		timeStr := strings.Join(course.Times, "、")
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

	// Build footer actions
	var footerContents []messaging_api.FlexComponentInterface

	// Course Outline button (label: 6 chars + emoji)
	if course.DetailURL != "" {
		footerContents = append(footerContents, lineutil.NewFlexButton(
			lineutil.NewURIAction("📄 課程大綱", course.DetailURL),
		).WithStyle("primary").WithHeight("sm").FlexButton)
	}

	// Course Query System button (label: 6 chars + emoji)
	courseQueryURL := fmt.Sprintf("https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.queryByKeyword?qYear=%d&qTerm=%d&courseno=%s&seq1=A&seq2=M",
		course.Year, course.Term, course.No)
	footerContents = append(footerContents, lineutil.NewFlexButton(
		lineutil.NewURIAction("🔍 查詢系統", courseQueryURL),
	).WithStyle("secondary").WithHeight("sm").FlexButton)

	// Teacher schedule button (if teachers exist)
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		// Truncate teacher name in display text if too long (using rune slicing for UTF-8 safety)
		displayText := lineutil.TruncateRunes(fmt.Sprintf("搜尋 %s 的授課課程", teacherName), 40)
		// Use course: prefix for proper postback routing
		footerContents = append(footerContents, lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText(
				"👤 教師課程",
				displayText,
				fmt.Sprintf("course:授課課程%s%s", bot.PostbackSplitChar, teacherName),
			),
		).WithStyle("secondary").WithHeight("sm").FlexButton)
	}

	bubble := lineutil.NewFlexBubble(
		header,
		hero.FlexBox,
		body.Build(),
		lineutil.NewFlexBox("vertical", footerContents...).WithSpacing("sm"),
	)

	// Limit altText to 400 chars (LINE API limit, using rune slicing for UTF-8 safety)
	altText := lineutil.TruncateRunes(fmt.Sprintf("課程：%s", course.Title), 400)
	msg := lineutil.NewFlexMessage(altText, bubble.FlexBubble)
	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg.Sender = sender

	// Add Quick Reply for related actions
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("查詢其他課程", "課程")},
		{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
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
		// 第三列：上課時間
		if len(course.Times) > 0 {
			// Display times with truncation if too many (max 4, then "等 N 節")
			carouselTimes := lineutil.FormatTimes(course.Times, 4)
			contents = append(contents,
				lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
				lineutil.NewFlexBox("horizontal",
					lineutil.NewFlexText("⏰ 上課時間：").WithSize("xs").WithColor(lineutil.ColorLabel).WithFlex(0).FlexText,
					lineutil.NewFlexText(carouselTimes).WithColor(lineutil.ColorSubtext).WithSize("xs").WithFlex(1).WithWrap(true).FlexText,
				).WithMargin("sm").WithSpacing("sm").FlexBox,
			)
		}
		// Footer with "View Detail" button - displayText shows course title
		displayText := fmt.Sprintf("查詢「%s」課程", lineutil.TruncateRunes(course.Title, 30))
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
	messages = lineutil.BuildCarouselMessages("課程列表", bubbles, sender)

	// Prepend warning message if results were truncated
	if truncated {
		warningMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 搜尋結果超過 %d 門課程，僅顯示前 %d 門。\n\n建議使用更精確的搜尋條件以縮小範圍。", originalCount, MaxCoursesPerSearch),
			sender,
		)
		messages = append([]messaging_api.MessageInterface{warningMsg}, messages...)
	}

	// Add Quick Reply to the last message
	lineutil.AddQuickReplyToMessages(messages,
		lineutil.QuickReplyItem{Action: lineutil.NewMessageAction("重新查詢", "課程")},
		lineutil.QuickReplyHelpAction(),
	)

	return messages
}
