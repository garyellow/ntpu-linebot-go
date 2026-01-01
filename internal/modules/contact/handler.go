// Package contact implements the contact directory module for the LINE bot.
// It handles queries for NTPU faculty and staff contact information.
package contact

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sliceutil"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/stringutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles contact-related queries.
// It depends on *storage.DB directly for data access.
type Handler struct {
	db               *storage.DB
	scraper          *scraper.Client
	metrics          *metrics.Metrics
	logger           *logger.Logger
	stickerManager   *sticker.Manager
	maxContactsLimit int // Maximum contacts per search (from config)

	// matchers contains all pattern-handler pairs sorted by priority.
	// Shared by CanHandle and HandleMessage for consistent routing.
	matchers []PatternMatcher
}

// Name returns the module name
func (h *Handler) Name() string {
	return ModuleName
}

// Module constants
const (
	ModuleName = "contact" // Module identifier for registration
	senderName = "聯繫小幫手"

	// Emergency phone numbers are hard-coded as constants for three critical reasons:
	//   1. Availability: No external dependency (database, scraper) - instant access
	//   2. Performance: Zero latency lookup for time-sensitive emergency situations
	//   3. Reliability: Infrequent changes managed through code review process
	//
	// Design decision: Hard-coded constants over database/config for critical data
	// Trade-off: Requires code deployment to update vs. runtime flexibility
	//
	// Emergency phone numbers (without hyphens for clipboard copy)
	// 三峽校區
	sanxiaNormalPhone    = "0286741111" // 總機
	sanxia24HPhone       = "0226731949" // 24H緊急行政電話
	sanxiaEmergencyPhone = "0226711234" // 24H急難救助電話(校安中心)
	sanxiaGatePhone      = "0226733920" // 大門哨所
	sanxiaDormPhone      = "0286716784" // 宿舍夜間緊急電話

	// 臺北校區
	taipeiNormalPhone    = "0225024654" // 總機
	taipeiEmergencyPhone = "0225023671" // 24H急難救助電話

	// 其他常用電話
	policeStation = "0226730561" // 北大派出所
	homHospital   = "0226723456" // 恩主公醫院
)

// Pattern priorities (lower = higher).
const (
	PriorityEmergency = 1 // Prefix "緊急"
	PriorityContact   = 2 // Regex match (e.g. "電話 xxx", "聯絡 xxx")
)

// PatternHandler processes a matched pattern and returns LINE messages.
type PatternHandler func(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface

// PatternMatcher represents a pattern-action pair sorted by priority.
type PatternMatcher struct {
	pattern   *regexp.Regexp
	priority  int
	handler   PatternHandler
	name      string            // For logging
	matchFunc func(string) bool // Optional custom matching logic (precedence over pattern)
}

// Valid keywords for contact queries
var (
	validContactKeywords = []string{
		// 繁體中文主要關鍵字
		"聯繫", "聯絡", "聯繫方式", "聯絡方式",
		// 簡體/異體字變體
		"連繫", "連絡",
		// 具體查詢類型
		"電話", "分機", "email", "信箱",
		// English keywords
		"touch", "contact", "connect",
	}

	contactRegex = bot.BuildKeywordRegex(validContactKeywords)
)

// NewHandler creates a new contact handler with required dependencies.
func NewHandler(
	db *storage.DB,
	scraper *scraper.Client,
	metrics *metrics.Metrics,
	logger *logger.Logger,
	stickerManager *sticker.Manager,
	maxContactsLimit int,
) *Handler {
	h := &Handler{
		db:               db,
		scraper:          scraper,
		metrics:          metrics,
		logger:           logger,
		stickerManager:   stickerManager,
		maxContactsLimit: maxContactsLimit,
	}
	h.initializeMatchers()
	return h
}

// initializeMatchers sets up the pattern-action table.
// Priority order: Emergency > Contact Regex.
func (h *Handler) initializeMatchers() {
	h.matchers = []PatternMatcher{
		{
			name:     "Emergency",
			priority: PriorityEmergency,
			matchFunc: func(text string) bool {
				return strings.HasPrefix(text, "緊急")
			},
			handler: h.handleEmergencyPattern,
		},
		{
			name:     "Contact Regex",
			priority: PriorityContact,
			pattern:  contactRegex,
			handler:  h.handleContactPattern,
		},
	}

	// Sort by priority (1 is highest)
	slices.SortFunc(h.matchers, func(a, b PatternMatcher) int {
		return a.priority - b.priority
	})
}

// Intent names for NLU dispatcher
const (
	IntentSearch    = "search"    // Contact search by name/organization
	IntentEmergency = "emergency" // Emergency phone numbers
)

// DispatchIntent handles NLU-parsed intents for the contact module.
// ... (DispatchIntent implementation remains the same)
func (h *Handler) DispatchIntent(ctx context.Context, intent string, params map[string]string) ([]messaging_api.MessageInterface, error) {
	// Validate parameters first (before logging) to support testing with nil dependencies
	switch intent {
	case IntentSearch:
		query, ok := params["query"]
		if !ok || query == "" {
			return nil, fmt.Errorf("%w: query", domerrors.ErrMissingParameter)
		}
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debugf("Dispatching contact intent: %s, query: %s", intent, query)
		}
		return h.handleContactSearch(ctx, query), nil

	case IntentEmergency:
		// Emergency intent doesn't require any parameters
		if h.logger != nil {
			h.logger.WithModule(ModuleName).Debug("Dispatching contact intent: emergency")
		}
		return h.handleEmergencyPhones(), nil

	default:
		return nil, fmt.Errorf("%w: %s", domerrors.ErrUnknownIntent, intent)
	}
}

// findMatcher returns the first matching pattern or nil.
// Used by both CanHandle and HandleMessage for consistent routing.
func (h *Handler) findMatcher(text string) *PatternMatcher {
	// Optimization: Callers (CanHandle, HandleMessage) are responsible for TrimSpace
	// to avoid redundant allocations.
	for i := range h.matchers {
		m := &h.matchers[i]
		// Use custom match function if provided, otherwise use regex
		if m.matchFunc != nil {
			if m.matchFunc(text) {
				return m
			}
		} else if m.pattern != nil && m.pattern.MatchString(text) {
			return m
		}
	}
	return nil
}

// CanHandle checks if the message is for the contact module
func (h *Handler) CanHandle(text string) bool {
	// Ensure input is trimmed before matching (optimization)
	text = strings.TrimSpace(text)
	return h.findMatcher(text) != nil
}

// HandleMessage handles text messages for the contact module
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	text = strings.TrimSpace(text)

	matcher := h.findMatcher(text)
	if matcher == nil {
		return []messaging_api.MessageInterface{}
	}

	log.Debugf("Route matched: %s (Priority: %d)", matcher.name, matcher.priority)

	var matches []string
	if matcher.pattern != nil {
		matches = matcher.pattern.FindStringSubmatch(text)
	}

	return matcher.handler(ctx, text, matches)
}

// Adapter functions for PatternHandler

func (h *Handler) handleEmergencyPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	return h.handleEmergencyPhones()
}

func (h *Handler) handleContactPattern(ctx context.Context, text string, matches []string) []messaging_api.MessageInterface {
	matchStr := matches[0] // Full match
	searchTerm := bot.ExtractSearchTerm(text, matchStr)

	if searchTerm == "" {
		return h.handleEmptySearchTerm()
	}
	return h.handleContactSearch(ctx, searchTerm)
}

func (h *Handler) handleEmptySearchTerm() []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg := lineutil.NewTextMessageWithConsistentSender("📞 請輸入查詢內容\n\n例如：\n• 聯絡 資工系\n• 電話 圖書館\n• 分機 學務處\n\n💡 提示：輸入「緊急」可查看緊急聯絡電話", sender)
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyContactNav())
	return []messaging_api.MessageInterface{msg}
}

// HandlePostback handles postback events for the contact module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	log.Infof("Handling contact postback: %s", data)

	// Strip module prefix if present (registry passes original data)
	data = strings.TrimPrefix(data, "contact:")

	// Handle "members" postback for viewing organization members
	// Format: "members${bot.PostbackSplitChar}{orgName}"
	if strings.HasPrefix(data, "members") {
		parts := strings.Split(data, bot.PostbackSplitChar)
		if len(parts) >= 2 {
			orgName := parts[1]
			return h.handleMembersQuery(ctx, orgName)
		}
	}

	return []messaging_api.MessageInterface{}
}

// handleEmergencyPhones returns emergency phone numbers
func (h *Handler) handleEmergencyPhones() []messaging_api.MessageInterface {
	// Helper to create a row with icon and optional color
	createRow := func(icon, label, value, color string) messaging_api.FlexComponentInterface {
		valColor := lineutil.ColorSubtext
		if color != "" {
			valColor = color
		}
		labelWithIcon := icon + " " + label
		// Use shrink-to-fit to prevent text truncation on long labels like "24H緊急行政電話"
		// Flex ratio 5:3 gives label more space; adjustMode shrinks text if still too long
		return lineutil.NewFlexBox("baseline",
			lineutil.NewFlexText(labelWithIcon).WithColor(lineutil.ColorLabel).WithSize("sm").WithFlex(5).WithAdjustMode("shrink-to-fit").FlexText,
			lineutil.NewFlexText(value).WithColor(valColor).WithSize("sm").WithWeight("bold").WithFlex(3).WithAlign("end").FlexText,
		).FlexBox
	}

	// Header - using standardized ColoredHeader for consistency with other modules
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: "🚨 緊急聯絡電話",
		Color: lineutil.ColorHeaderEmergency,
	})

	// Body Label - consistent with other modules (course, contact, id)
	bodyLabel := lineutil.NewBodyLabel(lineutil.BodyLabelInfo{
		Emoji: "☎️",
		Label: "校園緊急聯絡",
		Color: lineutil.ColorHeaderEmergency,
	})

	// Sanxia Campus Box
	sanxiaBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📍 三峽校區").WithWeight("bold").WithSize("md").WithColor(lineutil.ColorText).WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("📞", "總機", sanxiaNormalPhone, ""),
		createRow("🏢", "24H緊急行政電話", sanxia24HPhone, ""),
		createRow("🚨", "24H急難救助專線", sanxiaEmergencyPhone, lineutil.ColorDanger), // Highlight emergency
		createRow("🚪", "大門哨所", sanxiaGatePhone, ""),
		createRow("🏠", "宿舍夜間緊急電話", sanxiaDormPhone, ""),
		createRow("📱", "遺失物諮詢(分機66223)", sanxiaNormalPhone, ""),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// Taipei Campus Box
	taipeiBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📍 臺北校區").WithWeight("bold").WithSize("md").WithColor(lineutil.ColorText).WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("📞", "總機", taipeiNormalPhone, ""),
		createRow("🚨", "24H急難救助專線", taipeiEmergencyPhone, lineutil.ColorDanger),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// External Emergency Box
	externalBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("🚨 社會安全").WithWeight("bold").WithSize("md").WithColor(lineutil.ColorDanger).WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("👮", "警察局", "110", lineutil.ColorDanger),
		createRow("🚒", "消防/救護", "119", lineutil.ColorDanger),
		createRow("📱", "緊急救難專線", "112", lineutil.ColorDanger),
		createRow("🚔", "北大派出所", policeStation, ""),
		createRow("🏥", "恩主公醫院", homHospital, ""),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// Footer: Quick Action Buttons
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(lineutil.NewURIAction("🚨 撥打三峽校安", "tel:"+sanxiaEmergencyPhone)).WithStyle("primary").WithColor(lineutil.ColorButtonDanger).WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("🚨 撥打臺北校安", "tel:"+taipeiEmergencyPhone)).WithStyle("primary").WithColor(lineutil.ColorButtonDanger).WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("ℹ️ 查看更多", "https://new.ntpu.edu.tw/safety")).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm").FlexButton,
	).WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(
		header,
		nil,
		lineutil.NewFlexBox("vertical",
			bodyLabel.FlexBox, // Body label as first row
			sanxiaBox,
			taipeiBox,
			externalBox,
		),
		footer,
	)

	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg := lineutil.NewFlexMessage("緊急聯絡電話", bubble.FlexBubble)
	msg.Sender = sender

	// Add emergency image at the end with Quick Reply (must be on last message)
	imageURL := "https://raw.githubusercontent.com/garyellow/ntpu-linebot-go/main/assets/emergency.png"
	imgMsg := &messaging_api.ImageMessage{
		OriginalContentUrl: imageURL,
		PreviewImageUrl:    imageURL,
	}
	imgMsg.Sender = sender
	imgMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
		lineutil.QuickReplyContactAction(),
		lineutil.QuickReplyHelpAction(),
	})

	return []messaging_api.MessageInterface{msg, imgMsg}
}

// handleContactSearch handles contact search queries with a multi-tier search strategy:
//
// Search Strategy (2-tier parallel search + scraping fallback):
//
//  1. SQL LIKE (fast path): Direct database LIKE query for exact substrings.
//     SQL searches in: name, title fields only
//     Example: "資工" matches "資訊工程學系" via SQL LIKE '%資工%' (if consecutive)
//
//  2. Fuzzy character-set matching (ALWAYS runs in parallel with SQL LIKE):
//     Loads all cached contacts and checks if all runes in searchTerm exist in target.
//     Searches in: name, title, organization, superior (more fields than SQL LIKE)
//     Example: "資工系" matches "資訊工程學系" because all chars (資,工,系) exist in target
//     This enables abbreviation matching where chars are scattered in the full name.
//
//     Results from both strategies are merged and deduplicated by UID.
//
//  3. Web scraping with search variants (external fallback): If cache has no results,
//     scrape from NTPU website using multiple search variants.
//     buildSearchVariants() expands abbreviations for scraping only (not cache search)
//     because fuzzy matching already handles abbreviations in cached data.
//     Example: "資工" expands to ["資工", "資訊工程學系"] for scraping
//
// Performance notes:
//   - SQL LIKE is indexed and fast; most queries resolve here
//   - Fuzzy matching loads all contacts; runs in parallel for complete results
//   - Search variants only affect scraping, not cache lookups
func (h *Handler) handleContactSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	var contacts []storage.Contact

	// Step 1: Try SQL LIKE search first (fast path for exact substrings)
	// SQL LIKE searches in: name, title
	sqlContacts, err := h.db.SearchContactsByName(ctx, searchTerm)
	if err != nil {
		log.WithError(err).Error("Failed to search contacts in cache")
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithQuickReply("查詢聯絡資訊時發生問題", sender, "聯絡 "+searchTerm),
		}
	}
	contacts = append(contacts, sqlContacts...)

	// Step 2: ALWAYS try fuzzy character-set matching to find additional results
	// This catches cases like "資工系" -> "資訊工程學系" that SQL LIKE misses
	// Also searches more fields: name, title, organization, superior
	allContacts, err := h.db.GetAllContacts(ctx)
	if err == nil && len(allContacts) > 0 {
		for _, c := range allContacts {
			// Fuzzy character-set matching: check if all runes in searchTerm exist in target
			// Search Priority:
			// 1. Name: Covers Person Name ("王大明") and Organization Name ("資訊工程學系")
			// 2. Organization: Covers the department/unit a person belongs to ("資工系" finds members)
			// 3. Title/Superior: Supplementary info
			if stringutil.ContainsAllRunes(c.Name, searchTerm) ||
				stringutil.ContainsAllRunes(c.Title, searchTerm) ||
				stringutil.ContainsAllRunes(c.Organization, searchTerm) ||
				stringutil.ContainsAllRunes(c.Superior, searchTerm) {
				contacts = append(contacts, c)
			}
		}
	}

	// Deduplicate results by UID (SQL LIKE and fuzzy may find overlapping results)
	contacts = sliceutil.Deduplicate(contacts, func(c storage.Contact) string { return c.UID })

	// If found in cache, return results
	if len(contacts) > 0 {
		h.metrics.RecordCacheHit(ModuleName)
		log.Debugf("Cache hit for contact search: %s (found %d)", searchTerm, len(contacts))
		return h.formatContactResultsWithSearch(contacts, searchTerm)
	}

	// Cache miss - scrape from website
	// Try multiple search variants to increase hit rate
	h.metrics.RecordCacheMiss(ModuleName)
	log.Infof("Cache miss for contact search: %s, scraping...", searchTerm)

	// Build search variants (e.g., "資工系" -> also try "資訊工程")
	searchVariants := h.buildSearchVariants(searchTerm)

	var contactsPtr []*storage.Contact
	for _, variant := range searchVariants {
		log.Debugf("Trying search variant: %s", variant)
		result, err := ntpu.ScrapeContacts(ctx, h.scraper, variant)
		if err != nil {
			log.WithError(err).Debugf("Failed to scrape contacts for variant: %s", variant)
			continue
		}
		if len(result) > 0 {
			contactsPtr = result
			break
		}
	}

	if len(contactsPtr) == 0 {
		// Final attempt with original search term
		result, err := ntpu.ScrapeContacts(ctx, h.scraper, searchTerm)
		if err != nil {
			log.WithError(err).Errorf("Failed to scrape contacts for: %s", searchTerm)
			h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
			msg := lineutil.ErrorMessageWithDetailAndSender("無法取得聯絡資料，可能是網路問題或資料來源暫時無法使用", sender)
			if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
				textMsg.QuickReply = lineutil.NewQuickReply(append(
					lineutil.QuickReplyErrorRecovery("聯絡 "+searchTerm),
					lineutil.QuickReplyEmergencyAction(),
				))
			}
			return []messaging_api.MessageInterface{msg}
		}
		contactsPtr = result
	}

	// Convert []*storage.Contact to []storage.Contact
	contacts = make([]storage.Contact, len(contactsPtr))
	for i, c := range contactsPtr {
		contacts[i] = *c
	}

	if len(contacts) == 0 {
		h.metrics.RecordScraperRequest(ModuleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf(
			"🔍 查無「%s」的聯絡資料\n\n💡 建議\n• 確認關鍵字拼寫是否正確\n• 嘗試使用單位全名或簡稱\n• 若查詢人名，可嘗試只輸入姓氏",
			searchTerm,
		), sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyContactNav())
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache
	for i := range contacts {
		if err := h.db.SaveContact(ctx, &contacts[i]); err != nil {
			log.WithError(err).Warnf("Failed to save contact to cache: %s", contacts[i].Name)
		}
	}

	h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
	return h.formatContactResultsWithSearch(contacts, searchTerm)
}

// handleMembersQuery handles queries for organization members
// Uses cache first, falls back to scraping if not found
// Returns all individuals belonging to the specified organization
func (h *Handler) handleMembersQuery(ctx context.Context, orgName string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(ModuleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.Infof("Handling members query for organization: %s", orgName)

	// Step 1: Search cache for members of this organization
	// Use GetContactsByOrganization for organization-specific queries
	members, err := h.db.GetContactsByOrganization(ctx, orgName)
	if err != nil {
		log.WithError(err).Error("Failed to query organization members from cache")
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetailAndSender("查詢成員時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyContactNav())
		}
		return []messaging_api.MessageInterface{msg}
	}

	// Filter to only individuals (exclude the organization itself)
	var individuals []storage.Contact
	for _, c := range members {
		if c.Type == "individual" {
			individuals = append(individuals, c)
		}
	}

	if len(individuals) > 0 {
		h.metrics.RecordCacheHit(ModuleName)
		log.Infof("Found %d members in cache for organization: %s", len(individuals), orgName)
		return h.formatContactResults(individuals)
	}

	// Step 2: Cache miss - try scraping
	h.metrics.RecordCacheMiss(ModuleName)
	log.Infof("Cache miss for organization members: %s, scraping...", orgName)

	scrapedContacts, err := ntpu.ScrapeContacts(ctx, h.scraper, orgName)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape members for: %s", orgName)
		h.metrics.RecordScraperRequest(ModuleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 無法取得「%s」的成員資料\n\n💡 可能原因：\n• 網路問題\n• 該單位尚無成員資料", orgName),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(append(
			lineutil.QuickReplyErrorRecovery("聯絡 "+orgName),
			lineutil.QuickReplyEmergencyAction(),
		))
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache and filter individuals
	individuals = make([]storage.Contact, 0)
	for _, c := range scrapedContacts {
		if err := h.db.SaveContact(ctx, c); err != nil {
			log.WithError(err).Warnf("Failed to save contact to cache: %s", c.Name)
		}
		// Check if this contact belongs to the target organization and is an individual
		if c.Type == "individual" && (c.Organization == orgName || c.Superior == orgName) {
			individuals = append(individuals, *c)
		}
	}

	if len(individuals) == 0 {
		h.metrics.RecordScraperRequest(ModuleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無「%s」的成員資料\n\n💡 該單位可能尚未建立成員資訊", orgName),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyContactNav())
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordScraperRequest(ModuleName, "success", time.Since(startTime).Seconds())
	return h.formatContactResults(individuals)
}

// formatContactResults formats contact results as LINE messages
func (h *Handler) formatContactResults(contacts []storage.Contact) []messaging_api.MessageInterface {
	return h.formatContactResultsWithSearch(contacts, "")
}

// formatContactResultsWithSearch formats contact results as LINE messages with search term for sorting
func (h *Handler) formatContactResultsWithSearch(contacts []storage.Contact, searchTerm string) []messaging_api.MessageInterface {
	if len(contacts) == 0 {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		msg := lineutil.NewTextMessageWithConsistentSender("🔍 查無聯絡資料", sender)
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyContactNav())
		return []messaging_api.MessageInterface{msg}
	}

	// Sort contacts based on type:
	// - Organizations: by hierarchy level (top-level units first, i.e., units without superior field come first)
	// - Individuals: by match count (descending), then name, title, organization
	slices.SortStableFunc(contacts, func(a, b storage.Contact) int {
		// Organization comes before individual
		aIsOrg := a.Type == "organization"
		bIsOrg := b.Type == "organization"
		if aIsOrg && !bIsOrg {
			return -1
		}
		if !aIsOrg && bIsOrg {
			return 1
		}

		// Both are organizations: sort by hierarchy level (superior units first)
		// Units without superior come first (top-level), then units with superior
		if aIsOrg && bIsOrg {
			aHasSuperior := a.Superior != ""
			bHasSuperior := b.Superior != ""
			if !aHasSuperior && bHasSuperior {
				return -1
			}
			if aHasSuperior && !bHasSuperior {
				return 1
			}
			// Same level: sort by name
			if a.Name < b.Name {
				return -1
			}
			if a.Name > b.Name {
				return 1
			}
			return 0
		}

		// Both are individuals: sort by match count, then name, title, organization
		if searchTerm != "" {
			aMatchCount := countMatchRunes(a, searchTerm)
			bMatchCount := countMatchRunes(b, searchTerm)
			if aMatchCount != bMatchCount {
				return bMatchCount - aMatchCount // Higher match count first
			}
		}

		// Same match count or no search term: sort by name, then title, then organization
		if a.Name != b.Name {
			if a.Name < b.Name {
				return -1
			}
			return 1
		}
		if a.Title != b.Title {
			if a.Title < b.Title {
				return -1
			}
			return 1
		}
		if a.Organization < b.Organization {
			return -1
		}
		if a.Organization > b.Organization {
			return 1
		}
		return 0
	})

	sender := lineutil.GetSender(senderName, h.stickerManager)
	var messages []messaging_api.MessageInterface

	// Track if we hit the limit (likely more results available) - warning added at end
	truncated := h.maxContactsLimit > 0 && len(contacts) >= h.maxContactsLimit

	// Reserve 1 message slot for warning if truncated (LINE API: max 5 messages)
	maxMessages := 5
	if truncated {
		maxMessages = 4
	}

	for i := 0; i < len(contacts); i += lineutil.MaxBubblesPerCarousel {
		// Limit to maxMessages (LINE reply limit, minus 1 if truncated for warning)
		if len(messages) >= maxMessages {
			break
		}

		end := i + lineutil.MaxBubblesPerCarousel
		if end > len(contacts) {
			end = len(contacts)
		}

		displayContacts := contacts[i:end]
		var bubbles []messaging_api.FlexBubble

		for _, c := range displayContacts {
			// Format display name: if Chinese == English, show Chinese only
			// Otherwise show "ChineseName EnglishName"
			displayName := lineutil.FormatDisplayName(c.Name, c.NameEn)

			// Determine header/body label based on type
			var bodyLabel lineutil.BodyLabelInfo

			if c.Type == "organization" {
				bodyLabel = lineutil.BodyLabelInfo{
					Emoji: "🏢",
					Label: "組織",
					Color: lineutil.ColorHeaderOrg,
				}
			} else {
				bodyLabel = lineutil.BodyLabelInfo{
					Emoji: "👤",
					Label: "個人",
					Color: lineutil.ColorHeaderIndividual,
				}
			}

			// Header: Colored header with name (Consistent with Course module)
			header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
				Title: displayName,
				Color: bodyLabel.Color,
			})

			// Body: Details using BodyContentBuilder for cleaner code
			body := lineutil.NewBodyContentBuilder()

			// Add type label as first row
			body.AddComponent(lineutil.NewBodyLabel(bodyLabel).FlexBox)

			// Add Title if available (previously in Hero subtitle) - use shrink-to-fit for variable length
			if c.Title != "" && c.Type != "organization" {
				titleRow := lineutil.NewInfoRow("🔖", "職稱", c.Title, lineutil.CarouselInfoRowStyle())
				body.AddComponent(titleRow.FlexBox)
			}

			// Organization / Superior - use shrink-to-fit for variable length org names
			if c.Type == "organization" && c.Superior != "" {
				body.AddInfoRow("🏢", "上級單位", c.Superior, lineutil.CarouselInfoRowStyle())
			} else if c.Organization != "" {
				body.AddInfoRow("🏢", "所屬單位", c.Organization, lineutil.CarouselInfoRowStyle())
			}

			// Contact Info - Display full phone OR just extension (important, keep bold)
			if c.Phone != "" {
				body.AddInfoRow("📞", "聯絡電話", c.Phone, lineutil.BoldInfoRowStyle())
			} else if c.Extension != "" {
				body.AddInfoRow("☎️", "分機號碼", c.Extension, lineutil.BoldInfoRowStyle())
			}

			// Contact Info - Location and Email (variable length, use shrink-to-fit)
			body.AddInfoRowIf("📍", "辦公位置", c.Location, lineutil.CarouselInfoRowStyle())
			body.AddInfoRowIf("✉️", "電子郵件", c.Email, lineutil.CarouselInfoRowStyle())

			// Add cache time hint (unobtrusive, right-aligned)
			if hint := lineutil.NewCacheTimeHint(c.CachedAt); hint != nil {
				body.AddComponent(hint.FlexText)
			}

			// Footer: Multi-row button layout for optimal UX
			// Row 1: Phone actions (call, copy)
			// Row 2: Email actions (send, copy)
			// Row 3: Website (if available)
			var row1Buttons []*lineutil.FlexButton
			var row2Buttons []*lineutil.FlexButton
			var row3Buttons []*lineutil.FlexButton

			// Row 1: Phone-related buttons
			if c.Phone != "" {
				// Parse phone number - may be "mainPhone,extension" format or standalone
				var telURI string
				if strings.Contains(c.Phone, ",") {
					// Format: "0286741111,67114" - parse to extract components
					parts := strings.SplitN(c.Phone, ",", 2)
					telURI = lineutil.BuildTelURI(parts[0], parts[1])
				} else {
					// Standalone phone number
					telURI = lineutil.BuildTelURI(c.Phone, "")
				}
				row1Buttons = append(row1Buttons,
					lineutil.NewFlexButton(lineutil.NewURIAction("📞 撥打電話", telURI)).WithStyle("primary").WithColor(lineutil.ColorButtonAction).WithHeight("sm"))
				row1Buttons = append(row1Buttons,
					lineutil.NewFlexButton(lineutil.NewClipboardAction("📋 複製號碼", c.Phone)).WithStyle("secondary").WithHeight("sm"))
			} else if c.Extension != "" {
				// Only short extension (< 5 digits), can still dial via main + extension
				telURI := lineutil.BuildTelURI(sanxiaNormalPhone, c.Extension)
				row1Buttons = append(row1Buttons,
					lineutil.NewFlexButton(lineutil.NewURIAction("📞 撥打電話", telURI)).WithStyle("primary").WithColor(lineutil.ColorButtonAction).WithHeight("sm"))
				row1Buttons = append(row1Buttons,
					lineutil.NewFlexButton(lineutil.NewClipboardAction("📋 複製分機", c.Extension)).WithStyle("secondary").WithHeight("sm"))
			}

			// Row 2: Email actions
			if c.Email != "" {
				row2Buttons = append(row2Buttons,
					lineutil.NewFlexButton(lineutil.NewURIAction("✉️ 寄送郵件", "mailto:"+c.Email)).WithStyle("primary").WithColor(lineutil.ColorButtonAction).WithHeight("sm"))
				row2Buttons = append(row2Buttons,
					lineutil.NewFlexButton(lineutil.NewClipboardAction("📋 複製信箱", c.Email)).WithStyle("secondary").WithHeight("sm"))
			}

			// Row 3: Website button (standalone row for visibility) (外部連結使用藍色)
			if c.Website != "" {
				row3Buttons = append(row3Buttons,
					lineutil.NewFlexButton(lineutil.NewURIAction("🌐 開啟網站", c.Website)).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm"))
			}

			// Row 4: View Members button for organizations (separate row for better UX)
			// Allows querying all members belonging to this organization
			// Button color syncs with header for visual harmony
			var row4Buttons []*lineutil.FlexButton
			if c.Type == "organization" {
				displayText := lineutil.FormatLabel("查詢成員", c.Name, 40)
				row4Buttons = append(row4Buttons,
					lineutil.NewFlexButton(
						lineutil.NewPostbackActionWithDisplayText("👥 查看成員", displayText, fmt.Sprintf("contact:members%s%s", bot.PostbackSplitChar, c.Name)),
					).WithStyle("primary").WithColor(bodyLabel.Color).WithHeight("sm"))
			}

			// Assemble Bubble
			bubble := lineutil.NewFlexBubble(
				header,
				nil,          // Hero (replaced by Colored Header)
				body.Build(), // Body
				nil,          // Footer (handled below)
			)

			// Build footer with multi-row button layout
			if len(row1Buttons) > 0 || len(row2Buttons) > 0 || len(row3Buttons) > 0 || len(row4Buttons) > 0 {
				bubble.Footer = lineutil.NewButtonFooter(row1Buttons, row2Buttons, row3Buttons, row4Buttons).FlexBox
			}

			bubbles = append(bubbles, *bubble.FlexBubble)
		}

		carousel := lineutil.NewFlexCarousel(bubbles)

		altText := "聯絡資訊搜尋結果"
		if i > 0 {
			altText += fmt.Sprintf(" (%d-%d)", i+1, end)
		}

		msg := lineutil.NewFlexMessage(altText, carousel)
		msg.Sender = sender
		messages = append(messages, msg)
	}

	// Append warning message at the end if results were truncated
	if truncated {
		warningMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("⚠️ 搜尋結果達到上限 %d 筆\n\n可能有更多結果未顯示，建議使用更精確的關鍵字搜尋", h.maxContactsLimit),
			sender,
		)
		messages = append(messages, warningMsg)
	}

	// Add Quick Reply to the last message
	lineutil.AddQuickReplyToMessages(messages,
		lineutil.QuickReplyEmergencyAction(),
		lineutil.QuickReplyContactAction(),
	)

	return messages
}

// buildSearchVariants generates search variants for better matching
// Maps common abbreviations to full names that the school website understands
// Priority: Full name first (more likely to match), then abbreviations
func (h *Handler) buildSearchVariants(searchTerm string) []string {
	// Common department abbreviation mappings - prioritize full names first
	abbreviationMap := map[string][]string{
		// 電機資訊學院
		"資工":  {"資訊工程學系", "資訊工程", "資工系"},
		"資工系": {"資訊工程學系", "資訊工程"},
		"電機":  {"電機工程學系", "電機工程", "電機系"},
		"電機系": {"電機工程學系", "電機工程"},
		"通訊":  {"通訊工程學系", "通訊工程", "通訊系"},
		"通訊系": {"通訊工程學系", "通訊工程"},
		// 商學院
		"企管":  {"企業管理學系", "企業管理", "企管系"},
		"企管系": {"企業管理學系", "企業管理"},
		"會計":  {"會計學系", "會計系"},
		"會計系": {"會計學系"},
		"統計":  {"統計學系", "統計系"},
		"統計系": {"統計學系"},
		"金融":  {"金融與合作經營學系", "金融系"},
		"金融系": {"金融與合作經營學系"},
		"休運":  {"休閒運動管理學系", "休運系"},
		"休運系": {"休閒運動管理學系"},
		// 社會科學學院
		"經濟":  {"經濟學系", "經濟系"},
		"經濟系": {"經濟學系"},
		"社工":  {"社會工作學系", "社工系"},
		"社工系": {"社會工作學系"},
		"社學":  {"社會學系", "社學系"},
		"社學系": {"社會學系"},
		// 法律學院
		"法律":  {"法律學系", "法律系"},
		"法律系": {"法律學系"},
		// 公共事務學院
		"公行":  {"公共行政暨政策學系", "公共行政", "公行系"},
		"公行系": {"公共行政暨政策學系", "公共行政"},
		"財政":  {"財政學系", "財政系"},
		"財政系": {"財政學系"},
		"不動產": {"不動產與城鄉環境學系", "不動"},
		"不動":  {"不動產與城鄉環境學系"},
		// 人文學院
		"中文":  {"中國文學系", "中文系"},
		"中文系": {"中國文學系"},
		"應外":  {"應用外語學系", "應外系"},
		"應外系": {"應用外語學系"},
		"歷史":  {"歷史學系", "歷史系"},
		"歷史系": {"歷史學系"},
		// 行政單位
		"圖書館": {"圖書館", "圖書"},
		"學務處": {"學務處", "學務"},
		"教務處": {"教務處", "教務"},
		"總務處": {"總務處", "總務"},
		"研發處": {"研發處", "研究發展"},
		"人事室": {"人事室", "人事"},
		"註冊組": {"註冊組", "註冊"},
	}

	variants := []string{}

	// Check if search term matches any abbreviation
	if mappedVariants, ok := abbreviationMap[searchTerm]; ok {
		variants = append(variants, mappedVariants...)
	}

	// Also add the original term with/without "系" suffix
	if strings.HasSuffix(searchTerm, "系") {
		// Remove "系" suffix and add variants
		base := strings.TrimSuffix(searchTerm, "系")
		variants = append(variants, base)
	} else {
		// Add "系" suffix variant
		variants = append(variants, searchTerm+"系")
	}

	// Deduplicate variants while preserving order
	seen := make(map[string]bool)
	uniqueVariants := []string{}
	for _, v := range variants {
		if !seen[v] {
			seen[v] = true
			uniqueVariants = append(uniqueVariants, v)
		}
	}
	return uniqueVariants
}

// countMatchRunes counts how many runes from searchTerm appear in the contact's fields.
// Used for sorting individuals by relevance - higher match count = more relevant.
// Fields checked: name, title, organization, superior
func countMatchRunes(c storage.Contact, searchTerm string) int {
	if searchTerm == "" {
		return 0
	}

	count := 0
	searchLower := strings.ToLower(searchTerm)

	// Build a combined string of all searchable fields
	combined := strings.ToLower(c.Name + c.Title + c.Organization + c.Superior)
	combinedRunes := make(map[rune]struct{})
	for _, r := range combined {
		combinedRunes[r] = struct{}{}
	}

	// Count how many search runes exist in combined fields
	for _, r := range searchLower {
		if _, exists := combinedRunes[r]; exists {
			count++
		}
	}

	return count
}
