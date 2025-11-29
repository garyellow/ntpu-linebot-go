package syllabus

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// Pre-compiled regexes for performance
var (
	reScript   = regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`)
	reStyle    = regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`)
	reTags     = regexp.MustCompile(`<[^>]*>`)
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

// ScrapeSyllabus extracts syllabus content from a course's detail URL
// Returns the merged content (教學目標 + 內容綱要 + 教學進度) for embedding
func (s *Scraper) ScrapeSyllabus(ctx context.Context, course *storage.Course) (*Fields, error) {
	if course.DetailURL == "" {
		return nil, fmt.Errorf("course %s has no detail URL", course.UID)
	}

	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping syllabus: %w", err)
	}

	// Fetch the detail page
	doc, err := s.client.GetDocument(ctx, course.DetailURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch syllabus for %s: %w", course.UID, err)
	}

	// Parse syllabus fields
	fields := parseSyllabusPage(doc)
	return fields, nil
}

// parseSyllabusPage extracts syllabus fields from HTML document
// The page structure has TD cells like:
//
//	<td>教學目標 Course Objectives：<span class="font-c13">內容...</span></td>
//	<td>內容綱要/Course Outline：<span class="font-c13">內容...</span></td>
//	<td>教學進度(Teaching Schedule)：<table>...</table></td>
func parseSyllabusPage(doc *goquery.Document) *Fields {
	fields := &Fields{}

	// Helper to find content in TD cells containing a specific label
	findContent := func(label string) string {
		var content string

		doc.Find("td").Each(func(i int, td *goquery.Selection) {
			text := td.Text()
			if !strings.Contains(text, label) {
				return
			}

			// Found the TD with our label
			// Try to extract content from span.font-c13 first (primary method)
			span := td.Find("span.font-c13")
			if span.Length() > 0 {
				content = strings.TrimSpace(span.Text())
				return
			}

			// Try nested table (used for 教學進度, 評量方式)
			table := td.Find("table")
			if table.Length() > 0 {
				content = strings.TrimSpace(table.Text())
				return
			}

			// Fallback: extract text after the label
			html, _ := td.Html()
			content = extractContentAfterLabel(html, label)
		})

		return strings.TrimSpace(content)
	}

	fields.Objectives = findContent("教學目標")
	fields.Outline = findContent("內容綱要")
	fields.Schedule = findContent("教學進度")

	// Clean up the content
	fields.Objectives = cleanContent(fields.Objectives)
	fields.Outline = cleanContent(fields.Outline)
	fields.Schedule = cleanContent(fields.Schedule)

	return fields
}

// extractContentAfterLabel extracts text content after a label in HTML
func extractContentAfterLabel(html, label string) string {
	// Find the position of the label
	labelIndex := strings.Index(html, label)
	if labelIndex == -1 {
		return ""
	}

	// Get content after the label
	afterLabel := html[labelIndex+len(label):]

	// Remove leading colon, whitespace, and English label suffix
	// Patterns like "教學目標 Course Objectives：" or "內容綱要/Course Outline："
	afterLabel = strings.TrimPrefix(afterLabel, "：")
	afterLabel = strings.TrimPrefix(afterLabel, ":")
	afterLabel = strings.TrimSpace(afterLabel)

	// Remove common English label suffixes
	englishLabelPatterns := []string{
		"Course Objectives", "Course Outline", "Teaching Schedule",
		"Evaluation Methods", "Required Texts", "Other References",
		"Prerequisites", "Course Requirements",
	}
	for _, eng := range englishLabelPatterns {
		if strings.HasPrefix(afterLabel, eng) {
			afterLabel = strings.TrimPrefix(afterLabel, eng)
			afterLabel = strings.TrimPrefix(afterLabel, "：")
			afterLabel = strings.TrimPrefix(afterLabel, ":")
			afterLabel = strings.TrimSpace(afterLabel)
		}
		// Also handle "/English" pattern
		if strings.HasPrefix(afterLabel, "/"+eng) {
			afterLabel = strings.TrimPrefix(afterLabel, "/"+eng)
			afterLabel = strings.TrimPrefix(afterLabel, "：")
			afterLabel = strings.TrimPrefix(afterLabel, ":")
			afterLabel = strings.TrimSpace(afterLabel)
		}
	}

	// Also handle parenthetical English labels like "(Teaching Schedule)"
	if strings.HasPrefix(afterLabel, "(") {
		if endParen := strings.Index(afterLabel, ")"); endParen > 0 {
			// Check if it's an English label in parentheses
			parenContent := afterLabel[1:endParen]
			for _, eng := range englishLabelPatterns {
				if strings.Contains(parenContent, eng) {
					afterLabel = afterLabel[endParen+1:]
					afterLabel = strings.TrimPrefix(afterLabel, "：")
					afterLabel = strings.TrimPrefix(afterLabel, ":")
					afterLabel = strings.TrimSpace(afterLabel)
					break
				}
			}
		}
	}

	// Find the next section label or end
	// Common labels to look for (both Chinese and English)
	nextLabels := []string{
		"教學目標", "內容綱要", "教學進度", "評量方式", "教科書",
		"參考書目", "教學方法", "先修科目", "課程要求", "核心能力",
		"指定用書", "其他參考", "本課程包含",
	}

	endIndex := len(afterLabel)
	for _, nextLabel := range nextLabels {
		if idx := strings.Index(afterLabel, nextLabel); idx > 0 && idx < endIndex {
			endIndex = idx
		}
	}

	content := afterLabel[:endIndex]

	// Strip HTML tags
	content = stripHTMLTags(content)

	return content
}

// stripHTMLTags removes HTML tags from a string
func stripHTMLTags(s string) string {
	// Remove script elements
	s = reScript.ReplaceAllString(s, "")

	// Remove style elements
	s = reStyle.ReplaceAllString(s, "")

	// Remove HTML tags
	s = reTags.ReplaceAllString(s, " ")

	// Decode common HTML entities
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")

	return s
}

// cleanContent normalizes whitespace and removes garbage
func cleanContent(s string) string {
	if s == "" {
		return ""
	}

	// Normalize line breaks
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// Collapse multiple spaces
	s = reSpaces.ReplaceAllString(s, " ")

	// Collapse multiple newlines
	s = reNewlines.ReplaceAllString(s, "\n\n")

	// Trim each line
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
