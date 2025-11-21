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
		"touch", "contact", "connect", "聯繫", "聯絡", "聯繫方式", "聯絡方式",
		"連繫", "連絡", "連絡方式", "連絡方式", "電話", "分機", "email", "信箱",
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

	// Check for emergency keyword
	if strings.HasPrefix(text, "緊急") {
		return true
	}

	// Check for contact keywords
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

	// Handle contact search
	if match := contactRegex.FindString(text); match != "" {
		return h.handleContactSearch(ctx, match)
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
	// Format phone numbers for display (add hyphens)
	formatPhone := func(phone string) string {
		if len(phone) == 10 {
			return phone[:2] + "-" + phone[2:6] + "-" + phone[6:]
		} else if len(phone) == 3 {
			return phone // 110, 119
		}
		return phone
	}

	// Main emergency info message
	mainText := fmt.Sprintf(`🚨 緊急聯絡電話

【三峽校區】
總機：%s
24H 緊急行政：%s
24H 急難救助：%s
大門哨所：%s
宿舍夜間緊急：%s

【臺北校區】
總機：%s
24H 急難救助：%s`,
		formatPhone(sanxiaNormalPhone),
		formatPhone(sanxia24HPhone),
		formatPhone(sanxiaEmergencyPhone),
		formatPhone(sanxiaGatePhone),
		formatPhone(sanxiaDormPhone),
		formatPhone(taipeiNormalPhone),
		formatPhone(taipeiEmergencyPhone),
	)

	// Other emergency services
	otherText := fmt.Sprintf(`🚑 其他緊急服務

警察局：%s
消防局/救護車：%s
北大派出所：%s
恩主公醫院：%s

ℹ️ 行動電話收訊不良時請改撥 112`,
		formatPhone(policePhone),
		formatPhone(firePhone),
		formatPhone(policeStation),
		formatPhone(homHospital),
	)

	return []messaging_api.MessageInterface{
		// Main message with quick copy buttons
		lineutil.NewButtonsTemplate(
			"🚨 緊急電話",
			"校園緊急聯絡電話",
			"快速複製電話號碼",
			[]lineutil.Action{
				lineutil.NewClipboardAction("複製三峽24H行政", sanxia24HPhone),
				lineutil.NewClipboardAction("複製三峽24H急難", sanxiaEmergencyPhone),
				lineutil.NewClipboardAction("複製臺北24H急難", taipeiEmergencyPhone),
				lineutil.NewURIAction("查看校園安全網", "https://new.ntpu.edu.tw/safety"),
			},
		),
		// Detailed campus phone numbers
		lineutil.NewTextMessageWithSender(mainText, senderName, h.stickerManager.GetRandomSticker()),
		// Other emergency services
		lineutil.NewButtonsTemplate(
			"🚑 其他緊急服務",
			"其他常用緊急電話",
			"快速複製或撥打",
			[]lineutil.Action{
				lineutil.NewURIAction("撥打 110 警察", "tel:"+policePhone),
				lineutil.NewURIAction("撥打 119 消防/救護", "tel:"+firePhone),
				lineutil.NewClipboardAction("複製北大派出所", policeStation),
				lineutil.NewClipboardAction("複製恩主公醫院", homHospital),
			},
		),
		lineutil.NewTextMessageWithSender(otherText, senderName, h.stickerManager.GetRandomSticker()),
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
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithDetail("查詢聯絡資訊時發生問題"),
		}
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
		return []messaging_api.MessageInterface{
			lineutil.ErrorMessageWithDetail("無法取得聯絡資料，可能是網路問題或資料來源暫時無法使用"),
		}
	}

	// Convert []*storage.Contact to []storage.Contact
	contacts = make([]storage.Contact, len(contactsPtr))
	for i, c := range contactsPtr {
		contacts[i] = *c
	}

	if len(contacts) == 0 {
		h.metrics.RecordScraperRequest(moduleName, "success", time.Since(startTime).Seconds())
		return []messaging_api.MessageInterface{
			lineutil.NewTextMessageWithSender(fmt.Sprintf("🔍 查無包含「%s」的聯絡資料\n\n請確認關鍵字是否正確", searchTerm), senderName, h.stickerManager.GetRandomSticker()),
		}
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

	// Limit to 50 results
	if len(contacts) > 50 {
		contacts = contacts[:50]
	}

	messages := make([]messaging_api.MessageInterface, 0)

	// Group contacts: organizations first, then individuals
	var orgs []storage.Contact
	var individuals []storage.Contact

	for _, c := range contacts {
		if c.Type == "organization" {
			orgs = append(orgs, c)
		} else {
			individuals = append(individuals, c)
		}
	}

	// Format organizations
	if len(orgs) > 0 {
		var builder strings.Builder
		builder.WriteString("🏢 單位資訊：\n\n")

		for i, org := range orgs {
			if i >= 20 {
				break // Limit to 20 organizations
			}

			builder.WriteString(fmt.Sprintf("📌 %s\n", org.Name))
			if org.Superior != "" {
				builder.WriteString(fmt.Sprintf("上級單位：%s\n", org.Superior))
			}
			if org.Location != "" {
				builder.WriteString(fmt.Sprintf("地點：%s\n", org.Location))
			}
			if org.Website != "" {
				builder.WriteString(fmt.Sprintf("網站：%s\n", org.Website))
			}
			builder.WriteString("\n")
		}

		messages = append(messages, lineutil.NewTextMessageWithSender(builder.String(), senderName, h.stickerManager.GetRandomSticker()))
	}

	// Format individuals
	if len(individuals) > 0 {
		// Split into groups of 20 per message
		for i := 0; i < len(individuals); i += 20 {
			end := i + 20
			if end > len(individuals) {
				end = len(individuals)
			}

			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("👤 人員資訊 (第 %d-%d 筆)：\n\n", i+1, end))

			for j := i; j < end; j++ {
				person := individuals[j]
				builder.WriteString(fmt.Sprintf("📌 %s", person.Name))

				if person.Title != "" {
					builder.WriteString(fmt.Sprintf(" - %s", person.Title))
				}
				builder.WriteString("\n")

				if person.Organization != "" {
					builder.WriteString(fmt.Sprintf("單位：%s\n", person.Organization))
				}
				if person.Extension != "" {
					builder.WriteString(fmt.Sprintf("分機：%s\n", person.Extension))
				}
				if person.Phone != "" {
					builder.WriteString(fmt.Sprintf("電話：%s\n", person.Phone))
				}
				if person.Email != "" {
					builder.WriteString(fmt.Sprintf("Email：%s\n", person.Email))
				}
				builder.WriteString("\n")
			}

			messages = append(messages, lineutil.NewTextMessageWithSender(builder.String(), senderName, h.stickerManager.GetRandomSticker()))
		}
	}

	return messages
}
