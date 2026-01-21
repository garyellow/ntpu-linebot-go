package ntpu

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

const (
	courseQueryByKeywordPath       = "/pls/dev_stud/course_query_all.queryByKeyword"
	courseQueryByAllConditionsPath = "/pls/dev_stud/course_query_all.queryByAllConditions"

	// User-facing URLs use domain (not IP) for better UX
	// Scraper uses IP for efficiency, but generated URLs should be domain-based
	seaUserFacingURL = "https://sea.cc.ntpu.edu.tw"
)

// allEducationCodes contains education level codes (U=大學部, M=碩士班, N=碩士在職專班, P=博士班)
var allEducationCodes = []string{"U", "M", "N", "P"}

// Regex patterns for parsing course list page
var (
	classroomRegex = regexp.MustCompile(`(?:教室|上課地點)[:：為](.*?)(?:$|[ .，。；【])`)
	reBRTag        = regexp.MustCompile(`(?i)<br\s*/?>`)
	reHTMLTags     = regexp.MustCompile(`<[^>]*>`)
)

// ScrapeCoursesByYear scrapes ALL courses for a given year (both semesters)
// This is a convenience wrapper around ScrapeCourses with term=0 and empty title
// Note: Current warmup uses per-semester scraping (ScrapeCourses) for precise control
func ScrapeCoursesByYear(ctx context.Context, client *scraper.Client, year int) ([]*storage.Course, error) {
	return ScrapeCourses(ctx, client, year, 0, "")
}

// ProbeCoursesExist performs a lightweight check if courses exist for a semester.
// Uses a single education code (U = undergraduate) to minimize HTTP requests.
// Returns true if any courses are found, false otherwise.
// This is specifically designed for warmup probing and should not be used for data collection.
func ProbeCoursesExist(ctx context.Context, client *scraper.Client, year, term int) (bool, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("context canceled before probing courses: %w", err)
	}

	// Get working base URL with failover support
	courseBaseURL, err := seaCache(ctx, client)
	if err != nil {
		return false, fmt.Errorf("failed to get working SEA URL: %w", err)
	}

	// Use only U (undergraduate) education code for lightweight probing
	// Format: {baseURL}/pls/dev_stud/course_query_all.queryByKeyword?qYear={year}&qTerm={term}&seq1=A&seq2=M&courseno=U
	queryURL := fmt.Sprintf("%s%s?qYear=%d&qTerm=%d&seq1=A&seq2=M&courseno=U",
		courseBaseURL, courseQueryByKeywordPath, year, term)

	doc, err := client.GetDocument(ctx, queryURL)
	if err != nil {
		// Try to recover with failover if needed
		if scraper.IsNetworkError(err) {
			newURL, failoverErr := seaCache(ctx, client)
			if failoverErr == nil && newURL != courseBaseURL {
				queryURL = fmt.Sprintf("%s%s?qYear=%d&qTerm=%d&seq1=A&seq2=M&courseno=U",
					newURL, courseQueryByKeywordPath, year, term)
				doc, err = client.GetDocument(ctx, queryURL)
			}
		}
		if err != nil {
			return false, fmt.Errorf("failed to probe courses: %w", err)
		}
	}

	// Parse the page to check if any courses exist
	courses := parseCoursesPage(ctx, doc, year, term)
	return len(courses) > 0, nil
}

// ScrapeCourses scrapes courses by year, term, and optional filters
// For title search: uses POST to {baseURL}/pls/dev_stud/course_query_all.queryByAllConditions with 'cour' parameter
// For general query: uses GET to {baseURL}/pls/dev_stud/course_query_all.queryByKeyword with 'courseno' parameter
// When term=0, queries both semesters at once (more efficient for historical searches)
// Supports automatic URL failover across multiple SEA endpoints
func ScrapeCourses(ctx context.Context, client *scraper.Client, year, term int, title string) ([]*storage.Course, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping courses: %w", err)
	}

	var courses []*storage.Course

	// Get working base URL with failover support
	courseBaseURL, err := seaCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}

	// If title is provided, use POST to queryByAllConditions endpoint with 'cour' parameter
	if title != "" {
		queryURL := fmt.Sprintf("%s%s", courseBaseURL, courseQueryByAllConditionsPath)

		// Encode title to Big5 for SEA system compatibility
		big5Title, err := encodeToBig5(title)
		if err != nil {
			return nil, fmt.Errorf("failed to encode title to Big5: %w", err)
		}

		// Build POST form data with URL-encoded Big5 title
		// When term=0, omit qTerm to query both semesters at once
		var formData string
		if term == 0 {
			formData = fmt.Sprintf("qYear=%d&cour=%s&seq1=A&seq2=M",
				year, url.QueryEscape(big5Title))
		} else {
			formData = fmt.Sprintf("qYear=%d&qTerm=%d&cour=%s&seq1=A&seq2=M",
				year, term, url.QueryEscape(big5Title))
		}

		doc, err := client.PostFormDocumentRaw(ctx, queryURL, formData)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch courses: %w", err)
		}

		return parseCoursesPage(ctx, doc, year, term), nil
	}

	// Otherwise, use GET to queryByKeyword and iterate through all education codes
	// When term=0, omit qTerm to query both semesters
	var baseParams string
	if term == 0 {
		baseParams = fmt.Sprintf("?qYear=%d&seq1=A&seq2=M", year)
	} else {
		baseParams = fmt.Sprintf("?qYear=%d&qTerm=%d&seq1=A&seq2=M", year, term)
	}

	var lastErr error
	for _, eduCode := range allEducationCodes {
		// Check context before each request
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context canceled before scraping courses: %w", err)
		}

		queryURL := fmt.Sprintf("%s%s%s&courseno=%s", courseBaseURL, courseQueryByKeywordPath, baseParams, eduCode)
		doc, err := client.GetDocument(ctx, queryURL)
		if err != nil {
			// Try to recover with failover if needed
			if scraper.IsNetworkError(err) {
				newURL, failoverErr := seaCache(ctx, client)
				if failoverErr == nil && newURL != courseBaseURL {
					queryURL = fmt.Sprintf("%s%s%s&courseno=%s", newURL, courseQueryByKeywordPath, baseParams, eduCode)
					doc, err = client.GetDocument(ctx, queryURL)
				}
			}
		}

		if err != nil {
			lastErr = err
			continue
		}

		if newCourses := parseCoursesPage(ctx, doc, year, term); len(newCourses) > 0 {
			courses = append(courses, newCourses...)
		}
	}

	if len(courses) == 0 && lastErr != nil {
		return nil, fmt.Errorf("failed to fetch courses (last error): %w", lastErr)
	}

	return courses, nil
}

// ScrapeCoursesByTeacher scrapes courses by teacher name
// Uses POST to {baseURL}/pls/dev_stud/course_query_all.queryByAllConditions with 'teach' parameter
func ScrapeCoursesByTeacher(ctx context.Context, client *scraper.Client, year, term int, teacherName string) ([]*storage.Course, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping courses: %w", err)
	}

	// Get working base URL with failover support
	courseBaseURL, err := seaCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}

	queryURL := fmt.Sprintf("%s%s", courseBaseURL, courseQueryByAllConditionsPath)

	// Encode teacher name to Big5
	big5Teach, err := encodeToBig5(teacherName)
	if err != nil {
		return nil, fmt.Errorf("failed to encode teacher name to Big5: %w", err)
	}

	// Build POST form data with URL-encoded Big5 teacher name
	// When term=0, omit qTerm to query both semesters at once
	var formData string
	if term == 0 {
		formData = fmt.Sprintf("qYear=%d&teach=%s&seq1=A&seq2=M",
			year, url.QueryEscape(big5Teach))
	} else {
		formData = fmt.Sprintf("qYear=%d&qTerm=%d&teach=%s&seq1=A&seq2=M",
			year, term, url.QueryEscape(big5Teach))
	}

	doc, err := client.PostFormDocumentRaw(ctx, queryURL, formData)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch courses by teacher: %w", err)
	}

	return parseCoursesPage(ctx, doc, year, term), nil
}

// ScrapeCourseByUID scrapes a specific course by its UID (year+term+no)
// Example UID: 11312U123 (year=113, term=1, no=2U123)
// Supports automatic URL failover across multiple SEA endpoints
func ScrapeCourseByUID(ctx context.Context, client *scraper.Client, uid string) (*storage.Course, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before scraping course: %w", err)
	}

	if len(uid) < 5 {
		return nil, fmt.Errorf("invalid course UID: %s", uid)
	}

	// Get working base URL with failover support
	courseBaseURL, err := seaCache(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get working SEA URL: %w", err)
	}

	// Parse UID
	isOver99 := len(uid) >= 9

	var year, term int
	var no string

	if isOver99 {
		year, _ = strconv.Atoi(uid[:3])
		term, _ = strconv.Atoi(uid[3:4])
		no = uid[4:]
	} else {
		year, _ = strconv.Atoi(uid[:2])
		term, _ = strconv.Atoi(uid[2:3])
		no = uid[3:]
	}

	// Build query URL
	queryURL := fmt.Sprintf("%s%s?qYear=%d&qTerm=%d&courseno=%s&seq1=A&seq2=M",
		courseBaseURL, courseQueryByKeywordPath, year, term, no)

	doc, err := client.GetDocument(ctx, queryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch course: %w", err)
	}

	courses := parseCoursesPage(ctx, doc, year, term)
	if len(courses) == 0 {
		return nil, fmt.Errorf("course not found: %s", uid)
	}

	return courses[0], nil
}

// parseCoursesPage extracts course information from a search result page
// When term=0, extracts term from each row (field 2); otherwise uses the provided term value
func parseCoursesPage(ctx context.Context, doc *goquery.Document, year, term int) []*storage.Course {
	courses := make([]*storage.Course, 0)
	cachedAt := time.Now().Unix()

	// Find course table
	table := doc.Find("table")
	if table.Length() == 0 {
		return courses
	}

	// Parse each course row in tbody
	table.Find("tbody tr").Each(func(i int, tr *goquery.Selection) {
		tds := tr.Find("td")
		if tds.Length() < 14 {
			return
		}

		// Determine term: extract from row if term=0, otherwise use provided value
		rowTerm := term
		if term == 0 {
			termStr := strings.TrimSpace(tds.Eq(2).Text())
			if parsed, err := strconv.Atoi(termStr); err == nil && parsed > 0 {
				rowTerm = parsed
			} else {
				rowTerm = 1 // Default to first semester
			}
		}

		// Extract course number (field 3)
		no := strings.TrimSpace(tds.Eq(3).Text())

		// Extract raw program requirements from columns 5 (應修系級) and 6 (必選修別)
		// These are paired 1:1 by <br> tags in each column.
		// Used by syllabus scraper to match with full program names from detail page.
		rawProgramReqs := parseMajorAndTypeFields(tds.Eq(5), tds.Eq(6))

		// Extract title, detail URL, note, location (field 7)
		title, detailURL, note, location := parseTitleField(tds.Eq(7))

		// Skip courses without a title (parsing error or invalid data)
		if title == "" {
			slog.DebugContext(ctx, "Skipping course with empty title",
				"year", year,
				"term", term,
				"course_no", strings.TrimSpace(tds.Eq(3).Text()))
			return
		}

		// Extract teachers and teacher URLs (field 8)
		teachers, teacherURLs := parseTeacherField(tds.Eq(8))

		// Extract times and locations (field 13)
		times, locations := parseTimeLocationField(tds.Eq(13))

		// Add location from title field if present
		if location != "" {
			locations = append(locations, location)
		}

		// Generate UID
		uid := fmt.Sprintf("%d%d%s", year, rowTerm, no)

		// Build full detail URL with show_info=all for complete syllabus data
		// Original detailURL format: "?g_serial=U3556&g_year=114&g_term=1"
		// Target format: queryguide?g_year=114&g_term=1&g_serial=U3556&show_info=all
		fullDetailURL := ""
		if detailURL != "" {
			fullDetailURL = seaUserFacingURL + "/pls/dev_stud/course_query.queryguide" + detailURL + "&show_info=all"
		}

		course := &storage.Course{
			UID:            uid,
			Year:           year,
			Term:           rowTerm,
			No:             no,
			Title:          title,
			Teachers:       teachers,
			TeacherURLs:    teacherURLs,
			Times:          times,
			Locations:      locations,
			DetailURL:      fullDetailURL,
			Note:           note,
			CachedAt:       cachedAt,
			RawProgramReqs: rawProgramReqs,
		}

		courses = append(courses, course)
	})

	return courses
}

// parseTitleField parses the title field to extract title, detail URL, note, and location
func parseTitleField(td *goquery.Selection) (title, detailURL, note, location string) {
	// Get title from <a> tag
	link := td.Find("a")
	title = strings.TrimSpace(link.Text())

	// Get detail URL from href
	href, _ := link.Attr("href")
	if href != "" {
		parts := strings.Split(href, "?")
		if len(parts) > 1 {
			detailURL = "?" + parts[1]
		}
	}

	// Get note from <font> tag
	// Format: "備註：..." where "備註：" is 3 runes (9 bytes in UTF-8)
	font := td.Find("font")
	if font.Length() > 0 {
		noteText := font.Text()
		const notePrefix = "備註："
		if strings.HasPrefix(noteText, notePrefix) {
			note = strings.TrimSpace(noteText[len(notePrefix):])

			// Extract location from note using regex
			if matches := classroomRegex.FindStringSubmatch(note); len(matches) > 1 {
				location = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(matches[1], " "))
			}
		}
	}

	return
}

// parseTeacherField parses the teacher field to extract teacher names and URLs
// URLs are hard-coded to domain for user-facing display
func parseTeacherField(td *goquery.Selection) (teachers []string, teacherURLs []string) {
	teachers = make([]string, 0)
	teacherURLs = make([]string, 0)

	td.Find("a").Each(func(i int, a *goquery.Selection) {
		teacherName := strings.TrimSpace(a.Text())
		teachers = append(teachers, teacherName)

		// Get teacher URL (use domain for user-facing URLs)
		href, _ := a.Attr("href")
		if href != "" {
			parts := strings.Split(href, "?")
			if len(parts) > 1 {
				teacherURL := seaUserFacingURL + "/pls/faculty/tec_course_table.s_table?" + parts[1]
				teacherURLs = append(teacherURLs, teacherURL)
			}
		}
	})

	return
}

// parseTimeLocationField parses the time and location field to extract times and locations
func parseTimeLocationField(td *goquery.Selection) (times []string, locations []string) {
	times = make([]string, 0)
	locations = make([]string, 0)

	td.Find("a").Each(func(i int, a *goquery.Selection) {
		lineInfo := strings.TrimSpace(a.Text())

		// Skip "每週未維護" entries
		if strings.Contains(lineInfo, "每週未維護") {
			return
		}

		// Split by tab to get time and location
		infos := strings.SplitN(lineInfo, "\t", 2)
		if len(infos) > 0 {
			times = append(times, strings.TrimSpace(infos[0]))
		}
		if len(infos) > 1 {
			locations = append(locations, strings.TrimSpace(infos[1]))
		}
	})

	return
}

// parseMajorAndTypeFields extracts program/department requirements from columns 5 and 6.
// Column 5 (應修系級) and Column 6 (必選修別) are paired by <br> tags.
// Returns slice of RawProgramReq with (name, type) pairs.
func parseMajorAndTypeFields(majorTD, typeTD *goquery.Selection) []storage.RawProgramReq {
	// Get HTML content and split by <br>
	majorHTML, _ := majorTD.Html()
	typeHTML, _ := typeTD.Html()

	// Split by <br> tag (case insensitive)
	majorParts := splitByBR(majorHTML)
	typeParts := splitByBR(typeHTML)

	// Pair them up (use shorter length to avoid index out of bounds)
	n := minInt(len(majorParts), len(typeParts))
	result := make([]storage.RawProgramReq, 0, n)
	for i := range n {
		name := cleanMajorName(majorParts[i])
		courseType := cleanCourseType(typeParts[i])

		if name == "" || courseType == "" {
			continue
		}

		result = append(result, storage.RawProgramReq{
			Name:       name,
			CourseType: courseType,
		})
	}

	return result
}

// splitByBR splits HTML content by <br> tags (handles <br>, <br/>, <br />).
func splitByBR(htmlContent string) []string {
	// Replace all <br> variants with a delimiter
	normalized := reBRTag.ReplaceAllString(htmlContent, "\n")

	parts := strings.Split(normalized, "\n")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// cleanMajorName removes HTML tags, &nbsp;, and extra whitespace from a major/program name.
func cleanMajorName(s string) string {
	// Remove HTML tags (e.g., <a>, <img> for prerequisites)
	s = reHTMLTags.ReplaceAllString(s, "")
	// Decode HTML entities (&nbsp;, &amp;, etc.)
	s = html.UnescapeString(s)
	// Remove non-breaking space character (from &nbsp;)
	s = strings.ReplaceAll(s, "\u00a0", "")
	// Remove common inline annotations from list page
	for _, token := range []string{"有擋修", "有限制"} {
		s = strings.ReplaceAll(s, token, "")
	}
	// Trim whitespace
	return strings.TrimSpace(s)
}

// cleanCourseType extracts course type (必/選) from a string.
func cleanCourseType(s string) string {
	s = strings.TrimSpace(s)
	// Remove any HTML tags
	s = reHTMLTags.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)

	// Accept variations like "必修"/"選修"
	if strings.Contains(s, "必") {
		return "必"
	}
	if strings.Contains(s, "選") {
		return "選"
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
