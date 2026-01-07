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
	senderName = "配額小幫手"
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
		"用量", "配額", "額度",
		"quota", "usage", "limit",
	}
	usageRegex = bot.BuildKeywordRegex(usageKeywords)
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

// CanHandle returns true if the text matches usage keywords.
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)
	return usageRegex.MatchString(text)
}

// HandleMessage processes usage queries and returns a Flex Message with quota status.
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
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
	// Rate limiting uses chat ID as the primary key
	if chatID, ok := ctx.Value("chatID").(string); ok {
		return chatID
	}
	return ""
}

// buildUsageFlexMessage creates a Flex Message displaying usage statistics.
func (h *Handler) buildUsageFlexMessage(userStats, llmStats ratelimit.UsageStats, sender *messaging_api.Sender) *messaging_api.FlexMessage {
	// Hero section
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📊 使用配額狀態").
			WithSize("lg").
			WithWeight("bold").
			WithColor(lineutil.ColorHeroText).FlexText,
	).
		WithBackgroundColor(lineutil.ColorHeaderPrimary).
		WithPaddingAll("lg").
		WithPaddingBottom("md")

	// Body sections
	var bodyContents []messaging_api.FlexComponentInterface

	// User rate limit section (if available)
	if h.userLimiter != nil {
		bodyContents = append(bodyContents, h.buildUserRateLimitSection(userStats)...)
		bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("lg").FlexSeparator)
	}

	// LLM rate limit section (if available)
	if h.llmLimiter != nil {
		bodyContents = append(bodyContents, h.buildLLMRateLimitSection(llmStats)...)
	}

	body := lineutil.NewFlexBox("vertical", bodyContents...).WithSpacing("sm")

	// Footer with quick actions
	footer := lineutil.NewFlexBox("horizontal",
		lineutil.NewFlexButton(lineutil.NewMessageAction("📚 課程查詢", "課程")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonInternal).
			WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewMessageAction("📖 使用說明", "使用說明")).
			WithStyle("secondary").
			WithHeight("sm").
			WithMargin("sm").FlexButton,
	).WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(hero, nil, body, footer)
	msg := lineutil.NewFlexMessage("使用配額狀態", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}

	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyUsageNav())

	return msg
}

// buildUserRateLimitSection creates the user rate limit display section.
func (h *Handler) buildUserRateLimitSection(stats ratelimit.UsageStats) []messaging_api.FlexComponentInterface {
	available := int(math.Floor(stats.BurstAvailable))
	max := int(stats.BurstMax)
	percentage := stats.BurstAvailable / stats.BurstMax * 100

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

	return []messaging_api.FlexComponentInterface{
		lineutil.NewFlexText("⚡ 訊息頻率限制").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").FlexText,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText(fmt.Sprintf("可用: %d / %d 次", available, max)).
				WithSize("sm").
				WithColor(lineutil.ColorText).FlexText,
		).WithMargin("sm").FlexBox,
		h.buildProgressBar(percentage).WithMargin("sm").FlexBox,
		lineutil.NewFlexText(fmt.Sprintf("💡 %s", refillInfo)).
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").FlexText,
	}
}

// buildLLMRateLimitSection creates the LLM rate limit display section.
func (h *Handler) buildLLMRateLimitSection(stats ratelimit.UsageStats) []messaging_api.FlexComponentInterface {
	var contents []messaging_api.FlexComponentInterface

	contents = append(contents,
		lineutil.NewFlexText("🤖 AI 功能配額").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin("md").FlexText,
	)

	// Burst (short-term) quota
	burstAvailable := int(math.Floor(stats.BurstAvailable))
	burstMax := int(stats.BurstMax)
	burstPercentage := stats.BurstAvailable / stats.BurstMax * 100

	// Calculate hourly refill (convert from per-second)
	hourlyRefill := stats.BurstRefillRate * 3600

	contents = append(contents,
		lineutil.NewFlexText("📈 短期配額").
			WithSize("xs").
			WithColor(lineutil.ColorText).
			WithWeight("bold").
			WithMargin("md").FlexText,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText(fmt.Sprintf("可用: %d / %d 次", burstAvailable, burstMax)).
				WithSize("sm").
				WithColor(lineutil.ColorText).FlexText,
		).WithMargin("sm").FlexBox,
		h.buildProgressBar(burstPercentage).WithMargin("sm").FlexBox,
		lineutil.NewFlexText(fmt.Sprintf("💡 每小時恢復 %.0f 次", hourlyRefill)).
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").FlexText,
	)

	// Daily quota (if enabled)
	if stats.DailyMax > 0 {
		dailyRemaining := stats.DailyRemaining
		dailyMax := stats.DailyMax
		dailyPercentage := float64(dailyRemaining) / float64(dailyMax) * 100

		contents = append(contents,
			lineutil.NewFlexText("📅 每日配額").
				WithSize("xs").
				WithColor(lineutil.ColorText).
				WithWeight("bold").
				WithMargin("md").FlexText,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText(fmt.Sprintf("可用: %d / %d 次", dailyRemaining, dailyMax)).
					WithSize("sm").
					WithColor(lineutil.ColorText).FlexText,
			).WithMargin("sm").FlexBox,
			h.buildProgressBar(dailyPercentage).WithMargin("sm").FlexBox,
			lineutil.NewFlexText("💡 每日凌晨重置").
				WithSize("xs").
				WithColor(lineutil.ColorSubtext).
				WithMargin("sm").FlexText,
		)
	}

	return contents
}

// buildProgressBar creates a visual progress bar using box components.
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
		color = "#4CAF50" // Green
	case percentage > 20:
		color = "#FFC107" // Yellow
	default:
		color = "#F44336" // Red
	}

	// Create filled and empty parts
	// Use flex ratios to represent percentage (min 2 to ensure visibility)
	filledFlex := int32(percentage)
	emptyFlex := int32(100) - filledFlex

	// Ensure minimum visibility (at least 2%)
	if filledFlex > 0 && filledFlex < 2 {
		filledFlex = 2
		emptyFlex = 98
	} else if emptyFlex > 0 && emptyFlex < 2 {
		emptyFlex = 2
		filledFlex = 98
	}

	// Handle 0% and 100% cases
	if percentage <= 0 {
		filledFlex = 1
		emptyFlex = 99
	} else if percentage >= 100 {
		filledFlex = 99
		emptyFlex = 1
	}

	filledBox := lineutil.NewFlexBox("vertical").
		WithBackgroundColor(color).
		WithCornerRadius("sm")
	filledBox.Flex = filledFlex

	emptyBox := lineutil.NewFlexBox("vertical").
		WithBackgroundColor("#E0E0E0").
		WithCornerRadius("sm")
	emptyBox.Flex = emptyFlex

	return lineutil.NewFlexBox("horizontal",
		filledBox.FlexBox,
		emptyBox.FlexBox,
	).
		WithCornerRadius("sm").
		WithBackgroundColor("#E0E0E0")
}

// Ensure Handler implements bot interfaces
var (
	_ bot.Handler    = (*Handler)(nil)
	_ bot.NLUHandler = (*Handler)(nil)
)

// Compile-time check that usageRegex is valid
var _ *regexp.Regexp = usageRegex
