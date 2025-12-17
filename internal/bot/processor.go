package bot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"
)

// helpKeywords are the keywords that trigger the help message
var helpKeywords = []string{"使用說明", "help"}

// Processor handles the core logic of processing LINE events.
// It orchestrates rate limiting, NLU parsing, and dispatching to handlers.
type Processor struct {
	registry       *Registry
	intentParser   genai.IntentParser // Interface for multi-provider support
	llmLimiter     *ratelimit.LLMRateLimiter
	userLimiter    *ratelimit.UserRateLimiter
	stickerManager *sticker.Manager
	logger         *logger.Logger
	metrics        *metrics.Metrics

	// Configuration
	webhookTimeout      time.Duration
	llmRateLimitPerHour float64
}

// ProcessorConfig holds configuration for creating a new Processor.
type ProcessorConfig struct {
	Registry       *Registry
	IntentParser   genai.IntentParser // Interface for multi-provider support
	LLMRateLimiter *ratelimit.LLMRateLimiter
	UserLimiter    *ratelimit.UserRateLimiter
	StickerManager *sticker.Manager
	Logger         *logger.Logger
	Metrics        *metrics.Metrics
	BotConfig      *config.BotConfig
}

// NewProcessor creates a new event processor.
func NewProcessor(cfg ProcessorConfig) *Processor {
	return &Processor{
		registry:            cfg.Registry,
		intentParser:        cfg.IntentParser,
		llmLimiter:          cfg.LLMRateLimiter,
		userLimiter:         cfg.UserLimiter,
		stickerManager:      cfg.StickerManager,
		logger:              cfg.Logger,
		metrics:             cfg.Metrics,
		webhookTimeout:      cfg.BotConfig.WebhookTimeout,
		llmRateLimitPerHour: cfg.BotConfig.LLMRateLimitPerHour,
	}
}

// ProcessMessage handles a text message event.
func (p *Processor) ProcessMessage(ctx context.Context, event webhook.MessageEvent) ([]messaging_api.MessageInterface, error) {
	// Extract and inject context values for tracing and logging
	chatID := GetChatID(event.Source)
	userID := GetUserID(event.Source)

	// Inject context values for downstream handlers
	ctx = ctxutil.WithChatID(ctx, chatID)
	ctx = ctxutil.WithUserID(ctx, userID)

	// Check rate limit early to avoid unnecessary processing
	if allowed, rateLimitMsg := p.checkUserRateLimit(event.Source, chatID); !allowed {
		return rateLimitMsg, nil
	}

	// Handle sticker messages - only in personal chats
	if event.Message.GetType() == "sticker" {
		if IsPersonalChat(event.Source) {
			return p.handleStickerMessage(event), nil
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
		return nil, errors.New("failed to cast message to text")
	}

	text := textMsg.Text

	// Validate text length (LINE API allows up to 20000 characters)
	if len(text) == 0 {
		return nil, nil // Empty message, ignore
	}
	maxLen := 20000 // LINE API limit
	if len(text) > maxLen {
		p.logger.Infof("Text message too long: %d characters (limit: %d)", len(text), maxLen)
		sender := lineutil.GetSender("系統小幫手", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("❌ 訊息內容過長\n\n訊息長度超過 %d 字元，請縮短後重試。", maxLen),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}, nil
	}

	// Sanitize input: normalize whitespace, remove punctuation
	text = strings.TrimSpace(text)
	text = normalizeWhitespace(text)
	text = removePunctuation(text)
	text = normalizeWhitespace(text) // Final normalization after punctuation removal
	if len(text) == 0 {
		return nil, nil // Empty after sanitization
	}

	// Check for help keywords FIRST (before dispatching to bot modules)
	if slices.ContainsFunc(helpKeywords, func(k string) bool {
		return strings.EqualFold(text, k)
	}) {
		p.logger.Debug("User requested help/instruction")
		return p.getDetailedInstructionMessages(), nil
	}

	// Create context with timeout for bot processing.
	processCtx, cancel := context.WithTimeout(ctxutil.PreserveTracing(ctx), p.webhookTimeout)
	defer cancel()

	// Dispatch to appropriate bot module based on CanHandle
	if msgs := p.registry.DispatchMessage(processCtx, text); len(msgs) > 0 {
		return msgs, nil
	}

	// No handler matched - try NLU if available
	return p.handleUnmatchedMessage(processCtx, event.Source, textMsg, text)
}

// ProcessPostback handles a postback event.
func (p *Processor) ProcessPostback(ctx context.Context, event webhook.PostbackEvent) ([]messaging_api.MessageInterface, error) {
	// Extract and inject context values for tracing and logging
	chatID := GetChatID(event.Source)
	userID := GetUserID(event.Source)

	// Inject context values for downstream handlers
	ctx = ctxutil.WithChatID(ctx, chatID)
	ctx = ctxutil.WithUserID(ctx, userID)

	data := event.Postback.Data

	// Validate postback data
	if len(data) == 0 {
		p.logger.Debug("Empty postback data")
		return nil, nil
	}
	if len(data) > 300 { // LINE postback data limit is 300 bytes
		p.logger.Infof("Postback data too long: %d bytes (limit: 300)", len(data))
		sender := lineutil.GetSender("系統小幫手", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender("❌ 操作資料異常\n\n請使用下方按鈕重新操作", sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyContactAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}, nil
	}

	// Sanitize postback data
	data = strings.TrimSpace(data)

	p.logger.WithField("data", data).Debug("Received postback")

	// Check for help keywords FIRST (before dispatching to bot modules)
	if slices.ContainsFunc(helpKeywords, func(k string) bool {
		return strings.EqualFold(data, k)
	}) {
		p.logger.Debug("User requested help/instruction via postback")
		return p.getDetailedInstructionMessages(), nil
	}

	// Create context with timeout for postback processing.
	processCtx, cancel := context.WithTimeout(ctxutil.PreserveTracing(ctx), p.webhookTimeout)
	defer cancel()

	// Check module prefix or dispatch to all handlers
	if msgs := p.registry.DispatchPostback(processCtx, data); len(msgs) > 0 {
		return msgs, nil
	}

	// No handler matched - provide helpful guidance
	sender := lineutil.GetSender("系統小幫手", p.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender("⚠️ 操作已過期或無效\n\n請使用下方按鈕重新操作", sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyCourseAction(),
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyContactAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}, nil
}

// ProcessFollow handles a follow event.
// Returns a Flex Message welcome card with Quick Reply for better UX.
func (p *Processor) ProcessFollow(event webhook.FollowEvent) ([]messaging_api.MessageInterface, error) {
	p.logger.Info("New user followed the bot")

	nluEnabled := p.intentParser != nil && p.intentParser.IsEnabled()
	sender := lineutil.GetSender("初階小幫手", p.stickerManager)

	// Build welcome Flex Message
	welcomeMsg := p.buildWelcomeFlexMessage(nluEnabled, sender)

	return []messaging_api.MessageInterface{welcomeMsg}, nil
}

// buildWelcomeFlexMessage creates a structured welcome message for new users.
func (p *Processor) buildWelcomeFlexMessage(nluEnabled bool, sender *messaging_api.Sender) messaging_api.MessageInterface {
	// Hero section
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("泥好~~").WithSize("lg").WithColor(lineutil.ColorHeroText).WithWeight("bold").FlexText,
		lineutil.NewFlexText("我是北大查詢小工具 🔍").WithSize("md").WithColor(lineutil.ColorHeroText).WithMargin("sm").FlexText,
	).
		WithBackgroundColor(lineutil.ColorHeroBg).
		WithPaddingAll("xl").
		WithPaddingBottom("lg")

	// Feature list based on AI availability
	var features []messaging_api.FlexComponentInterface

	if nluEnabled {
		features = append(features,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("💬").WithSize("sm").WithFlex(0).FlexText,
				lineutil.NewFlexText("直接用自然語言問我").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
			).FlexBox,
		)
	}

	features = append(features,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📚").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("課程查詢：課程 微積分").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
		).FlexBox,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("🎓").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("學號查詢：學號 王小明").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
		).FlexBox,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📞").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("聯絡查詢：聯絡 資工系").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
		).FlexBox,
	)

	// Body section
	bodyContents := []messaging_api.FlexComponentInterface{
		lineutil.NewFlexText("🎯 主要功能").WithWeight("bold").WithColor(lineutil.ColorText).WithSize("sm").FlexText,
	}
	bodyContents = append(bodyContents, features...)

	// Data source note
	bodyContents = append(bodyContents,
		lineutil.NewFlexSeparator().WithMargin("lg").FlexSeparator,
		lineutil.NewFlexText("📊 資料來源").WithWeight("bold").WithColor(lineutil.ColorText).WithSize("sm").WithMargin("lg").FlexText,
		lineutil.NewFlexText("課程查詢系統、數位學苑 2.0、校園聯絡簿").WithSize("xs").WithColor(lineutil.ColorSubtext).WithMargin("sm").WithWrap(true).FlexText,
		lineutil.NewFlexSeparator().WithMargin("lg").FlexSeparator,
		lineutil.NewFlexText("⚠️ 部分內容是由相關資料推斷，不一定為正確資訊").WithSize("xs").WithColor(lineutil.ColorNote).WithMargin("md").WithWrap(true).FlexText,
	)

	body := lineutil.NewFlexBox("vertical", bodyContents...).WithSpacing("sm")

	// Footer with help button
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(lineutil.NewMessageAction("📖 查看使用說明", "使用說明")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonPrimary).
			WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("❓ 常見問題 / 回報 Bug", "https://github.com/garyellow/ntpu-linebot-go")).
			WithStyle("secondary").
			WithHeight("sm").
			WithMargin("sm").FlexButton,
	).WithSpacing("none")

	bubble := lineutil.NewFlexBubble(nil, hero.FlexBox, body, footer)
	msg := lineutil.NewFlexMessage("歡迎使用北大查詢小工具", bubble.FlexBubble)
	msg.Sender = sender

	// Add Quick Reply for immediate actions
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyCourseAction(),
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyContactAction(),
		lineutil.QuickReplyEmergencyAction(),
		lineutil.QuickReplyHelpAction(),
	})

	return msg
}

// handleUnmatchedMessage handles messages that don't match any keyword pattern.
func (p *Processor) handleUnmatchedMessage(ctx context.Context, source webhook.SourceInterface, textMsg webhook.TextMessageContent, sanitizedText string) ([]messaging_api.MessageInterface, error) {
	// Check if we're in a group chat
	isGroup := !IsPersonalChat(source)

	// For group chats, only respond if bot is mentioned
	if isGroup {
		if !isBotMentioned(textMsg) {
			// No @Bot mention in group - silently ignore
			return nil, nil
		}
		// Remove @Bot mentions from ORIGINAL text for NLU processing
		if textMsg.Mention != nil {
			mentionlessText := removeBotMentions(textMsg.Text, textMsg.Mention)
			if mentionlessText == "" {
				return p.getHelpMessage(), nil
			}
			// Apply same sanitization as original text processing
			sanitizedText = strings.TrimSpace(mentionlessText)
			sanitizedText = normalizeWhitespace(sanitizedText)
			sanitizedText = removePunctuation(sanitizedText)
			sanitizedText = normalizeWhitespace(sanitizedText)
			if sanitizedText == "" {
				return p.getHelpMessage(), nil
			}
		}
	}

	// Try NLU if available
	if p.intentParser != nil && p.intentParser.IsEnabled() {
		chatID := GetChatID(source)
		return p.handleWithNLU(ctx, sanitizedText, source, chatID)
	}

	// NLU not available - return help message
	return p.getHelpMessage(), nil
}

// handleWithNLU processes the message using NLU intent parsing.
func (p *Processor) handleWithNLU(ctx context.Context, text string, source webhook.SourceInterface, chatID string) ([]messaging_api.MessageInterface, error) {
	// Check LLM rate limit before making API call
	if allowed, rateLimitMsg := p.checkLLMRateLimit(source, chatID); !allowed {
		return rateLimitMsg, nil
	}

	result, err := p.intentParser.Parse(ctx, text)

	if err != nil {
		p.logger.WithError(err).Warn("NLU intent parsing failed")
		// Metrics are recorded by FallbackIntentParser
		return p.getHelpMessage(), nil
	}

	if result == nil {
		// Metrics are recorded by FallbackIntentParser
		return p.getHelpMessage(), nil
	}

	if result.ClarificationText != "" {
		p.logger.WithField("clarification", result.ClarificationText).Debug("NLU returned clarification")

		sender := lineutil.GetSender("小幫手", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(result.ClarificationText, sender)
		// Add Quick Reply to guide user for clarification responses
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyContactAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return []messaging_api.MessageInterface{msg}, nil
	}

	p.logger.WithField("module", result.Module).
		WithField("intent", result.Intent).
		WithField("params", result.Params).
		Debug("NLU intent parsed")
	// Metrics are recorded by FallbackIntentParser

	return p.dispatchIntent(ctx, result)
}

// dispatchIntent dispatches the parsed intent to the appropriate handler.
func (p *Processor) dispatchIntent(ctx context.Context, result *genai.ParseResult) ([]messaging_api.MessageInterface, error) {
	if result.Module == "help" {
		return p.getDetailedInstructionMessages(), nil
	}

	handler := p.registry.GetHandler(result.Module)
	if handler == nil {
		p.logger.WithField("module", result.Module).Warn("Unknown module from NLU")
		return p.getHelpMessage(), nil
	}

	if nluHandler, ok := handler.(NLUHandler); ok {
		msgs, err := nluHandler.DispatchIntent(ctx, result.Intent, result.Params)
		if err != nil {
			p.logger.WithError(err).WithField("intent", result.Intent).Warn("Dispatch failed")
			return p.getHelpMessage(), nil
		}
		return msgs, nil
	}

	p.logger.WithField("module", result.Module).Warn("Handler does not support NLU")
	return p.getHelpMessage(), nil
}

// checkUserRateLimit checks if the user has exceeded their rate limit.
func (p *Processor) checkUserRateLimit(source webhook.SourceInterface, chatID string) (bool, []messaging_api.MessageInterface) {
	if chatID == "" {
		return true, nil
	}

	if p.userLimiter.Allow(chatID) {
		return true, nil
	}

	logChatID := chatID
	if len(chatID) > 8 {
		logChatID = chatID[:8] + "..."
	}
	p.logger.WithField("chat_id", logChatID).Warn("User rate limit exceeded")

	if IsPersonalChat(source) {
		sender := lineutil.GetSender("系統小幫手", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⏳ 訊息過於頻繁，請稍後再試\n\n💡 稍等幾秒後即可繼續使用",
			sender,
		)
		// Add Quick Reply to guide user when rate limit expires
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyStudentAction(),
			lineutil.QuickReplyContactAction(),
			lineutil.QuickReplyHelpAction(),
		})
		return false, []messaging_api.MessageInterface{msg}
	}

	return false, nil
}

// checkLLMRateLimit checks if the user has exceeded their LLM API rate limit.
func (p *Processor) checkLLMRateLimit(source webhook.SourceInterface, chatID string) (bool, []messaging_api.MessageInterface) {
	if chatID == "" || p.llmLimiter == nil {
		return true, nil
	}

	if p.llmLimiter.Allow(chatID) {
		return true, nil
	}

	logChatID := chatID
	if len(chatID) > 8 {
		logChatID = chatID[:8] + "..."
	}
	p.logger.WithField("chat_id", logChatID).Warn("LLM rate limit exceeded")

	if IsPersonalChat(source) {
		available := p.llmLimiter.GetAvailable(chatID)
		resetMinutes := int((p.llmRateLimitPerHour - available) * 3600 / p.llmRateLimitPerHour / 60)
		if resetMinutes < 1 {
			resetMinutes = 1
		}

		sender := lineutil.GetSender("系統小幫手", p.stickerManager)
		message := fmt.Sprintf(
			"⏳ AI 功能使用次數已達上限\n\n"+
				"📊 本小時配額：%.0f 次（已用完）\n"+
				"⏰ 約 %d 分鐘後重置\n\n"+
				"💡 您仍可使用關鍵字查詢：\n"+
				"• 課程：課程 微積分\n"+
				"• 學號：學生 王小明\n"+
				"• 聯絡：聯繫 資工系",
			p.llmRateLimitPerHour,
			resetMinutes,
		)

		msg := lineutil.NewTextMessageWithConsistentSender(message, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyHelpAction(),
			lineutil.QuickReplyCourseAction(),
		})

		return false, []messaging_api.MessageInterface{
			msg,
		}
	}

	return false, nil
}

// handleStickerMessage processes sticker messages
func (p *Processor) handleStickerMessage(_ webhook.MessageEvent) []messaging_api.MessageInterface {
	p.logger.Debug("Received sticker message, replying with random sticker image")

	stickerURL := p.stickerManager.GetRandomSticker()
	sender := lineutil.GetSender("貼圖小幫手", p.stickerManager)

	imageMsg := &messaging_api.ImageMessage{
		OriginalContentUrl: stickerURL,
		PreviewImageUrl:    stickerURL,
		Sender:             sender,
	}

	return []messaging_api.MessageInterface{imageMsg}
}

// getHelpMessage returns a simplified help message
func (p *Processor) getHelpMessage() []messaging_api.MessageInterface {
	var helpText string

	if p.intentParser != nil && p.intentParser.IsEnabled() {
		helpText = "🔍 NTPU 查詢小工具\n\n" +
			"💬 直接用自然語言問我，例如：\n" +
			"• 「微積分的課有哪些」\n" +
			"• 「王小明的學號」\n" +
			"• 「資工系電話」\n\n" +
			"📖 或使用關鍵字：\n" +
			"• 課程：「課程 微積分」「老師 王教授」\n" +
			"• 學號：「學號 王小明」「系 資工」\n" +
			"• 聯絡：「聯絡 資工系」「緊急」\n\n" +
			"💡 輸入「使用說明」查看完整說明"
	} else {
		helpText = "🔍 NTPU 查詢小工具\n\n" +
			"📚 課程查詢\n" +
			"• 「課程 微積分」「老師 王教授」\n" +
			"• 「U0001」（課號查詢）\n" +
			"• 「找課 Python」（智慧搜尋）\n\n" +
			"🎓 學號查詢\n" +
			"• 「學號 王小明」「系 資工」\n" +
			"• 「412345678」（直接輸入學號）\n\n" +
			"📞 聯絡資訊\n" +
			"• 「聯絡 資工系」「電話 學務處」\n" +
			"• 「緊急」（緊急聯絡電話）\n\n" +
			"💡 輸入「使用說明」查看完整說明"
	}

	sender := lineutil.GetSender("幫助小幫手", p.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender(helpText, sender)
	msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyCourseAction(),
		lineutil.QuickReplyStudentAction(),
		lineutil.QuickReplyContactAction(),
		lineutil.QuickReplyEmergencyAction(),
		lineutil.QuickReplyHelpAction(),
	})
	return []messaging_api.MessageInterface{msg}
}

// getDetailedInstructionMessages returns detailed instruction messages
// Total messages: 4 (AI mode) or 3 (keyword mode) - within LINE's 5-message limit
func (p *Processor) getDetailedInstructionMessages() []messaging_api.MessageInterface {
	senderName := "小幫手"
	nluEnabled := p.intentParser != nil && p.intentParser.IsEnabled()
	sender := lineutil.GetSender(senderName, p.stickerManager)

	var messages []messaging_api.MessageInterface

	// AI mode introduction (if enabled)
	if nluEnabled {
		aiMsg := "🤖 使用說明 - AI 模式\n\n" +
			"💬 直接用自然語言問我，例如：\n" +
			"• 「微積分的課有哪些」\n" +
			"• 「王小明的學號是多少」\n" +
			"• 「資工系辦公室在哪裡」\n" +
			"• 「緊急電話幾號」\n\n" +
			"✨ AI 會自動理解您的問題"
		messages = append(messages, lineutil.NewTextMessageWithConsistentSender(aiMsg, sender))
	}

	// Keyword mode instructions (always show) - MERGED into ONE message
	keywordTitle := "📖 使用說明 - 關鍵字模式"
	if nluEnabled {
		keywordTitle = "📖 關鍵字模式"
	}

	// Merge all keyword instructions into ONE message to stay within 5-message limit
	allFeaturesMsg := keywordTitle + "\n\n" +
		"📚 課程查詢\n" +
		"• 精確：課程 微積分 / 老師 王教授\n" +
		"• 智慧：找課 線上實體混合\n" +
		"• 課號：U0001 或 1131U0001\n\n" +
		"🎓 學號查詢\n" +
		"• 姓名：學號 王小明\n" +
		"• 科系：系 資工 / 系代碼 87\n" +
		"• 學年：學年 112\n" +
		"• 直接輸入：412345678\n\n" +
		"📞 聯絡資訊\n" +
		"• 單位：聯絡 資工系\n" +
		"• 電話：電話 圖書館\n" +
		"• 信箱：信箱 教務處\n" +
		"• 緊急：緊急"
	messages = append(messages, lineutil.NewTextMessageWithConsistentSender(allFeaturesMsg, sender))

	// Tips message (combined with usage hints)
	tipsMsg := "💡 使用提示\n" +
		"• 關鍵字必須在句首，之後加空格\n" +
		"• 支援中英文關鍵字\n" +
		"• 大部分查詢支援模糊搜尋\n" +
		"• 資料每 7 天自動更新"
	if nluEnabled {
		tipsMsg = "💡 使用提示\n" +
			"• AI 模式：直接對話，不需關鍵字\n" +
			"• 關鍵字模式：關鍵字在句首 + 空格\n" +
			"• AI 配額用盡時自動使用關鍵字\n" +
			"• 資料每 7 天自動更新"
	}
	messages = append(messages, lineutil.NewTextMessageWithConsistentSender(tipsMsg, sender))

	// Add data source information with Flex Message
	dataSourceFlex := p.buildDataSourceFlexMessage(sender)
	messages = append(messages, dataSourceFlex)

	return messages
}

// buildDataSourceFlexMessage creates a Flex Message displaying data sources
func (p *Processor) buildDataSourceFlexMessage(sender *messaging_api.Sender) messaging_api.MessageInterface {
	// Hero section
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📊 資料來源").
			WithSize("lg").
			WithWeight("bold").
			WithColor("#FFFFFF"),
	).
		WithBackgroundColor(lineutil.ColorButtonPrimary).
		WithPaddingAll("md").
		WithPaddingBottom("lg")

	// Body section with data sources (simplified)
	body := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("所有查詢資料來自北大公開網站").
			WithSize("sm").
			WithColor(lineutil.ColorText).
			WithWeight("bold").
			WithMargin("none"),
		lineutil.NewFlexSeparator().WithMargin("md"),

		// Course data source
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📚").
				WithSize("sm").
				WithFlex(0),
			lineutil.NewFlexText("課程查詢系統 (SEA)").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithMargin("sm").
				WithWrap(true),
		).WithMargin("md").FlexBox,

		// Student data source
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("🎓").
				WithSize("sm").
				WithFlex(0),
			lineutil.NewFlexText("數位學苑 2.0 (LMS)").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithMargin("sm").
				WithWrap(true),
		).WithMargin("sm").FlexBox,

		// Contact data source
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📞").
				WithSize("sm").
				WithFlex(0),
			lineutil.NewFlexText("校園聯絡簿 (SEA)").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithMargin("sm").
				WithWrap(true),
		).WithMargin("sm").FlexBox,

		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		lineutil.NewFlexText("點擊下方按鈕查看原始網站").
			WithSize("xs").
			WithColor(lineutil.ColorNote).
			WithMargin("md").
			WithAlign("center").
			WithWrap(true).FlexText,
	).
		WithSpacing("none")

	// Footer with URL buttons
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(lineutil.NewURIAction("課程查詢系統", "https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.html")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonExternal).
			WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("數位學苑", "https://lms.ntpu.edu.tw")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonExternal).
			WithHeight("sm").
			WithMargin("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("校園聯絡簿", "https://sea.cc.ntpu.edu.tw/pls/web_pro/stdcontactadm_showlist.show_list")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonExternal).
			WithHeight("sm").
			WithMargin("sm").FlexButton,
	).
		WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(hero, nil, body, footer)
	msg := lineutil.NewFlexMessage("資料來源", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}
	return msg
}

// Helper functions

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func removePunctuation(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == ' ',
			r >= 0x4E00 && r <= 0x9FFF,
			r >= 0x3400 && r <= 0x4DBF:
			result.WriteRune(r)
		case r >= 0x3000 && r <= 0x303F:
			if r == 0x3000 {
				result.WriteRune(' ')
			}
		default:
		}
	}
	return result.String()
}
