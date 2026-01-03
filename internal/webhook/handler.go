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
			h.logger.Info("Invalid signature")
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
		h.logger.Infof("Too many events in single webhook: %d, truncating", len(cb.Events))
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

	// Best effort, non-blocking
	if loadErr := h.showLoadingAnimation(event); loadErr != nil {
		h.logger.WithError(loadErr).Warn("Failed to show loading animation")
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
	default:
		// Unsupported event type, skip
		h.logger.WithField("event_type", fmt.Sprintf("%T", e)).Debug("Unsupported event type")
		return
	}

	duration := time.Since(eventStart).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		h.logger.WithError(err).WithField("event_type", eventType).Error("Failed to handle event")
	}
	h.metrics.RecordWebhook(eventType, status, duration)

	if len(messages) > 0 && err == nil {
		// LINE API restriction: max messages per reply
		if len(messages) > h.maxMessagesPerReply {
			h.logger.Warnf("Message count %d exceeds limit, truncating to %d", len(messages), h.maxMessagesPerReply)
			messages = messages[:h.maxMessagesPerReply-1]
			sender := lineutil.GetSender("北大小幫手", h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender(
				"ℹ️ 由於訊息數量限制，部分內容未完整顯示\n\n💡 請使用更具體的關鍵字縮小查詢範圍",
				sender,
			)
			msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
			messages = append(messages, msg)
		}

		replyToken := h.getReplyToken(event)
		if replyToken == "" {
			h.logger.Debug("Empty reply token, skipping reply")
			return
		}

		// Validate reply token format
		if len(replyToken) < h.minReplyTokenLength {
			h.logger.WithField("token_length", len(replyToken)).Debug("Invalid reply token format")
			return
		}

		// Check global rate limit
		if !h.rateLimiter.Allow() {
			h.logger.Warn("Global rate limit exceeded, waiting...")
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
				h.logger.WithError(err).Debug("Reply token already used or invalid")
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

// showLoadingAnimation shows a loading circle animation
func (h *Handler) showLoadingAnimation(event webhook.EventInterface) error {
	chatID := h.getChatID(event)
	if chatID == "" {
		return nil
	}

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
