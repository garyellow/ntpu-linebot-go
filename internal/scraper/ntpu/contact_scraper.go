// Package ntpu provides scrapers for NTPU websites including student ID,
// course catalog, and contact directory information.
package ntpu

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
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

// seaCache is a package-level helper for SEA URL caching.
// Returns the cached working URL or detects a new one.
func seaCache(ctx context.Context, client *scraper.Client) (string, error) {
	return scraper.NewURLCache(client, "sea").Get(ctx)
}

// ScrapeContacts scrapes contacts by search term
// URL: {baseURL}/pls/ld/CAMPUS_DIR_M.pq?q={searchTerm}
// The search term must be URL-encoded in Big5 encoding
// Supports automatic URL failover across multiple SEA endpoints with cache invalidation
func ScrapeContacts(ctx context.Context, client *scraper.Client, searchTerm string) ([]*storage.Contact, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping contacts: %w", err)
	}

	// Get working base URL with failover support
	contactBaseURL, err := seaCache(ctx, client)
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

// BuildContactSearchURL generates the URL for viewing contact search results.
// The search term is encoded in Big5 format as required by the NTPU directory.
// Returns empty string if encoding fails.
// This is used for the "資料來源" button to open the original contact page.
func BuildContactSearchURL(searchTerm string) string {
	// Use default SEA URL (not dynamic to avoid async issues in button generation)
	const defaultSEAURL = "https://sea.cc.ntpu.edu.tw"

	big5Encoded, err := encodeToBig5(searchTerm)
	if err != nil {
		return ""
	}
	encodedTerm := url.QueryEscape(big5Encoded)
	return fmt.Sprintf("%s%s?q=%s", defaultSEAURL, contactSearchPath, encodedTerm)
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
				extension = strings.TrimSpace(extension)
				phone := buildFullPhone(sanxiaNormalPhone, extension)

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
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping administrative contacts: %w", err)
	}

	contactBaseURL, err := seaCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}
	url := contactBaseURL + administrativePath
	return scrapeContactPages(ctx, client, contactBaseURL, url)
}

// ScrapeAcademicContacts scrapes all academic contacts
// Supports automatic URL failover across multiple SEA endpoints
func ScrapeAcademicContacts(ctx context.Context, client *scraper.Client) ([]*storage.Contact, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping academic contacts: %w", err)
	}

	contactBaseURL, err := seaCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}
	url := contactBaseURL + academicPath
	return scrapeContactPages(ctx, client, contactBaseURL, url)
}

// scrapeContactPages scrapes contact information from department listing pages
func scrapeContactPages(ctx context.Context, client *scraper.Client, contactBaseURL, url string) ([]*storage.Contact, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before fetching contact pages: %w", err)
	}

	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch contact pages: %w", err)
	}

	allContacts := make([]*storage.Contact, 0)
	var scrapeErrors []string
	var successCount int

	// Find all department links: <div class="card-header">
	doc.Find("div.card-header").Each(func(i int, s *goquery.Selection) {
		// Check context cancellation within loop
		if ctx.Err() != nil {
			return
		}

		link := s.Find("a")
		href, exists := link.Attr("href")
		if !exists {
			return
		}

		deptURL := fmt.Sprintf("%s/pls/ld/%s", contactBaseURL, href)

		// Fetch department page
		deptDoc, err := client.GetDocument(ctx, deptURL)
		if err != nil {
			// Record error but continue with other departments
			scrapeErrors = append(scrapeErrors, fmt.Sprintf("dept %s: %v", href, err))
			return
		}

		// Parse contacts from department page
		contacts := parseContactsPage(deptDoc)
		allContacts = append(allContacts, contacts...)
		successCount++
	})

	// Check if context was canceled during processing
	if err := ctx.Err(); err != nil {
		return allContacts, fmt.Errorf("context canceled during contact scraping (partial results: %d departments): %w", successCount, err)
	}

	// If all requests failed, return error; otherwise return partial results
	if len(allContacts) == 0 && len(scrapeErrors) > 0 {
		return nil, fmt.Errorf("all department requests failed (%d errors): %v", len(scrapeErrors), scrapeErrors)
	}

	return allContacts, nil
}

// buildFullPhone creates a full phone number string combining main phone and extension.
// Format: "0286741111,12345" (main phone + comma + extension first 5 digits)
// Returns empty string if extension < 5 digits.
func buildFullPhone(mainPhone, extension string) string {
	if len(extension) < 5 {
		return ""
	}
	return mainPhone + "," + extension[:5]
}
