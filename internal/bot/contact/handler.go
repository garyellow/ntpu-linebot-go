package contact

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles contact-related queries
type Handler struct {
	db             *storage.DB
	scraper        *scraper.Client
	metrics        *metrics.Metrics
	logger         *logger.Logger
	stickerManager *sticker.Manager
}

const (
	moduleName = "contact"
	splitChar  = "$"
	senderName = "聯繫魔法師"

	// Emergency phone numbers (without hyphens for clipboard copy)
	// 三峽校區
	sanxiaNormalPhone    = "0286741111" // 總機
	sanxia24HPhone       = "0226731949" // 24H緊急行政電話
	sanxiaEmergencyPhone = "0226711234" // 24H急難救助電話（校安中心）
	sanxiaGatePhone      = "0226733920" // 大門哨所
	sanxiaDormPhone      = "0286716784" // 宿舍夜間緊急電話

	// 臺北校區
	taipeiNormalPhone    = "0225024654" // 總機
	taipeiEmergencyPhone = "0225023671" // 24H急難救助電話

	// 其他常用電話
	policePhone   = "110"        // 警察局24H緊急救助
	firePhone     = "119"        // 消防局(含救護車)24H緊急救助
	policeStation = "0226730561" // 北大派出所
	homHospital   = "0226723456" // 恩主公醫院
)

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

	contactRegex = buildRegex(validContactKeywords)
)

// buildRegex creates a regex pattern from keywords
// Sorts keywords by length (longest first) to ensure correct regex alternation matching
func buildRegex(keywords []string) *regexp.Regexp {
	// Create a copy to avoid modifying the original slice
	sortedKeywords := make([]string, len(keywords))
	copy(sortedKeywords, keywords)

	// Sort by length in descending order (longest first)
	sort.Slice(sortedKeywords, func(i, j int) bool {
		return len(sortedKeywords[i]) > len(sortedKeywords[j])
	})

	pattern := "(?i)" + strings.Join(sortedKeywords, "|")
	return regexp.MustCompile(pattern)
}

// NewHandler creates a new contact handler
func NewHandler(db *storage.DB, scraper *scraper.Client, metrics *metrics.Metrics, logger *logger.Logger, stickerManager *sticker.Manager) *Handler {
	return &Handler{
		db:             db,
		scraper:        scraper,
		metrics:        metrics,
		logger:         logger,
		stickerManager: stickerManager,
	}
}

// CanHandle checks if the message is for the contact module
func (h *Handler) CanHandle(text string) bool {
	text = strings.TrimSpace(text)

	// Check for emergency keyword (must be at start)
	if strings.HasPrefix(text, "緊急") {
		return true
	}

	// Check for contact keywords (includes 電話, 分機, email, 信箱, etc.)
	if contactRegex.MatchString(text) {
		return true
	}

	return false
}

// HandleMessage handles text messages for the contact module
func (h *Handler) HandleMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	text = strings.TrimSpace(text)

	log.Infof("Handling contact message: %s", text)

	// Handle emergency phone request
	if strings.HasPrefix(text, "緊急") {
		return h.handleEmergencyPhones()
	}

	// Handle contact search - extract search term after keyword
	if match := contactRegex.FindString(text); match != "" {
		// Determine if keyword is at the beginning or end
		var searchTerm string
		if strings.HasPrefix(text, match) {
			// Keyword at beginning: "聯絡 資工系" -> extract after
			searchTerm = strings.TrimSpace(strings.TrimPrefix(text, match))
		} else if strings.HasSuffix(text, match) {
			// Keyword at end: "資工系聯絡" -> extract before
			searchTerm = strings.TrimSpace(strings.TrimSuffix(text, match))
		} else {
			// Keyword in middle: remove it and use the rest
			searchTerm = strings.TrimSpace(strings.Replace(text, match, "", 1))
		}

		if searchTerm == "" {
			// If no search term provided, give helpful message
			sender := lineutil.GetSender(senderName, h.stickerManager)
			msg := lineutil.NewTextMessageWithConsistentSender("📞 請輸入查詢內容\n\n例如：\n• 聯絡 資工系\n• 電話 圖書館\n• 分機 學務處\n\n💡 也可直接輸入「緊急」查看緊急聯絡電話", sender)
			msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("🚨 緊急電話", "緊急")},
				{Action: lineutil.NewMessageAction("📖 使用說明", "使用說明")},
			})
			return []messaging_api.MessageInterface{msg}
		}
		return h.handleContactSearch(ctx, searchTerm)
	}

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the contact module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	log.Infof("Handling contact postback: %s", data)

	// Handle "查看更多" postback (with or without prefix)
	if strings.HasPrefix(data, "查看更多") {
		parts := strings.Split(data, splitChar)
		if len(parts) >= 2 {
			name := parts[1]
			return h.handleContactSearch(ctx, name)
		}
	}

	// Handle "查看資訊" postback (with or without prefix)
	if strings.HasPrefix(data, "查看資訊") {
		parts := strings.Split(data, splitChar)
		if len(parts) >= 2 {
			name := parts[1]
			return h.handleContactSearch(ctx, name)
		}
	}

	// Handle "members" postback for viewing organization members
	// Format: "contact:members${splitChar}{orgName}"
	if strings.HasPrefix(data, "members") {
		parts := strings.Split(data, splitChar)
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
		return lineutil.NewFlexBox("baseline",
			lineutil.NewFlexText(labelWithIcon).WithColor(lineutil.ColorLabel).WithSize("sm").WithFlex(3).FlexText,
			lineutil.NewFlexText(value).WithWrap(true).WithColor(valColor).WithSize("sm").WithWeight("bold").WithFlex(4).WithAlign("end").FlexText,
		).FlexBox
	}

	// Header - using standardized component (with emergency red color variant)
	header := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexBox("baseline",
			lineutil.NewFlexText("🚨").WithSize("lg").FlexText,
			lineutil.NewFlexText("緊急聯絡電話").WithWeight("bold").WithColor(lineutil.ColorDanger).WithSize("sm").WithMargin("sm").FlexText,
		).FlexBox,
	)

	// Sanxia Campus Box
	sanxiaBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📍 三峽校區").WithWeight("bold").WithSize("md").WithColor(lineutil.ColorPrimary).WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("📞", "總機", sanxiaNormalPhone, ""),
		createRow("🏢", "24H行政", sanxia24HPhone, ""),
		createRow("🚨", "24H校安", sanxiaEmergencyPhone, lineutil.ColorDanger), // Highlight emergency
		createRow("🚪", "大門哨所", sanxiaGatePhone, ""),
		createRow("🏠", "宿舍夜間", sanxiaDormPhone, ""),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// Taipei Campus Box
	taipeiBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📍 台北校區").WithWeight("bold").WithSize("md").WithColor(lineutil.ColorPrimary).WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("📞", "總機", taipeiNormalPhone, ""),
		createRow("🚨", "24H校安", taipeiEmergencyPhone, lineutil.ColorDanger),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// External Emergency Box
	externalBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("🚨 校外緊急").WithWeight("bold").WithSize("md").WithColor(lineutil.ColorDanger).WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("👮", "警察局", "110", lineutil.ColorDanger),
		createRow("🚒", "消防/救護", "119", lineutil.ColorDanger),
		createRow("🏢", "北大派出所", policeStation, ""),
		createRow("🏥", "恩主公醫院", homHospital, ""),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// Footer: Quick Action Buttons
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(lineutil.NewURIAction("🚨 撥打三峽校安", "tel:"+sanxiaEmergencyPhone)).WithStyle("primary").WithColor(lineutil.ColorDanger).WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("🚨 撥打台北校安", "tel:"+taipeiEmergencyPhone)).WithStyle("primary").WithColor(lineutil.ColorDanger).WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("ℹ️ 查看更多", "https://new.ntpu.edu.tw/safety")).WithStyle("secondary").WithHeight("sm").FlexButton,
	).WithSpacing("sm")

	bubble := lineutil.NewFlexBubble(
		header,
		nil,
		lineutil.NewFlexBox("vertical",
			sanxiaBox,
			taipeiBox,
			externalBox,
		),
		footer,
	)

	sender := lineutil.GetSender(senderName, h.stickerManager)
	msg := lineutil.NewFlexMessage("緊急聯絡電話", bubble.FlexBubble)
	msg.Sender = sender

	return []messaging_api.MessageInterface{msg}
}

// handleContactSearch handles contact search queries with a multi-tier search strategy:
//
// Search Strategy (3-tier cascade):
//
//  1. SQL LIKE (fast path): Direct database LIKE query for exact substrings.
//     Example: "資工" matches "資訊工程學系" via SQL LIKE '%資工%'
//
//  2. Fuzzy character-set matching (cache fallback): If SQL LIKE returns no results,
//     loads all cached contacts and checks if all runes in searchTerm exist in target.
//     Example: "資工系" matches "資訊工程學系" because all chars (資,工,系) exist in target
//     This enables abbreviation matching where chars are scattered in the full name.
//
//  3. Web scraping (external fallback): If cache has no results,
//     scrape from NTPU website. The website natively supports partial name matching,
//     so we simply pass the original search term directly.
//
// Performance notes:
//   - SQL LIKE is indexed and fast; most queries resolve here
//   - Fuzzy matching loads up to 1000 contacts; acceptable since it's a fallback
func (h *Handler) handleContactSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	var contacts []storage.Contact

	// Step 1: Try SQL LIKE search first (fast path for exact substrings)
	sqlContacts, err := h.db.SearchContactsByName(searchTerm)
	if err != nil {
		log.WithError(err).Error("Failed to search contacts in cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetailAndSender("查詢聯絡資訊時發生問題", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重試", "聯絡 "+searchTerm)},
				{Action: lineutil.NewMessageAction("緊急電話", "緊急")},
			})
		}
		return []messaging_api.MessageInterface{msg}
	}
	contacts = sqlContacts

	// Step 2: If SQL LIKE didn't find results, try fuzzy character-set matching
	// This enables "資工系" to match "資訊工程學系" by checking if all characters exist
	if len(contacts) == 0 {
		allContacts, err := h.db.GetAllContacts()
		if err == nil && len(allContacts) > 0 {
			for _, c := range allContacts {
				// Fuzzy character-set matching: check if all runes in searchTerm exist in target
				if lineutil.ContainsAllRunes(c.Name, searchTerm) ||
					lineutil.ContainsAllRunes(c.Organization, searchTerm) ||
					lineutil.ContainsAllRunes(c.Superior, searchTerm) {
					contacts = append(contacts, c)
				}
			}
		}
	}

	// If found in cache, return results
	if len(contacts) > 0 {
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Cache hit for contact search: %s (found %d)", searchTerm, len(contacts))
		return h.formatContactResults(contacts)
	}

	// Cache miss - scrape from website
	// NTPU website natively supports partial name matching, so we use the original term directly
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("Cache miss for contact search: %s, scraping...", searchTerm)

	contactsPtr, err := ntpu.ScrapeContacts(ctx, h.scraper, searchTerm)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape contacts for: %s", searchTerm)
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetailAndSender("無法取得聯絡資料，可能是網路問題或資料來源暫時無法使用", sender)
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("緊急電話", "緊急")},
				{Action: lineutil.NewMessageAction("使用說明", "使用說明")},
			})
		}
		return []messaging_api.MessageInterface{msg}
	}

	// Convert []*storage.Contact to []storage.Contact
	contacts = make([]storage.Contact, len(contactsPtr))
	for i, c := range contactsPtr {
		contacts[i] = *c
	}

	if len(contacts) == 0 {
		h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(fmt.Sprintf(
			"🔍 查無包含「%s」的聯絡資料\n\n建議：\n• 確認關鍵字拼寫是否正確\n• 嘗試使用單位全名或簡稱\n• 若查詢人名，可嘗試只輸入姓氏",
			searchTerm,
		), sender)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("重新搜尋", "聯絡")},
			{Action: lineutil.NewMessageAction("緊急電話", "緊急")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache
	for i := range contacts {
		if err := h.db.SaveContact(&contacts[i]); err != nil {
			log.WithError(err).Warnf("Failed to save contact to cache: %s", contacts[i].Name)
		}
	}

	h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
	return h.formatContactResults(contacts)
}

// handleMembersQuery handles queries for organization members
// Uses cache first, falls back to scraping if not found
// Returns all individuals belonging to the specified organization
func (h *Handler) handleMembersQuery(ctx context.Context, orgName string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()
	sender := lineutil.GetSender(senderName, h.stickerManager)

	log.Infof("Handling members query for organization: %s", orgName)

	// Step 1: Search cache for members of this organization
	members, err := h.db.GetContactsByOrganization(orgName)
	if err != nil {
		log.WithError(err).Error("Failed to query organization members from cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetailAndSender("查詢成員時發生問題", sender)
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
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Found %d members in cache for organization: %s", len(individuals), orgName)
		return h.formatContactResults(individuals)
	}

	// Step 2: Cache miss - try scraping
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("Cache miss for organization members: %s, scraping...", orgName)

	scrapedContacts, err := ntpu.ScrapeContacts(ctx, h.scraper, orgName)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape members for: %s", orgName)
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 無法取得「%s」的成員資料\n\n💡 可能原因：\n• 網路問題\n• 該單位尚無成員資料", orgName),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("🔄 重試", "聯絡 "+orgName)},
			{Action: lineutil.NewMessageAction("🚨 緊急電話", "緊急")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	// Save to cache and filter individuals
	individuals = make([]storage.Contact, 0)
	for _, c := range scrapedContacts {
		if err := h.db.SaveContact(c); err != nil {
			log.WithError(err).Warnf("Failed to save contact to cache: %s", c.Name)
		}
		// Check if this contact belongs to the target organization and is an individual
		if c.Type == "individual" && (c.Organization == orgName || c.Superior == orgName) {
			individuals = append(individuals, *c)
		}
	}

	if len(individuals) == 0 {
		h.metrics.RecordScraperRequest(moduleName, "not_found", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("🔍 查無「%s」的成員資料\n\n💡 該單位可能尚未建立成員資訊", orgName),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
			{Action: lineutil.NewMessageAction("🔍 重新搜尋", "聯絡")},
			{Action: lineutil.NewMessageAction("🚨 緊急電話", "緊急")},
		})
		return []messaging_api.MessageInterface{msg}
	}

	h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
	return h.formatContactResults(individuals)
}

// formatContactResults formats contact results as LINE messages
func (h *Handler) formatContactResults(contacts []storage.Contact) []messaging_api.MessageInterface {
	if len(contacts) == 0 {
		sender := lineutil.GetSender(senderName, h.stickerManager)
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithConsistentSender("🔍 查無聯絡資料", sender),
		}
	}

	// Build a set of organization names that appear as Superior of other contacts
	// These "parent" organizations should be sorted first
	parentOrgNames := make(map[string]bool)
	for _, c := range contacts {
		if c.Superior != "" {
			parentOrgNames[c.Superior] = true
		}
	}

	// Sort contacts: parent organizations first, then other organizations, then individuals
	// Within each group, sort alphabetically by name
	sort.SliceStable(contacts, func(i, j int) bool {
		// Priority: parent org (3) > org (2) > individual (1)
		getPriority := func(c storage.Contact) int {
			if c.Type == "organization" {
				if parentOrgNames[c.Name] {
					return 3 // Parent organization
				}
				return 2 // Regular organization
			}
			return 1 // Individual
		}

		pi, pj := getPriority(contacts[i]), getPriority(contacts[j])
		if pi != pj {
			return pi > pj // Higher priority first
		}
		// Same priority: sort by name
		return contacts[i].Name < contacts[j].Name
	})

	sender := lineutil.GetSender(senderName, h.stickerManager)
	var messages []messaging_api.MessageInterface
	chunkSize := 10 // LINE API limit: max 10 bubbles per Flex Carousel

	for i := 0; i < len(contacts); i += chunkSize {
		// Limit to 5 messages (LINE reply limit)
		if len(messages) >= 5 {
			break
		}

		end := i + chunkSize
		if end > len(contacts) {
			end = len(contacts)
		}

		displayContacts := contacts[i:end]
		var bubbles []messaging_api.FlexBubble

		for _, c := range displayContacts {
			// Format display name: if Chinese == English, show Chinese only
			// Otherwise show "ChineseName EnglishName"
			displayName := lineutil.FormatDisplayName(c.Name, c.NameEn)

			// Determine subtitle - show friendly text, fallback to empty if no meaningful info
			var subText string
			if c.Type == "organization" {
				subText = "單位"
			} else if c.Title != "" {
				subText = c.Title
			}
			// If c.Type is "individual" with no title, subText remains empty
			// NewHeroBox will handle empty subtitle gracefully

			// Header: Contact badge (using standardized component)
			header := lineutil.NewHeaderBadge("📞", "聯絡資訊")

			// Hero: Name with colored background (using standardized component)
			hero := lineutil.NewHeroBox(displayName, subText)

			// Body: Details - avoid duplicating phone/extension info
			var bodyContents []messaging_api.FlexComponentInterface

			// Organization / Superior - use vertical layout
			if c.Type == "organization" && c.Superior != "" {
				bodyContents = append(bodyContents,
					lineutil.NewInfoRowWithMargin("🏢", "上級單位", c.Superior, lineutil.DefaultInfoRowStyle(), "lg"))
			} else if c.Organization != "" {
				bodyContents = append(bodyContents,
					lineutil.NewInfoRowWithMargin("🏢", "所屬單位", c.Organization, lineutil.DefaultInfoRowStyle(), "lg"))
			}

			// Contact Info - Display full phone (main+extension) OR just extension
			// This prevents duplicate display of extension in both body and footer
			if c.Phone != "" {
				// Show full phone number (e.g., "0286741111,12345")
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents,
					lineutil.NewInfoRowWithMargin("📞", "聯絡電話", c.Phone, lineutil.BoldInfoRowStyle(), "md"))
			} else if c.Extension != "" {
				// Only extension available (no full phone)
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents,
					lineutil.NewInfoRowWithMargin("☎️", "分機號碼", c.Extension, lineutil.BoldInfoRowStyle(), "md"))
			}

			// Contact Info - Location
			if c.Location != "" {
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents,
					lineutil.NewInfoRowWithMargin("📍", "辦公位置", c.Location, lineutil.DefaultInfoRowStyle(), "md"))
			}

			// Contact Info - Email
			if c.Email != "" {
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents,
					lineutil.NewInfoRowWithMargin("✉️", "電子郵件", c.Email, lineutil.DefaultInfoRowStyle(), "md"))
			}

			// Footer: 2-row button layout
			// Organization: Row 1 = Website (secondary), Row 2 = View Members (primary)
			// Individual: Row 1 = Phone (primary + secondary), Row 2 = Email (primary + secondary)
			// Color scheme:
			//   - primary (green): Main action user wants to do (call, email, view members)
			//   - secondary (blue): Supporting action (copy, open website)
			var row1Buttons []*lineutil.FlexButton
			var row2Buttons []*lineutil.FlexButton

			if c.Type == "organization" {
				// Organization: Website + View Members (2 rows)
				if c.Website != "" {
					row1Buttons = append(row1Buttons,
						lineutil.NewFlexButton(lineutil.NewURIAction("🌐 開啟網站", c.Website)).WithStyle("secondary").WithHeight("sm"))
				}
				displayText := fmt.Sprintf("查詢「%s」的成員", lineutil.TruncateRunes(c.Name, 20))
				row2Buttons = append(row2Buttons,
					lineutil.NewFlexButton(
						lineutil.NewPostbackActionWithDisplayText("👥 查看成員", displayText, fmt.Sprintf("contact:members%s%s", splitChar, c.Name)),
					).WithStyle("primary").WithHeight("sm"))
			} else {
				// Individual: Phone + Email (2 rows)
				// Row 1: Phone - primary action is calling, secondary is copy
				if c.Phone != "" {
					var telURI string
					if strings.Contains(c.Phone, ",") {
						parts := strings.SplitN(c.Phone, ",", 2)
						telURI = lineutil.BuildTelURI(parts[0], parts[1])
					} else {
						telURI = lineutil.BuildTelURI(c.Phone, "")
					}
					row1Buttons = append(row1Buttons,
						lineutil.NewFlexButton(lineutil.NewURIAction("📞 撥打電話", telURI)).WithStyle("primary").WithHeight("sm"))
					row1Buttons = append(row1Buttons,
						lineutil.NewFlexButton(lineutil.NewClipboardAction("📋 複製號碼", c.Phone)).WithStyle("secondary").WithHeight("sm"))
				} else if c.Extension != "" {
					telURI := lineutil.BuildTelURI(sanxiaNormalPhone, c.Extension)
					row1Buttons = append(row1Buttons,
						lineutil.NewFlexButton(lineutil.NewURIAction("📞 撥打電話", telURI)).WithStyle("primary").WithHeight("sm"))
					row1Buttons = append(row1Buttons,
						lineutil.NewFlexButton(lineutil.NewClipboardAction("📋 複製分機", c.Extension)).WithStyle("secondary").WithHeight("sm"))
				}

				// Row 2: Email - primary action is sending, secondary is copy
				if c.Email != "" {
					row2Buttons = append(row2Buttons,
						lineutil.NewFlexButton(lineutil.NewURIAction("✉️ 寄送郵件", "mailto:"+c.Email)).WithStyle("primary").WithHeight("sm"))
					row2Buttons = append(row2Buttons,
						lineutil.NewFlexButton(lineutil.NewClipboardAction("📋 複製信箱", c.Email)).WithStyle("secondary").WithHeight("sm"))
				}
			}

			// Assemble Bubble
			bubble := lineutil.NewFlexBubble(
				header,
				hero.FlexBox,
				lineutil.NewFlexBox("vertical", bodyContents...).WithSpacing("sm"), // Body
				nil, // Footer (handled below)
			)

			// Build footer with multi-row button layout
			if len(row1Buttons) > 0 || len(row2Buttons) > 0 {
				bubble.Footer = lineutil.NewButtonFooter(row1Buttons, row2Buttons).FlexBox
			}

			bubbles = append(bubbles, *bubble.FlexBubble)
		}

		carousel := &messaging_api.FlexCarousel{
			Contents: bubbles,
		}

		altText := "聯絡資訊搜尋結果"
		if i > 0 {
			altText += fmt.Sprintf(" (%d-%d)", i+1, end)
		}

		msg := lineutil.NewFlexMessage(altText, carousel)
		msg.Sender = sender
		messages = append(messages, msg)
	}

	// Add Quick Reply to the last message
	if len(messages) > 0 {
		lastMsg := messages[len(messages)-1]
		if flexMsg, ok := lastMsg.(*messaging_api.FlexMessage); ok {
			flexMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("緊急電話", "緊急")},
				{Action: lineutil.NewMessageAction("查詢其他", "聯絡")},
			})
		}
	}

	return messages
}
