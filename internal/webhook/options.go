// Package webhook provides functional options for Handler configuration.
package webhook

import (
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
)

// HandlerOption is a functional option for configuring Handler.
// This pattern improves API ergonomics by:
// - Making optional parameters explicit
// - Enabling backward-compatible API evolution
// - Reducing constructor parameter count
type HandlerOption func(*Handler)

// WithBotConfig sets the bot configuration.
func WithBotConfig(cfg *config.BotConfig) HandlerOption {
	return func(h *Handler) {
		h.webhookTimeout = cfg.WebhookTimeout
		h.userRateLimitTokens = cfg.UserRateLimitTokens
		h.userRateLimitRefillRate = cfg.UserRateLimitRefillRate
		h.llmRateLimitPerHour = cfg.LLMRateLimitPerHour
		h.maxMessagesPerReply = cfg.MaxMessagesPerReply
		h.maxEventsPerWebhook = cfg.MaxEventsPerWebhook
		h.minReplyTokenLength = cfg.MinReplyTokenLength
	}
}

// WithWebhookTimeout sets the webhook processing timeout.
func WithWebhookTimeout(timeout time.Duration) HandlerOption {
	return func(h *Handler) {
		h.webhookTimeout = timeout
	}
}

// WithUserRateLimit sets the per-user rate limit configuration.
func WithUserRateLimit(tokens, refillRate float64) HandlerOption {
	return func(h *Handler) {
		h.userRateLimitTokens = tokens
		h.userRateLimitRefillRate = refillRate
	}
}

// WithLLMRateLimit sets the LLM rate limit (requests per hour).
func WithLLMRateLimit(perHour float64) HandlerOption {
	return func(h *Handler) {
		h.llmRateLimitPerHour = perHour
	}
}

// WithStickerManager sets the sticker manager.
func WithStickerManager(sm *sticker.Manager) HandlerOption {
	return func(h *Handler) {
		h.stickerManager = sm
	}
}

// WithIntentParser sets the NLU intent parser.
func WithIntentParser(parser genai.IntentParser) HandlerOption {
	return func(h *Handler) {
		h.intentParser = parser
	}
}

// WithLLMRateLimiter sets the LLM rate limiter.
func WithLLMRateLimiter(limiter *ratelimit.LLMRateLimiter) HandlerOption {
	return func(h *Handler) {
		h.llmLimiter = limiter
	}
}
