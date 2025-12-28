package ntpu

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
)

// ProgramInfo contains information about an academic program.
type ProgramInfo struct {
	Name     string
	Category string
	URL      string
}

// ProgramFolder represents a category of programs on the LMS.
type ProgramFolder struct {
	ID       string
	Category string
}

// programFolders defines all program category folders to scrape with their category names.
var programFolders = []ProgramFolder{
	{"115531", "碩士學分學程"},
	{"115532", "學士學分學程"},
	{"115533", "學士暨碩士學分學程"},
	{"198807", "碩士跨域微學程"},
	{"198808", "學士跨域微學程"},
	{"198809", "學士暨碩士跨域微學程"},
	{"198811", "碩士單一領域微學程"},
	{"198812", "學士單一領域微學程"},
}

const (
	lmsCourseID = "28286"
	maxPages    = 10 // Safety limit to prevent infinite loops

	// User-facing URLs use domain (not IP) for better UX
	lmsUserFacingURL = "https://lms.ntpu.edu.tw"
)

// ScrapePrograms dynamically discovers all programs by crawling the LMS folders.
// It handles pagination automatically by detecting "Next" links.
// Supports automatic URL failover via URLCache.
func ScrapePrograms(ctx context.Context, client *scraper.Client) ([]ProgramInfo, error) {
	// Get working base URL (e.g., IP address) for scraping
	baseURL, err := lmsCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working LMS URL: %w", err)
	}

	seen := make(map[string]bool) // Track by cid to avoid duplicates
	var programs []ProgramInfo

	for _, folder := range programFolders {
		folderPrograms, err := scrapeFolderAllPages(ctx, client, baseURL, folder, seen)
		if err != nil {
			// Log error but continue with other folders
			continue
		}
		programs = append(programs, folderPrograms...)
	}

	return programs, nil
}

// scrapeFolderAllPages scrapes all pages of a folder.
func scrapeFolderAllPages(ctx context.Context, client *scraper.Client, baseURL string, folder ProgramFolder, seen map[string]bool) ([]ProgramInfo, error) {
	var allPrograms []ProgramInfo

	for page := 1; page <= maxPages; page++ {
		pageURL := buildFolderURL(baseURL, folder.ID, page)

		doc, err := client.GetDocument(ctx, pageURL)
		if err != nil {
			if page == 1 {
				return nil, err // First page fail is an error
			}
			break // Subsequent page fail means we're done
		}

		programs, hasNext := extractProgramsFromPage(doc, seen, folder.Category)
		allPrograms = append(allPrograms, programs...)

		// Stop if no "Next" link found or no new programs were found
		if !hasNext || len(programs) == 0 {
			break
		}
	}

	return allPrograms, nil
}

// buildFolderURL constructs the URL for a folder page.
// Appends /board.php to the base URL (which might be an IP).
func buildFolderURL(baseURL, folderID string, page int) string {
	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	if page == 1 {
		return fmt.Sprintf("%s/board.php?courseID=%s&f=doclist&folderID=%s", baseURL, lmsCourseID, folderID)
	}
	return fmt.Sprintf("%s/board.php?courseID=%s&f=doclist&folderID=%s&page=%d", baseURL, lmsCourseID, folderID, page)
}

// extractProgramsFromPage extracts program info from a page and checks for "Next" link.
func extractProgramsFromPage(doc *goquery.Document, seen map[string]bool, category string) ([]ProgramInfo, bool) {
	var results []ProgramInfo
	hasNext := false

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		// Check for "Next" pagination link
		text := strings.TrimSpace(s.Text())
		if text == "Next" || text == "下一頁" {
			hasNext = true
			return
		}

		// Parse URL
		u, err := url.Parse(href)
		if err != nil {
			return
		}

		q := u.Query()

		// Must be a document link (f=doc)
		if q.Get("f") != "doc" {
			return
		}

		// Must have cid parameter
		cid := q.Get("cid")
		if cid == "" {
			return
		}

		// Skip if already seen
		if seen[cid] {
			return
		}

		// Get program name
		name := strings.TrimSpace(s.Text())
		if name == "" {
			return
		}

		// Filter: must contain "學程" to be a program
		if !strings.Contains(name, "學程") {
			return
		}

		// Skip discontinued programs (廢止)
		if strings.Contains(name, "廢止") {
			return
		}

		// Clean up name: remove annotations like "(112-1更名，原名：...)" or "★跨校..."
		cleanName := cleanProgramName(name)

		// Build absolute URL using user-facing domain (NOT the IP used for scraping)
		fullURL := href
		if !strings.HasPrefix(fullURL, "http") {
			if strings.HasPrefix(href, "/") {
				fullURL = lmsUserFacingURL + href
			} else {
				fullURL = lmsUserFacingURL + "/" + href
			}
		}

		seen[cid] = true
		results = append(results, ProgramInfo{
			Name:     cleanName,
			Category: category,
			URL:      fullURL,
		})
	})

	return results, hasNext
}

// cleanProgramName removes annotation suffixes from program names.
// Examples:
//   - "金融科技與量化金融學士學分學程（112-1更名，原名：金融科技學士學分學程)" -> "金融科技與量化金融學士學分學程"
//   - "創新創業學士學分學程(104學年度更名，原名：創新產業管理學士學分學程) ★跨校（北醫、北科大）★" -> "創新創業學士學分學程"
//
// Pattern to match content in parentheses containing "更名" or "學年度"
// Matches: ( ...更名... ) or ( ...學年度... ) or fullwidth （...）
var reRename = regexp.MustCompile(`[（(].*?(更名|學年度|原名).*?[)）]`)

func cleanProgramName(name string) string {
	// 1. Remove content in parentheses that contains "更名", "學年度", or "原名"
	name = reRename.ReplaceAllString(name, "")

	// 2. Remove "★跨校...★" type annotations
	// Often appears as suffix
	if idx := strings.Index(name, "★"); idx > 0 {
		name = name[:idx]
	}

	// 3. Trim whitespace
	return strings.TrimSpace(name)
}
