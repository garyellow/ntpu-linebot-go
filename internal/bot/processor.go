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
	llmLimiter     *ratelimit.KeyedLimiter
	userLimiter    *ratelimit.KeyedLimiter
	stickerManager *sticker.Manager
	logger         *logger.Logger
	metrics        *metrics.Metrics

	// Configuration
	webhookTimeout time.Duration
}

// ProcessorConfig holds configuration for creating a new Processor.
type ProcessorConfig struct {
	Registry       *Registry
	IntentParser   genai.IntentParser // Interface for multi-provider support
	LLMLimiter     *ratelimit.KeyedLimiter
	UserLimiter    *ratelimit.KeyedLimiter
	StickerManager *sticker.Manager
	Logger         *logger.Logger
	Metrics        *metrics.Metrics
	BotConfig      *config.BotConfig
}

// isNLUEnabled returns true if NLU intent parser is available.
func (p *Processor) isNLUEnabled() bool {
	return p.intentParser != nil && p.intentParser.IsEnabled()
}

// NewProcessor creates a new event processor.
func NewProcessor(cfg ProcessorConfig) *Processor {
	return &Processor{
		registry:       cfg.Registry,
		intentParser:   cfg.IntentParser,
		llmLimiter:     cfg.LLMLimiter,
		userLimiter:    cfg.UserLimiter,
		stickerManager: cfg.StickerManager,
		logger:         cfg.Logger,
		metrics:        cfg.Metrics,
		webhookTimeout: cfg.BotConfig.WebhookTimeout,
	}
}

// injectContextValues adds tracing values (chatID, userID) to context for logging and monitoring.
func (p *Processor) injectContextValues(ctx context.Context, source webhook.SourceInterface) context.Context {
	chatID := GetChatID(source)
	userID := GetUserID(source)
	ctx = ctxutil.WithChatID(ctx, chatID)
	ctx = ctxutil.WithUserID(ctx, userID)
	return ctx
}

// ProcessMessage handles a text message event.
func (p *Processor) ProcessMessage(ctx context.Context, event webhook.MessageEvent) ([]messaging_api.MessageInterface, error) {
	// Inject context values for tracing and logging
	ctx = p.injectContextValues(ctx, event.Source)

	// Extract QuoteToken early for Quote Reply functionality.
	// Both Text and Sticker messages have this field, but LINE API only supports
	// displaying quote tokens in TextMessage replies (other message types ignore it).
	var quoteToken string
	switch m := event.Message.(type) {
	case webhook.TextMessageContent:
		quoteToken = m.QuoteToken
	case webhook.StickerMessageContent:
		quoteToken = m.QuoteToken
	}

	if quoteToken != "" {
		ctx = ctxutil.WithQuoteToken(ctx, quoteToken)
	}

	// Check rate limit early to avoid unnecessary processing
	// This happens AFTER extracting quoteToken so rate limit messages can quote the user
	if allowed, rateLimitMsg := p.checkUserRateLimit(ctx, event.Source, GetChatID(event.Source)); !allowed {
		lineutil.SetQuoteTokenToFirst(rateLimitMsg, ctxutil.GetQuoteToken(ctx))
		return rateLimitMsg, nil
	}

	// Handle sticker messages - only in personal chats
	if event.Message.GetType() == "sticker" {
		if IsPersonalChat(event.Source) {
			p.logger.WithField("message_type", "sticker").InfoContext(ctx, "Received direct message")
			msgs := p.handleStickerMessage(ctx, event)
			lineutil.SetQuoteTokenToFirst(msgs, ctxutil.GetQuoteToken(ctx))
			return msgs, nil
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
	p.logger.WithField("message_type", "text").
		WithField("text", text).
		InfoContext(ctx, "Received text message")

	// Validate text length (LINE API allows up to config.LINEMaxTextMessageLength characters)
	if len(text) == 0 {
		return nil, nil // Empty message, ignore
	}
	if len(text) > config.LINEMaxTextMessageLength {
		p.logger.WithField("limit", config.LINEMaxTextMessageLength).
			WarnContext(ctx, "Text message exceeds LINE length limit")
		sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("❌ 訊息內容過長\n\n訊息長度超過 %d 字元，請縮短後重試。", config.LINEMaxTextMessageLength),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
		// Apply quote token to error message for context
		lineutil.SetQuoteToken(msg, ctxutil.GetQuoteToken(ctx))
		return []messaging_api.MessageInterface{msg}, nil
	}

	// Sanitize input: normalize whitespace, remove punctuation
	text = sanitizeText(text)
	if len(text) == 0 {
		return nil, nil // Empty after sanitization
	}

	// Check for help keywords FIRST (before dispatching to bot modules)
	if slices.ContainsFunc(helpKeywords, func(k string) bool {
		return strings.EqualFold(text, k)
	}) {
		p.logger.InfoContext(ctx, "User requested help/instruction")
		msgs := p.getDetailedInstructionMessages()
		lineutil.SetQuoteTokenToFirst(msgs, ctxutil.GetQuoteToken(ctx))
		return msgs, nil
	}

	// Create context with timeout for bot processing.
	// PreserveTracing also preserves quoteToken for downstream handlers.
	processCtx, cancel := context.WithTimeout(ctxutil.PreserveTracing(ctx), p.webhookTimeout)
	defer cancel()

	// Dispatch to appropriate bot module based on CanHandle
	if msgs := p.registry.DispatchMessage(processCtx, text); len(msgs) > 0 {
		lineutil.SetQuoteTokenToFirst(msgs, ctxutil.GetQuoteToken(processCtx))
		return msgs, nil
	}

	// No handler matched - try NLU if available
	msgs, err := p.handleUnmatchedMessage(processCtx, event.Source, textMsg, text)
	if err == nil && len(msgs) > 0 {
		lineutil.SetQuoteTokenToFirst(msgs, ctxutil.GetQuoteToken(processCtx))
	}
	return msgs, err
}

// ProcessPostback handles a postback event.
func (p *Processor) ProcessPostback(ctx context.Context, event webhook.PostbackEvent) ([]messaging_api.MessageInterface, error) {
	// Inject context values for tracing and logging
	ctx = p.injectContextValues(ctx, event.Source)

	data := event.Postback.Data

	// Validate postback data
	if len(data) == 0 {
		p.logger.DebugContext(ctx, "Empty postback data")
		return nil, nil
	}
	if len(data) > config.LINEMaxPostbackDataLength {
		p.logger.WithField("data", data).
			WithField("limit", config.LINEMaxPostbackDataLength).
			WarnContext(ctx, "Postback data exceeds LINE length limit")
		sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender("❌ 操作資料異常\n\n請使用下方按鈕重新操作", sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
		return []messaging_api.MessageInterface{msg}, nil
	}

	// Sanitize postback data
	data = strings.TrimSpace(data)
	if len(data) == 0 {
		p.logger.DebugContext(ctx, "Empty postback data after trim")
		return nil, nil
	}

	p.logger.WithField("data", data).InfoContext(ctx, "Received postback")

	// Check for help keywords FIRST (before dispatching to bot modules)
	if slices.ContainsFunc(helpKeywords, func(k string) bool {
		return strings.EqualFold(data, k)
	}) {
		p.logger.InfoContext(ctx, "User requested help/instruction via postback")
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
	sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender("⚠️ 操作已過期或無效\n\n請使用下方按鈕重新操作", sender)
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
	return []messaging_api.MessageInterface{msg}, nil
}

// ProcessFollow handles a follow event.
// Returns a Flex Message welcome card with Quick Reply for better UX.
func (p *Processor) ProcessFollow(ctx context.Context, event webhook.FollowEvent) ([]messaging_api.MessageInterface, error) {
	ctx = p.injectContextValues(ctx, event.Source)
	p.logger.InfoContext(ctx, "Follow event received")

	sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)

	// Build welcome Flex Message
	welcomeMsg := p.buildWelcomeFlexMessage(p.isNLUEnabled(), sender)

	return []messaging_api.MessageInterface{welcomeMsg}, nil
}

// ProcessJoin handles a join event.
// Returns a Flex Message welcome card with Quick Reply for better UX.
func (p *Processor) ProcessJoin(ctx context.Context, event webhook.JoinEvent) ([]messaging_api.MessageInterface, error) {
	ctx = p.injectContextValues(ctx, event.Source)
	p.logger.InfoContext(ctx, "Join event received")

	sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)

	// Build welcome Flex Message
	welcomeMsg := p.buildWelcomeFlexMessage(p.isNLUEnabled(), sender)

	return []messaging_api.MessageInterface{welcomeMsg}, nil
}

// buildWelcomeFlexMessage creates a structured welcome message for new users.
func (p *Processor) buildWelcomeFlexMessage(nluEnabled bool, sender *messaging_api.Sender) messaging_api.MessageInterface {
	// Hero section with blue theme
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("泥好~~").WithSize("lg").WithColor(lineutil.ColorHeroText).WithWeight("bold").FlexText,
		lineutil.NewFlexText("我是 NTPU 小工具 🔍").WithSize("md").WithColor(lineutil.ColorHeroText).WithMargin("sm").FlexText,
	).
		WithBackgroundColor(lineutil.ColorHeaderPrimary).
		WithPaddingAll("xl").
		WithPaddingBottom("lg")

	// Feature list based on AI availability
	var features []messaging_api.FlexComponentInterface

	if nluEnabled {
		features = append(features,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("💬").WithSize("sm").WithFlex(0).FlexText,
				lineutil.NewFlexText("支援自然語言對話").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
			).WithMargin("xs").FlexBox,
		)
	}

	features = append(features,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📚").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("課程查詢：課程 微積分").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
		).WithMargin("xs").FlexBox,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("🔮").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("智慧搜尋：找課 資料分析").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
		).WithMargin("xs").FlexBox,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("🎓").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("學號查詢：學號 王小明").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
		).WithMargin("xs").FlexBox,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📞").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("聯絡查詢：聯絡 資工系").WithSize("sm").WithColor(lineutil.ColorText).WithMargin("sm").WithWrap(true).FlexText,
		).WithMargin("xs").FlexBox,
	)

	// Body section (preallocate capacity: 1 initial + features + 3 data source elements)
	bodyContents := make([]messaging_api.FlexComponentInterface, 0, 1+len(features)+3)
	bodyContents = append(bodyContents,
		lineutil.NewFlexText("🎯 主要功能").WithWeight("bold").WithColor(lineutil.ColorText).WithSize("sm").FlexText,
	)
	bodyContents = append(bodyContents, features...)

	// Data source note
	bodyContents = append(bodyContents,
		lineutil.NewFlexSeparator().WithMargin("lg").FlexSeparator,
		lineutil.NewFlexText("📊 資料來源").WithWeight("bold").WithColor(lineutil.ColorText).WithSize("sm").WithMargin("lg").FlexText,
		lineutil.NewFlexText("課程查詢系統、數位學苑 2.0、校園聯絡簿").WithSize("xs").WithColor(lineutil.ColorSubtext).WithMargin("sm").WithWrap(true).FlexText,
	)

	body := lineutil.NewFlexBox("vertical", bodyContents...).WithSpacing("sm")

	// Footer with help and feedback buttons
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(lineutil.NewMessageAction("📖 查看使用說明", "使用說明")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonInternal).
			WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("🐛 回報 Bug / ✨ 功能許願", "https://github.com/garyellow/ntpu-linebot-go/issues/new/choose")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonExternal).
			WithHeight("sm").
			WithMargin("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("👨‍💻 聯繫作者", "https://linktr.ee/garyellow")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonExternal).
			WithHeight("sm").
			WithMargin("sm").FlexButton,
	).WithSpacing("none")

	bubble := lineutil.NewFlexBubble(nil, hero.FlexBox, body, footer)
	msg := lineutil.NewFlexMessage("歡迎使用 NTPU 小工具", bubble.FlexBubble)
	msg.Sender = sender

	// Add Quick Reply for immediate actions
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNav())

	return msg
}

// handleUnmatchedMessage handles messages that don't match any keyword pattern.
func (p *Processor) handleUnmatchedMessage(ctx context.Context, source webhook.SourceInterface, textMsg webhook.TextMessageContent, sanitizedText string) ([]messaging_api.MessageInterface, error) {
	// Check if we're in a group chat
	isGroup := !IsPersonalChat(source)

	// For group chats, only respond if bot is mentioned
	if isGroup {
		if !IsBotMentioned(textMsg) {
			// No @Bot mention in group - silently ignore
			return nil, nil
		}
		// Remove @Bot mentions from ORIGINAL text for NLU processing
		if textMsg.Mention != nil {
			mentionlessText := removeBotMentions(textMsg.Text, textMsg.Mention)
			if mentionlessText == "" {
				return p.getHelpMessage(FallbackGeneric), nil
			}
			// Apply same sanitization as original text processing
			sanitizedText = sanitizeText(mentionlessText)
			if sanitizedText == "" {
				return p.getHelpMessage(FallbackGeneric), nil
			}
		}
	}

	// Try NLU if available
	if p.isNLUEnabled() {
		chatID := GetChatID(source)
		return p.handleWithNLU(ctx, sanitizedText, source, chatID)
	}

	// NLU not available - return help message with context
	return p.getHelpMessage(FallbackNLUDisabled), nil
}

// handleWithNLU processes the message using NLU intent parsing.
// With forced function calling (ANY/required mode), the model always returns a function call.
func (p *Processor) handleWithNLU(ctx context.Context, text string, source webhook.SourceInterface, chatID string) ([]messaging_api.MessageInterface, error) {
	// Check LLM rate limit before making API call
	if allowed, rateLimitMsg := p.checkLLMRateLimit(ctx, source, chatID); !allowed {
		return rateLimitMsg, nil
	}

	result, err := p.intentParser.Parse(ctx, text)

	if err != nil {
		p.logger.WithError(err).WarnContext(ctx, "NLU intent parsing failed")
		// Metrics are recorded by FallbackIntentParser
		return p.getHelpMessage(FallbackNLUFailed), nil
	}

	if result == nil {
		// Metrics are recorded by FallbackIntentParser
		return p.getHelpMessage(FallbackNLUFailed), nil
	}

	p.logger.WithField("module", result.Module).
		WithField("intent", result.Intent).
		WithField("params", result.Params).
		InfoContext(ctx, "NLU intent parsed")
	// Metrics are recorded by FallbackIntentParser

	return p.dispatchIntent(ctx, result)
}

// dispatchIntent dispatches the parsed intent to the appropriate handler.
func (p *Processor) dispatchIntent(ctx context.Context, result *genai.ParseResult) ([]messaging_api.MessageInterface, error) {
	if result.Module == "help" {
		return p.getDetailedInstructionMessages(), nil
	}

	// Handle direct_reply from NLU (used for greetings, clarifications, off-topic queries)
	if result.Module == "direct_reply" {
		message, ok := result.Params["message"]
		if !ok || message == "" {
			p.logger.WarnContext(ctx, "direct_reply missing message parameter")
			return p.getHelpMessage(FallbackGeneric), nil
		}
		sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(message, sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
		return []messaging_api.MessageInterface{msg}, nil
	}

	handler := p.registry.GetHandler(result.Module)
	if handler == nil {
		p.logger.WithField("module", result.Module).WarnContext(ctx, "Unknown module from NLU")
		return p.getHelpMessage(FallbackUnknownModule), nil
	}

	if nluHandler, ok := handler.(NLUHandler); ok {
		msgs, err := nluHandler.DispatchIntent(ctx, result.Intent, result.Params)
		if err != nil {
			p.logger.WithError(err).WithField("intent", result.Intent).WarnContext(ctx, "Dispatch failed")
			return p.getHelpMessage(FallbackDispatchFailed), nil
		}
		return msgs, nil
	}

	p.logger.WithField("module", result.Module).WarnContext(ctx, "Handler does not support NLU")
	return p.getHelpMessage(FallbackDispatchFailed), nil
}

// checkUserRateLimit checks if the user has exceeded their rate limit.
func (p *Processor) checkUserRateLimit(ctx context.Context, source webhook.SourceInterface, chatID string) (bool, []messaging_api.MessageInterface) {
	if chatID == "" {
		return true, nil
	}

	if p.userLimiter.Allow(chatID) {
		return true, nil
	}

	p.logger.WarnContext(ctx, "User rate limit exceeded")

	if IsPersonalChat(source) {
		sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender(
			"⏳ 訊息過於頻繁，請稍後再試\n💡 稍等幾秒後即可繼續使用",
			sender,
		)
		// Add Quick Reply to guide user when rate limit expires
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())
		return false, []messaging_api.MessageInterface{msg}
	}

	return false, nil
}

// checkLLMRateLimit checks if the user has exceeded their LLM API rate limit.
func (p *Processor) checkLLMRateLimit(ctx context.Context, source webhook.SourceInterface, chatID string) (bool, []messaging_api.MessageInterface) {
	if chatID == "" || p.llmLimiter == nil {
		return true, nil
	}

	if p.llmLimiter.Allow(chatID) {
		return true, nil
	}

	p.logger.WarnContext(ctx, "LLM rate limit exceeded")

	if IsPersonalChat(source) {
		sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
		msg := p.buildLLMRateLimitFlexMessage(sender)

		return false, []messaging_api.MessageInterface{
			msg,
		}
	}

	return false, nil
}

// handleStickerMessage processes sticker messages
func (p *Processor) handleStickerMessage(ctx context.Context, _ webhook.MessageEvent) []messaging_api.MessageInterface {
	p.logger.InfoContext(ctx, "Sticker message received; replying with random sticker")

	stickerURL := p.stickerManager.GetRandomSticker()
	sender := lineutil.GetSender("貼圖小幫手", p.stickerManager)

	imageMsg := &messaging_api.ImageMessage{
		OriginalContentUrl: stickerURL,
		PreviewImageUrl:    stickerURL,
		Sender:             sender,
	}

	return []messaging_api.MessageInterface{imageMsg}
}

// FallbackContext provides context for why the fallback message is being shown
type FallbackContext string

// Fallback context types for error message classification
const (
	FallbackGeneric        FallbackContext = ""           // Generic/unspecified (group chat with only @Bot mention)
	FallbackNLUDisabled    FallbackContext = "nlu_off"    // NLU not available and no keyword match
	FallbackNLUFailed      FallbackContext = "nlu_failed" // NLU parsing failed
	FallbackDispatchFailed FallbackContext = "dispatch"   // Intent dispatch failed
	FallbackUnknownModule  FallbackContext = "module"     // Unknown module from NLU
)

// getHelpMessage returns a contextualized fallback message as Flex Message for better UX
// context parameter helps provide transparent feedback to users about why their input wasn't understood
func (p *Processor) getHelpMessage(context FallbackContext) []messaging_api.MessageInterface {
	sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
	nluEnabled := p.isNLUEnabled()

	// Hero section with contextualized message
	var heroTitle, heroSubtext string
	switch context {
	case FallbackNLUDisabled:
		heroTitle = "📖 請使用關鍵字"
		heroSubtext = "目前僅支援關鍵字查詢"
	case FallbackNLUFailed:
		heroTitle = "😅 無法理解訊息"
		heroSubtext = "請試著換個方式說明，或使用關鍵字"
	case FallbackDispatchFailed, FallbackUnknownModule:
		heroTitle = "⚠️ 處理失敗"
		heroSubtext = "系統暫時無法處理此請求"
	case FallbackGeneric:
		fallthrough
	default:
		heroTitle = "🔍 NTPU 小工具"
		if nluEnabled {
			heroSubtext = "直接對話或使用關鍵字查詢"
		} else {
			heroSubtext = "使用關鍵字快速查詢"
		}
	}

	// Hero section - Contextualized feedback
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText(heroTitle).
			WithSize("md").
			WithWeight("bold").
			WithColor(lineutil.ColorHeroText).FlexText,
		lineutil.NewFlexText(heroSubtext).
			WithSize("sm").
			WithColor(lineutil.ColorHeroText).
			WithMargin("sm").FlexText,
	).
		WithBackgroundColor(lineutil.ColorHeaderPrimary).
		WithPaddingAll("lg").
		WithPaddingBottom("md")

	// Build body content based on AI availability
	var bodyContents []messaging_api.FlexComponentInterface

	if nluEnabled {
		// AI mode examples
		bodyContents = append(bodyContents,
			lineutil.NewFlexText("💬 直接問我").
				WithWeight("bold").
				WithColor(lineutil.ColorText).
				WithSize("sm").FlexText,
			lineutil.NewFlexText("• 微積分的課有哪些\n• 王小明的學號\n• 資工系電話").
				WithSize("xs").
				WithColor(lineutil.ColorSubtext).
				WithMargin("sm").
				WithWrap(true).FlexText,
			lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,
		)
	}

	// Keyword examples (always show)
	bodyContents = append(bodyContents,
		lineutil.NewFlexText("📖 關鍵字查詢").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin(func() string {
				if nluEnabled {
					return "md"
				}
				return "none"
			}()).FlexText,
		lineutil.NewFlexText("📚 課程 微積分、課程 王教授\n🎓 學號 王小明、系 資工\n📞 聯絡 資工系、緊急").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").
			WithWrap(true).FlexText,
	)

	body := lineutil.NewFlexBox("vertical", bodyContents...).WithSpacing("none")

	// Footer with help button
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(lineutil.NewMessageAction("📖 查看完整說明", "使用說明")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonInternal).
			WithHeight("sm").FlexButton,
	).WithSpacing("none")

	bubble := lineutil.NewFlexBubble(nil, hero.FlexBox, body, footer)
	msg := lineutil.NewFlexMessage("NTPU 小工具", bubble.FlexBubble)
	msg.Sender = sender
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNav())

	return []messaging_api.MessageInterface{msg}
}

// getDetailedInstructionMessages returns detailed instruction messages
// Total messages: 3 or 4 Flex Messages - within LINE's 5-message limit
func (p *Processor) getDetailedInstructionMessages() []messaging_api.MessageInterface {
	sender := lineutil.GetSender("NTPU 小工具", p.stickerManager)
	nluEnabled := p.isNLUEnabled()

	var messages []messaging_api.MessageInterface

	// AI mode introduction (if enabled)
	if nluEnabled {
		aiModeFlex := p.buildAIModeFlexMessage(sender)
		messages = append(messages, aiModeFlex)
	}

	// Keyword mode instructions (always show)
	keywordFlex := p.buildKeywordModeFlexMessage(nluEnabled, sender)
	messages = append(messages, keywordFlex)

	// Tips message
	tipsFlex := p.buildTipsFlexMessage(nluEnabled, sender)
	messages = append(messages, tipsFlex)

	// Add data source information with Flex Message
	dataSourceFlex := p.buildDataSourceFlexMessage(sender)
	messages = append(messages, dataSourceFlex)

	return messages
}

// buildAIModeFlexMessage creates a Flex Message for AI mode instructions
func (p *Processor) buildAIModeFlexMessage(sender *messaging_api.Sender) messaging_api.MessageInterface {
	// Hero section - Primary instruction (core feature)
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("🤖 AI 模式").
			WithSize("lg").
			WithWeight("bold").
			WithColor(lineutil.ColorHeroText).FlexText,
		lineutil.NewFlexText("直接用自然語言問我").
			WithSize("md").
			WithColor(lineutil.ColorHeroText).
			WithMargin("sm").FlexText,
	).
		WithBackgroundColor(lineutil.ColorHeaderPrimary).
		WithPaddingAll("xl").
		WithPaddingBottom("lg")

	// Body section with examples
	body := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("💬 使用範例").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin("none").FlexText,
		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		// Example 1
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithFlex(0).FlexText,
			lineutil.NewFlexText("「微積分的課有哪些」").
				WithSize("sm").
				WithColor(lineutil.ColorText).
				WithMargin("sm").
				WithWrap(true).FlexText,
		).WithMargin("md").FlexBox,

		// Example 2
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithFlex(0).FlexText,
			lineutil.NewFlexText("「王小明的學號是多少」").
				WithSize("sm").
				WithColor(lineutil.ColorText).
				WithMargin("sm").
				WithWrap(true).FlexText,
		).WithMargin("sm").FlexBox,

		// Example 3
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithFlex(0).FlexText,
			lineutil.NewFlexText("「人工智慧學程有什麼課」").
				WithSize("sm").
				WithColor(lineutil.ColorText).
				WithMargin("sm").
				WithWrap(true).FlexText,
		).WithMargin("sm").FlexBox,

		// Example 4
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithFlex(0).FlexText,
			lineutil.NewFlexText("「資工系的電話是多少」").
				WithSize("sm").
				WithColor(lineutil.ColorText).
				WithMargin("sm").
				WithWrap(true).FlexText,
		).WithMargin("sm").FlexBox,

		// Example 5
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").
				WithSize("sm").
				WithColor(lineutil.ColorSubtext).
				WithFlex(0).FlexText,
			lineutil.NewFlexText("「緊急電話幾號」").
				WithSize("sm").
				WithColor(lineutil.ColorText).
				WithMargin("sm").
				WithWrap(true).FlexText,
		).WithMargin("sm").FlexBox,

		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		lineutil.NewFlexText("✨ AI 會自動理解您的問題").
			WithSize("xs").
			WithColor(lineutil.ColorNote).
			WithMargin("md").
			WithAlign("center").
			WithWrap(true).FlexText,
	).WithSpacing("none")

	bubble := lineutil.NewFlexBubble(hero, nil, body, nil)
	msg := lineutil.NewFlexMessage("AI 模式說明", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}

	// Add Quick Reply for convenient navigation
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainFeatures())

	return msg
}

// buildKeywordModeFlexMessage creates a Flex Message for keyword mode instructions
func (p *Processor) buildKeywordModeFlexMessage(nluEnabled bool, sender *messaging_api.Sender) messaging_api.MessageInterface {
	titleText := "📖 關鍵字模式"
	if !nluEnabled {
		titleText = "📖 使用說明"
	}

	// Hero section - Primary instruction (core feature)
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText(titleText).
			WithSize("lg").
			WithWeight("bold").
			WithColor(lineutil.ColorHeroText).FlexText,
		lineutil.NewFlexText("使用關鍵字進行查詢").
			WithSize("md").
			WithColor(lineutil.ColorHeroText).
			WithMargin("sm").FlexText,
	).
		WithBackgroundColor(lineutil.ColorHeaderPrimary).
		WithPaddingAll("xl").
		WithPaddingBottom("lg")

	// Body section with all features
	body := lineutil.NewFlexBox("vertical",
		// Course search
		lineutil.NewFlexText("📚 課程查詢").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin("none").FlexText,
		lineutil.NewFlexText("• 精確：課程 微積分 / 課程 王教授").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 智慧：找課 線上實體混合").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 課號：U0001 或 1131U0001").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,

		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		// Program search
		lineutil.NewFlexText("🎯 學程查詢").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin("md").FlexText,
		lineutil.NewFlexText("• 列表：學程 或 所有學程").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 搜尋：學程 人工智慧").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,

		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		// Student ID search
		lineutil.NewFlexText("🎓 學號查詢").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin("md").FlexText,
		lineutil.NewFlexText("• 姓名：學號 王小明").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 科系：系 資工 / 系代碼 87").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 學年：學年 112").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 系代碼：學士系代碼 / 碩士系代碼").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 直接輸入：412345678").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,

		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		// Contact search
		lineutil.NewFlexText("📞 聯絡資訊").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin("md").FlexText,
		lineutil.NewFlexText("• 單位：聯絡 資工系").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 電話：電話 圖書館").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 信箱：信箱 教務處").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 緊急：緊急").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,

		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		// Usage query
		lineutil.NewFlexText("📊 配額查詢").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithSize("sm").
			WithMargin("md").FlexText,
		lineutil.NewFlexText("• 查詢：配額 / 用量 / 額度").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("sm").
			WithWrap(true).FlexText,
		lineutil.NewFlexText("• 顯示：訊息額度與 AI 額度").
			WithSize("xs").
			WithColor(lineutil.ColorSubtext).
			WithMargin("xs").
			WithWrap(true).FlexText,
	).WithSpacing("none")

	bubble := lineutil.NewFlexBubble(hero, nil, body, nil)
	msg := lineutil.NewFlexMessage("關鍵字模式說明", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}

	// Add Quick Reply for convenient navigation
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainFeatures())

	return msg
}

// buildTipsFlexMessage creates a Flex Message for usage tips
func (p *Processor) buildTipsFlexMessage(nluEnabled bool, sender *messaging_api.Sender) messaging_api.MessageInterface {
	// Hero section - Tips and suggestions
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("💡 使用提示").
			WithSize("lg").
			WithWeight("bold").
			WithColor(lineutil.ColorHeroText).FlexText,
	).
		WithBackgroundColor(lineutil.ColorHeaderTips).
		WithPaddingAll("xl").
		WithPaddingBottom("lg")

	// Body section with tips
	var bodyContents []messaging_api.FlexComponentInterface

	if nluEnabled {
		bodyContents = []messaging_api.FlexComponentInterface{
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("AI 模式：直接對話，不需關鍵字").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("none").FlexBox,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("關鍵字模式：關鍵字在句首 + 空格").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("sm").FlexBox,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("AI 配額用完時請改用關鍵字").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("sm").FlexBox,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("課程/聯絡資料每天更新").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("sm").FlexBox,
		}
	} else {
		bodyContents = []messaging_api.FlexComponentInterface{
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("關鍵字必須在句首，之後加空格").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("none").FlexBox,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("支援中英文關鍵字").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("sm").FlexBox,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("大部分查詢支援模糊搜尋").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("sm").FlexBox,
			lineutil.NewFlexBox("horizontal",
				lineutil.NewFlexText("•").
					WithSize("sm").
					WithColor(lineutil.ColorSubtext).
					WithFlex(0).FlexText,
				lineutil.NewFlexText("課程/聯絡資料每天更新").
					WithSize("sm").
					WithColor(lineutil.ColorText).
					WithMargin("sm").
					WithWrap(true).FlexText,
			).WithMargin("sm").FlexBox,
		}
	}

	body := lineutil.NewFlexBox("vertical", bodyContents...).WithSpacing("none")

	bubble := lineutil.NewFlexBubble(hero, nil, body, nil)
	msg := lineutil.NewFlexMessage("使用提示", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}

	// Add Quick Reply for convenient navigation
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainFeatures())

	return msg
}

// buildDataSourceFlexMessage creates a Flex Message displaying data sources
func (p *Processor) buildDataSourceFlexMessage(sender *messaging_api.Sender) messaging_api.MessageInterface {
	// Hero section - Informational
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📊 資料來源").
			WithSize("lg").
			WithWeight("bold").
			WithColor(lineutil.ColorHeroText),
	).
		WithBackgroundColor(lineutil.ColorHeaderInfo).
		WithPaddingAll("xl").
		WithPaddingBottom("lg")

	// Body section with data sources (simplified)
	body := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("所有查詢資料來自 NTPU 公開網站").
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
			lineutil.NewFlexText("課程查詢系統").
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
			lineutil.NewFlexText("數位學苑 2.0").
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
			lineutil.NewFlexText("校園聯絡簿").
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
		lineutil.NewFlexButton(lineutil.NewURIAction("課程查詢系統", "https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.chi_main")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonExternal).
			WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("數位學苑 2.0", "https://lms.ntpu.edu.tw")).
			WithStyle("primary").
			WithColor(lineutil.ColorButtonExternal).
			WithHeight("sm").
			WithMargin("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("校園聯絡簿", "https://sea.cc.ntpu.edu.tw/pls/ld/campus_dir_m.main")).
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

	// Add Quick Reply for convenient navigation
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainFeatures())

	return msg
}

// buildLLMRateLimitFlexMessage creates a Flex Message for LLM rate limit notification.
// It displays quota status, reset time, and alternative keyword options.
func (p *Processor) buildLLMRateLimitFlexMessage(sender *messaging_api.Sender) *messaging_api.FlexMessage {
	// Hero section - warning style
	hero := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("⏳ AI 配額已用完").
			WithSize("md").
			WithWeight("bold").
			WithColor(lineutil.ColorHeroText).FlexText,
	).
		WithBackgroundColor(lineutil.ColorWarning).
		WithPaddingAll("lg").
		WithPaddingBottom("md")

	// Body section - simplified message and alternatives
	body := lineutil.NewFlexBox("vertical",
		// Simple status
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("📊").WithSize("sm").WithFlex(0).FlexText,
			lineutil.NewFlexText("目前配額已用完，請稍後再試").
				WithSize("sm").
				WithColor(lineutil.ColorText).
				WithMargin("sm").
				WithWrap(true).FlexText,
		).FlexBox,

		lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator,

		// Alternative options header
		lineutil.NewFlexText("💡 配額重置前僅能使用關鍵字查詢").
			WithSize("sm").
			WithWeight("bold").
			WithColor(lineutil.ColorText).
			WithMargin("md").FlexText,

		// Alternative options list
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").WithSize("xs").WithColor(lineutil.ColorSubtext).WithFlex(0).FlexText,
			lineutil.NewFlexText("課程 微積分").WithSize("xs").WithColor(lineutil.ColorSubtext).WithMargin("sm").FlexText,
		).WithMargin("sm").FlexBox,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").WithSize("xs").WithColor(lineutil.ColorSubtext).WithFlex(0).FlexText,
			lineutil.NewFlexText("學號 王小明").WithSize("xs").WithColor(lineutil.ColorSubtext).WithMargin("sm").FlexText,
		).WithMargin("xs").FlexBox,
		lineutil.NewFlexBox("horizontal",
			lineutil.NewFlexText("•").WithSize("xs").WithColor(lineutil.ColorSubtext).WithFlex(0).FlexText,
			lineutil.NewFlexText("聯絡 資工系").WithSize("xs").WithColor(lineutil.ColorSubtext).WithMargin("sm").FlexText,
		).WithMargin("xs").FlexBox,
	).WithSpacing("none")

	bubble := lineutil.NewFlexBubble(hero, nil, body, nil)
	msg := lineutil.NewFlexMessage("AI 配額已用完", bubble.FlexBubble)
	if sender != nil {
		msg.Sender = sender
	}

	// Add Quick Reply for convenient access to keyword features
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyMainNavCompact())

	return msg
}

// Helper functions

// sanitizeText performs complete text sanitization:
// 1. Trim spaces
// 2. Normalize whitespace
// 3. Remove punctuation
// 4. Final normalization
func sanitizeText(text string) string {
	text = strings.TrimSpace(text)
	text = normalizeWhitespace(text)
	text = removePunctuation(text)
	return normalizeWhitespace(text)
}

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
