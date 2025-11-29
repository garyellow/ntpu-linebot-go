package syllabus

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple tag",
			input: "<p>Hello</p>",
			want:  " Hello ",
		},
		{
			name:  "nested tags",
			input: "<div><p>Hello <b>World</b></p></div>",
			want:  "  Hello  World   ", // Each tag becomes a space
		},
		{
			name:  "script tag",
			input: "<script>alert('xss')</script>Content",
			want:  "Content",
		},
		{
			name:  "style tag",
			input: "<style>.class{color:red}</style>Content",
			want:  "Content",
		},
		{
			name:  "HTML entities",
			input: "&nbsp;&lt;tag&gt;&amp;",
			want:  " <tag>&",
		},
		{
			name:  "br tags",
			input: "Line1<br>Line2<br/>Line3",
			want:  "Line1 Line2 Line3",
		},
		{
			name:  "no tags",
			input: "Plain text",
			want:  "Plain text",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			if got != tt.want {
				t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "simple text",
			input: "Hello World",
			want:  "Hello World",
		},
		{
			name:  "multiple spaces",
			input: "Hello    World",
			want:  "Hello World",
		},
		{
			name:  "tabs",
			input: "Hello\t\tWorld",
			want:  "Hello World",
		},
		{
			name:  "multiple newlines",
			input: "Line1\n\n\n\nLine2",
			want:  "Line1\n\nLine2",
		},
		{
			name:  "CRLF normalization",
			input: "Line1\r\nLine2\rLine3",
			want:  "Line1\nLine2\nLine3",
		},
		{
			name:  "leading/trailing spaces on lines",
			input: "  Line1  \n  Line2  ",
			want:  "Line1\nLine2",
		},
		{
			name:  "empty lines at start and end",
			input: "\n\nContent\n\n",
			want:  "Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanContent(tt.input)
			if got != tt.want {
				t.Errorf("cleanContent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractContentAfterLabel(t *testing.T) {
	tests := []struct {
		name  string
		html  string
		label string
		want  string
	}{
		{
			name:  "basic extraction",
			html:  `<span>教學目標：</span>培養程式設計能力`,
			label: "教學目標",
			want:  "培養程式設計能力",
		},
		{
			name:  "with colon variant",
			html:  `教學目標:培養程式設計能力`,
			label: "教學目標",
			want:  "培養程式設計能力",
		},
		{
			name:  "label not found",
			html:  `<span>其他內容</span>`,
			label: "教學目標",
			want:  "",
		},
		{
			name:  "stops at next label",
			html:  `教學目標：培養能力 內容綱要：課程大綱`,
			label: "教學目標",
			want:  "培養能力",
		},
		{
			name:  "with HTML tags",
			html:  `教學目標：<br/>培養<b>程式</b>能力`,
			label: "教學目標",
			want:  "培養 程式 能力",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContentAfterLabel(tt.html, tt.label)
			// Clean up whitespace for comparison
			got = strings.TrimSpace(got)
			want := strings.TrimSpace(tt.want)
			if got != want {
				t.Errorf("extractContentAfterLabel() = %q, want %q", got, want)
			}
		})
	}
}

func TestParseSyllabusPage(t *testing.T) {
	tests := []struct {
		name           string
		html           string
		wantObjectives string
		wantOutline    string
		wantSchedule   string
		wantEmpty      bool
	}{
		{
			name:      "empty HTML",
			html:      `<html><body></body></html>`,
			wantEmpty: true,
		},
		{
			name: "basic syllabus structure with span.font-c13",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<span class="font-c13">培養程式設計能力</span></td></tr>
					<tr><td>內容綱要/Course Outline：<span class="font-c13">變數與資料型態</span></td></tr>
					<tr><td>教學進度(Teaching Schedule)：<table><tr><td>第1週 課程介紹</td></tr></table></td></tr>
				</table>
			</body></html>`,
			wantObjectives: "培養程式設計能力",
			wantOutline:    "變數與資料型態",
			wantSchedule:   "第1週 課程介紹",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			fields := parseSyllabusPage(doc)

			if tt.wantEmpty {
				if !fields.IsEmpty() {
					t.Errorf("Expected empty fields, got: %+v", fields)
				}
				return
			}

			// Check if expected content is present (may have some variation due to cleaning)
			if tt.wantObjectives != "" && !strings.Contains(fields.Objectives, tt.wantObjectives) {
				t.Errorf("Objectives = %q, want to contain %q", fields.Objectives, tt.wantObjectives)
			}
			if tt.wantOutline != "" && !strings.Contains(fields.Outline, tt.wantOutline) {
				t.Errorf("Outline = %q, want to contain %q", fields.Outline, tt.wantOutline)
			}
			if tt.wantSchedule != "" && !strings.Contains(fields.Schedule, tt.wantSchedule) {
				t.Errorf("Schedule = %q, want to contain %q", fields.Schedule, tt.wantSchedule)
			}
		})
	}
}

func TestNewScraper(t *testing.T) {
	// Test that NewScraper creates a valid scraper
	scraper := NewScraper(nil)
	if scraper == nil {
		t.Error("NewScraper(nil) returned nil, expected valid scraper")
	}
}
