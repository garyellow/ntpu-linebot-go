// Package webhook provides LINE webhook handling and message dispatching
// to appropriate bot modules based on message content and postback data.
package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/course"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/id"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
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

// LINE API limits and constraints
const (
	MaxMessagesPerReply = 5
	MaxEventsPerWebhook = 100
	MinReplyTokenLength = 10
	MaxMessageLength    = 20000
	MaxPostbackDataSize = 300
	// WebhookTimeout is now defined in internal/config/timeouts.go
	// as config.WebhookProcessing with detailed documentation on
	// why 25 seconds was chosen (LINE API constraints, user patience, etc.)
)

// helpKeywords are the keywords that trigger the help message
var helpKeywords = []string{"使用說明", "help"}

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
	webhookTimeout time.Duration    // Timeout for bot processing

	// Rate limit configuration
	userRateLimitTokens     float64
	userRateLimitRefillRate float64

	// NLU intent parser (optional - requires Gemini API key)
	intentParser genai.IntentParserInterface
}

// NewHandler creates a new webhook handler
func NewHandler(channelSecret, channelToken string, db *storage.DB, scraperClient *scraper.Client, m *metrics.Metrics, log *logger.Logger, stickerManager *sticker.Manager, webhookTimeout time.Duration, userRateLimitTokens, userRateLimitRefillRate float64) (*Handler, error) {
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

	// Per-user rate limiter with metrics support
	userRateLimiter := NewUserRateLimiter(5*time.Minute, m)

	return &Handler{
		channelSecret:           channelSecret,
		client:                  client,
		metrics:                 m,
		logger:                  log,
		idHandler:               idHandler,
		contactHandler:          contactHandler,
		courseHandler:           courseHandler,
		rateLimiter:             globalRateLimiter,
		userLimiter:             userRateLimiter,
		stickerManager:          stickerManager,
		webhookTimeout:          webhookTimeout,
		userRateLimitTokens:     userRateLimitTokens,
		userRateLimitRefillRate: userRateLimitRefillRate,
	}, nil
}

// Stop gracefully stops the handler's background goroutines.
// This should be called during server shutdown to prevent goroutine leaks.
func (h *Handler) Stop() {
	if h.userLimiter != nil {
		h.userLimiter.Stop()
	}
}

// GetCourseHandler returns the course handler for external configuration
// Used to set VectorDB for semantic search from main.go
func (h *Handler) GetCourseHandler() *course.Handler {
	return h.courseHandler
}

// SetIntentParser sets the NLU intent parser for the handler.
// This is optional - if not set, the handler falls back to keyword matching only.
func (h *Handler) SetIntentParser(parser genai.IntentParserInterface) {
	h.intentParser = parser
}

// Handle processes incoming webhook requests
func (h *Handler) Handle(c *gin.Context) {
	start := time.Now()

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
		if errors.Is(err, webhook.ErrInvalidSignature) {
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

	// Process each event (max events per webhook per LINE API spec)
	if len(cb.Events) > MaxEventsPerWebhook {
		h.logger.Warnf("Too many events in single webhook: %d", len(cb.Events))
		cb.Events = cb.Events[:MaxEventsPerWebhook] // Limit to prevent DoS
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

			// LINE API restriction: max messages per reply
			if len(messages) > MaxMessagesPerReply {
				h.logger.Warnf("Message count %d exceeds limit, truncating to %d", len(messages), MaxMessagesPerReply)
				// Add a warning message at the end (keep room for warning)
				messages = messages[:MaxMessagesPerReply-1]
				sender := lineutil.GetSender("系統小幫手", h.stickerManager)
				messages = append(messages, lineutil.NewTextMessageWithConsistentSender(
					"ℹ️ 由於訊息數量限制，部分內容未完全顯示。\n請使用更具體的關鍵字縮小搜尋範圍。",
					sender,
				))
			}

			// Reply to the event
			replyToken := h.getReplyToken(event)
			if replyToken == "" {
				h.logger.Warn("Empty reply token, skipping reply")
				continue
			}

			// Validate reply token format (should not be empty or too short)
			if len(replyToken) < MinReplyTokenLength {
				h.logger.WithField("token_length", len(replyToken)).Warn("Invalid reply token format")
				continue
			}

			// Check global rate limit (user rate limit is checked in handleMessageEvent)
			if !h.rateLimiter.Allow() {
				h.logger.Warn("Global rate limit exceeded, waiting...")
				h.metrics.RecordHTTPError("rate_limit_global", "webhook")
				h.metrics.RecordRateLimiterDrop("global")
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

// isPersonalChat checks if the event source is a personal (1-on-1) chat
func (h *Handler) isPersonalChat(source webhook.SourceInterface) bool {
	_, ok := source.(webhook.UserSource)
	return ok
}

// checkUserRateLimit checks if the user has exceeded their rate limit.
// Returns (allowed bool, rateLimitMessage []MessageInterface).
// For personal chats, returns a friendly message when rate limited.
// For group/room chats, returns nil to silently ignore (avoid spamming groups).
func (h *Handler) checkUserRateLimit(source webhook.SourceInterface, chatID string) (bool, []messaging_api.MessageInterface) {
	if chatID == "" {
		return true, nil // No chat ID, allow by default
	}

	if h.userLimiter.Allow(chatID, h.userRateLimitTokens, h.userRateLimitRefillRate) {
		return true, nil // Rate limit not exceeded
	}

	// Rate limit exceeded - log it
	logChatID := chatID
	if len(chatID) > 8 {
		logChatID = chatID[:8] + "..."
	}
	h.logger.WithField("chat_id", logChatID).Warn("User rate limit exceeded")
	h.metrics.RecordHTTPError("rate_limit_user", "webhook")

	// For personal chats, return a friendly message
	if h.isPersonalChat(source) {
		sender := lineutil.GetSender("系統小幫手", h.stickerManager)
		return false, []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender(
				"⏳ 訊息過於頻繁，請稍後再試",
				sender,
			),
		}
	}

	// For group/room chats, return nil to silently ignore
	return false, nil
}

// handleMessageEvent processes text message events
func (h *Handler) handleMessageEvent(ctx context.Context, event webhook.MessageEvent) ([]messaging_api.MessageInterface, error) {
	// Check rate limit early to avoid unnecessary processing
	chatID := h.getChatIDFromSource(event.Source)
	if allowed, rateLimitMsg := h.checkUserRateLimit(event.Source, chatID); !allowed {
		return rateLimitMsg, nil
	}

	// Handle sticker messages - only in personal chats
	if event.Message.GetType() == "sticker" {
		if h.isPersonalChat(event.Source) {
			return h.handleStickerMessage(event), nil
		}
		// Ignore sticker messages in group/room chats
		return nil, nil
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

	// Validate text length (LINE API allows up to MaxMessageLength characters)
	if len(text) == 0 {
		return nil, nil // Empty message, ignore
	}
	if len(text) > MaxMessageLength {
		h.logger.Warnf("Text message too long: %d characters", len(text))
		sender := lineutil.GetSender("系統小幫手", h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender(
				fmt.Sprintf("❌ 訊息內容過長\n\n訊息長度超過 %d 字元，請縮短後重試。", MaxMessageLength),
				sender,
			),
		}, nil
	}

	// Sanitize input: normalize whitespace, remove punctuation
	text = strings.TrimSpace(text)
	text = normalizeWhitespace(text)
	text = removePunctuation(text)
	text = normalizeWhitespace(text) // Final normalization after punctuation removal
	if len(text) == 0 {
		return nil, nil // Empty after sanitization
	}

	h.logger.WithField("text", text).Debug("Received text message")

	// Check for help keywords FIRST (before dispatching to bot modules)
	for _, keyword := range helpKeywords {
		if strings.EqualFold(text, keyword) {
			h.logger.Info("User requested help/instruction")
			return h.getDetailedInstructionMessages(), nil
		}
	}

	// Create context with timeout for bot processing (derived from request context)
	processCtx, cancel := context.WithTimeout(ctx, h.webhookTimeout)
	defer cancel()

	// Dispatch to appropriate bot module based on CanHandle
	// Order matters: Contact and Course have more specific keywords,
	// ID handler's "系" keyword is too broad and would catch "聯繫 資工系"
	if h.contactHandler.CanHandle(text) {
		return h.contactHandler.HandleMessage(processCtx, text), nil
	}

	if h.courseHandler.CanHandle(text) {
		return h.courseHandler.HandleMessage(processCtx, text), nil
	}

	if h.idHandler.CanHandle(text) {
		return h.idHandler.HandleMessage(processCtx, text), nil
	}

	// No handler matched - try NLU if available
	return h.handleUnmatchedMessage(processCtx, event.Source, textMsg, text)
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
		sender := lineutil.GetSender("系統小幫手", h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("❌ 操作資料異常\n\n請重新使用功能。", sender),
		}, nil
	}

	// Sanitize postback data
	data = strings.TrimSpace(data)

	h.logger.WithField("data", data).Debug("Received postback")

	// Check for help keywords FIRST (before dispatching to bot modules)
	for _, keyword := range helpKeywords {
		if strings.EqualFold(data, keyword) {
			h.logger.Info("User requested help/instruction via postback")
			return h.getDetailedInstructionMessages(), nil
		}
	}

	// Create context with timeout (derived from request context)
	processCtx, cancel := context.WithTimeout(ctx, h.webhookTimeout)
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

	// No handler matched
	sender := lineutil.GetSender("系統小幫手", h.stickerManager)
	return []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithConsistentSender("操作已過期或無效", sender),
	}, nil
}

// handleStickerMessage processes sticker messages (reply with random sticker image)
func (h *Handler) handleStickerMessage(_ webhook.MessageEvent) []messaging_api.MessageInterface {
	h.logger.Info("Received sticker message, replying with random sticker image")

	// Get random sticker URL and create consistent sender
	stickerURL := h.stickerManager.GetRandomSticker()
	sender := lineutil.GetSender("貼圖小幫手", h.stickerManager)

	// Reply with image message using the sticker URL
	// Note: ImageMessage supports Sender field for consistent visual identity
	imageMsg := &messaging_api.ImageMessage{
		OriginalContentUrl: stickerURL,
		PreviewImageUrl:    stickerURL,
		Sender:             sender,
	}

	return []messaging_api.MessageInterface{imageMsg}
}

// handleFollowEvent processes follow events (when user adds the bot)
//
//nolint:unparam // error is kept for interface consistency with other event handlers
func (h *Handler) handleFollowEvent(_ webhook.FollowEvent) ([]messaging_api.MessageInterface, error) {
	h.logger.Info("New user followed the bot")

	// Send welcome message
	sender := lineutil.GetSender("初階小幫手", h.stickerManager)
	messages := []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithConsistentSender("泥好~~我是北大查詢小工具🔍", sender),
		lineutil.NewTextMessageWithConsistentSender("使用說明請點選下方選單\n或輸入「使用說明」查看", sender),
		lineutil.NewTextMessageWithConsistentSender("有疑問可以先去看常見問題\n若無法解決或有發現 Bug\n歡迎到 GitHub 提出", sender),
		lineutil.NewTextMessageWithConsistentSender("部分內容是由相關資料推斷\n不一定為正確資訊", sender),
		lineutil.NewTextMessageWithConsistentSender("資料來源：國立臺北大學\n數位學苑2.0(已無新資料)\n校園聯絡簿\n課程查詢系統", sender),
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

// normalizeWhitespace replaces all whitespace characters with single space
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// removePunctuation removes punctuation characters
// Pattern: ASCII punctuation + CJK punctuation (full-width)
func removePunctuation(s string) string {
	var result strings.Builder
	for _, r := range s {
		// Keep alphanumeric, CJK characters, and spaces
		// Remove: ASCII punctuation, CJK punctuation (full-width)
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == ' ',
			r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
			r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
			result.WriteRune(r)
		// Explicitly exclude common CJK punctuation (full-width)
		case r >= 0x3000 && r <= 0x303F: // CJK Symbols and Punctuation
			if r == 0x3000 { // Ideographic space (keep as regular space)
				result.WriteRune(' ')
			}
			// Skip: 、。，！？「」『』【】（）：；
		default:
			// Skip all other punctuation and special characters
		}
	}
	return result.String()
}

// getChatID extracts chat ID from event (supports user, group, and room sources)
func (h *Handler) getChatID(event webhook.EventInterface) string {
	var source webhook.SourceInterface

	switch e := event.(type) {
	case webhook.MessageEvent:
		source = e.Source
	case webhook.PostbackEvent:
		source = e.Source
	case webhook.FollowEvent:
		source = e.Source
	default:
		return ""
	}

	switch s := source.(type) {
	case webhook.UserSource:
		return s.UserId
	case webhook.GroupSource:
		return s.GroupId
	case webhook.RoomSource:
		return s.RoomId
	}
	return ""
}

// getChatIDFromSource extracts chat ID directly from a source interface
func (h *Handler) getChatIDFromSource(source webhook.SourceInterface) string {
	switch s := source.(type) {
	case webhook.UserSource:
		return s.UserId
	case webhook.GroupSource:
		return s.GroupId
	case webhook.RoomSource:
		return s.RoomId
	}
	return ""
}

// getHelpMessage returns a simplified help message (fallback when no handler matches)
func (h *Handler) getHelpMessage() []messaging_api.MessageInterface {
	helpText := "🔍 NTPU 查詢小工具\n\n" +
		"📚 課程查詢\n" +
		"   • 課程/教師：「課程 微積分」、「課 王小明」\n" +
		"   • 課程編號：「3141U0001」\n\n" +
		"🎓 學號查詢\n" +
		"   • 直接輸入：「412345678」\n" +
		"   • 姓名查詢：「學生 王小明」\n" +
		"   • 按學年查：「學年 112」\n\n" +
		"📞 聯絡資訊\n" +
		"   • 單位查詢：「聯絡 資工系」\n" +
		"   • 緊急電話：「緊急」\n\n" +
		"💡 輸入「使用說明」查看完整說明"

	sender := lineutil.GetSender("幫助小幫手", h.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyHelpAction(),
		lineutil.QuickReplyCourseAction(),
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyContactAction(),
		lineutil.QuickReplyEmergencyAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// handleUnmatchedMessage handles messages that don't match any keyword pattern.
// It uses NLU intent parsing if available, otherwise returns help message.
// For group chats without @Bot mention, it silently ignores the message.
func (h *Handler) handleUnmatchedMessage(ctx context.Context, source webhook.SourceInterface, textMsg webhook.TextMessageContent, sanitizedText string) ([]messaging_api.MessageInterface, error) {
	// Check if we're in a group chat
	isGroup := !h.isPersonalChat(source)

	// For group chats, only respond if bot is mentioned
	if isGroup {
		if !isBotMentioned(textMsg) {
			// No @Bot mention in group - silently ignore
			return nil, nil
		}
		// Remove @Bot mentions from ORIGINAL text for NLU processing
		// Note: Must use textMsg.Text (not sanitizedText) because mention.Index/Length
		// refer to character positions in the original message, not the sanitized version
		if textMsg.Mention != nil {
			mentionlessText := removeBotMentions(textMsg.Text, textMsg.Mention)
			mentionlessText = normalizeWhitespaceForMention(mentionlessText)
			if mentionlessText == "" {
				// Only @Bot mention, no actual content - return help
				return h.getHelpMessage(), nil
			}
			// Apply same sanitization as original text processing
			sanitizedText = strings.TrimSpace(mentionlessText)
			sanitizedText = normalizeWhitespace(sanitizedText)
			sanitizedText = removePunctuation(sanitizedText)
			sanitizedText = normalizeWhitespace(sanitizedText)
			if sanitizedText == "" {
				return h.getHelpMessage(), nil
			}
		}
	}

	// Try NLU if available
	if h.intentParser != nil && h.intentParser.IsEnabled() {
		return h.handleWithNLU(ctx, sanitizedText)
	}

	// NLU not available - return help message
	return h.getHelpMessage(), nil
}

// handleWithNLU processes the message using NLU intent parsing.
func (h *Handler) handleWithNLU(ctx context.Context, text string) ([]messaging_api.MessageInterface, error) {
	start := time.Now()

	result, err := h.intentParser.Parse(ctx, text)
	duration := time.Since(start).Seconds()

	if err != nil {
		// NLU error - log warning and fallback to help message
		h.logger.WithError(err).Warn("NLU intent parsing failed")
		h.metrics.RecordNLURequest("error", "", duration)
		h.metrics.RecordNLUFallback()
		return h.getHelpMessage(), nil
	}

	if result == nil {
		// No result - fallback to help message
		h.metrics.RecordNLURequest("error", "", duration)
		h.metrics.RecordNLUFallback()
		return h.getHelpMessage(), nil
	}

	// Check if model returned clarification text instead of function call
	if result.ClarificationText != "" {
		h.logger.WithField("clarification", result.ClarificationText).Debug("NLU returned clarification")
		h.metrics.RecordNLURequest("clarification", "", duration)

		// Return clarification text as a message
		sender := lineutil.GetSender("小幫手", h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender(result.ClarificationText, sender),
		}, nil
	}

	// Successfully parsed intent
	h.logger.WithField("module", result.Module).
		WithField("intent", result.Intent).
		WithField("params", result.Params).
		Debug("NLU intent parsed")
	h.metrics.RecordNLURequest("success", result.FunctionName, duration)

	// Dispatch to appropriate handler based on intent
	return h.dispatchIntent(ctx, result)
}

// dispatchIntent dispatches the parsed intent to the appropriate handler.
func (h *Handler) dispatchIntent(ctx context.Context, result *genai.ParseResult) ([]messaging_api.MessageInterface, error) {
	switch result.Module {
	case "course":
		msgs, err := h.courseHandler.DispatchIntent(ctx, result.Intent, result.Params)
		if err != nil {
			h.logger.WithError(err).WithField("intent", result.Intent).Warn("Course dispatch failed")
			return h.getHelpMessage(), nil
		}
		return msgs, nil
	case "id":
		msgs, err := h.idHandler.DispatchIntent(ctx, result.Intent, result.Params)
		if err != nil {
			h.logger.WithError(err).WithField("intent", result.Intent).Warn("ID dispatch failed")
			return h.getHelpMessage(), nil
		}
		return msgs, nil
	case "contact":
		msgs, err := h.contactHandler.DispatchIntent(ctx, result.Intent, result.Params)
		if err != nil {
			h.logger.WithError(err).WithField("intent", result.Intent).Warn("Contact dispatch failed")
			return h.getHelpMessage(), nil
		}
		return msgs, nil
	case "help":
		return h.getDetailedInstructionMessages(), nil
	default:
		h.logger.WithField("module", result.Module).Warn("Unknown module from NLU")
		return h.getHelpMessage(), nil
	}
}

// getDetailedInstructionMessages returns detailed instruction messages
func (h *Handler) getDetailedInstructionMessages() []messaging_api.MessageInterface {
	senderName := "小幫手"

	// Message 1: Main instruction text
	instructionText := "使用說明：\n\n" +
		"輸入「學生 {學號}」查詢學生\n" +
		"輸入「學生 {姓名}」查詢學生\n" +
		"輸入「科系 {系名}」查詢系代碼\n" +
		"輸入「系代碼 {系代碼}」查詢系名\n" +
		"輸入「學年 {入學年份}」後選科系查學生名單\n\n" +
		"輸入「課程 {課程名/教師名}」搜尋課程\n" +
		"輸入「課程 {學年} {課程名}」查詢歷史課程\n\n" +
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
		"歷史課程：`課程 110 微積分`\n" +
		"教師：`課 李小美`、`課程 王`\n\n" +
		"聯繫：`聯繫 資工系`\n\n" +
		"PS 符號``中的部分是實際要輸入的"

	// Message 3: Disclaimer
	disclaimerText := "部分內容是由相關資料推斷\n不一定為正確資訊"

	// Message 4: Data source
	dataSourceText := "資料來源：國立臺北大學\n數位學苑2.0(已無新資料)\n校園聯絡簿\n課程查詢系統"

	// Use GetSender pattern for consistent avatar across all 4 messages
	sender := lineutil.GetSender(senderName, h.stickerManager)
	return []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithConsistentSender(instructionText, sender),
		lineutil.NewTextMessageWithConsistentSender(exampleText, sender),
		lineutil.NewTextMessageWithConsistentSender(disclaimerText, sender),
		lineutil.NewTextMessageWithConsistentSender(dataSourceText, sender),
	}
}
