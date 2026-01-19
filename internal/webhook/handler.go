// Package webhook provides LINE webhook handling and message dispatching
// to appropriate bot modules based on message content and postback data.
package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
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
	processor      *bot.Processor
	rateLimiter    *ratelimit.Limiter // Global rate limiter for API calls
	stickerManager *sticker.Manager   // Sticker manager for avatar URLs
	wg             sync.WaitGroup     // WaitGroup for async event processing

	// LINE API constraints (from config.BotConfig)
	maxMessagesPerReply int
	maxEventsPerWebhook int
	minReplyTokenLength int
}

// HandlerConfig holds configuration for creating a new Handler
type HandlerConfig struct {
	ChannelSecret  string
	ChannelToken   string
	BotConfig      *config.BotConfig
	Metrics        *metrics.Metrics
	Logger         *logger.Logger
	Processor      *bot.Processor
	StickerManager *sticker.Manager
}

// NewHandler creates a new webhook handler.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	client, err := messaging_api.NewMessagingApiAPI(cfg.ChannelToken)
	if err != nil {
		return nil, fmt.Errorf("create messaging API client: %w", err)
	}

	h := &Handler{
		channelSecret:       cfg.ChannelSecret,
		client:              client,
		metrics:             cfg.Metrics,
		logger:              cfg.Logger,
		processor:           cfg.Processor,
		stickerManager:      cfg.StickerManager,
		maxMessagesPerReply: cfg.BotConfig.MaxMessagesPerReply,
		maxEventsPerWebhook: cfg.BotConfig.MaxEventsPerWebhook,
		minReplyTokenLength: cfg.BotConfig.MinReplyTokenLength,
	}

	h.rateLimiter = ratelimit.New(cfg.BotConfig.GlobalRateRPS, cfg.BotConfig.GlobalRateRPS)

	return h, nil
}

// Handle is the Gin handler for the webhook endpoint
func (h *Handler) Handle(c *gin.Context) {
	// 1. Parse request
	cb, err := webhook.ParseRequest(h.channelSecret, c.Request)
	if err != nil {
		if errors.Is(err, webhook.ErrInvalidSignature) {
			h.logger.Warn("Invalid webhook signature")
			c.Status(http.StatusBadRequest)
		} else {
			h.logger.WithError(err).Error("Failed to parse webhook request")
			c.Status(http.StatusInternalServerError)
		}
		return
	}

	// 2. Return 200 OK immediately (LINE requirement)
	c.Status(http.StatusOK)

	// 3. Process events asynchronously
	start := time.Now()
	h.metrics.RecordWebhook("batch", "received", 0)

	// Validate event count (max events per webhook per LINE API spec)
	if len(cb.Events) > h.maxEventsPerWebhook {
		h.logger.WithField("event_count", len(cb.Events)).
			WithField("limit", h.maxEventsPerWebhook).
			Warn("Too many events in webhook batch; truncating")
		cb.Events = cb.Events[:h.maxEventsPerWebhook] // Limit to prevent DoS
	}

	// Copy events to avoid race condition after HTTP response completes
	events := make([]webhook.EventInterface, len(cb.Events))
	copy(events, cb.Events)

	// Process events asynchronously using WaitGroup.Go (Go 1.25+)
	h.wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.WithField("panic", r).Error("Panic in async event processing")
			}
		}()

		processingCtx := context.Background()
		for _, event := range events {
			h.processEvent(processingCtx, event, start)
		}
	})
}

// processEvent handles a single webhook event asynchronously
func (h *Handler) processEvent(ctx context.Context, event webhook.EventInterface, webhookStart time.Time) {
	eventStart := time.Now()
	var messages []messaging_api.MessageInterface
	var eventType string
	var err error

	eventID, eventTimestamp, isRedelivery := extractEventMeta(event)
	if eventID != "" {
		ctx = ctxutil.WithRequestID(ctx, eventID)
	}

	log := h.logger
	if eventID != "" {
		log = log.WithRequestID(eventID)
	}
	if isRedelivery != nil {
		log = log.WithField("is_redelivery", *isRedelivery)
	}
	if eventTimestamp > 0 {
		log = log.WithField("event_timestamp_ms", eventTimestamp)
	}

	// Show loading animation only when response is expected
	// Skip for group chats without @mention or stickers in groups (no response)
	if h.shouldShowLoading(event) {
		if loadErr := h.showLoadingAnimation(event); loadErr != nil {
			log.WithError(loadErr).Warn("Failed to show loading animation")
		}
	}

	switch e := event.(type) {
	case webhook.MessageEvent:
		eventType = "message"
		messages, err = h.processor.ProcessMessage(ctx, e)
	case webhook.PostbackEvent:
		eventType = "postback"
		messages, err = h.processor.ProcessPostback(ctx, e)
	case webhook.FollowEvent:
		eventType = "follow"
		messages, err = h.processor.ProcessFollow(e)
	case webhook.JoinEvent:
		eventType = "join"
		messages, err = h.processor.ProcessJoin(e)
	default:
		// Unsupported event type, skip
		log.WithField("event_type", fmt.Sprintf("%T", e)).Debug("Unsupported event type")
		return
	}

	eventDurationMs := time.Since(eventStart).Milliseconds()
	durationSeconds := float64(eventDurationMs) / 1000.0
	status := "success"
	if err != nil {
		status = "error"
		log.WithError(err).WithField("event_type", eventType).Error("Failed to handle event")
	}
	h.metrics.RecordWebhook(eventType, status, durationSeconds)

	if len(messages) > 0 && err == nil {
		// LINE API restriction: max messages per reply
		if len(messages) > h.maxMessagesPerReply {
			log.WithField("message_count", len(messages)).
				WithField("limit", h.maxMessagesPerReply).
				Warn("Message count exceeds limit; truncating")
			messages = messages[:h.maxMessagesPerReply-1]
			sender := lineutil.GetSender("NTPU 小工具", h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender(
				"ℹ️ 由於訊息數量限制，部分內容未完整顯示\n\n💡 請使用更具體的關鍵字縮小查詢範圍",
				sender,
			)
			msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
			messages = append(messages, msg)
		}

		replyToken := h.getReplyToken(event)
		if replyToken == "" {
			log.Debug("Empty reply token, skipping reply")
			return
		}

		// Validate reply token format
		if len(replyToken) < h.minReplyTokenLength {
			log.WithField("token_length", len(replyToken)).Debug("Invalid reply token format")
			return
		}

		// Check global rate limit
		if !h.rateLimiter.Allow() {
			log.Warn("Global rate limit exceeded; waiting")
			h.metrics.RecordRateLimiterDrop("global")
			h.rateLimiter.WaitSimple()
		}

		if _, err := h.client.ReplyMessage(
			&messaging_api.ReplyMessageRequest{
				ReplyToken: replyToken,
				Messages:   messages,
			},
		); err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "Invalid reply token") {
				log.WithError(err).Debug("Reply token already used or invalid")
			} else if strings.Contains(errMsg, "rate limit") {
				log.WithError(err).Error("Rate limit exceeded")
			} else {
				log.WithError(err).WithField("reply_token", replyToken[:8]+"...").Error("Failed to send reply")
			}
			h.metrics.RecordWebhook(eventType, "reply_error", time.Since(eventStart).Seconds())
		}
	}

	// Log overall processing duration
	batchDurationMs := time.Since(webhookStart).Milliseconds()
	log.WithField("event_type", eventType).
		WithField("event_duration_ms", eventDurationMs).
		WithField("batch_duration_ms", batchDurationMs).
		Info("Event processed")
}

func extractEventMeta(event webhook.EventInterface) (string, int64, *bool) {
	switch e := event.(type) {
	case webhook.MessageEvent:
		return e.WebhookEventId, e.Timestamp, boolPtr(e.DeliveryContext)
	case webhook.PostbackEvent:
		return e.WebhookEventId, e.Timestamp, boolPtr(e.DeliveryContext)
	case webhook.FollowEvent:
		return e.WebhookEventId, e.Timestamp, boolPtr(e.DeliveryContext)
	case webhook.JoinEvent:
		return e.WebhookEventId, e.Timestamp, boolPtr(e.DeliveryContext)
	default:
		return "", 0, nil
	}
}

func boolPtr(ctx *webhook.DeliveryContext) *bool {
	if ctx == nil {
		return nil
	}
	val := ctx.IsRedelivery
	return &val
}

// shouldShowLoading determines if loading animation should be shown for an event.
// Returns false for events that won't result in a response:
// - Group/Room text messages without @Bot mention
// - Sticker messages in group/room chats
func (h *Handler) shouldShowLoading(event webhook.EventInterface) bool {
	switch e := event.(type) {
	case webhook.MessageEvent:
		return h.shouldShowLoadingForMessage(e)
	case webhook.PostbackEvent:
		// Postback always gets a response
		return true
	case webhook.FollowEvent:
		// Follow gets a response (welcome message)
		return true
	case webhook.JoinEvent:
		// Join gets a response (welcome message)
		return true
	default:
		// Unknown event types - don't show loading
		return false
	}
}

// shouldShowLoadingForMessage determines if loading should be shown for a message event.
// Personal chats always get responses. Group/Room chats only respond to:
// - Text messages with @Bot mention
// - Stickers are ignored in groups
func (h *Handler) shouldShowLoadingForMessage(e webhook.MessageEvent) bool {
	// Personal chats always get responses
	if _, ok := e.Source.(webhook.UserSource); ok {
		return true
	}

	// Group/Room: Check message type
	msgType := e.Message.GetType()

	// Stickers in groups are ignored (no response)
	if msgType == "sticker" {
		return false
	}

	// Group/Room text messages: need @mention to get response
	if msgType == "text" {
		textMsg, ok := e.Message.(webhook.TextMessageContent)
		if !ok {
			return false
		}
		return bot.IsBotMentioned(textMsg)
	}

	// Unknown message types - don't show loading
	return false
}

// showLoadingAnimation shows a loading circle animation.
// Uses LINE API maximum of 60 seconds to match webhook processing timeout.
func (h *Handler) showLoadingAnimation(event webhook.EventInterface) error {
	chatID := h.getChatID(event)
	if chatID == "" {
		return nil
	}

	// LINE API: loadingSeconds must be 5-60 seconds, multiple of 5.
	// Set to max (60s) to align with bot operation timeout (config.WebhookProcessing)
	// applied via ctxutil.PreserveTracing() for individual bot operations.
	var loadingSeconds int32 = 60

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
	case webhook.JoinEvent:
		return e.ReplyToken
	default:
		return ""
	}
}

// getChatID extracts chat ID from event
func (h *Handler) getChatID(event webhook.EventInterface) string {
	var source webhook.SourceInterface

	switch e := event.(type) {
	case webhook.MessageEvent:
		source = e.Source
	case webhook.PostbackEvent:
		source = e.Source
	case webhook.FollowEvent:
		source = e.Source
	case webhook.JoinEvent:
		source = e.Source
	default:
		return ""
	}

	return bot.GetChatID(source)
}

// Shutdown waits for all async event processing to complete.
// It returns an error if the context is canceled before completion.
func (h *Handler) Shutdown(ctx context.Context) error {
	c := make(chan struct{})
	go func() {
		defer close(c)
		h.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
