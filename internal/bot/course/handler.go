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
	moduleName = "course"
	splitChar  = "$"
	senderName = "課程魔法師"
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
	uidRegex     = regexp.MustCompile(`\d{3,4}[UMNPumnp]\d{4}`)
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
	if match := courseRegex.FindString(text); match != "" {
		// Extract what comes after the keyword
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))
		if searchTerm == "" {
			// If no search term provided, give helpful message
			return []messaging_api.MessageInterface{
				lineutil.NewTextMessageWithSender("請在關鍵字後輸入課程名稱\n\n例如：課 程式設計、課程 微積分", senderName, h.stickerManager.GetRandomSticker()),
			}
		}
		return h.handleCourseTitleSearch(ctx, searchTerm)
	}

	// Check for teacher search - extract term after keyword
	if match := teacherRegex.FindString(text); match != "" {
		// Extract what comes after the keyword
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))
		if searchTerm == "" {
			// If no search term provided, give helpful message
			return []messaging_api.MessageInterface{
				lineutil.NewTextMessageWithSender("請在關鍵字後輸入教師姓名\n\n例如：老師 王小明、教師 李大華", senderName, h.stickerManager.GetRandomSticker()),
			}
		}
		return h.handleTeacherSearch(ctx, searchTerm)
	}

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the course module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	log.Infof("Handling course postback: %s", data)

	// Check for course UID in postback
	if uidRegex.MatchString(data) {
		return h.handleCourseUIDQuery(ctx, data)
	}

	// Handle "授課課程" postback
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

	// Normalize UID to uppercase
	uid = strings.ToUpper(uid)

	// Check cache first
	course, err := h.db.GetCourseByUID(uid)
	if err != nil {
		log.WithError(err).Error("Failed to query cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithDetail("查詢課程時發生問題"),
		}
	}

	if course != nil {
		// Cache hit
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Cache hit for course UID: %s", uid)
		return h.formatCourseResponse(course, true)
	}

	// Cache miss - scrape from website
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("Cache miss for course UID: %s, scraping...", uid)

	course, err = ntpu.ScrapeCourseByUID(ctx, h.scraper, uid)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape course UID: %s", uid)
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithSender(fmt.Sprintf("❌ 查無課程編號 %s\n\n請確認課程編號是否正確", uid), senderName, h.stickerManager.GetRandomSticker()),
		}
	}

	// Save to cache
	if err := h.db.SaveCourse(course); err != nil {
		log.WithError(err).Warn("Failed to save course to cache")
	}

	h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
	return h.formatCourseResponse(course, false)
}

// handleCourseTitleSearch handles course title search queries
func (h *Handler) handleCourseTitleSearch(ctx context.Context, title string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()

	// Search in cache first
	courses, err := h.db.SearchCoursesByTitle(title)
	if err != nil {
		log.WithError(err).Error("Failed to search courses in cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithDetail("搜尋課程時發生問題"),
		}
	}

	if len(courses) > 0 {
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Found %d courses for title: %s", len(courses), title)
		return h.formatCourseListResponse(courses)
	}

	// No results found
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("No courses found for title: %s", title)
	return []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithSender(fmt.Sprintf(
			"🔍 查無包含「%s」的課程\n\n請確認課程名稱是否正確，或使用課程編號查詢。",
			title,
		), senderName, h.stickerManager.GetRandomSticker()),
	}
}

// handleTeacherSearch handles teacher search queries
func (h *Handler) handleTeacherSearch(ctx context.Context, teacherName string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()

	// Search in cache
	courses, err := h.db.SearchCoursesByTeacher(teacherName)
	if err != nil {
		log.WithError(err).Error("Failed to search courses by teacher")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithDetail("搜尋教師課程時發生問題"),
		}
	}

	if len(courses) == 0 {
		h.metrics.RecordCacheMiss(moduleName)
		log.Infof("No courses found for teacher: %s", teacherName)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithSender(fmt.Sprintf(
				"🔍 查無教師「%s」的授課課程\n\n請確認教師姓名是否正確，或使用課程編號查詢。",
				teacherName,
			), senderName, h.stickerManager.GetRandomSticker()),
		}
	}

	h.metrics.RecordCacheHit(moduleName)
	log.Infof("Found %d courses for teacher: %s", len(courses), teacherName)
	return h.formatCourseListResponse(courses)
}

// formatCourseResponse formats a single course as a LINE message
func (h *Handler) formatCourseResponse(course *storage.Course, fromCache bool) []messaging_api.MessageInterface {
	// Build body contents
	contents := []messaging_api.FlexComponentInterface{
		lineutil.NewFlexText(course.Title).WithWeight("bold").WithSize("xl"),
		lineutil.NewFlexText(course.UID).WithSize("xs").WithColor("#aaaaaa").WithWrap(true),
		lineutil.NewFlexSeparator().WithMargin("md"),
	}

	// Add details
	if len(course.Teachers) > 0 {
		contents = append(contents, lineutil.NewKeyValueRow("教師", strings.Join(course.Teachers, "、")).WithMargin("md"))
	}
	contents = append(contents, lineutil.NewKeyValueRow("學期", fmt.Sprintf("%d-%d", course.Year, course.Term)))
	if len(course.Times) > 0 {
		contents = append(contents, lineutil.NewKeyValueRow("時間", strings.Join(course.Times, "、")))
	}
	if len(course.Locations) > 0 {
		contents = append(contents, lineutil.NewKeyValueRow("地點", strings.Join(course.Locations, "、")))
	}
	if course.Note != "" {
		contents = append(contents, lineutil.NewKeyValueRow("備註", course.Note))
	}

	// Build footer actions
	var footerContents []messaging_api.FlexComponentInterface

	// Course Outline button
	if course.DetailURL != "" {
		footerContents = append(footerContents, lineutil.NewFlexButton(
			lineutil.NewURIAction("課程大綱", course.DetailURL),
		).WithStyle("primary").WithColor("#00b900"))
	}

	// Course Query System button
	courseQueryURL := fmt.Sprintf("https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.queryByKeyword?qYear=%d&qTerm=%d&courseno=%s&seq1=A&seq2=M",
		course.Year, course.Term, course.No)
	footerContents = append(footerContents, lineutil.NewFlexButton(
		lineutil.NewURIAction("課程查詢系統", courseQueryURL),
	).WithStyle("secondary"))

	// Teacher schedule button (if teachers exist)
	if len(course.Teachers) > 0 {
		teacherName := course.Teachers[0]
		footerContents = append(footerContents, lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText(
				"查看教師資訊",
				fmt.Sprintf("搜尋 %s 的授課課程", teacherName),
				fmt.Sprintf("授課課程%s%s", splitChar, teacherName),
			),
		).WithStyle("secondary"))
	}

	bubble := lineutil.NewFlexBubble(
		nil,
		nil,
		lineutil.NewFlexBox("vertical", contents...),
		lineutil.NewFlexBox("vertical", footerContents...).WithSpacing("sm"),
	)

	msg := lineutil.NewFlexMessage(fmt.Sprintf("課程：%s", course.Title), bubble.FlexBubble)

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
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithSender("🔍 查無課程資料", senderName, h.stickerManager.GetRandomSticker()),
		}
	}

	// Limit to 50 courses
	if len(courses) > 50 {
		courses = courses[:50]
	}

	var messages []messaging_api.MessageInterface

	// Create bubbles for carousel (max 12 per carousel)
	var bubbles []messaging_api.FlexBubble
	for _, course := range courses {
		// Build body contents
		contents := []messaging_api.FlexComponentInterface{
			lineutil.NewFlexText(course.Title).WithWeight("bold").WithSize("md").WithWrap(true),
			lineutil.NewFlexText(course.UID).WithSize("xs").WithColor("#aaaaaa"),
			lineutil.NewFlexSeparator().WithMargin("md"),
		}

		if len(course.Teachers) > 0 {
			contents = append(contents, lineutil.NewKeyValueRow("教師", strings.Join(course.Teachers, "、")).WithMargin("md"))
		}
		contents = append(contents, lineutil.NewKeyValueRow("時間", strings.Join(course.Times, "、")))

		// Footer with "View Detail" button
		footer := lineutil.NewFlexBox("vertical",
			lineutil.NewFlexButton(
				lineutil.NewPostbackActionWithDisplayText("查看詳細", fmt.Sprintf("查詢課程 %s", course.UID), course.UID),
			).WithStyle("primary").WithHeight("sm"),
		)

		bubble := lineutil.NewFlexBubble(
			nil,
			nil,
			lineutil.NewFlexBox("vertical", contents...),
			footer,
		)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Split bubbles into carousels (max 12 bubbles per carousel)
	for i := 0; i < len(bubbles); i += 12 {
		end := i + 12
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
