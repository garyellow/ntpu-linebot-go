package ntpu

import (
	"context"
	"fmt"
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

// AllEduCodes contains education level codes (U=大學部, M=碩士班, N=碩士在職專班, P=博士班)
var AllEduCodes = []string{"U", "M", "N", "P"}

// Classroom regex patterns
var classroomRegex = regexp.MustCompile(`(?:教室|上課地點)[:：為](.*?)(?:$|[ .，。；【])`)

// ScrapeCoursesByYear scrapes ALL courses for a given year (both semesters)
// This is a convenience wrapper around ScrapeCourses with term=0 and empty title
// Note: Current warmup uses per-semester scraping (ScrapeCourses) for precise control
func ScrapeCoursesByYear(ctx context.Context, client *scraper.Client, year int) ([]*storage.Course, error) {
	return ScrapeCourses(ctx, client, year, 0, "")
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
	for _, eduCode := range AllEduCodes {
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

		// Extract program requirements from field 5 (應修系級) and field 6 (必選修別)
		programs := parseProgramFields(tds.Eq(5), tds.Eq(6))

		// Extract title, detail URL, note, location (field 7)
		title, detailURL, note, location := parseTitleField(tds.Eq(7))

		// Skip courses without a title (parsing error or invalid data)
		if title == "" {
			slog.DebugContext(ctx, "skipping course with empty title",
				"year", year,
				"term", term,
				"courseNo", strings.TrimSpace(tds.Eq(3).Text()))
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
			UID:         uid,
			Year:        year,
			Term:        rowTerm,
			No:          no,
			Title:       title,
			Teachers:    teachers,
			TeacherURLs: teacherURLs,
			Times:       times,
			Locations:   locations,
			DetailURL:   fullDetailURL,
			Note:        note,
			Programs:    programs,
			CachedAt:    cachedAt,
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

// parseProgramFields parses field 5 (應修系級) and field 6 (必選修別) to extract program requirements.
// Only items ending with "學程" are included (e.g., "智慧財產權學士學分學程", "智慧財產權學士微學程").
// Each row in field 5 corresponds to a row in field 6.
//
// HTML format:
//
//	Field 5: <p align="left">智慧財產權學士學分學程 &nbsp;<br>電機系1 &nbsp;<br></p>
//	Field 6: 必<br>必<br>
func parseProgramFields(td5, td6 *goquery.Selection) []storage.ProgramRequirement {
	programs := make([]storage.ProgramRequirement, 0)

	// Get raw HTML and split by <br> to get individual items
	// Field 5: 應修系級 - contains department/program names
	field5HTML, _ := td5.Html()
	field5Items := splitByBR(field5HTML)

	// Field 6: 必選修別 - contains course types (必/選/通/etc.)
	field6HTML, _ := td6.Html()
	field6Items := splitByBR(field6HTML)

	// Match items from both fields
	for i, item := range field5Items {
		// Clean up the item: remove &nbsp;, trim whitespace
		item = strings.ReplaceAll(item, "&nbsp;", "")
		item = strings.ReplaceAll(item, "\u00a0", "") // non-breaking space
		item = strings.TrimSpace(item)

		// Only include items ending with "學程"
		if !strings.HasSuffix(item, "學程") {
			continue
		}

		// Get corresponding course type (default to "選" if not available)
		courseType := "選"
		if i < len(field6Items) {
			courseType = strings.TrimSpace(field6Items[i])
			if courseType == "" {
				courseType = "選"
			}
		}

		programs = append(programs, storage.ProgramRequirement{
			ProgramName: cleanProgramName(item),
			CourseType:  courseType,
		})
	}

	return programs
}

// splitByBR splits HTML content by <br> tags and returns cleaned text items.
func splitByBR(html string) []string {
	// Remove <p> tags and other wrapper elements
	html = regexp.MustCompile(`<p[^>]*>`).ReplaceAllString(html, "")
	html = strings.ReplaceAll(html, "</p>", "")

	// Split by <br> variants: <br>, <br/>, <br />
	parts := regexp.MustCompile(`<br\s*/?>`).Split(html, -1)

	items := make([]string, 0, len(parts))
	for _, part := range parts {
		// Strip any remaining HTML tags
		part = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(part, "")
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}

	return items
}
