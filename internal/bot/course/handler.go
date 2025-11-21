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
		"class", "course", "課", "課程", "科目", "課名", "課程名", "課程名稱", "科目名",
	}
	validTeacherKeywords = []string{
		"dr", "prof", "teacher", "professor", "doctor", "師", "老師", "教師", "教授",
		"老師名", "教師名", "教授名", "老師名稱", "教師名稱", "教授名稱",
		"授課教師", "授課老師", "授課教授",
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

	// Check for course UID
	if match := uidRegex.FindString(text); match != "" {
		return h.handleCourseUIDQuery(ctx, match)
	}

	// Check for course title search
	if match := courseRegex.FindString(text); match != "" {
		return h.handleCourseTitleSearch(ctx, match)
	}

	// Check for teacher search
	if match := teacherRegex.FindString(text); match != "" {
		return h.handleTeacherSearch(ctx, match)
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
	// Format course information
	var builder strings.Builder
	builder.WriteString("📚 課程資訊\n\n")
	builder.WriteString(fmt.Sprintf("課程名稱：%s\n", course.Title))
	builder.WriteString(fmt.Sprintf("課程編號：%s\n", course.UID))
	builder.WriteString(fmt.Sprintf("學年學期：%d 學年第 %d 學期\n", course.Year, course.Term))

	if len(course.Teachers) > 0 {
		builder.WriteString(fmt.Sprintf("授課教師：%s\n", strings.Join(course.Teachers, "、")))
	}

	if len(course.Times) > 0 {
		builder.WriteString(fmt.Sprintf("上課時間：%s\n", strings.Join(course.Times, "、")))
	}

	if len(course.Locations) > 0 {
		builder.WriteString(fmt.Sprintf("上課地點：%s\n", strings.Join(course.Locations, "、")))
	}

	if course.Note != "" {
		builder.WriteString(fmt.Sprintf("\n備註：%s\n", course.Note))
	}

	if fromCache {
		builder.WriteString("\n📌 資料來自快取")
	}

	messages := []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithSender(builder.String(), senderName, h.stickerManager.GetRandomSticker()),
	}

	// Add detail URL button if available
	if course.DetailURL != "" {
		actions := []lineutil.Action{
			lineutil.NewURIAction("查看課程大綱", course.DetailURL),
		}

		messages = append(messages, lineutil.NewButtonsTemplate(
			"課程資訊",
			"",
			"點擊查看更多資訊",
			actions,
		))
	}

	return messages
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

	messages := make([]messaging_api.MessageInterface, 0)

	// Split into groups of 20 per message
	for i := 0; i < len(courses); i += 20 {
		end := i + 20
		if end > len(courses) {
			end = len(courses)
		}

		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("📚 課程列表 (第 %d-%d 筆，共 %d 筆)：\n\n", i+1, end, len(courses)))

		for j := i; j < end; j++ {
			course := courses[j]

			// Format: Title (UID) - Teachers
			builder.WriteString(fmt.Sprintf("📖 %s\n", course.Title))
			builder.WriteString(fmt.Sprintf("編號：%s\n", course.UID))

			if len(course.Teachers) > 0 {
				builder.WriteString(fmt.Sprintf("教師：%s\n", strings.Join(course.Teachers, "、")))
			}

			builder.WriteString(fmt.Sprintf("學期：%d-%d\n", course.Year, course.Term))

			if len(course.Times) > 0 {
				builder.WriteString(fmt.Sprintf("時間：%s\n", strings.Join(course.Times, "、")))
			}

			builder.WriteString("\n")
		}

		// Add helpful text at the end
		if i == 0 {
			builder.WriteString("💡 提示：輸入課程編號可查看詳細資訊")
		}

		messages = append(messages, lineutil.NewTextMessageWithSender(builder.String(), senderName, h.stickerManager.GetRandomSticker()))
	}

	return messages
}
