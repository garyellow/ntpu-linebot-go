// Package usage implements the usage query module for the LINE bot.
// It handles user queries about their current rate limit usage status.
package usage

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Module constants
const (
	ModuleName = "usage"
	senderName = "額度小幫手"
)

// Handler handles usage-related queries.
type Handler struct {
	userLimiter    *ratelimit.KeyedLimiter
	llmLimiter     *ratelimit.KeyedLimiter
	logger         *logger.Logger
	stickerManager *sticker.Manager
}

// Keyword definitions for usage queries
var (
	usageKeywords = []string{
		"用量", "配額", "額度", "扣打",
		"quota", "usage", "limit",
	}
	usageRegex = bot.BuildKeywordRegex(usageKeywords)

	// Quota explanation keyword - triggers explanation of what consumes quota
	quotaExplainKeyword = "額度說明"
)

// Name returns the module name
func (h *Handler) Name() string {
	return ModuleName
}

// NewHandler creates a new usage handler.
func NewHandler(
	userLimiter *ratelimit.KeyedLimiter,
	llmLimiter *ratelimit.KeyedLimiter,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
) *Handler {
	return &Handler{
		userLimiter:    userLimiter,
		llmLimiter:     llmLimiter,
		logger:         logger,
		stickerManager: stickerManager,
	}
}

// CanHandle returns true if the text matches usage or quota explanation keywords.
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)
	return usageRegex.MatchString(text) || strings.EqualFold(text, quotaExplainKeyword)
}

// HandleMessage processes usage queries and returns a Flex Message with quota status.
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)

	// Check for quota explanation request
	if strings.EqualFold(strings.TrimSpace(text), quotaExplainKeyword) {
		log.Debug("Handling quota explanation request")
		var sender *messaging_api.Sender
		if h.stickerManager != nil {
			sender = lineutil.GetSender(senderName, h.stickerManager)
		}
		return []messaging_api.MessageInterface{h.buildQuotaExplanationFlexMessage(sender)}
	}

	log.Debug("Handling usage query")

	// Get user ID from context for per-user quota lookup
	userID := getUserIDFromContext(ctx)

	// Get usage stats
	var userStats, llmStats ratelimit.UsageStats
	if h.userLimiter != nil {
		userStats = h.userLimiter.GetUsageStats(userID)
	}
	if h.llmLimiter != nil {
		llmStats = h.llmLimiter.GetUsageStats(userID)
	}

	// Build Flex Message
	var sender *messaging_api.Sender
	if h.stickerManager != nil {
		sender = lineutil.GetSender(senderName, h.stickerManager)
	}
	msg := h.buildUsageFlexMessage(userStats, llmStats, sender)

	return []messaging_api.MessageInterface{msg}
}

// Intent names for NLU dispatcher
const (
	IntentQuery = "query" // Query usage status
)

// DispatchIntent handles NLU-parsed intents.
func (h *Handler) DispatchIntent(ctx context.Context, intent string, params map[string]string) ([]messaging_api.MessageInterface, error) {
	switch intent {
	case IntentQuery:
		return h.HandleMessage(ctx, ""), nil
	default:
		return nil, fmt.Errorf("%w: %s", domerrors.ErrUnknownIntent, intent)
	}
}

// HandlePostback handles postback events for the usage module.
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	// Strip module prefix if present
	data = strings.TrimPrefix(data, "usage:")

	if data == "query" || data == "配額" {
		return h.HandleMessage(ctx, "")
	}

	return []messaging_api.MessageInterface{}
}

// getUserIDFromContext extracts chat ID from context for rate limiter lookup.
// Rate limiters use chatID as the key (consistent with webhook/processor).
// Returns empty string if not found (which will return full quota).
func getUserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	// Use ctxutil.GetChatID which uses the correct typed context key
	return ctxutil.GetChatID(ctx)
}

// buildUsageFlexMessage creates a Flex Message displaying usage statistics.
//
// Layout (Colored Header pattern):
//
//	┌──────────────────────────┐
//	│   📊 使用額度狀態        │  <- Colored header (sky blue)
//	├──────────────────────────┤
//	│ ⚡ 訊息額度              │  <- User quota section
//	│ 可用: X / Y 次           │
//	│ [colored bar 8px]        │  <- Progress bar (green/yellow/red)
//	│ 💡 恢復說明              │
//	├──────────────────────────┤
//	│ 🤖 AI 功能額度           │  <- LLM rate limit section
//	│ ...                      │
//	├──────────────────────────┤
//	│     [📖 使用說明]        │  <- Single footer button
//	└──────────────────────────┘
func (h *Handler) buildUsageFlexMessage(userStats, llmStats ratelimit.UsageStats, sender *messaging_api.Sender) *messaging_api.FlexMessage {
	// Header: Colored header with title (matching other modules)
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: "📊 使用額度狀態",
		Color: lineutil.ColorHeaderInfo, // Sky blue for info display
	})

	// Body: Use BodyContentBuilder for consistent structure
	body := lineutil.NewBodyContentBuilder()

	// User rate limit section (if available)
	if h.userLimiter != nil {
		h.addUserRateLimitSection(body, userStats)
	}

	// LLM rate limit section (if available)
	if h.llmLimiter != nil {
		h.addLLMRateLimitSection(body, llmStats)
	}

	// Footer: Quota explanation button + Help button
	quotaExplainBtn := lineutil.NewFlexButton(
		lineutil.NewMessageAction("❓ 額度說明", "額度說明"),
	).WithStyle("secondary").WithHeight("sm")

	helpBtn := lineutil.NewFlexButton(
		lineutil.NewMessageAction("📖 使用說明", "使用說明"),
	).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm")

	footer := lineutil.NewButtonFooter([]*lineutil.FlexButton{quotaExplainBtn, helpBtn})

	bubble := lineutil.NewFlexBubble(header, nil, body.Build(), footer)
	msg := lineutil.NewFlexMessage("使用額度狀態", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}

	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyUsageNav())

	return msg
}

// buildQuotaExplanationFlexMessage creates a Flex Message explaining what operations consume quota.
func (h *Handler) buildQuotaExplanationFlexMessage(sender *messaging_api.Sender) *messaging_api.FlexMessage {
	// Header
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: "❓ 額度說明",
		Color: lineutil.ColorHeaderTips,
	})

	// Body
	body := lineutil.NewBodyContentBuilder()

	// User quota section
	body.AddComponent(lineutil.NewFlexText("⚡ 訊息額度").
		WithWeight("bold").
		WithColor(lineutil.ColorText).
		WithSize("sm").FlexText)
	body.AddComponent(lineutil.NewFlexText("每則訊息都會扣除 1 次，包括文字、貼圖等。").
		WithSize("xs").
		WithColor(lineutil.ColorSubtext).
		WithWrap(true).
		WithMargin("sm").FlexText)

	// AI quota section
	body.AddComponent(lineutil.NewFlexText("🤖 AI 額度").
		WithWeight("bold").
		WithColor(lineutil.ColorText).
		WithSize("sm").
		WithMargin("lg").FlexText)
	body.AddComponent(lineutil.NewFlexText("以下操作會扣除 AI 額度：").
		WithSize("xs").
		WithColor(lineutil.ColorSubtext).
		WithWrap(true).
		WithMargin("sm").FlexText)
	body.AddComponent(lineutil.NewFlexText("• 自然語言對話（非關鍵字查詢）\n• 智慧搜尋（找課）").
		WithSize("xs").
		WithColor(lineutil.ColorSubtext).
		WithWrap(true).
		WithMargin("xs").FlexText)

	// Tips section
	body.AddComponent(lineutil.NewFlexText("💡 省 AI 額度技巧").
		WithWeight("bold").
		WithColor(lineutil.ColorText).
		WithSize("sm").
		WithMargin("lg").FlexText)
	body.AddComponent(lineutil.NewFlexText("使用關鍵字查詢不扣 AI 額度。").
		WithSize("xs").
		WithColor(lineutil.ColorSubtext).
		WithWrap(true).
		WithMargin("sm").FlexText)

	// Footer
	checkQuotaBtn := lineutil.NewFlexButton(
		lineutil.NewMessageAction("📊 查看額度", "額度"),
	).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm")

	footer := lineutil.NewButtonFooter([]*lineutil.FlexButton{checkQuotaBtn})

	bubble := lineutil.NewFlexBubble(header, nil, body.Build(), footer)
	msg := lineutil.NewFlexMessage("額度說明", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}

	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyUsageNav())

	return msg
}

// addUserRateLimitSection adds the user rate limit display to the body builder.
func (h *Handler) addUserRateLimitSection(body *lineutil.BodyContentBuilder, stats ratelimit.UsageStats) {
	available := int(math.Floor(stats.BurstAvailable))
	maxBurst := int(stats.BurstMax)
	var percentage float64
	if stats.BurstMax > 0 {
		percentage = stats.BurstAvailable / stats.BurstMax * 100
	}

	// Calculate refill info
	refillInfo := "持續恢復中"
	if stats.BurstRefillRate > 0 {
		secondsPerToken := 1.0 / stats.BurstRefillRate
		if secondsPerToken >= 1 {
			refillInfo = fmt.Sprintf("每 %.0f 秒恢復 1 次", secondsPerToken)
		} else {
			refillInfo = fmt.Sprintf("每秒恢復 %.1f 次", stats.BurstRefillRate)
		}
	}

	// Section title
	body.AddComponent(lineutil.NewFlexText("⚡ 訊息額度").
		WithWeight("bold").
		WithColor(lineutil.ColorText).
		WithSize("sm").FlexText)

	// Usage info and progress bar
	body.AddComponent(lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText(fmt.Sprintf("可用: %d / %d 次", available, maxBurst)).
			WithSize("sm").
			WithColor(lineutil.ColorText).FlexText,
		h.buildProgressBar(percentage).WithMargin("sm").FlexBox,
		lineutil.NewFlexText(fmt.Sprintf("💡 %s", refillInfo)).
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").FlexText,
	).FlexBox)
}

// addLLMRateLimitSection adds the LLM rate limit display to the body builder.
func (h *Handler) addLLMRateLimitSection(body *lineutil.BodyContentBuilder, stats ratelimit.UsageStats) {
	// Section title
	body.AddComponent(lineutil.NewFlexText("🤖 AI 功能額度").
		WithWeight("bold").
		WithColor(lineutil.ColorText).
		WithSize("sm").FlexText)

	// Burst (short-term) quota
	burstAvailable := int(math.Floor(stats.BurstAvailable))
	burstMax := int(stats.BurstMax)
	var burstPercentage float64
	if stats.BurstMax > 0 {
		burstPercentage = stats.BurstAvailable / stats.BurstMax * 100
	}

	// Calculate hourly refill (convert from per-second)
	hourlyRefill := stats.BurstRefillRate * 3600

	// Short-term quota subsection
	body.AddComponent(lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📈 短期額度").
			WithSize("xs").
			WithColor(lineutil.ColorText).
			WithWeight("bold").FlexText,
		lineutil.NewFlexText(fmt.Sprintf("可用: %d / %d 次", burstAvailable, burstMax)).
			WithSize("sm").
			WithColor(lineutil.ColorText).
			WithMargin("sm").FlexText,
		h.buildProgressBar(burstPercentage).WithMargin("sm").FlexBox,
		lineutil.NewFlexText(fmt.Sprintf("💡 每小時恢復 %.0f 次", hourlyRefill)).
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").FlexText,
	).FlexBox)

	// Daily quota (if enabled)
	if stats.DailyMax > 0 {
		dailyRemaining := stats.DailyRemaining
		dailyMax := stats.DailyMax
		dailyPercentage := float64(dailyRemaining) / float64(dailyMax) * 100

		body.AddComponent(lineutil.NewFlexBox("vertical",
			lineutil.NewFlexText("📅 每日額度").
				WithSize("xs").
				WithColor(lineutil.ColorText).
				WithWeight("bold").FlexText,
			lineutil.NewFlexText(fmt.Sprintf("可用: %d / %d 次", dailyRemaining, dailyMax)).
				WithSize("sm").
				WithColor(lineutil.ColorText).
				WithMargin("sm").FlexText,
			h.buildProgressBar(dailyPercentage).WithMargin("sm").FlexBox,
			lineutil.NewFlexText("💡 滾動 24 小時計算").
				WithSize("xs").
				WithColor(lineutil.ColorSubtext).
				WithMargin("sm").FlexText,
		).FlexBox)
	}
}

// buildProgressBar creates a visual progress bar using box components.
// The bar uses explicit height to ensure visibility in LINE Flex Messages.
// Uses flex ratios to represent percentage: filled vs empty parts.
func (h *Handler) buildProgressBar(percentage float64) *lineutil.FlexBox {
	// Clamp percentage to 0-100
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 100 {
		percentage = 100
	}

	// Determine color based on percentage
	var color string
	switch {
	case percentage > 50:
		color = "#4CAF50" // Green - healthy
	case percentage > 20:
		color = "#FFC107" // Yellow - warning
	default:
		color = "#F44336" // Red - critical
	}

	// Calculate flex ratios (integer values, range 1-100)
	filledFlex := int32(percentage)
	emptyFlex := int32(100 - percentage)

	// Ensure minimum visibility of 1 for non-zero values
	if filledFlex == 0 && percentage > 0 {
		filledFlex = 1
		emptyFlex = 99
	}
	if emptyFlex == 0 && percentage < 100 {
		emptyFlex = 1
		filledFlex = 99
	}

	// Handle exact 0% and 100%
	if percentage <= 0 {
		filledFlex = 0
		emptyFlex = 100
	} else if percentage >= 100 {
		filledFlex = 100
		emptyFlex = 0
	}

	// Create progress bar container with explicit height for visibility
	// LINE Flex Message requires height for empty boxes to render
	const barHeight = "8px"

	// Build filled and empty box parts
	filledBox := lineutil.NewFlexBox("vertical").
		WithBackgroundColor(color).
		WithHeight(barHeight)
	filledBox.Flex = filledFlex

	emptyBox := lineutil.NewFlexBox("vertical").
		WithBackgroundColor("#E0E0E0").
		WithHeight(barHeight)
	emptyBox.Flex = emptyFlex

	// Create horizontal container for the bar
	bar := lineutil.NewFlexBox("horizontal",
		filledBox.FlexBox,
		emptyBox.FlexBox,
	).WithCornerRadius("sm").WithHeight(barHeight)

	return bar
}

// Ensure Handler implements bot interfaces
var (
	_ bot.Handler    = (*Handler)(nil)
	_ bot.NLUHandler = (*Handler)(nil)
)

// Compile-time check that usageRegex is valid
var _ *regexp.Regexp = usageRegex
