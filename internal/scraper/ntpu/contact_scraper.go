package ntpu

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

const (
	contactSearchPath  = "/pls/ld/CAMPUS_DIR_M.pq"
	administrativePath = "/pls/ld/CAMPUS_DIR_M.p1?kind=1"
	academicPath       = "/pls/ld/CAMPUS_DIR_M.p1?kind=2"

	// Phone constants for Sanxia campus (assumed default)
	sanxiaNormalPhone = "0286741111"
)

// getWorkingSEABaseURL gets the working SEA base URL using URLCache abstraction.
// Returns cached URL if available (fast path), otherwise triggers failover detection.
// Auto-recovery: caller should call clearSEACache() on scrape errors.
func getWorkingSEABaseURL(ctx context.Context, client *scraper.Client) (string, error) {
	cache := scraper.NewURLCache(client, "sea")
	return cache.Get(ctx)
}

// clearSEACache invalidates the SEA URL cache to trigger re-detection on next request.
// Call this when a scrape operation fails to enable automatic failover.
func clearSEACache(client *scraper.Client) {
	cache := scraper.NewURLCache(client, "sea")
	cache.Clear()
}

// ScrapeContacts scrapes contacts by search term
// URL: {baseURL}/pls/ld/CAMPUS_DIR_M.pq?q={searchTerm}
// The search term must be URL-encoded in Big5 encoding
// Supports automatic URL failover across multiple SEA endpoints with cache invalidation
func ScrapeContacts(ctx context.Context, client *scraper.Client, searchTerm string) ([]*storage.Contact, error) {
	// Get working base URL with failover support
	contactBaseURL, err := getWorkingSEABaseURL(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}

	// Encode search term to Big5
	big5Encoded, err := encodeToBig5(searchTerm)
	if err != nil {
		return nil, fmt.Errorf("failed to encode search term: %w", err)
	}

	// URL encode the Big5 bytes
	encodedTerm := url.QueryEscape(big5Encoded)

	url := fmt.Sprintf("%s%s?q=%s", contactBaseURL, contactSearchPath, encodedTerm)

	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		// Clear cached URL on error to trigger re-detection on next request
		clearSEACache(client)
		return nil, fmt.Errorf("failed to fetch contacts: %w", err)
	}

	return parseContactsPage(doc), nil
}

// encodeToBig5 encodes a string to Big5 encoding
func encodeToBig5(s string) (string, error) {
	encoder := traditionalchinese.Big5.NewEncoder()
	encoded, _, err := transform.String(encoder, s)
	if err != nil {
		return "", err
	}
	return encoded, nil
}

// parseContactsPage parses contact information from the search results page
func parseContactsPage(doc *goquery.Document) []*storage.Contact {
	contacts := make([]*storage.Contact, 0)
	cachedAt := time.Now().Unix()

	// Find all organization sections: <div class="alert alert-info mt-0 mb-0">
	doc.Find("div.alert.alert-info.mt-0.mb-0").Each(func(i int, orgDiv *goquery.Selection) {
		// Extract organization information
		orgLinks := orgDiv.Find("a.lang.lang-zh-Hant.mx-2")

		var superior, orgName string
		if orgLinks.Length() == 1 {
			orgName = strings.TrimSpace(orgLinks.First().Text())
		} else if orgLinks.Length() > 1 {
			superior = strings.TrimSpace(orgLinks.First().Text())
			orgName = strings.TrimSpace(orgLinks.Eq(1).Text())
		}

		// Extract organization details
		var location, website string
		orgDiv.Find("li").Each(func(j int, li *goquery.Selection) {
			text := li.Text()
			if j == 2 && strings.Contains(text, "：") {
				parts := strings.Split(text, "：")
				if len(parts) > 1 {
					location = strings.TrimSpace(parts[1])
				}
			} else if j == 3 {
				website = strings.TrimSpace(li.Find("a").Text())
			}
		})

		// Create organization contact
		orgUID := generateUID("org", orgName)
		orgContact := &storage.Contact{
			UID:      orgUID,
			Type:     "organization",
			Name:     orgName,
			Superior: superior,
			Location: location,
			Website:  website,
			CachedAt: cachedAt,
		}
		contacts = append(contacts, orgContact)

		// Find member table (first sibling after organization div)
		// The w100 div contains the member table directly after the alert-info div
		memberTable := orgDiv.Next()
		if memberTable.HasClass("w100") {
			// Parse individual members from table
			memberTable.Find("tbody tr").Each(func(k int, tr *goquery.Selection) {
				tds := tr.Find("td")
				if tds.Length() < 5 {
					return
				}

				// Extract member information - Chinese name from lang-zh-Hant span
				nameCell := tds.Eq(0)
				memberName := strings.TrimSpace(nameCell.Find("span.lang-zh-Hant").Text())
				if memberName == "" {
					// Fallback: try first span if class not found
					memberName = strings.TrimSpace(nameCell.Find("span").First().Text())
				}

				// Extract English name from lang-en span (if exists)
				memberNameEn := strings.TrimSpace(nameCell.Find("span.lang-en").Text())

				title := strings.TrimSpace(tds.Eq(1).Text())
				// Extension field may have multiple spans (zh-Hant and en-US with same value)
				// Just take the first span's text to avoid duplication
				extension := strings.TrimSpace(tds.Eq(2).Find("span").First().Text())

				// Build full phone number if extension >= 5 digits
				phone := lineutil.BuildFullPhone(sanxiaNormalPhone, strings.TrimSpace(extension))

				// Extract email (may contain @ as image)
				email := ""
				emailSpan := tds.Eq(4).Find("span")
				emailSpan.Contents().Each(func(l int, node *goquery.Selection) {
					if goquery.NodeName(node) == "#text" {
						email += node.Text()
					} else if goquery.NodeName(node) == "img" {
						email += "@"
					}
				})
				email = strings.TrimSpace(email)

				memberUID := generateUID("individual", memberName, orgName)
				memberContact := &storage.Contact{
					UID:          memberUID,
					Type:         "individual",
					Name:         memberName,
					NameEn:       memberNameEn,
					Organization: orgName,
					Title:        title,
					Extension:    extension,
					Phone:        phone,
					Email:        email,
					CachedAt:     cachedAt,
				}
				contacts = append(contacts, memberContact)
			})
		}
	})

	return contacts
}

// generateUID generates a unique identifier for contacts
func generateUID(parts ...string) string {
	return strings.Join(parts, "_")
}

// ScrapeAdministrativeContacts scrapes all administrative contacts
// Supports automatic URL failover across multiple SEA endpoints
func ScrapeAdministrativeContacts(ctx context.Context, client *scraper.Client) ([]*storage.Contact, error) {
	contactBaseURL, err := getWorkingSEABaseURL(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}
	url := contactBaseURL + administrativePath
	contacts, err := scrapeContactPages(ctx, client, contactBaseURL, url)
	if err != nil {
		// Clear cached URL on error to trigger re-detection
		clearSEACache(client)
		return nil, err
	}
	return contacts, nil
}

// ScrapeAcademicContacts scrapes all academic contacts
// Supports automatic URL failover across multiple SEA endpoints
func ScrapeAcademicContacts(ctx context.Context, client *scraper.Client) ([]*storage.Contact, error) {
	contactBaseURL, err := getWorkingSEABaseURL(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}
	url := contactBaseURL + academicPath
	contacts, err := scrapeContactPages(ctx, client, contactBaseURL, url)
	if err != nil {
		// Clear cached URL on error to trigger re-detection
		clearSEACache(client)
		return nil, err
	}
	return contacts, nil
}

// scrapeContactPages scrapes contact information from department listing pages
func scrapeContactPages(ctx context.Context, client *scraper.Client, contactBaseURL, url string) ([]*storage.Contact, error) {
	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact pages: %w", err)
	}

	allContacts := make([]*storage.Contact, 0)

	// Find all department links: <div class="card-header">
	doc.Find("div.card-header").Each(func(i int, s *goquery.Selection) {
		link := s.Find("a")
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		deptURL := fmt.Sprintf("%s/pls/ld/%s", contactBaseURL, href)

		// Fetch department page
		deptDoc, err := client.GetDocument(ctx, deptURL)
		if err != nil {
			return
		}

		// Parse contacts from department page
		contacts := parseContactsPage(deptDoc)
		allContacts = append(allContacts, contacts...)
	})

	return allContacts, nil
}
