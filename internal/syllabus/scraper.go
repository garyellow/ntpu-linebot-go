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
// Returns the merged content (教學目標 + 內容綱要 + 教學進度) for BM25 indexing
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
// Handles two format types:
// 1. Separated format (5 fields): 教學目標, Course Objectives, 內容綱要, Course Outline, 教學進度
// 2. Merged format (3 fields): 教學目標 Course Objectives, 內容綱要/Course Outline, 教學進度
//
// The page structure has TD cells like:
// Separated: <td>教學目標：<div class="font-c13">內容...</div></td>
//
//	<td>Course Objectives：<div class="font-c13">內容...</div></td>
//
// Merged:    <td>教學目標 Course Objectives：<span class="font-c13">內容...</span></td>
func parseSyllabusPage(doc *goquery.Document) *Fields {
	fields := &Fields{}

	// Helper to find content in TD cells containing a specific label
	// Returns (content, hasBothLanguages) where hasBothLanguages indicates if the TD contains both CN and EN labels
	findContent := func(label string) (string, bool) {
		var content string
		var hasBothLanguages bool

		doc.Find("td").Each(func(i int, td *goquery.Selection) {
			text := td.Text()
			if !strings.Contains(text, label) {
				return
			}

			// Check if this TD contains both Chinese and English labels (merged format)
			// e.g., "教學目標 Course Objectives：" in the same cell
			if label == "教學目標" && strings.Contains(text, "Course Objectives") {
				hasBothLanguages = true
			} else if label == "內容綱要" && strings.Contains(text, "Course Outline") {
				hasBothLanguages = true
			}

			// Found the TD with our label
			// Try to extract content from div.font-c13 first (newer format)
			div := td.Find("div.font-c13")
			if div.Length() > 0 {
				content = strings.TrimSpace(div.Text())
				return
			}

			// Try span.font-c13 (older format)
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

		return strings.TrimSpace(content), hasBothLanguages
	}

	// Helper to find teaching schedule content (only the schedule column)
	// The table structure is:
	// - Row 0 (header): 週別 | 日期 | 教學預定進度 | 教學方法與教學活動
	// - Row 1+: Week 1 | 20250911 | Content | Methods
	// We only extract the 3rd column (教學預定進度) with Week prefix
	findScheduleContent := func() string {
		var scheduleItems []string
		found := false

		// Find TD cells that contain "教學進度" label (not just any text)
		// The structure is: <td>教學進度...<table>...</table></td>
		doc.Find("td").Each(func(i int, td *goquery.Selection) {
			if found {
				return // Already found, skip remaining
			}

			// Get the direct text of this TD (not including nested elements)
			// We need to check if this TD is the label cell for 教學進度
			tdHTML, _ := td.Html()

			// Check if this TD starts with 教學進度 label (various formats)
			// Patterns: "教學進度(Teaching Contents)：" or "教學進度(Teaching Schedule)："
			if !strings.Contains(tdHTML, "教學進度") {
				return
			}

			// Find the direct child table (not nested tables within)
			table := td.ChildrenFiltered("table").First()
			if table.Length() == 0 {
				// Try finding first nested table
				table = td.Find("table").First()
			}
			if table.Length() == 0 {
				return
			}

			// Validate this is the correct table by checking header row
			// Header should contain: 週別, 日期, 教學預定進度
			headerRow := table.Find("tr").First()
			headerText := headerRow.Text()
			if !strings.Contains(headerText, "週別") && !strings.Contains(headerText, "Weekly") {
				return // Not the schedule table
			}

			found = true

			// Extract only the schedule content from each row
			// Skip row 0 (header), start from row 1
			table.Find("tr").Each(func(rowIdx int, tr *goquery.Selection) {
				// Skip header row
				if rowIdx == 0 {
					return
				}

				// Find all td cells in this row
				tds := tr.Find("td")
				if tds.Length() >= 3 {
					// Get the 1st column (週別) for context
					weekCell := tds.Eq(0)
					weekText := strings.TrimSpace(weekCell.Text())

					// Get the 3rd column (教學預定進度)
					scheduleCell := tds.Eq(2)
					scheduleText := strings.TrimSpace(scheduleCell.Text())

					// Skip empty or special rows
					if scheduleText == "" || scheduleText == "彈性補充教學" {
						return
					}

					// Format: "Week X: Content" or just content if no week
					if weekText != "" && strings.HasPrefix(weekText, "Week") {
						scheduleItems = append(scheduleItems, weekText+": "+scheduleText)
					} else if scheduleText != "" {
						scheduleItems = append(scheduleItems, scheduleText)
					}
				}
			})
		})

		if len(scheduleItems) == 0 {
			return ""
		}

		return strings.Join(scheduleItems, "\n")
	}

	// Parse objectives
	objectivesCN, hasBothLanguages := findContent("教學目標")
	if hasBothLanguages {
		// Combined format: CN label already contains EN content (e.g., "教學目標 Course Objectives：...")
		fields.ObjectivesCN = objectivesCN
		// No separate EN field needed
	} else {
		fields.ObjectivesCN = objectivesCN
		// Separate format: look for English objectives in a different TD
		objectivesEN, _ := findContent("Course Objectives")
		fields.ObjectivesEN = objectivesEN
	}

	// Parse outline
	outlineCN, hasBothLanguages := findContent("內容綱要")
	if hasBothLanguages {
		// Combined format: CN label already contains EN content
		fields.OutlineCN = outlineCN
		// No separate EN field needed
	} else {
		fields.OutlineCN = outlineCN
		// Separate format: look for English outline in a different TD
		outlineEN, _ := findContent("Course Outline")
		fields.OutlineEN = outlineEN
	}

	// Parse schedule (special handling for table format)
	fields.Schedule = findScheduleContent()

	// If schedule extraction failed, try the old method as fallback
	if fields.Schedule == "" {
		scheduleContent, _ := findContent("教學進度")
		fields.Schedule = scheduleContent
	}

	// Clean up the content
	fields.ObjectivesCN = cleanContent(fields.ObjectivesCN)
	fields.ObjectivesEN = cleanContent(fields.ObjectivesEN)
	fields.OutlineCN = cleanContent(fields.OutlineCN)
	fields.OutlineEN = cleanContent(fields.OutlineEN)
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
