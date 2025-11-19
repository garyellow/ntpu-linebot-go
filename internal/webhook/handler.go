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
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/pkg/lineutil"
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
}

// NewHandler creates a new webhook handler
func NewHandler(channelSecret, channelToken string, db *storage.DB, scraper *scraper.Client, m *metrics.Metrics, log *logger.Logger) (*Handler, error) {
	// Create messaging API client
	client, err := messaging_api.NewMessagingApiAPI(channelToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create messaging API client: %w", err)
	}

	// Initialize bot module handlers
	idHandler := id.NewHandler(db, scraper, m, log)
	contactHandler := contact.NewHandler(db, scraper, m, log)
	courseHandler := course.NewHandler(db, scraper, m, log)

	return &Handler{
		channelSecret:  channelSecret,
		client:         client,
		metrics:        m,
		logger:         log,
		idHandler:      idHandler,
		contactHandler: contactHandler,
		courseHandler:  courseHandler,
	}, nil
}

// Handle processes incoming webhook requests
func (h *Handler) Handle(c *gin.Context) {
	start := time.Now()

	// Parse webhook request
	cb, err := webhook.ParseRequest(h.channelSecret, c.Request)
	if err != nil {
		h.logger.WithError(err).Error("Failed to parse webhook request")
		if err == webhook.ErrInvalidSignature {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse request"})
		}
		return
	}

	// Process each event
	for _, event := range cb.Events {
		eventStart := time.Now()
		var messages []messaging_api.MessageInterface
		var eventType string
		var err error

		switch e := event.(type) {
		case webhook.MessageEvent:
			eventType = "message"
			messages, err = h.handleMessageEvent(e)
		case webhook.PostbackEvent:
			eventType = "postback"
			messages, err = h.handlePostbackEvent(e)
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
			// Show loading animation
			if err := h.showLoadingAnimation(event); err != nil {
				h.logger.WithError(err).Warn("Failed to show loading animation")
			}

			// Limit to max 5 messages (LINE API restriction)
			if len(messages) > 5 {
				messages = messages[:5]
			}

			// Reply to the event
			replyToken := h.getReplyToken(event)
			if replyToken != "" {
				if _, err := h.client.ReplyMessage(
					&messaging_api.ReplyMessageRequest{
						ReplyToken: replyToken,
						Messages:   messages,
					},
				); err != nil {
					h.logger.WithError(err).Error("Failed to send reply")
				}
			}
		}
	}

	// Return success response
	duration := time.Since(start).Seconds()
	h.logger.WithField("duration", duration).Debug("Webhook processed")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleMessageEvent processes text message events
func (h *Handler) handleMessageEvent(event webhook.MessageEvent) ([]messaging_api.MessageInterface, error) {
	// Only handle text messages for now
	if event.Message.GetType() != "text" {
		return nil, nil
	}

	textMsg, ok := event.Message.(webhook.TextMessageContent)
	if !ok {
		return nil, fmt.Errorf("failed to cast message to text")
	}

	text := textMsg.Text
	h.logger.WithField("text", text).Debug("Received text message")

	ctx := context.Background()

	// Dispatch to appropriate bot module based on CanHandle
	if h.idHandler.CanHandle(text) {
		return h.idHandler.HandleMessage(ctx, text), nil
	}

	if h.contactHandler.CanHandle(text) {
		return h.contactHandler.HandleMessage(ctx, text), nil
	}

	if h.courseHandler.CanHandle(text) {
		return h.courseHandler.HandleMessage(ctx, text), nil
	}

	// No handler matched - return help message
	return h.getHelpMessage(), nil
}

// handlePostbackEvent processes postback events
func (h *Handler) handlePostbackEvent(event webhook.PostbackEvent) ([]messaging_api.MessageInterface, error) {
	data := event.Postback.Data
	h.logger.WithField("data", data).Debug("Received postback")

	ctx := context.Background()

	// Check module prefix or dispatch to all handlers
	if strings.HasPrefix(data, "id:") {
		return h.idHandler.HandlePostback(ctx, strings.TrimPrefix(data, "id:")), nil
	}

	if strings.HasPrefix(data, "contact:") {
		return h.contactHandler.HandlePostback(ctx, strings.TrimPrefix(data, "contact:")), nil
	}

	if strings.HasPrefix(data, "course:") {
		return h.courseHandler.HandlePostback(ctx, strings.TrimPrefix(data, "course:")), nil
	}

	// Try dispatching to all handlers (for backward compatibility with handlers without prefix)
	if messages := h.idHandler.HandlePostback(ctx, data); len(messages) > 0 {
		return messages, nil
	}

	if messages := h.contactHandler.HandlePostback(ctx, data); len(messages) > 0 {
		return messages, nil
	}

	if messages := h.courseHandler.HandlePostback(ctx, data); len(messages) > 0 {
		return messages, nil
	}

	// No handler matched
	return []messaging_api.MessageInterface{
		lineutil.NewTextMessage("操作已過期或無效"),
	}, nil
}

// handleFollowEvent processes follow events (when user adds the bot)
func (h *Handler) handleFollowEvent(event webhook.FollowEvent) ([]messaging_api.MessageInterface, error) {
	h.logger.Info("New user followed the bot")

	// Send welcome message
	messages := []messaging_api.MessageInterface{
		&messaging_api.TextMessage{
			Text: "歡迎使用 NTPU 查詢機器人！👋\n\n" +
				"本機器人提供以下功能：\n" +
				"📚 課程查詢\n" +
				"📞 聯絡資訊查詢\n" +
				"🎓 學號查詢\n\n" +
				"請直接輸入關鍵字開始查詢！",
		},
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

// getHelpMessage returns a help message with available commands
func (h *Handler) getHelpMessage() []messaging_api.MessageInterface {
	helpText := "🤖 NTPU 查詢機器人\n\n" +
		"📚 課程查詢：\n" +
		"  • 輸入課程編號（如：1131U1234）\n" +
		"  • 輸入課程名稱關鍵字\n" +
		"  • 輸入教師姓名\n\n" +
		"📞 聯絡資訊查詢：\n" +
		"  • 輸入單位或人名關鍵字\n" +
		"  • 輸入 '緊急' 查看緊急電話\n\n" +
		"🎓 學號查詢：\n" +
		"  • 輸入學號（8-9位數字）\n" +
		"  • 輸入學生姓名\n" +
		"  • 輸入學年度（如：112）\n" +
		"  • 輸入 '所有系代碼' 查看系所代碼\n\n" +
		"直接輸入關鍵字開始查詢！"

	return []messaging_api.MessageInterface{
		lineutil.NewTextMessage(helpText),
	}
}
