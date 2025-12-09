package bot

import (
	"context"
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
	intentParser   *genai.GeminiIntentParser
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
	IntentParser   *genai.GeminiIntentParser
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
		return nil, fmt.Errorf("failed to cast message to text")
	}

	text := textMsg.Text

	// Validate text length (LINE API allows up to 20000 characters)
	if len(text) == 0 {
		return nil, nil // Empty message, ignore
	}
	maxLen := 20000 // LINE API limit
	if len(text) > maxLen {
		p.logger.Warnf("Text message too long: %d characters", len(text))
		sender := lineutil.GetSender("系統小幫手", p.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender(
				fmt.Sprintf("❌ 訊息內容過長\n\n訊息長度超過 %d 字元，請縮短後重試。", maxLen),
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

	p.logger.WithField("text", text).Debug("Received text message")

	// Check for help keywords FIRST (before dispatching to bot modules)
	if slices.ContainsFunc(helpKeywords, func(k string) bool {
		return strings.EqualFold(text, k)
	}) {
		p.logger.Info("User requested help/instruction")
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
		p.logger.Warn("Empty postback data")
		return nil, nil
	}
	if len(data) > 300 { // LINE postback data limit is 300 bytes
		p.logger.Warnf("Postback data too long: %d bytes", len(data))
		sender := lineutil.GetSender("系統小幫手", p.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("❌ 操作資料異常\n\n請重新使用功能。", sender),
		}, nil
	}

	// Sanitize postback data
	data = strings.TrimSpace(data)

	p.logger.WithField("data", data).Debug("Received postback")

	// Check for help keywords FIRST (before dispatching to bot modules)
	if slices.ContainsFunc(helpKeywords, func(k string) bool {
		return strings.EqualFold(data, k)
	}) {
		p.logger.Info("User requested help/instruction via postback")
		return p.getDetailedInstructionMessages(), nil
	}

	// Create context with timeout for postback processing.
	processCtx, cancel := context.WithTimeout(ctxutil.PreserveTracing(ctx), p.webhookTimeout)
	defer cancel()

	// Check module prefix or dispatch to all handlers
	if msgs := p.registry.DispatchPostback(processCtx, data); len(msgs) > 0 {
		return msgs, nil
	}

	// No handler matched
	sender := lineutil.GetSender("系統小幫手", p.stickerManager)
	return []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithConsistentSender("操作已過期或無效", sender),
	}, nil
}

// ProcessFollow handles a follow event.
func (p *Processor) ProcessFollow(event webhook.FollowEvent) ([]messaging_api.MessageInterface, error) {
	p.logger.Info("New user followed the bot")

	// Check feature availability
	nluEnabled := p.intentParser != nil && p.intentParser.IsEnabled()

	// Send welcome message
	sender := lineutil.GetSender("初階小幫手", p.stickerManager)

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

	start := time.Now()

	result, err := p.intentParser.Parse(ctx, text)
	duration := time.Since(start).Seconds()

	if err != nil {
		p.logger.WithError(err).Warn("NLU intent parsing failed")
		p.metrics.RecordLLMRequest("nlu", "error", duration)
		p.metrics.RecordLLMFallback("nlu")
		return p.getHelpMessage(), nil
	}

	if result == nil {
		p.metrics.RecordLLMRequest("nlu", "error", duration)
		p.metrics.RecordLLMFallback("nlu")
		return p.getHelpMessage(), nil
	}

	if result.ClarificationText != "" {
		p.logger.WithField("clarification", result.ClarificationText).Debug("NLU returned clarification")
		p.metrics.RecordLLMRequest("nlu", "clarification", duration)

		sender := lineutil.GetSender("小幫手", p.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender(result.ClarificationText, sender),
		}, nil
	}

	p.logger.WithField("module", result.Module).
		WithField("intent", result.Intent).
		WithField("params", result.Params).
		Debug("NLU intent parsed")
	p.metrics.RecordLLMRequest("nlu", "success", duration)

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
		return false, []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender(
				"⏳ 訊息過於頻繁，請稍後再試",
				sender,
			),
		}
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
				"💡 您仍可使用：\n"+
				"• 關鍵字查詢：課程 微積分\n"+
				"• 課號查詢：1131U0001",
			p.llmRateLimitPerHour,
			resetMinutes,
		)

		msg := lineutil.NewTextMessageWithConsistentSender(message, sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			lineutil.QuickReplyCourseAction(),
			lineutil.QuickReplyHelpAction(),
		})

		return false, []messaging_api.MessageInterface{
			msg,
		}
	}

	return false, nil
}

// handleStickerMessage processes sticker messages
func (p *Processor) handleStickerMessage(_ webhook.MessageEvent) []messaging_api.MessageInterface {
	p.logger.Info("Received sticker message, replying with random sticker image")

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
			"📖 或使用關鍵字查詢：\n" +
			"• 課程：「課程 微積分」\n" +
			"• 學生：「學生 王小明」\n" +
			"• 聯繫：「聯繫 資工系」\n\n" +
			"💡 輸入「使用說明」查看完整說明"
	} else {
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

	sender := lineutil.GetSender("幫助小幫手", p.stickerManager)
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

// getDetailedInstructionMessages returns detailed instruction messages
func (p *Processor) getDetailedInstructionMessages() []messaging_api.MessageInterface {
	senderName := "小幫手"
	nluEnabled := p.intentParser != nil && p.intentParser.IsEnabled()

	sender := lineutil.GetSender(senderName, p.stickerManager)
	var msg string
	if nluEnabled {
		msg = "使用說明 (AI 模式)\n\n您可以直接跟我聊天，例如：\n「幫我查微積分」\n「資工系辦公室在哪」"
	} else {
		msg = "使用說明 (關鍵字模式)\n\n請使用以下格式：\n課程 [關鍵字]\n學生 [關鍵字]\n聯繫 [關鍵字]"
	}

	return []messaging_api.MessageInterface{
		lineutil.NewTextMessageWithConsistentSender(msg, sender),
	}
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
