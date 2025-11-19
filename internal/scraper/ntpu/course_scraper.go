package ntpu

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

const (
	// Base URL for course search
	courseBaseURL   = "https://sea.cc.ntpu.edu.tw"
	courseQueryPath = "/pls/dev_stud/course_query_all.queryByKeyword"
)

// Education level codes (U=大學部, M=碩士班, N=碩士在職專班, P=博士班)
var AllEduCodes = []string{"U", "M", "N", "P"}

// Classroom regex patterns
var classroomRegex = regexp.MustCompile(`(?:教室|上課地點)[:：為](.*?)(?:$|[ .，。；【])`)

// ScrapeCourses scrapes courses by year, term, and optional filters
// URL: https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.queryByKeyword
// Parameters: qYear, qTerm, courseno (optional), seq1=A, seq2=M
func ScrapeCourses(ctx context.Context, client *scraper.Client, year, term int, title string) ([]*storage.Course, error) {
	courses := make([]*storage.Course, 0)

	// Build base URL and parameters
	baseParams := fmt.Sprintf("?qYear=%d&qTerm=%d&seq1=A&seq2=M", year, term)

	// If title is provided, search specifically
	if title != "" {
		url := fmt.Sprintf("%s%s%s&title=%s", courseBaseURL, courseQueryPath, baseParams, title)
		doc, err := client.GetDocument(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch courses: %w", err)
		}
		return parseCoursesPage(doc, year, term), nil
	}

	// Otherwise, iterate through all education codes
	for _, eduCode := range AllEduCodes {
		url := fmt.Sprintf("%s%s%s&courseno=%s", courseBaseURL, courseQueryPath, baseParams, eduCode)

		doc, err := client.GetDocument(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch courses for code %s: %w", eduCode, err)
		}

		pageCourses := parseCoursesPage(doc, year, term)
		courses = append(courses, pageCourses...)
	}

	return courses, nil
}

// ScrapeCourseByUID scrapes a specific course by its UID (year+term+no)
// Example UID: 11312U123 (year=113, term=1, no=2U123)
func ScrapeCourseByUID(ctx context.Context, client *scraper.Client, uid string) (*storage.Course, error) {
	if len(uid) < 5 {
		return nil, fmt.Errorf("invalid course UID: %s", uid)
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
	url := fmt.Sprintf("%s%s?qYear=%d&qTerm=%d&courseno=%s&seq1=A&seq2=M",
		courseBaseURL, courseQueryPath, year, term, no)

	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch course: %w", err)
	}

	courses := parseCoursesPage(doc, year, term)
	if len(courses) == 0 {
		return nil, fmt.Errorf("course not found: %s", uid)
	}

	return courses[0], nil
}

// parseCoursesPage extracts course information from a search result page
func parseCoursesPage(doc *goquery.Document, year, term int) []*storage.Course {
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

		// Extract course number (field 3)
		no := strings.TrimSpace(tds.Eq(3).Text())

		// Extract title, detail URL, note, location (field 7)
		title, detailURL, note, location := parseTitleField(tds.Eq(7))

		// Extract teachers and teacher URLs (field 8)
		teachers, teacherURLs := parseTeacherField(tds.Eq(8))

		// Extract times and locations (field 13)
		times, locations := parseTimeLocationField(tds.Eq(13))

		// Add location from title field if present
		if location != "" {
			locations = append(locations, location)
		}

		// Generate UID
		uid := fmt.Sprintf("%d%d%s", year, term, no)

		// Build full detail URL
		fullDetailURL := ""
		if detailURL != "" {
			fullDetailURL = courseBaseURL + "/pls/dev_stud/course_query.queryGuide" + detailURL
		}

		course := &storage.Course{
			UID:       uid,
			Year:      year,
			Term:      term,
			No:        no,
			Title:     title,
			Teachers:  teachers,
			Times:     times,
			Locations: locations,
			DetailURL: fullDetailURL,
			Note:      note,
			CachedAt:  cachedAt,
		}

		// Also store teacher URLs in Note if needed (or extend storage model)
		if len(teacherURLs) > 0 {
			// For now, we'll just keep them in memory
			// Could extend storage.Course to include TeacherURLs []string
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
	font := td.Find("font")
	if font.Length() > 0 {
		noteText := font.Text()
		if len(noteText) > 3 {
			note = strings.TrimSpace(noteText[3:])

			// Extract location from note using regex
			if matches := classroomRegex.FindStringSubmatch(note); len(matches) > 1 {
				location = strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(matches[1], " "))
			}
		}
	}

	return
}

// parseTeacherField parses the teacher field to extract teacher names and URLs
func parseTeacherField(td *goquery.Selection) (teachers []string, teacherURLs []string) {
	teachers = make([]string, 0)
	teacherURLs = make([]string, 0)

	td.Find("a").Each(func(i int, a *goquery.Selection) {
		teacherName := strings.TrimSpace(a.Text())
		teachers = append(teachers, teacherName)

		// Get teacher URL
		href, _ := a.Attr("href")
		if href != "" {
			parts := strings.Split(href, "?")
			if len(parts) > 1 {
				teacherURL := courseBaseURL + "/pls/faculty/tec_course_table.s_table?" + parts[1]
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
