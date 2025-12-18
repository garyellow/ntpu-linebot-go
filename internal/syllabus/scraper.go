package syllabus

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// Pre-compiled regexes for performance
var (
	reSpaces   = regexp.MustCompile(`[ \t]+`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
)

// Scraper extracts syllabus content from course detail pages
type Scraper struct {
	client *scraper.Client
}

// NewScraper creates a new syllabus scraper
func NewScraper(client *scraper.Client) *Scraper {
	return &Scraper{client: client}
}

// ScrapeSyllabus extracts syllabus content from a course's detail URL (show_info=all format)
// Returns the merged content (教學目標 + 內容綱要 + 教學進度) for BM25 indexing
func (s *Scraper) ScrapeSyllabus(ctx context.Context, course *storage.Course) (*Fields, error) {
	if course.DetailURL == "" {
		return nil, fmt.Errorf("course %s has no detail URL", course.UID)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping syllabus: %w", err)
	}

	start := time.Now()
	slog.DebugContext(ctx, "scraping syllabus",
		"uid", course.UID,
		"detail_url", course.DetailURL)

	doc, err := s.client.GetDocument(ctx, course.DetailURL)
	if err != nil {
		slog.WarnContext(ctx, "failed to scrape syllabus",
			"uid", course.UID,
			"detail_url", course.DetailURL,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err)
		return nil, fmt.Errorf("failed to fetch syllabus for %s: %w", course.UID, err)
	}

	fields := parseSyllabusPage(doc)

	slog.DebugContext(ctx, "syllabus scraped successfully",
		"uid", course.UID,
		"is_empty", fields.IsEmpty(),
		"content_length", len(fields.ContentForIndexing(course.Title)),
		"duration_ms", time.Since(start).Milliseconds())

	return fields, nil
}

// parseSyllabusPage extracts syllabus fields from HTML document
// Expects show_info=all format with merged CN+EN content in single TD cells:
// - "教學目標 Course Objectives：" (objectives)
// - "內容綱要/Course Outline：" (outline)
// - "教學進度(Teaching Schedule)：" (schedule table)
func parseSyllabusPage(doc *goquery.Document) *Fields {
	fields := &Fields{}

	// findContentByPrefix locates TD starting with the prefix and extracts font-c13 content
	findContentByPrefix := func(prefix string) string {
		var content string

		doc.Find("td").Each(func(i int, td *goquery.Selection) {
			if content != "" {
				return // Already found
			}

			text := strings.TrimSpace(td.Text())
			if !strings.HasPrefix(text, prefix) {
				return
			}

			// Extract content from font-c13 span (most common format)
			if span := td.Find("span.font-c13"); span.Length() > 0 {
				content = strings.TrimSpace(span.First().Text())
				return
			}

			// Fallback: try div.font-c13
			if div := td.Find("div.font-c13"); div.Length() > 0 {
				content = strings.TrimSpace(div.First().Text())
				return
			}
		})

		return content
	}

	// findObjectives extracts merged objectives content from show_info=all format
	findObjectives := func() string {
		return findContentByPrefix("教學目標 Course Objectives")
	}

	// findOutline extracts merged outline content from show_info=all format
	findOutline := func() string {
		return findContentByPrefix("內容綱要/Course Outline")
	}

	// findSchedule extracts schedule from table format
	// Table structure: 週別 | 日期 | 教學預定進度 | 教學方法與教學活動
	findSchedule := func() string {
		var items []string

		doc.Find("td").Each(func(i int, td *goquery.Selection) {
			if len(items) > 0 {
				return // Already found
			}

			text := strings.TrimSpace(td.Text())
			if !strings.HasPrefix(text, "教學進度") {
				return
			}

			table := td.Find("table").First()
			if table.Length() == 0 {
				return
			}

			// Validate header contains 週別
			headerText := table.Find("tr").First().Text()
			if !strings.Contains(headerText, "週別") && !strings.Contains(headerText, "Weekly") {
				return
			}

			// Extract schedule content from each row
			table.Find("tr").Each(func(rowIdx int, tr *goquery.Selection) {
				if rowIdx == 0 {
					return // Skip header
				}

				tds := tr.Find("td")
				if tds.Length() < 3 {
					return
				}

				week := strings.TrimSpace(tds.Eq(0).Text())
				schedule := strings.TrimSpace(tds.Eq(2).Text())

				if schedule == "" || schedule == "彈性補充教學" {
					return
				}

				if strings.HasPrefix(week, "Week") {
					items = append(items, week+": "+schedule)
				} else if schedule != "" {
					items = append(items, schedule)
				}
			})
		})

		return strings.Join(items, "\n")
	}

	// Parse all fields
	fields.Objectives = cleanContent(findObjectives())
	fields.Outline = cleanContent(findOutline())
	fields.Schedule = cleanContent(findSchedule())

	return fields
}

// joinNonEmpty joins two strings with newline, skipping empty strings
func joinNonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n" + b
}

// cleanContent normalizes whitespace
func cleanContent(s string) string {
	if s == "" {
		return ""
	}

	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = reSpaces.ReplaceAllString(s, " ")
	s = reNewlines.ReplaceAllString(s, "\n\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}

	// Remove empty lines at start and end
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}
