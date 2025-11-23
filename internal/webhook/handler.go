package webhook

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/course"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/id"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"
)

// Handler handles LINE webhook events
type Handler struct {
	channelSecret  string
	client         *messaging_api.MessagingApiAPI
	metrics        *metrics.Metrics
	logger         *logger.Logger
	idHandler      *id.Handler
	contactHandler *contact.Handler
	courseHandler  *course.Handler
	rateLimiter    *RateLimiter     // Global rate limiter for API calls
	userLimiter    *UserRateLimiter // Per-user rate limiter
	stickerManager *sticker.Manager // Sticker manager for avatar URLs
}

// NewHandler creates a new webhook handler
func NewHandler(channelSecret, channelToken string, db *storage.DB, scraperClient *scraper.Client, m *metrics.Metrics, log *logger.Logger, stickerManager *sticker.Manager) (*Handler, error) {
	// Create messaging API client
	client, err := messaging_api.NewMessagingApiAPI(channelToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create messaging API client: %w", err)
	}

	// Initialize bot module handlers with sticker manager
	idHandler := id.NewHandler(db, scraperClient, m, log, stickerManager)
	contactHandler := contact.NewHandler(db, scraperClient, m, log, stickerManager)
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerManager)

	// Initialize rate limiters
	// LINE API rate limits: https://developers.line.biz/en/reference/messaging-api/#rate-limits
	// Global: 100 requests per second (we use 80 to be safe)
	globalRateLimiter := NewRateLimiter(80.0, 80.0)

	// Per-user: 10 requests per second per user
	userRateLimiter := NewUserRateLimiter(5 * time.Minute)

	return &Handler{
		channelSecret:  channelSecret,
		client:         client,
		metrics:        m,
		logger:         log,
		idHandler:      idHandler,
		contactHandler: contactHandler,
		courseHandler:  courseHandler,
		rateLimiter:    globalRateLimiter,
		userLimiter:    userRateLimiter,
		stickerManager: stickerManager,
	}, nil
}

// Handle processes incoming webhook requests
func (h *Handler) Handle(c *gin.Context) {
	start := time.Now()

	// Record request size
	if c.Request.ContentLength > 0 {
		h.metrics.RecordHTTPRequest("/callback", c.Request.Method, c.Request.ContentLength)
	}

	// Validate Content-Length to prevent abuse
	if c.Request.ContentLength > 1<<20 { // 1MB limit
		h.logger.Warn("Request body too large")
		h.metrics.RecordHTTPError("request_too_large", "webhook")
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request too large"})
		return
	}

	// Parse webhook request with signature verification
	cb, err := webhook.ParseRequest(h.channelSecret, c.Request)
	if err != nil {
		h.logger.WithError(err).Error("Failed to parse webhook request")
		if err == webhook.ErrInvalidSignature {
			// Invalid signature - potential security threat
			h.metrics.RecordWebhook("invalid_signature", "error", time.Since(start).Seconds())
			h.metrics.RecordHTTPError("invalid_signature", "webhook")
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		} else {
			h.metrics.RecordHTTPError("parse_error", "webhook")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse request"})
		}
		return
	}

	// Process each event (max 100 events per webhook)
	if len(cb.Events) > 100 {
		h.logger.Warnf("Too many events in single webhook: %d", len(cb.Events))
		cb.Events = cb.Events[:100] // Limit to prevent DoS
	}

	for _, event := range cb.Events {
		eventStart := time.Now()
		var messages []messaging_api.MessageInterface
		var eventType string
		var err error

		switch e := event.(type) {
		case webhook.MessageEvent:
			eventType = "message"
			messages, err = h.handleMessageEvent(c.Request.Context(), e)
		case webhook.PostbackEvent:
			eventType = "postback"
			messages, err = h.handlePostbackEvent(c.Request.Context(), e)
		case webhook.FollowEvent:
			eventType = "follow"
			messages, err = h.handleFollowEvent(e)
		default:
			// Unsupported event type, skip
			h.logger.WithField("event_type", fmt.Sprintf("%T", e)).Debug("Unsupported event type")
			continue
		}

		// Record metrics
		duration := time.Since(eventStart).Seconds()
		status := "success"
		if err != nil {
			status = "error"
			h.logger.WithError(err).WithField("event_type", eventType).Error("Failed to handle event")
		}
		h.metrics.RecordWebhook(eventType, status, duration)

		// Send reply if we have messages
		if len(messages) > 0 && err == nil {
			// Show loading animation (non-blocking, best effort)
			if err := h.showLoadingAnimation(event); err != nil {
				h.logger.WithError(err).Debug("Failed to show loading animation")
			}

			// LINE API restriction: max 5 messages per reply
			if len(messages) > 5 {
				h.logger.Warnf("Message count %d exceeds limit, truncating to 5", len(messages))
				// Add a warning message at the end
				messages = messages[:4]
				messages = append(messages, lineutil.NewTextMessageWithSender(
					"ℹ️ 由於訊息數量限制，部分內容未完全顯示。\n請使用更具體的關鍵字縮小搜尋範圍。",
					"系統魔法師",
					h.stickerManager.GetRandomSticker(),
				))
			}

			// Reply to the event
			replyToken := h.getReplyToken(event)
			if replyToken == "" {
				h.logger.Warn("Empty reply token, skipping reply")
				continue
			}

			// Validate reply token format (should not be empty or too short)
			if len(replyToken) < 10 {
				h.logger.WithField("token_length", len(replyToken)).Warn("Invalid reply token format")
				continue
			}

			// Check rate limit before sending
			chatID := h.getChatID(event)
			if chatID != "" && !h.userLimiter.Allow(chatID, 10.0, 2.0) {
				h.logger.WithField("chat_id", chatID[:8]+"...").Warn("User rate limit exceeded")
				h.metrics.RecordWebhook(eventType, "rate_limited", time.Since(eventStart).Seconds())
				h.metrics.RecordHTTPError("rate_limit_user", "webhook")
				continue
			}

			// Check global rate limit
			if !h.rateLimiter.Allow() {
				h.logger.Warn("Global rate limit exceeded, waiting...")
				h.metrics.RecordHTTPError("rate_limit_global", "webhook")
				h.rateLimiter.WaitForToken()
			}

			// Send reply with error handling
			if _, err := h.client.ReplyMessage(
				&messaging_api.ReplyMessageRequest{
					ReplyToken: replyToken,
					Messages:   messages,
				},
			); err != nil {
				// Check for specific error types
				errMsg := err.Error()
				if strings.Contains(errMsg, "Invalid reply token") {
					h.logger.WithError(err).Warn("Reply token already used or invalid")
				} else if strings.Contains(errMsg, "rate limit") {
					h.logger.WithError(err).Error("Rate limit exceeded")
				} else {
					h.logger.WithError(err).WithField("reply_token", replyToken[:8]+"...").Error("Failed to send reply")
				}
				h.metrics.RecordWebhook(eventType, "reply_error", time.Since(eventStart).Seconds())
			}
		}
	}

	// Return success response
	duration := time.Since(start).Seconds()
	h.logger.WithField("duration", duration).Debug("Webhook processed")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleMessageEvent processes text message events
func (h *Handler) handleMessageEvent(ctx context.Context, event webhook.MessageEvent) ([]messaging_api.MessageInterface, error) {
	// Handle sticker messages
	if event.Message.GetType() == "sticker" {
		return h.handleStickerMessage(event), nil
	}

	// Only handle text messages
	if event.Message.GetType() != "text" {
		return nil, nil
	}

	textMsg, ok := event.Message.(webhook.TextMessageContent)
	if !ok {
		return nil, fmt.Errorf("failed to cast message to text")
	}

	text := textMsg.Text

	// Validate text length (LINE allows up to 20,000 characters)
	if len(text) == 0 {
		return nil, nil // Empty message, ignore
	}
	if len(text) > 20000 {
		h.logger.Warnf("Text message too long: %d characters", len(text))
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithSender("❌ 訊息內容過長\n\n訊息長度超過 20,000 字元，請縮短後重試。", "系統魔法師", h.stickerManager.GetRandomSticker()),
		}, nil
	}

	// Sanitize input: trim whitespace and remove control characters
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil, nil // Empty after trimming
	}

	h.logger.WithField("text", text).Debug("Received text message")

	// Check for help keywords FIRST (before dispatching to bot modules)
	helpKeywords := []string{"使用說明", "help", "Help", "HELP"}
	for _, keyword := range helpKeywords {
		if strings.EqualFold(text, keyword) {
			h.logger.Info("User requested help/instruction")
			return h.getDetailedInstructionMessages(), nil
		}
	}

	// Create context with timeout for bot processing (derived from request context)
	processCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	// Dispatch to appropriate bot module based on CanHandle
	if h.idHandler.CanHandle(text) {
		return h.idHandler.HandleMessage(processCtx, text), nil
	}

	if h.contactHandler.CanHandle(text) {
		return h.contactHandler.HandleMessage(processCtx, text), nil
	}

	if h.courseHandler.CanHandle(text) {
		return h.courseHandler.HandleMessage(processCtx, text), nil
	}

	// No handler matched - return help message
	// Note: Unlike Python version, we don't check for data source availability here
	// since the database and failover mechanisms handle that automatically
	return h.getHelpMessage(), nil
}

// handlePostbackEvent processes postback events
func (h *Handler) handlePostbackEvent(ctx context.Context, event webhook.PostbackEvent) ([]messaging_api.MessageInterface, error) {
	data := event.Postback.Data

	// Validate postback data
	if len(data) == 0 {
		h.logger.Warn("Empty postback data")
		return nil, nil
	}
	if len(data) > 300 { // LINE postback data limit is 300 bytes
		h.logger.Warnf("Postback data too long: %d bytes", len(data))
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithSender("❌ 操作資料異常\n\n請重新使用功能。", "系統魔法師", h.stickerManager.GetRandomSticker()),
		}, nil
	}

	// Sanitize postback data
	data = strings.TrimSpace(data)

	h.logger.WithField("data", data).Debug("Received postback")

	// Check for help keywords FIRST (before dispatching to bot modules)
	helpKeywords := []string{"使用說明", "help", "Help", "HELP"}
	for _, keyword := range helpKeywords {
		if strings.EqualFold(data, keyword) {
			h.logger.Info("User requested help/instruction via postback")
			return h.getDetailedInstructionMessages(), nil
		}
	}

	// Create context with timeout (derived from request context)
	processCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	// Check module prefix or dispatch to all handlers
	if strings.HasPrefix(data, "id:") {
		return h.idHandler.HandlePostback(processCtx, strings.TrimPrefix(data, "id:")), nil
	}

	if strings.HasPrefix(data, "contact:") {
		return h.contactHandler.HandlePostback(processCtx, strings.TrimPrefix(data, "contact:")), nil
	}

	if strings.HasPrefix(data, "course:") {
		return h.courseHandler.HandlePostback(processCtx, strings.TrimPrefix(data, "course:")), nil
	}

	// Try dispatching to all handlers (for backward compatibility with handlers without prefix)
	if messages := h.idHandler.HandlePostback(processCtx, data); len(messages) > 0 {
		return messages, nil
	}

	if messages := h.contactHandler.HandlePostback(processCtx, data); len(messages) > 0 {
		return messages, nil
	}

	if messages := h.courseHandler.HandlePostback(processCtx, data); len(messages) > 0 {
		return messages, nil
	}

	// No handler matched
	return []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithSender("操作已過期或無效", "系統魔法師", h.stickerManager.GetRandomSticker()),
	}, nil
}

// handleStickerMessage processes sticker messages (reply with random sticker image)
func (h *Handler) handleStickerMessage(event webhook.MessageEvent) []messaging_api.MessageInterface {
	h.logger.Info("Received sticker message, replying with random sticker image")

	// Get random sticker URL
	stickerURL := h.stickerManager.GetRandomSticker()

	// Reply with image message using the sticker URL
	return []messaging_api.MessageInterface{
		&messaging_api.ImageMessage{
			OriginalContentUrl: stickerURL,
			PreviewImageUrl:    stickerURL,
		},
	}
}

// handleFollowEvent processes follow events (when user adds the bot)
func (h *Handler) handleFollowEvent(event webhook.FollowEvent) ([]messaging_api.MessageInterface, error) {
	h.logger.Info("New user followed the bot")

	// Send welcome message (matching Python version style)
	senderName := "初階魔法師"
	messages := []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithSender("泥好~~我是北大查詢小工具🔍", senderName, h.stickerManager.GetRandomSticker()),
		lineutil.NewTextMessageWithSender("使用說明請點選下方選單\n或輸入「使用說明」查看", senderName, h.stickerManager.GetRandomSticker()),
		lineutil.NewTextMessageWithSender("有疑問可以先去看常見問題\n若無法解決或有發現 Bug\n歡迎到 GitHub 提出", senderName, h.stickerManager.GetRandomSticker()),
		lineutil.NewTextMessageWithSender("部分內容是由相關資料推斷\n不一定為正確資訊", senderName, h.stickerManager.GetRandomSticker()),
		lineutil.NewTextMessageWithSender("資料來源：國立臺北大學\n數位學苑2.0(已無新資料)\n校園聯絡簿\n課程查詢系統", senderName, h.stickerManager.GetRandomSticker()),
	}

	return messages, nil
}

// showLoadingAnimation shows a loading circle animation
func (h *Handler) showLoadingAnimation(event webhook.EventInterface) error {
	chatID := h.getChatID(event)
	if chatID == "" {
		return nil
	}

	// Use ShowLoadingAnimation API
	req := &messaging_api.ShowLoadingAnimationRequest{
		ChatId: chatID,
	}

	if _, err := h.client.ShowLoadingAnimation(req); err != nil {
		return fmt.Errorf("failed to show loading animation: %w", err)
	}

	return nil
}

// getReplyToken extracts reply token from event
func (h *Handler) getReplyToken(event webhook.EventInterface) string {
	switch e := event.(type) {
	case webhook.MessageEvent:
		return e.ReplyToken
	case webhook.PostbackEvent:
		return e.ReplyToken
	case webhook.FollowEvent:
		return e.ReplyToken
	default:
		return ""
	}
}

// getChatID extracts chat ID from event
func (h *Handler) getChatID(event webhook.EventInterface) string {
	switch e := event.(type) {
	case webhook.MessageEvent:
		if userSource, ok := e.Source.(webhook.UserSource); ok {
			return userSource.UserId
		}
	case webhook.PostbackEvent:
		if userSource, ok := e.Source.(webhook.UserSource); ok {
			return userSource.UserId
		}
	case webhook.FollowEvent:
		if userSource, ok := e.Source.(webhook.UserSource); ok {
			return userSource.UserId
		}
	}
	return ""
}

// getHelpMessage returns a simplified help message (fallback when no handler matches)
func (h *Handler) getHelpMessage() []messaging_api.MessageInterface {
	helpText := "🔍 NTPU 查詢小工具\n\n" +
		"📚 課程查詢：輸入課程編號、課程名稱或教師姓名\n" +
		"📞 聯絡資訊：輸入單位或人名關鍵字\n" +
		"🎓 學號查詢：輸入學號、姓名或學年度\n" +
		"🚨 緊急電話：輸入 '緊急' 查看緊急聯絡電話\n\n" +
		"💡 輸入「使用說明」查看詳細說明和範例"

	msg := lineutil.NewTextMessageWithSender(helpText, "幫助魔法師", h.stickerManager.GetRandomSticker())
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
		{Action: lineutil.NewMessageAction("📚 查詢課程", "課程")},
		{Action: lineutil.NewMessageAction("📞 查詢聯絡", "聯絡")},
		{Action: lineutil.NewMessageAction("🚨 緊急電話", "緊急")},
	})
	return []messaging_api.MessageInterface{msg}
}

// getDetailedInstructionMessages returns detailed instruction messages (matches Python version)
func (h *Handler) getDetailedInstructionMessages() []messaging_api.MessageInterface {
	senderName := "進階魔法師"
	stickerURL := h.stickerManager.GetRandomSticker()

	// Message 1: Main instruction text
	instructionText := "使用說明：\n\n" +
		"輸入「學生 {學號}」查詢學生\n" +
		"輸入「學生 {姓名}」查詢學生\n" +
		"輸入「科系 {系名}」查詢系代碼\n" +
		"輸入「系代碼 {系代碼}」查詢系名\n" +
		"輸入「學年 {入學年份}」後選科系查學生名單\n\n" +
		"輸入「課程 {課程名}」尋找課程\n" +
		"輸入「教師 {教師名}」尋找教師開的課\n\n" +
		"輸入「聯繫 {單位/姓名}」尋找聯繫方式\n\n" +
		"PS 符號{}中的部分要換成實際值\n" +
		"PPS 學生相關功能已無113學年後的資料"

	// Message 2: Examples
	currentYear := time.Now().Year()
	lastYear := currentYear - 1
	rocYear := lastYear - 1911

	exampleText := "範例：\n\n" +
		"學號：`學生 412345678`\n" +
		"姓名：`學生 小明` or `學生 林小明`\n" +
		"系名：`科系 資工系` or `科系 資訊工程學系`\n" +
		"系代碼：`系代碼 85`\n" +
		fmt.Sprintf("入學年：`學年 %d` or `學年 %d`\n\n", rocYear, lastYear) +
		"課程：`課程 程式設計`\n" +
		"教師：`教師 李小美`\n\n" +
		"聯繫：`聯繫 資工系`\n\n" +
		"PS 符號``中的部分是實際要輸入的"

	// Message 3: Disclaimer
	disclaimerText := "部分內容是由相關資料推斷\n不一定為正確資訊"

	// Message 4: Data source
	dataSourceText := "資料來源：國立臺北大學\n數位學苑2.0(已無新資料)\n校園聯絡簿\n課程查詢系統"

	return []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithSender(instructionText, senderName, stickerURL),
		lineutil.NewTextMessageWithSender(exampleText, senderName, h.stickerManager.GetRandomSticker()),
		lineutil.NewTextMessageWithSender(disclaimerText, senderName, h.stickerManager.GetRandomSticker()),
		lineutil.NewTextMessageWithSender(dataSourceText, senderName, h.stickerManager.GetRandomSticker()),
	}
}
