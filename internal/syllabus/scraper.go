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
	reHTMLTags = regexp.MustCompile(`<[^>]*>`)
)

// Scraper extracts syllabus content and program requirements from course detail pages
type Scraper struct {
	client *scraper.Client
}

// NewScraper creates a new syllabus scraper
func NewScraper(client *scraper.Client) *Scraper {
	return &Scraper{client: client}
}

// ScrapeResult contains all data extracted from a course detail page
type ScrapeResult struct {
	Fields   *Fields                      // Syllabus content (objectives, outline, schedule)
	Programs []storage.ProgramRequirement // Academic programs this course belongs to
}

// ScrapeCourseDetail extracts syllabus content AND program requirements from a course's detail URL.
// Returns structured fields and program list, or error if URL is invalid.
// This is the primary method for extracting course detail data.
func (s *Scraper) ScrapeCourseDetail(ctx context.Context, course *storage.Course) (*ScrapeResult, error) {
	if course.DetailURL == "" {
		return nil, fmt.Errorf("course %s has no detail URL", course.UID)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping: %w", err)
	}

	start := time.Now()
	slog.DebugContext(ctx, "scraping course detail",
		"uid", course.UID,
		"detail_url", course.DetailURL)

	doc, err := s.client.GetDocument(ctx, course.DetailURL)
	if err != nil {
		slog.WarnContext(ctx, "failed to scrape course detail",
			"uid", course.UID,
			"detail_url", course.DetailURL,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err)
		return nil, fmt.Errorf("failed to fetch course detail for %s: %w", course.UID, err)
	}

	fields := parseSyllabusPage(doc)
	programs := parseProgramsFromDetailPage(doc)

	slog.DebugContext(ctx, "course detail scraped successfully",
		"uid", course.UID,
		"syllabus_empty", fields.IsEmpty(),
		"programs_count", len(programs),
		"duration_ms", time.Since(start).Milliseconds())

	return &ScrapeResult{
		Fields:   fields,
		Programs: programs,
	}, nil
}

// ScrapeSyllabus extracts syllabus content from a course's detail URL.
// Returns structured fields (objectives, outline, schedule) or error if URL is invalid.
// Empty syllabi are valid - use Fields.IsEmpty to check.
//
// Deprecated: Use ScrapeCourseDetail instead to also get program requirements.
func (s *Scraper) ScrapeSyllabus(ctx context.Context, course *storage.Course) (*Fields, error) {
	result, err := s.ScrapeCourseDetail(ctx, course)
	if err != nil {
		return nil, err
	}
	return result.Fields, nil
}

// parseSyllabusPage extracts syllabus fields from HTML document.
// Handles multiple page formats and returns empty fields if parsing fails.
func parseSyllabusPage(doc *goquery.Document) *Fields {
	fields := &Fields{}

	// findContentByPrefix extracts content from TD starting with the prefix
	// Supports multiple font-c13 elements and falls back to text content if none found
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

			// Try to extract all font-c13 elements (both span and div)
			var parts []string
			td.Find("span.font-c13, div.font-c13").Each(func(j int, elem *goquery.Selection) {
				if part := strings.TrimSpace(elem.Text()); part != "" {
					parts = append(parts, part)
				}
			})

			if len(parts) > 0 {
				content = strings.Join(parts, "")
				return
			}

			// Fallback: extract text content after prefix (for pages without font-c13)
			// Remove prefix and colon, trim whitespace
			afterPrefix := strings.TrimSpace(strings.TrimPrefix(text, prefix))
			afterPrefix = strings.TrimPrefix(afterPrefix, "：")
			afterPrefix = strings.TrimPrefix(afterPrefix, ":")
			content = strings.TrimSpace(afterPrefix)
		})

		return content
	}

	// findObjectives extracts objectives content, supporting both merged and separate formats
	findObjectives := func() string {
		// Try merged format first: "教學目標 Course Objectives："
		merged := findContentByPrefix("教學目標 Course Objectives")
		if merged != "" {
			return merged
		}

		// Try separate format: "教學目標：" and "Course Objectives："
		cn := findContentByPrefix("教學目標")
		en := findContentByPrefix("Course Objectives")

		// Merge CN and EN if both exist
		if cn != "" && en != "" {
			return cn + " " + en
		}
		if cn != "" {
			return cn
		}
		return en
	}

	// findOutline extracts outline content, supporting both merged and separate formats
	findOutline := func() string {
		// Try merged format first: "內容綱要/Course Outline："
		merged := findContentByPrefix("內容綱要/Course Outline")
		if merged != "" {
			return merged
		}

		// Try separate format: "內容綱要：" and "Course Outline："
		cn := findContentByPrefix("內容綱要")
		en := findContentByPrefix("Course Outline")

		// Merge CN and EN if both exist
		if cn != "" && en != "" {
			return cn + " " + en
		}
		if cn != "" {
			return cn
		}
		return en
	}

	// findSchedule extracts schedule from table format
	// Table structure: 週別 | 日期 | 教學預定進度 | 教學方法與教學活動
	// Validates table by checking for "週別" or "Weekly" in header row
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

// parseProgramsFromDetailPage extracts program requirements from the course detail page.
// The detail page (queryguide) contains complete and accurate program names in the Major field.
// Format: "應修系級 Major:<b class="font-c15">統計學系3 ...,商業智慧與大數據分析學士學分學程 ...</b>"
// Programs are comma-separated, only items ending with "學程" are included.
func parseProgramsFromDetailPage(doc *goquery.Document) []storage.ProgramRequirement {
	programs := make([]storage.ProgramRequirement, 0)

	// Find the Major field - it contains both departments and programs
	// The detail page shows: "應修系級 Major:" followed by <b class="font-c15">content</b>
	doc.Find("td.font-g13").Each(func(i int, td *goquery.Selection) {
		text := td.Text()

		// Look for the Major/應修系級 field
		if !strings.Contains(text, "Major:") && !strings.Contains(text, "應修系級") {
			return
		}

		// Extract content from <b class="font-c15">
		bold := td.Find("b.font-c15")
		if bold.Length() == 0 {
			return
		}

		// Get the HTML content and split by comma
		content, _ := bold.Html()
		if content == "" {
			content = bold.Text()
		}

		// Split by comma (programs and departments are comma-separated)
		parts := strings.Split(content, ",")

		for _, part := range parts {
			// Clean up: remove HTML tags, &nbsp;, whitespace
			part = reHTMLTags.ReplaceAllString(part, "")
			part = strings.ReplaceAll(part, "&nbsp;", "")
			part = strings.ReplaceAll(part, "\u00a0", "") // non-breaking space
			part = strings.TrimSpace(part)

			// Only include items ending with "學程"
			if !strings.HasSuffix(part, "學程") {
				continue
			}

			// Skip empty or invalid entries
			if part == "" || part == "學程" {
				continue
			}

			// All programs from detail page use "選" (elective) as default
			// The detail page doesn't specify required/elective per program
			programs = append(programs, storage.ProgramRequirement{
				ProgramName: part,
				CourseType:  "選",
			})
		}
	})

	return programs
}
