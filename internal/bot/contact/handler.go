package contact

import (
	"context"
	"fmt"
	"regexp"
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
func buildRegex(keywords []string) *regexp.Regexp {
	pattern := "(?i)" + strings.Join(keywords, "|")
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
		// Extract what comes after the keyword
		searchTerm := strings.TrimSpace(strings.Replace(text, match, "", 1))
		if searchTerm == "" {
			// If no search term provided, give helpful message
			return []messaging_api.MessageInterface{
				lineutil.NewTextMessageWithSender("請在關鍵字後輸入查詢內容\n\n例如：聯絡 資工系、電話 圖書館", senderName, h.stickerManager.GetRandomSticker()),
			}
		}
		return h.handleContactSearch(ctx, searchTerm)
	}

	// Handle phone/extension queries (fallback if not caught by regex)
	if strings.Contains(text, "電話") || strings.Contains(text, "分機") {
		// Extract the term (remove common keywords)
		searchTerm := text
		searchTerm = strings.ReplaceAll(searchTerm, "電話", "")
		searchTerm = strings.ReplaceAll(searchTerm, "分機", "")
		searchTerm = strings.TrimSpace(searchTerm)

		if searchTerm != "" {
			return h.handleContactSearch(ctx, searchTerm)
		}
	}

	return []messaging_api.MessageInterface{}
}

// HandlePostback handles postback events for the contact module
func (h *Handler) HandlePostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	log.Infof("Handling contact postback: %s", data)

	// Handle "查看更多" postback
	if strings.HasPrefix(data, "查看更多") {
		parts := strings.Split(data, splitChar)
		if len(parts) >= 2 {
			name := parts[1]
			return h.handleContactSearch(ctx, name)
		}
	}

	// Handle "查看資訊" postback
	if strings.HasPrefix(data, "查看資訊") {
		parts := strings.Split(data, splitChar)
		if len(parts) >= 2 {
			name := parts[1]
			return h.handleContactSearch(ctx, name)
		}
	}

	return []messaging_api.MessageInterface{}
}

// handleEmergencyPhones returns emergency phone numbers
func (h *Handler) handleEmergencyPhones() []messaging_api.MessageInterface {
	// Helper to create a row with icon and optional color
	createRow := func(icon, label, value, color string) messaging_api.FlexComponentInterface {
		valColor := "#666666"
		if color != "" {
			valColor = color
		}
		labelWithIcon := icon + " " + label
		return lineutil.NewFlexBox("baseline",
			lineutil.NewFlexText(labelWithIcon).WithColor("#888888").WithSize("sm").WithFlex(3).FlexText,
			lineutil.NewFlexText(value).WithWrap(true).WithColor(valColor).WithSize("sm").WithWeight("bold").WithFlex(4).WithAlign("end").FlexText,
		).FlexBox
	}

	// Header
	header := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexBox("baseline",
			lineutil.NewFlexText("🚨").WithSize("xl").FlexText,
			lineutil.NewFlexText("緊急聯絡電話").WithWeight("bold").WithSize("lg").WithColor("#ff3333").WithMargin("sm").FlexText,
		).FlexBox,
	)

	// Sanxia Campus Box
	sanxiaBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📍 三峽校區").WithWeight("bold").WithSize("md").WithColor("#1DB446").WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("📞", "總機", sanxiaNormalPhone, ""),
		createRow("🏢", "24H行政", sanxia24HPhone, ""),
		createRow("🚨", "24H校安", sanxiaEmergencyPhone, "#ff3333"), // Highlight emergency
		createRow("🚪", "大門哨所", sanxiaGatePhone, ""),
		createRow("🏠", "宿舍夜間", sanxiaDormPhone, ""),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// Taipei Campus Box
	taipeiBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("📍 台北校區").WithWeight("bold").WithSize("md").WithColor("#1DB446").WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("📞", "總機", taipeiNormalPhone, ""),
		createRow("🚨", "24H校安", taipeiEmergencyPhone, "#ff3333"),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// External Emergency Box
	externalBox := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexText("🚨 校外緊急").WithWeight("bold").WithSize("md").WithColor("#ff3333").WithMargin("lg").FlexText,
		lineutil.NewFlexSeparator().WithMargin("sm").FlexSeparator,
		createRow("👮", "警察局", "110", "#ff3333"),
		createRow("🚒", "消防/救護", "119", "#ff3333"),
		createRow("🏢", "北大派出所", policeStation, ""),
		createRow("🏥", "恩主公醫院", homHospital, ""),
	).WithSpacing("sm").WithMargin("sm").FlexBox

	// Footer: Quick Action Buttons
	footer := lineutil.NewFlexBox("vertical",
		lineutil.NewFlexButton(lineutil.NewURIAction("🚨 撥打三峽校安", "tel:"+sanxiaEmergencyPhone)).WithStyle("primary").WithColor("#ff3333").WithHeight("sm").FlexButton,
		lineutil.NewFlexButton(lineutil.NewURIAction("🚨 撥打台北校安", "tel:"+taipeiEmergencyPhone)).WithStyle("primary").WithColor("#ff3333").WithHeight("sm").FlexButton,
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

	return []messaging_api.MessageInterface{
		lineutil.NewFlexMessage("緊急聯絡電話", bubble.FlexBubble),
	}
}

// handleContactSearch handles contact search queries
func (h *Handler) handleContactSearch(ctx context.Context, searchTerm string) []messaging_api.MessageInterface {
	log := h.logger.WithModule(moduleName)
	startTime := time.Now()

	// Search in cache first
	contacts, err := h.db.SearchContactsByName(searchTerm)
	if err != nil {
		log.WithError(err).Error("Failed to search contacts in cache")
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetail("查詢聯絡資訊時發生問題")
		if textMsg, ok := msg.(*messaging_api.TextMessage); ok {
			textMsg.QuickReply = lineutil.NewQuickReply([]lineutil.QuickReplyItem{
				{Action: lineutil.NewMessageAction("重試", "聯絡 "+searchTerm)},
				{Action: lineutil.NewMessageAction("緊急電話", "緊急")},
			})
		}
		return []messaging_api.MessageInterface{msg}
	}

	// If found in cache and not expired, return results
	if len(contacts) > 0 {
		h.metrics.RecordCacheHit(moduleName)
		log.Infof("Cache hit for contact search: %s", searchTerm)
		return h.formatContactResults(contacts)
	}

	// Cache miss - scrape from website
	h.metrics.RecordCacheMiss(moduleName)
	log.Infof("Cache miss for contact search: %s, scraping...", searchTerm)

	contactsPtr, err := ntpu.ScrapeContacts(ctx, h.scraper, searchTerm)
	if err != nil {
		log.WithError(err).Errorf("Failed to scrape contacts for: %s", searchTerm)
		h.metrics.RecordScraperRequest(moduleName, "error", time.Since(startTime).Seconds())
		msg := lineutil.ErrorMessageWithDetail("無法取得聯絡資料，可能是網路問題或資料來源暫時無法使用")
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
		h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
		msg := lineutil.NewTextMessageWithSender(fmt.Sprintf("🔍 查無包含「%s」的聯絡資料\n\n請確認關鍵字是否正確", searchTerm), senderName, h.stickerManager.GetRandomSticker())
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

// formatContactResults formats contact results as LINE messages
func (h *Handler) formatContactResults(contacts []storage.Contact) []messaging_api.MessageInterface {
	if len(contacts) == 0 {
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithSender("🔍 查無聯絡資料", senderName, h.stickerManager.GetRandomSticker()),
		}
	}

	var messages []messaging_api.MessageInterface
	chunkSize := 10 // LINE Carousel max limit

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
			// Determine display text
			headerText := c.Name
			subText := c.Type
			if c.Type == "organization" {
				subText = "單位"
			} else if c.Title != "" {
				subText = c.Title
			}

			// Hero: Name with colored background
			hero := lineutil.NewFlexBox("vertical",
				lineutil.NewFlexText(headerText).WithWeight("bold").WithSize("lg").WithColor("#ffffff").WithWrap(true).FlexText,
				lineutil.NewFlexText(subText).WithSize("xs").WithColor("#ffffff").WithMargin("xs").FlexText,
			).FlexBox
			hero.BackgroundColor = "#1DB446"
			hero.PaddingAll = "15px"

			// Body: Details
			var bodyContents []messaging_api.FlexComponentInterface

			// Organization / Superior
			if c.Type == "organization" && c.Superior != "" {
				// Truncate superior name if too long (max ~30 chars)
				superiorName := c.Superior
				if len(superiorName) > 30 {
					superiorName = superiorName[:27] + "..."
				}
				bodyContents = append(bodyContents, lineutil.NewKeyValueRow("🏢 上級", superiorName).WithMargin("lg").FlexBox)
			} else if c.Organization != "" {
				// Truncate organization name if too long (max ~30 chars)
				orgName := c.Organization
				if len(orgName) > 30 {
					orgName = orgName[:27] + "..."
				}
				bodyContents = append(bodyContents, lineutil.NewKeyValueRow("🏢 單位", orgName).WithMargin("lg").FlexBox)
			}

			// Contact Info
			if c.Extension != "" {
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents, lineutil.NewKeyValueRow("☎️ 分機", c.Extension).WithMargin("md").FlexBox)
			}
			if c.Phone != "" {
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents, lineutil.NewKeyValueRow("📞 專線", c.Phone).WithMargin("md").FlexBox)
			}
			if c.Location != "" {
				// Truncate location if too long (max ~35 chars)
				location := c.Location
				if len(location) > 35 {
					location = location[:32] + "..."
				}
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents, lineutil.NewKeyValueRow("📍 地點", location).WithMargin("md").FlexBox)
			}
			if c.Email != "" {
				// Truncate email if too long to prevent layout break
				email := c.Email
				if len(email) > 30 {
					email = email[:27] + "..."
				}
				if len(bodyContents) > 0 {
					bodyContents = append(bodyContents, lineutil.NewFlexSeparator().WithMargin("md").FlexSeparator)
				}
				bodyContents = append(bodyContents, lineutil.NewKeyValueRow("✉️ Email", email).WithMargin("md").FlexBox)
			}

			// Footer: Actions
			var footerContents []messaging_api.FlexComponentInterface

			// Call button (Extension or Phone)
			if c.Phone != "" {
				// Clean phone number for tel link
				phoneNum := strings.ReplaceAll(c.Phone, "-", "")
				phoneNum = strings.ReplaceAll(phoneNum, " ", "")
				footerContents = append(footerContents, lineutil.NewFlexButton(
					lineutil.NewURIAction("📞 撥打專線", "tel:"+phoneNum),
				).WithStyle("primary").WithHeight("sm").FlexButton)
			} else if c.Extension != "" {
				// For extension, we can't dial directly, but we can copy
				footerContents = append(footerContents, lineutil.NewFlexButton(
					lineutil.NewClipboardAction("📋 複製分機", c.Extension),
				).WithStyle("primary").WithHeight("sm").FlexButton)
			}

			// Email button
			if c.Email != "" {
				footerContents = append(footerContents, lineutil.NewFlexButton(
					lineutil.NewURIAction("✉️ 寄送郵件", "mailto:"+c.Email),
				).WithStyle("secondary").WithHeight("sm").FlexButton)
			}

			// Website button (for organizations)
			if c.Website != "" {
				footerContents = append(footerContents, lineutil.NewFlexButton(
					lineutil.NewURIAction("🌐 瀏覽網站", c.Website),
				).WithStyle("secondary").WithHeight("sm").FlexButton)
			}

			// Assemble Bubble
			bubble := lineutil.NewFlexBubble(
				nil, // Header
				hero,
				lineutil.NewFlexBox("vertical", bodyContents...).WithSpacing("sm"), // Body
				nil, // Footer (handled below)
			)

			if len(footerContents) > 0 {
				bubble.Footer = lineutil.NewFlexBox("vertical", footerContents...).WithSpacing("sm").FlexBox
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
