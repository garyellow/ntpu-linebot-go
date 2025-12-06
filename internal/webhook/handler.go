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
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"
)

// contextKey is a private type for context keys to avoid collisions.
// Using a private type ensures our context keys don't conflict with other packages.
type contextKey string

const (
	// chatIDContextKey holds the chat ID (user/group/room ID) for the current request.
	// This enables request-scoped data to flow through the call stack without
	// changing function signatures, following Go's context best practices.
	chatIDContextKey contextKey = "chatID"
)

// WithChatID returns a new context with the chat ID embedded.
// This allows bot modules to access the chat ID for features like rate limiting
// without tight coupling through function parameters.
func WithChatID(ctx context.Context, chatID string) context.Context {
	return context.WithValue(ctx, chatIDContextKey, chatID)
}

// GetChatID extracts the chat ID from context.
// Returns empty string if chat ID is not present or has wrong type.
// This is safe to call from any layer that receives the context.
func GetChatID(ctx context.Context) string {
	if chatID, ok := ctx.Value(chatIDContextKey).(string); ok {
		return chatID
	}
	return ""
}

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
	rateLimiter    *RateLimiter              // Global rate limiter for API calls
	userLimiter    *UserRateLimiter          // Per-user rate limiter
	llmLimiter     *ratelimit.LLMRateLimiter // LLM-specific rate limiter (NLU + query expansion)
	stickerManager *sticker.Manager          // Sticker manager for avatar URLs
	webhookTimeout time.Duration             // Timeout for bot processing

	// Rate limit configuration
	userRateLimitTokens     float64
	userRateLimitRefillRate float64
	llmRateLimitPerHour     float64 // LLM requests per user per hour

	// NLU intent parser (optional - requires Gemini API key)
	intentParser genai.IntentParser
}

// NewHandler creates a new webhook handler
func NewHandler(channelSecret, channelToken string, db *storage.DB, scraperClient *scraper.Client, m *metrics.Metrics, log *logger.Logger, stickerManager *sticker.Manager, webhookTimeout time.Duration, userRateLimitTokens, userRateLimitRefillRate, llmRateLimitPerHour float64) (*Handler, error) {
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

	// LLM-specific rate limiter (per hour, with cleanup every 5 minutes)
	llmRateLimiter := ratelimit.NewLLMRateLimiter(llmRateLimitPerHour, 5*time.Minute, m)

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
		llmLimiter:              llmRateLimiter,
		stickerManager:          stickerManager,
		webhookTimeout:          webhookTimeout,
		userRateLimitTokens:     userRateLimitTokens,
		userRateLimitRefillRate: userRateLimitRefillRate,
		llmRateLimitPerHour:     llmRateLimitPerHour,
	}, nil
}

// Stop gracefully stops the handler's background goroutines.
// This should be called during server shutdown to prevent goroutine leaks.
func (h *Handler) Stop() {
	if h.userLimiter != nil {
		h.userLimiter.Stop()
	}
	if h.llmLimiter != nil {
		h.llmLimiter.Stop()
	}
}

// GetCourseHandler returns the course handler for external configuration
// Used to set BM25Index and QueryExpander for smart search from main.go
func (h *Handler) GetCourseHandler() *course.Handler {
	return h.courseHandler
}

// GetLLMRateLimiter returns the LLM rate limiter for sharing with bot modules.
// This allows course handler to use the same limiter for query expansion,
// ensuring unified LLM quota management across all AI features.
func (h *Handler) GetLLMRateLimiter() *ratelimit.LLMRateLimiter {
	return h.llmLimiter
}

// SetIntentParser sets the NLU intent parser for the handler.
// This is optional - if not set, the handler falls back to keyword matching only.
func (h *Handler) SetIntentParser(parser genai.IntentParser) {
	h.intentParser = parser
}

// Handle processes incoming webhook requests following LINE Best Practice:
// 1. Respond with HTTP 200 within 2 seconds (immediately after signature verification)
// 2. Process events asynchronously in background goroutines
// 3. Reply ASAP for good UX (reply token valid ~20 min, but should reply within 60s)
//
// Reference: https://developers.line.biz/en/docs/partner-docs/development-guidelines/
func (h *Handler) Handle(c *gin.Context) {
	start := time.Now()

	// Validate Content-Length to prevent abuse
	if c.Request.ContentLength > 1<<20 { // 1MB limit
		h.logger.Warn("Request body too large")
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request too large"})
		return
	}

	// Parse webhook request with signature verification (SYNCHRONOUS - must complete before HTTP response)
	cb, err := webhook.ParseRequest(h.channelSecret, c.Request)
	if err != nil {
		h.logger.WithError(err).Error("Failed to parse webhook request")
		if errors.Is(err, webhook.ErrInvalidSignature) {
			// Invalid signature - potential security threat
			h.metrics.RecordWebhook("invalid_signature", "error", time.Since(start).Seconds())
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse request"})
		}
		return
	}

	// Immediately return HTTP 200 OK (LINE Best Practice: respond within 2 seconds)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})

	// Capture context for async processing (detached from HTTP request lifecycle)
	// Use background context to ensure processing completes even if client disconnects
	processingCtx := context.Background()

	// Validate event count (max events per webhook per LINE API spec)
	if len(cb.Events) > MaxEventsPerWebhook {
		h.logger.Warnf("Too many events in single webhook: %d", len(cb.Events))
		cb.Events = cb.Events[:MaxEventsPerWebhook] // Limit to prevent DoS
	}

	// Copy events to avoid race condition after HTTP response completes
	// The cb variable may be garbage collected after handler returns
	events := make([]webhook.EventInterface, len(cb.Events))
	copy(events, cb.Events)

	// Process events asynchronously in goroutine
	// Each event is processed independently to avoid blocking
	go func() {
		// Recover from panics to prevent server crash
		// Goroutine is outside Gin middleware chain, so gin.Recovery() won't catch panics here
		defer func() {
			if r := recover(); r != nil {
				h.logger.WithField("panic", r).Error("Panic in async event processing")
			}
		}()
		for _, event := range events {
			h.processEvent(processingCtx, event, start)
		}
	}()
}

// processEvent handles a single webhook event asynchronously
func (h *Handler) processEvent(ctx context.Context, event webhook.EventInterface, webhookStart time.Time) {
	eventStart := time.Now()
	var messages []messaging_api.MessageInterface
	var eventType string
	var err error

	// Show loading animation BEFORE processing (best effort, non-blocking)
	// LINE Best Practice: Display loading indicator immediately to
	// inform users the bot is processing their request.
	// Note: Only works for 1-on-1 chats; group/room chats are not supported.
	if loadErr := h.showLoadingAnimation(event); loadErr != nil {
		h.logger.WithError(loadErr).Debug("Failed to show loading animation")
	}

	switch e := event.(type) {
	case webhook.MessageEvent:
		eventType = "message"
		messages, err = h.handleMessageEvent(ctx, e)
	case webhook.PostbackEvent:
		eventType = "postback"
		messages, err = h.handlePostbackEvent(ctx, e)
	case webhook.FollowEvent:
		eventType = "follow"
		messages, err = h.handleFollowEvent(e)
	default:
		// Unsupported event type, skip
		h.logger.WithField("event_type", fmt.Sprintf("%T", e)).Debug("Unsupported event type")
		return
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
		// LINE API restriction: max messages per reply
		if len(messages) > MaxMessagesPerReply {
			h.logger.Warnf("Message count %d exceeds limit, truncating to %d", len(messages), MaxMessagesPerReply)
			// Add a warning message at the end (keep room for warning)
			messages = messages[:MaxMessagesPerReply-1]
			sender := lineutil.GetSender("系統小幫手", h.stickerManager)
			messages = append(messages, lineutil.NewTextMessageWithConsistentSender(
				"ℹ️ 由於訊息數量限制,部分內容未完全顯示。\n請使用更具體的關鍵字縮小搜尋範圍。",
				sender,
			))
		}

		// Reply to the event
		replyToken := h.getReplyToken(event)
		if replyToken == "" {
			h.logger.Warn("Empty reply token, skipping reply")
			return
		}

		// Validate reply token format (should not be empty or too short)
		if len(replyToken) < MinReplyTokenLength {
			h.logger.WithField("token_length", len(replyToken)).Warn("Invalid reply token format")
			return
		}

		// Check global rate limit (user rate limit is checked in handleMessageEvent)
		if !h.rateLimiter.Allow() {
			h.logger.Warn("Global rate limit exceeded, waiting...")
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

	// Log overall processing duration
	totalDuration := time.Since(webhookStart).Seconds()
	h.logger.WithField("total_duration", totalDuration).WithField("event_type", eventType).Debug("Event processed")
}

// isPersonalChat checks if the event source is a personal (1-on-1) chat
func (h *Handler) isPersonalChat(source webhook.SourceInterface) bool {
	_, ok := source.(webhook.UserSource)
	return ok
}

// checkLLMRateLimit checks if the user has exceeded their LLM API rate limit.
// Returns (allowed bool, rateLimitMessage []MessageInterface).
// For personal chats, returns a detailed message showing quota and reset time.
// For group/room chats, returns nil to silently ignore (avoid spamming groups).
func (h *Handler) checkLLMRateLimit(source webhook.SourceInterface, chatID string) (bool, []messaging_api.MessageInterface) {
	if chatID == "" || h.llmLimiter == nil {
		return true, nil // No chat ID or limiter not initialized, allow by default
	}

	if h.llmLimiter.Allow(chatID, h.llmRateLimitPerHour) {
		return true, nil // Rate limit not exceeded
	}

	// Rate limit exceeded - log it
	logChatID := chatID
	if len(chatID) > 8 {
		logChatID = chatID[:8] + "..."
	}
	h.logger.WithField("chat_id", logChatID).Warn("LLM rate limit exceeded")

	// For personal chats, return a detailed message
	if h.isPersonalChat(source) {
		available := h.llmLimiter.GetAvailable(chatID, h.llmRateLimitPerHour)
		// Calculate approximate reset time in minutes
		// Formula: (maxPerHour - available) tokens * 3600 seconds/hour / maxPerHour / 60 seconds/minute
		resetMinutes := int((h.llmRateLimitPerHour - available) * 3600 / h.llmRateLimitPerHour / 60)
		if resetMinutes < 1 {
			resetMinutes = 1 // At least 1 minute
		}

		sender := lineutil.GetSender("系統小幫手", h.stickerManager)
		message := fmt.Sprintf(
			"⏳ AI 功能使用次數已達上限\n\n"+
				"📊 本小時配額：%.0f 次（已用完）\n"+
				"⏰ 約 %d 分鐘後重置\n\n"+
				"💡 您仍可使用：\n"+
				"• 關鍵字查詢：課程 微積分\n"+
				"• 課號查詢：1131U0001",
			h.llmRateLimitPerHour,
			resetMinutes,
		)

		// Add Quick Reply actions to guide user to alternative methods
		msg := lineutil.NewTextMessageWithConsistentSender(message, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})

		return false, []messaging_api.MessageInterface{
			msg,
		}
	}

	// For group/room chats, return nil to silently ignore
	return false, nil
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
	// Extract chat ID and inject into context for request-scoped access
	// This allows downstream handlers (like course module) to access chat ID
	// for rate limiting without tight coupling through function parameters
	chatID := h.getChatIDFromSource(event.Source)
	ctx = WithChatID(ctx, chatID)

	// Check rate limit early to avoid unnecessary processing
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

	// Create context with timeout for bot processing.
	// Use WithoutCancel to detach from HTTP request context, preventing
	// cancellation when client disconnects (e.g., LINE server closes connection).
	// This ensures database queries and scraping operations complete even if
	// the original request is canceled, which is important because:
	// 1. LINE may close the connection before we finish processing
	// 2. We still want to send the reply via the reply token
	// 3. Partial work (started DB queries) shouldn't be wasted
	processCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.webhookTimeout)
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

	// Create context with timeout for postback processing.
	// Use WithoutCancel to detach from HTTP request context (same reason as handleMessageEvent).
	processCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.webhookTimeout)
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

	// Check feature availability
	nluEnabled := h.intentParser != nil && h.intentParser.IsEnabled()

	// Send welcome message
	sender := lineutil.GetSender("初階小幫手", h.stickerManager)

	// Build welcome messages based on features
	var featureHint string
	if nluEnabled {
		featureHint = "💬 直接用自然語言問我！\n或輸入「使用說明」查看詳細功能"
	} else {
		featureHint = "使用說明請點選下方選單\n或輸入「使用說明」查看"
	}

	messages := []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithConsistentSender("泥好~~我是北大查詢小工具🔍", sender),
		lineutil.NewTextMessageWithConsistentSender(featureHint, sender),
		lineutil.NewTextMessageWithConsistentSender("有疑問可以先去看常見問題\n若無法解決或有發現 Bug\n歡迎到 GitHub 提出", sender),
		lineutil.NewTextMessageWithConsistentSender("部分內容是由相關資料推斷\n不一定為正確資訊", sender),
		lineutil.NewTextMessageWithConsistentSender("資料來源：國立臺北大學\n數位學苑2.0(已無新資料)\n校園聯絡簿\n課程查詢系統", sender),
	}

	return messages, nil
}

// showLoadingAnimation shows a loading circle animation to inform users
// the bot is processing their request. This is a LINE best practice.
//
// Important notes:
//   - Only works for 1-on-1 chats (personal chats); not supported for groups/rooms
//   - Animation auto-dismisses when a message is sent or after LoadingSeconds
//   - If called multiple times, the timer resets to the new LoadingSeconds value
//   - Non-blocking: if the user isn't viewing the chat, no notification is shown
func (h *Handler) showLoadingAnimation(event webhook.EventInterface) error {
	chatID := h.getChatID(event)
	if chatID == "" {
		return nil
	}

	// LINE allows 5-60 seconds; we use 20s as a reasonable default
	// that covers most operations without being excessive.
	// The animation will auto-dismiss when we send the reply.
	var loadingSeconds int32 = 20

	req := &messaging_api.ShowLoadingAnimationRequest{
		ChatId:         chatID,
		LoadingSeconds: loadingSeconds,
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

	return h.getChatIDFromSource(source)
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
// The message content varies based on whether NLU is enabled.
func (h *Handler) getHelpMessage() []messaging_api.MessageInterface {
	var helpText string

	if h.intentParser != nil && h.intentParser.IsEnabled() {
		// NLU enabled - emphasize natural language capability
		helpText = "🔍 NTPU 查詢小工具\n\n" +
			"💬 直接用自然語言問我，例如：\n" +
			"• 「微積分的課有哪些」\n" +
			"• 「王小明的學號」\n" +
			"• 「資工系電話」\n\n" +
			"📖 或使用關鍵字查詢：\n" +
			"• 課程：「課程 微積分」\n" +
			"• 學生：「學生 王小明」\n" +
			"• 聯繫：「聯繫 資工系」\n\n" +
			"💡 輸入「使用說明」查看完整說明"
	} else {
		// NLU disabled - emphasize keyword format
		helpText = "🔍 NTPU 查詢小工具\n\n" +
			"📚 課程查詢\n" +
			"• 課程/教師：「課程 微積分」\n" +
			"• 課程編號：直接輸入編號\n\n" +
			"🎓 學生查詢\n" +
			"• 學號/姓名：「學生 王小明」\n" +
			"• 按學年查：「學年 112」\n\n" +
			"📞 聯絡資訊\n" +
			"• 單位查詢：「聯繫 資工系」\n" +
			"• 緊急電話：「緊急」\n\n" +
			"💡 輸入「使用說明」查看完整說明"
	}

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
			// removeBotMentions already normalizes whitespace internally
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
		// Extract chatID from context or source
		chatID := h.getChatIDFromSource(source)
		return h.handleWithNLU(ctx, sanitizedText, source, chatID)
	}

	// NLU not available - return help message
	return h.getHelpMessage(), nil
}

// handleWithNLU processes the message using NLU intent parsing.
func (h *Handler) handleWithNLU(ctx context.Context, text string, source webhook.SourceInterface, chatID string) ([]messaging_api.MessageInterface, error) {
	// Check LLM rate limit before making API call
	if allowed, rateLimitMsg := h.checkLLMRateLimit(source, chatID); !allowed {
		return rateLimitMsg, nil
	}

	start := time.Now()

	result, err := h.intentParser.Parse(ctx, text)
	duration := time.Since(start).Seconds()

	if err != nil {
		// NLU error - log warning and fallback to help message
		h.logger.WithError(err).Warn("NLU intent parsing failed")
		h.metrics.RecordLLMRequest("nlu", "error", duration)
		h.metrics.RecordLLMFallback("nlu")
		return h.getHelpMessage(), nil
	}

	if result == nil {
		// No result - fallback to help message
		h.metrics.RecordLLMRequest("nlu", "error", duration)
		h.metrics.RecordLLMFallback("nlu")
		return h.getHelpMessage(), nil
	}

	// Check if model returned clarification text instead of function call
	if result.ClarificationText != "" {
		h.logger.WithField("clarification", result.ClarificationText).Debug("NLU returned clarification")
		h.metrics.RecordLLMRequest("nlu", "clarification", duration)

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
	h.metrics.RecordLLMRequest("nlu", "success", duration)

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
// Content varies based on whether NLU and smart search are enabled
func (h *Handler) getDetailedInstructionMessages() []messaging_api.MessageInterface {
	senderName := "小幫手"

	// Check feature availability
	nluEnabled := h.intentParser != nil && h.intentParser.IsEnabled()
	smartEnabled := h.courseHandler != nil && h.courseHandler.IsBM25SearchEnabled()

	// Message 1: Main instruction text
	// Common content for both NLU enabled/disabled
	baseInstructions := "• 學生：「學生 {學號/姓名}」\n" +
		"• 科系：「科系 {系名}」「系代碼 {代碼}」\n" +
		"• 學年：「學年 {入學年}」選科系查名單\n" +
		"• 課程：「課程 {課名/教師}」\n" +
		"• 歷史：「課程 {學年} {課名}」\n" +
		"• 聯繫：「聯繫 {單位/姓名}」\n" +
		"• 緊急：「緊急」查看緊急電話"
	if smartEnabled {
		baseInstructions += "\n• 找課：「找課 {描述}」智慧搜尋"
	}

	var instructionText string
	if nluEnabled {
		// NLU enabled - emphasize natural language first
		instructionText = "使用說明：\n\n" +
			"💬 直接用自然語言問我！\n" +
			"例如「微積分的課」、「王小明學號」\n\n" +
			"📖 或使用關鍵字查詢：\n" +
			baseInstructions +
			"\n\n⚠️ 學生功能已無113學年後的資料"
	} else {
		// NLU disabled - keyword format only
		instructionText = "使用說明：\n\n" +
			baseInstructions +
			"\n\nPS 符號{}請換成實際值\n" +
			"⚠️ 學生功能已無113學年後的資料"
	}

	// Message 2: Examples
	// Common keyword examples for both versions
	rocYear := time.Now().Year() - 1 - 1911 // Last year in ROC format
	baseExamples := "• `學生 412345678`、`學生 林小明`\n" +
		"• `科系 資工系`、`系代碼 85`\n" +
		fmt.Sprintf("• `學年 %d`\n", rocYear) +
		"• `課程 程式設計`、`課程 110 微積分`\n" +
		"• `聯繫 資工系`、`緊急`"
	if smartEnabled {
		baseExamples += "\n• `找課 想學程式設計`"
	}

	var exampleText string
	if nluEnabled {
		// NLU enabled - show natural language examples first
		exampleText = "範例：\n\n" +
			"💬 自然語言：\n" +
			"• 「微積分有哪些課」\n" +
			"• 「王小明老師的課表」\n" +
			"• 「資工系辦公室電話」\n" +
			"• 「圖書館怎麼聯絡」\n\n" +
			"📖 關鍵字：\n" +
			baseExamples
	} else {
		// NLU disabled - keyword examples only
		exampleText = "範例：\n\n" +
			baseExamples +
			"\n\nPS 符號``中的部分是實際輸入值"
	}

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
