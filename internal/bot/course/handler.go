package course

import (
	"context"
	"fmt"
	"regexp"
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
	splitChar            = "$"
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

	courseRegex  = buildRegex(validCourseKeywords)
	teacherRegex = buildRegex(validTeacherKeywords)
	// UID format: {year}{term}{course_no} where term is U/M/N/P (case-insensitive)
	// Example: 11312U0001, 11312u0001, 11312M0001, 11312m0001
	uidRegex = regexp.MustCompile(`(?i)\d{3,4}[umnp]\d{4}`)
)

// buildRegex creates a regex pattern from keywords
func buildRegex(keywords []string) *regexp.Regexp {
	pattern := "(?i)" + strings.Join(keywords, "|")
	return regexp.MustCompile(pattern)
}

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

	// Check for course title search - extract term after keyword
	// Support both "keyword term" and "term keyword" patterns
	if courseRegex.MatchString(text) {
		match := courseRegex.FindString(text)
		// Try extracting term after keyword first
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))

		// If keyword is at the end or no search term, extract from beginning
		if searchTerm == "" || strings.HasSuffix(text, match) {
			// Extract what comes before the keyword
			searchTerm = strings.TrimSpace(strings.TrimSuffix(text, match))
		}

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
		// Try extracting term after keyword first
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))

		// If keyword is at the end or no search term, extract from beginning
		if searchTerm == "" || strings.HasSuffix(text, match) {
			// Extract what comes before the keyword
			searchTerm = strings.TrimSpace(strings.TrimSuffix(text, match))
		}

		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender("👨‍🏫 請輸入教師姓名\n\n例如：\n• 老師 王小明\n• 教師 李大華\n• 王小明老師\n\n💡 只輸入姓氏也可以（如：老師 王）", sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("📚 按課程查詢", "課程")},
				{Action: lineutil.NewMessageAction("📌 使用說明", "使用說明")},
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

	// Check for course UID in postback (with or without prefix)
	if uidRegex.MatchString(data) {
		return h.handleCourseUIDQuery(ctx, data)
	}

	// Handle "授課課程" postback (with or without prefix)
	if strings.HasPrefix(data, "授課課程") {
		parts := strings.Split(data, splitChar)
		if len(parts) >= 2 {
			teacherName := parts[1]
			return h.handleTeacherSearch(ctx, teacherName)
		}
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
		msg := lineutil.ErrorMessageWithDetailAndSender("查詢課程時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重試", uid)},
				{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
			})
		}
		return []messaging_api.MessageInterface{msg}
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
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf("❌ 查無課程編號 %s\n\n請確認課程編號是否正確", uid), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("按課名查詢", "課程")},
			{Action: lineutil.NewMessageAction("按教師查詢", "老師")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Check if course was found (prevent nil pointer dereference)
	if course == nil {
		log.Warnf("Course UID %s not found after scraping", uid)
		h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("❌ 查無課程編號 %s\n\n💡 請確認：\n• 課程編號拼寫是否正確\n• 該課程是否在本學期或上學期開設", uid),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("📚 按課名查詢", "課程")},
			{Action: lineutil.NewMessageAction("👨‍🏫 按教師查詢", "老師")},
			{Action: lineutil.NewMessageAction("📌 使用說明", "使用說明")},
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
		msg := lineutil.ErrorMessageWithDetailAndSender("搜尋課程時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重試", "課程 "+title)},
				{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
			})
		}
		return []messaging_api.MessageInterface{msg}
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
		"🔍 查無包含「%s」的課程\n\n請確認：\n• 課程名稱是否正確\n• 該課程是否在本學期或上學期開設\n• 或使用課程編號直接查詢（如：3141U0001）",
		title,
	), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("重新查詢", "課程")},
		{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
	})
	return []messaging_api.MessageInterface{msg}
}

// handleTeacherSearch handles teacher search queries
func (h *Handler) handleTeacherSearch(ctx context.Context, teacherName string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Search in cache
	courses, err := h.db.SearchCoursesByTeacher(teacherName)
	if err != nil {
		log.WithError(err).Error("Failed to search courses by teacher")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetailAndSender("搜尋教師課程時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重試", "老師 "+teacherName)},
				{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
			})
		}
		return []messaging_api.MessageInterface{msg}
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

			// Check if teacher matches
			for _, teacher := range course.Teachers {
				if strings.Contains(teacher, teacherName) {
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
		"🔍 查無教師「%s」的授課課程\n\n請確認：\n• 教師姓名是否正確（可嘗試只輸入姓氏）\n• 該教師本學期或上學期是否有開課\n• 若為兼任或新進教師，資料可能尚未更新",
		teacherName,
	), sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("重試", "老師 "+teacherName)},
		{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
	})
	return []messaging_api.MessageInterface{msg}
}

// formatCourseResponse formats a single course as a LINE message
func (h *Handler) formatCourseResponse(course *storage.Course) []messaging_api.MessageInterface {
	// Header: Course badge
	header := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexBox("baseline",
			lineutil.NewFlexText("📚").WithSize("lg").FlexText,
			lineutil.NewFlexText("課程資訊").WithWeight("bold").WithColor("#1DB446").WithSize("sm").WithMargin("sm").FlexText,
		).FlexBox,
	)

	// Hero: Course title and code
	// Truncate title if too long (max ~60 chars for better display)
	// Use rune slicing for proper UTF-8 multi-byte character handling
	displayTitle := lineutil.TruncateRunes(course.Title, MaxTitleDisplayChars)
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText(displayTitle).WithWeight("bold").WithSize("xl").WithColor("#ffffff").WithWrap(true).WithMaxLines(2).FlexText,
		lineutil.NewFlexText(course.UID).WithSize("xs").WithColor("#ffffff").WithMargin("md").FlexText,
	).FlexBox
	hero.BackgroundColor = "#1DB446"
	hero.PaddingAll = "20px"
	hero.PaddingBottom = "16px"

	// Build body contents
	contents := []messaging_api.FlexComponentInterface{}

	// Add details
	if len(course.Teachers) > 0 {
		// Truncate teacher names if too long (max ~40 chars, using rune slicing)
		teacherNames := lineutil.TruncateRunes(strings.Join(course.Teachers, "、"), 40)
		contents = append(contents, lineutil.NewKeyValueRow("👨‍🏫 教師", teacherNames).WithMargin("lg").FlexBox)
	}
	contents = append(contents,
		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
		lineutil.NewKeyValueRow("📅 學期", fmt.Sprintf("%d-%d", course.Year, course.Term)).WithMargin("md").FlexBox,
	)
	if len(course.Times) > 0 {
		// Truncate times if too long (max ~50 chars, using rune slicing)
		timeStr := lineutil.TruncateRunes(strings.Join(course.Times, "、"), 50)
		contents = append(contents,
			lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
			lineutil.NewKeyValueRow("⏰ 時間", timeStr).WithMargin("md").FlexBox,
		)
	}
	if len(course.Locations) > 0 {
		// Truncate locations if too long (max ~40 chars, using rune slicing)
		locationStr := lineutil.TruncateRunes(strings.Join(course.Locations, "、"), 40)
		contents = append(contents,
			lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
			lineutil.NewKeyValueRow("📍 地點", locationStr).WithMargin("md").FlexBox,
		)
	}
	if course.Note != "" {
		// Truncate note if too long (max ~80 chars for better readability, using rune slicing)
		noteStr := lineutil.TruncateRunes(course.Note, 80)
		contents = append(contents,
			lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
			lineutil.NewKeyValueRow("📝 備註", noteStr).WithMargin("md").FlexBox,
		)
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

	// Teacher schedule button (if teachers exist) (label: 6 chars + emoji)
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		// Truncate teacher name in display text if too long (using rune slicing for UTF-8 safety)
		displayText := lineutil.TruncateRunes(fmt.Sprintf("搜尋 %s 的授課課程", teacherName), 40)
		footerContents = append(footerContents, lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText(
				"👤 教師課程",
				displayText,
				fmt.Sprintf("授課課程%s%s", splitChar, teacherName),
			),
		).WithStyle("secondary").WithHeight("sm").FlexButton)
	}

	bubble := lineutil.NewFlexBubble(
		header,
		hero,
		lineutil.NewFlexBox("vertical", contents...).WithSpacing("sm"),
		lineutil.NewFlexBox("vertical", footerContents...).WithSpacing("sm"),
	)

	// Limit altText to 400 chars (LINE API limit, using rune slicing for UTF-8 safety)
	altText := lineutil.TruncateRunes(fmt.Sprintf("課程：%s", course.Title), 400)
	msg := lineutil.NewFlexMessage(altText, bubble.FlexBubble)

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

	// Limit to 50 courses - add warning if truncated
	originalCount := len(courses)
	if len(courses) > MaxCoursesPerSearch {
		courses = courses[:MaxCoursesPerSearch]
		warningMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 搜尋結果超過 %d 門課程，僅顯示前 %d 門。\n\n建議使用更精確的搜尋條件以縮小範圍。", originalCount, MaxCoursesPerSearch),
			sender,
		)
		messages = append(messages, warningMsg)
	}

	// Create bubbles for carousel (LINE API limit: max 10 bubbles per Flex Carousel)
	var bubbles []messaging_api.FlexBubble
	for _, course := range courses {
		// Hero: Course title with color background
		// Truncate title for carousel display (max ~50 chars, using rune slicing)
		carouselTitle := lineutil.TruncateRunes(course.Title, 50)
		hero := lineutil.NewFlexBox("vertical",
			lineutil.NewFlexText(carouselTitle).WithWeight("bold").WithSize("md").WithColor("#ffffff").WithWrap(true).WithMaxLines(2).FlexText,
		).FlexBox
		hero.BackgroundColor = "#1DB446"
		hero.PaddingAll = "15px"
		hero.PaddingBottom = "13px"

		// Build body contents
		contents := []messaging_api.FlexComponentInterface{
			lineutil.NewFlexText(course.UID).WithSize("xs").WithColor("#999999").WithMargin("md").FlexText,
			lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		}

		if len(course.Teachers) > 0 {
			// Truncate teachers for carousel (max ~30 chars, using rune slicing)
			carouselTeachers := lineutil.TruncateRunes(strings.Join(course.Teachers, "、"), 30)
			contents = append(contents, lineutil.NewKeyValueRow("👨‍🏫 教師", carouselTeachers).WithMargin("md").FlexBox)
		}
		if len(course.Times) > 0 {
			// Truncate times for carousel (max ~35 chars, using rune slicing)
			carouselTimes := lineutil.TruncateRunes(strings.Join(course.Times, "、"), 35)
			contents = append(contents,
				lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
				lineutil.NewKeyValueRow("⏰ 時間", carouselTimes).WithMargin("sm").FlexBox,
			)
		}
		// Footer with "View Detail" button
		footer := lineutil.NewFlexBox("vertical",
			lineutil.NewFlexButton(
				lineutil.NewPostbackActionWithDisplayText("📝 查看詳細", fmt.Sprintf("查詢課程 %s", course.UID), course.UID),
			).WithStyle("primary").WithHeight("sm").FlexButton,
		).WithSpacing("sm")

		bubble := lineutil.NewFlexBubble(
			nil,
			hero,
			lineutil.NewFlexBox("vertical", contents...).WithSpacing("sm"),
			footer,
		)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Split bubbles into carousels (LINE API limit: max 10 bubbles per Flex Carousel)
	for i := 0; i < len(bubbles); i += 10 {
		end := i + 10
		if end > len(bubbles) {
			end = len(bubbles)
		}

		carouselBubbles := bubbles[i:end]
		carousel := &messaging_api.FlexCarousel{
			Contents: carouselBubbles,
		}

		messages = append(messages, lineutil.NewFlexMessage("課程列表", carousel))
	}

	// Add Quick Reply to the last message
	if len(messages) > 0 {
		if flexMsg, ok := messages[len(messages)-1].(*messaging_api.FlexMessage); ok {
			flexMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重新查詢", "課程")},
				{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
			})
		}
	}

	return messages
}
