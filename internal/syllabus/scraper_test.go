package syllabus

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

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
			name: "standard format with font-c13 span",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<span class="font-c13">培養程式設計能力</span></td></tr>
					<tr><td>內容綱要/Course Outline：<span class="font-c13">變數與資料型態</span></td></tr>
					<tr><td>教學進度(Teaching Schedule)：
						<table>
							<tr><td>週別</td><td>日期</td><td>教學預定進度</td><td>方法</td></tr>
							<tr><td>Week 1</td><td>20250911</td><td>課程介紹</td><td>講授</td></tr>
						</table>
					</td></tr>
				</table>
			</body></html>`,
			wantObjectives: "培養程式設計能力",
			wantOutline:    "變數與資料型態",
			wantSchedule:   "Week 1: 課程介紹",
		},
		{
			name: "standard format with font-c13 div",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<div class="font-c13">學習演算法設計</div></td></tr>
					<tr><td>內容綱要/Course Outline：<div class="font-c13">排序、搜尋、動態規劃</div></td></tr>
				</table>
			</body></html>`,
			wantObjectives: "學習演算法設計",
			wantOutline:    "排序、搜尋、動態規劃",
		},
		{
			name: "schedule table with 4 columns",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<span class="font-c13">學習演算法</span></td></tr>
					<tr><td>教學進度(Teaching Schedule)：
						<table>
							<tr>
								<td>週別/Weekly</td>
								<td>日期</td>
								<td>教學預定進度</td>
								<td>教學方法與教學活動</td>
							</tr>
							<tr>
								<td>Week 1</td>
								<td>20250911</td>
								<td>課程介紹與環境設定</td>
								<td>講授</td>
							</tr>
							<tr>
								<td>Week 2</td>
								<td>20250918</td>
								<td>基礎資料結構複習</td>
								<td>講授、實作</td>
							</tr>
						</table>
					</td></tr>
				</table>
			</body></html>`,
			wantObjectives: "學習演算法",
			wantSchedule:   "Week 1: 課程介紹與環境設定",
		},
		{
			name: "schedule table filters 彈性補充教學 rows",
			html: `<html><body>
				<table>
					<tr><td>教學進度(Teaching Schedule)：
						<table>
							<tr><td>週別</td><td>日期</td><td>教學預定進度</td><td>方法</td></tr>
							<tr><td>Week 1</td><td>20250911</td><td>課程介紹</td><td>講授</td></tr>
							<tr><td>Week 2</td><td>20250918</td><td>彈性補充教學</td><td>-</td></tr>
							<tr><td>Week 3</td><td>20250925</td><td>期中考</td><td>考試</td></tr>
						</table>
					</td></tr>
				</table>
			</body></html>`,
			wantSchedule: "Week 1: 課程介紹",
		},
		{
			name: "schedule table skips empty cells",
			html: `<html><body>
				<table>
					<tr><td>教學進度(Teaching Schedule)：
						<table>
							<tr><td>週別</td><td>日期</td><td>教學預定進度</td><td>方法</td></tr>
							<tr><td>Week 1</td><td>20250911</td><td>課程介紹</td><td>講授</td></tr>
							<tr><td>Week 2</td><td>20250918</td><td></td><td></td></tr>
						</table>
					</td></tr>
				</table>
			</body></html>`,
			wantSchedule: "Week 1: 課程介紹",
		},
		{
			name: "no schedule table when header invalid",
			html: `<html><body>
				<table>
					<tr><td>教學進度(Teaching Schedule)：
						<table>
							<tr><td>無效表頭</td></tr>
							<tr><td>第一週 課程介紹</td></tr>
						</table>
					</td></tr>
				</table>
			</body></html>`,
			wantSchedule: "",
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

func TestJoinNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{
			name: "both non-empty",
			a:    "中文內容",
			b:    "English content",
			want: "中文內容\nEnglish content",
		},
		{
			name: "a empty",
			a:    "",
			b:    "English content",
			want: "English content",
		},
		{
			name: "b empty",
			a:    "中文內容",
			b:    "",
			want: "中文內容",
		},
		{
			name: "both empty",
			a:    "",
			b:    "",
			want: "",
		},
		{
			name: "a only whitespace",
			a:    "   ",
			b:    "English content",
			want: "English content",
		},
		{
			name: "b only whitespace",
			a:    "中文內容",
			b:    "\t\n  ",
			want: "中文內容",
		},
		{
			name: "both whitespace",
			a:    "  ",
			b:    "\t\n",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinNonEmpty(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("joinNonEmpty(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
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
