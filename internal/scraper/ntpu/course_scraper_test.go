package ntpu

import (
	"regexp"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestAllEduCodes tests if all education codes are present
func TestAllEduCodes(t *testing.T) {
	t.Parallel()
	expectedCodes := []string{"U", "M", "N", "P"}

	if len(allEducationCodes) != len(expectedCodes) {
		t.Errorf("Expected %d education codes, got %d", len(expectedCodes), len(allEducationCodes))
	}

	for _, code := range expectedCodes {
		found := false
		for _, allCode := range allEducationCodes {
			if allCode == code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected education code %q not found", code)
		}
	}
}

// TestClassroomRegex tests the classroom regex pattern
func TestClassroomRegex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		matches bool
	}{
		{
			name:    "Valid classroom format",
			input:   "8F12",
			matches: true,
		},
		{
			name:    "Valid with B prefix",
			input:   "B123",
			matches: true,
		},
		{
			name:    "Valid multi-digit floor",
			input:   "12F01",
			matches: true,
		},
		{
			name:    "Invalid format",
			input:   "ABC",
			matches: false,
		},
	}

	// The regex pattern from the scraper
	classroomRegex := regexp.MustCompile(`[0-9]*[FB]?[0-9]+`)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			match := classroomRegex.MatchString(tt.input)
			if match != tt.matches {
				t.Errorf("Expected match=%v for %q, got %v", tt.matches, tt.input, match)
			}
		})
	}
}

func TestParseMajorAndTypeFields(t *testing.T) {
	t.Parallel()
	html := `<html><body>
	<table><tbody><tr>
		<td></td><td></td><td></td><td></td><td></td>
		<td>
			商業智慧與大數據分析學程&nbsp;<br>
			資工系3 <a href="#">有擋修</a><br>
			人工智慧學士學分學程<br>
		</td>
		<td>
			必<br>
			必修<br>
			選修<br>
		</td>
	</tr></tbody></table>
	</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	row := doc.Find("table tbody tr").First()
	tds := row.Find("td")
	if tds.Length() < 7 {
		t.Fatalf("Expected at least 7 columns, got %d", tds.Length())
	}

	got := parseMajorAndTypeFields(tds.Eq(5), tds.Eq(6))
	if len(got) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(got))
	}

	if got[0].Name != "商業智慧與大數據分析學程" || got[0].CourseType != "必" {
		t.Errorf("Item 0 = %#v, want name=%q type=%q", got[0], "商業智慧與大數據分析學程", "必")
	}
	if got[1].Name != "資工系3" || got[1].CourseType != "必" {
		t.Errorf("Item 1 = %#v, want name=%q type=%q", got[1], "資工系3", "必")
	}
	if got[2].Name != "人工智慧學士學分學程" || got[2].CourseType != "選" {
		t.Errorf("Item 2 = %#v, want name=%q type=%q", got[2], "人工智慧學士學分學程", "選")
	}
}

// Note: UID parsing logic is tested in the course handler module.
// Scraper tests focus on format validation and regex patterns only.
// Course name extraction uses standard library strings.TrimSpace - no need to test stdlib.
