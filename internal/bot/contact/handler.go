package contact

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper/ntpu"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/pkg/lineutil"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Handler handles contact-related queries
type Handler struct {
	db      *storage.DB
	scraper *scraper.Client
	metrics *metrics.Metrics
	logger  *logger.Logger
}

const (
	moduleName = "contact"
	splitChar  = "$"
)

// Valid keywords for contact queries
var (
	validContactKeywords = []string{
		"touch", "contact", "connect", "聯繫", "聯絡", "聯繫方式", "聯絡方式",
		"連繫", "連絡", "連絡方式", "連絡方式", "電話", "分機", "email", "信箱",
	}

	contactRegex = buildRegex(validContactKeywords)

	// Emergency contact constants
	sanxiaNormalPhone    = "02-8674-1111"
	sanxia24HPhone       = "02-2673-2123"
	sanxiaEmergencyPhone = "02-2671-0310"
	taipeiNormalPhone    = "02-2502-4654"
	taipeiEmergencyPhone = "02-2388-9996"
)

// buildRegex creates a regex pattern from keywords
func buildRegex(keywords []string) *regexp.Regexp {
	pattern := "(?i)" + strings.Join(keywords, "|")
	return regexp.MustCompile(pattern)
}

// NewHandler creates a new contact handler
func NewHandler(db *storage.DB, scraper *scraper.Client, metrics *metrics.Metrics, logger *logger.Logger) *Handler {
	return &Handler{
		db:      db,
		scraper: scraper,
		metrics: metrics,
		logger:  logger,
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
	text := fmt.Sprintf(`🚨 緊急聯絡電話

【三峽校區】
一般電話：%s
24H 緊急行政：%s
24H 急難救助：%s

【台北校區】
一般電話：%s
24H 急難救助：%s

更多資訊請參考：
https://new.ntpu.edu.tw/safety`,
		sanxiaNormalPhone,
		sanxia24HPhone,
		sanxiaEmergencyPhone,
		taipeiNormalPhone,
		taipeiEmergencyPhone,
	)

	return []messaging_api.MessageInterface{
		lineutil.NewButtonsTemplate(
			"緊急電話",
			"緊急聯絡電話",
			"點擊查看更多資訊",
			[]lineutil.Action{
				lineutil.NewURIAction("查看校園安全網", "https://new.ntpu.edu.tw/safety"),
			},
		),
		lineutil.NewTextMessage(text),
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
			lineutil.ErrorMessage(fmt.Errorf("資料庫查詢失敗")),
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
			lineutil.ErrorMessage(fmt.Errorf("無法取得聯絡資料")),
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
			lineutil.NewTextMessage(fmt.Sprintf("🔍 查無包含「%s」的聯絡資料\n\n請確認關鍵字是否正確", searchTerm)),
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
			lineutil.NewTextMessage("🔍 查無聯絡資料"),
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

		messages = append(messages, lineutil.NewTextMessage(builder.String()))
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

			messages = append(messages, lineutil.NewTextMessage(builder.String()))
		}
	}

	return messages
}
