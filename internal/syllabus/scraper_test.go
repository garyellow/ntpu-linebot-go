package syllabus

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestCleanContent(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			got := cleanContent(tt.input)
			if got != tt.want {
				t.Errorf("cleanContent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSyllabusPage(t *testing.T) {
	t.Parallel()
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
			name: "merged format with font-c13 span",
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
			name: "separate format CN and EN objectives",
			html: `<html><body>
				<table>
					<tr><td>教學目標：<span class="font-c13">本課程介紹研習財務工程學所需之數學理論</span></td></tr>
					<tr><td>Course Objectives：<span class="font-c13">This course introduces the mathematical theory</span></td></tr>
					<tr><td>內容綱要：<span class="font-c13">1. Brownian Motion 2. Ito integral</span></td></tr>
					<tr><td>Course Outline：<span class="font-c13">1. Brownian Motion 2. Ito integral</span></td></tr>
				</table>
			</body></html>`,
			wantObjectives: "本課程介紹研習財務工程學所需之數學理論 This course introduces the mathematical theory",
			wantOutline:    "1. Brownian Motion 2. Ito integral 1. Brownian Motion 2. Ito integral",
		},
		{
			name: "separate format CN only",
			html: `<html><body>
				<table>
					<tr><td>教學目標：精進程式語言的的程式設計語法和實作技巧</td></tr>
					<tr><td>內容綱要：精進C語言的程式設計語法和實作技巧</td></tr>
				</table>
			</body></html>`,
			wantObjectives: "精進程式語言的的程式設計語法和實作技巧",
			wantOutline:    "精進C語言的程式設計語法和實作技巧",
		},
		{
			name: "separate format EN only",
			html: `<html><body>
				<table>
					<tr><td>Course Objectives：To practice basic ideas and techniques</td></tr>
					<tr><td>Course Outline：1. Introduction 2. Structured Program Development</td></tr>
				</table>
			</body></html>`,
			wantObjectives: "To practice basic ideas and techniques",
			wantOutline:    "1. Introduction 2. Structured Program Development",
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
			name: "schedule table filters flexible teaching rows",
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
		{
			name: "missing font-c13 class falls back to text content",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：培養批判性思維</td></tr>
					<tr><td>內容綱要/Course Outline：論證分析、邏輯推理</td></tr>
				</table>
			</body></html>`,
			wantObjectives: "培養批判性思維",
			wantOutline:    "論證分析、邏輯推理",
		},
		{
			name: "multiple font-c13 elements concatenated",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<span class="font-c13">目標一</span><span class="font-c13">目標二</span></td></tr>
				</table>
			</body></html>`,
			wantObjectives: "目標一目標二",
		},
		{
			name: "nested tables in schedule section",
			html: `<html><body>
				<table>
					<tr><td>教學進度(Teaching Schedule)：
						<table>
							<tr><td>週別</td><td>內容</td></tr>
							<tr>
								<td>Week 1</td>
								<td>
									<table><tr><td>子表格內容</td></tr></table>
								</td>
							</tr>
						</table>
					</td></tr>
				</table>
			</body></html>`,
			wantSchedule: "", // 因 header 驗證失敗應返回空
		},
		{
			name: "excessive whitespace normalized",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<span class="font-c13">   多餘    空白   測試   </span></td></tr>
					<tr><td>內容綱要/Course Outline：<span class="font-c13">

						多行

						換行

						測試

					</span></td></tr>
				</table>
			</body></html>`,
			wantObjectives: "多餘 空白 測試",
			wantOutline:    "多行\n\n換行\n\n測試",
		},
		{
			name: "HTML entities decoded",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<span class="font-c13">學習 C&amp;C++ &lt;程式設計&gt;</span></td></tr>
				</table>
			</body></html>`,
			wantObjectives: "學習 C&C++ <程式設計>",
		},
		{
			name: "mixed span and div with font-c13",
			html: `<html><body>
				<table>
					<tr><td>教學目標 Course Objectives：<span class="font-c13">Span內容</span><div class="font-c13">Div內容</div></td></tr>
				</table>
			</body></html>`,
			wantObjectives: "Span內容Div內容",
		},
		{
			name: "schedule with only 3 columns (no method column)",
			html: `<html><body>
				<table>
					<tr><td>教學進度(Teaching Schedule)：
						<table>
							<tr><td>週別</td><td>日期</td><td>教學預定進度</td></tr>
							<tr><td>Week 1</td><td>20250911</td><td>課程介紹</td></tr>
						</table>
					</td></tr>
				</table>
			</body></html>`,
			wantSchedule: "Week 1: 課程介紹",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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

func TestNewScraper(t *testing.T) {
	t.Parallel()
	// Test that NewScraper creates a valid scraper
	scraper := NewScraper(nil)
	if scraper == nil {
		t.Error("NewScraper(nil) returned nil, expected valid scraper")
	}
}

func TestParseProgramNamesFromDetailPage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		html         string
		wantCount    int
		wantPrograms []string
		wantNotIncl  []string
	}{
		{
			name:      "empty HTML",
			html:      `<html><body></body></html>`,
			wantCount: 0,
		},
		{
			name: "single program",
			html: `<html><body><table><tr>
				<td class="font-g13">
					應修系級 Major:<b class="font-c15">商業智慧與大數據分析學士學分學程 &nbsp;</b>
				</td>
			</tr></table></body></html>`,
			wantCount:    1,
			wantPrograms: []string{"商業智慧與大數據分析學士學分學程"},
		},
		{
			name: "multiple programs with departments",
			html: `<html><body><table><tr>
				<td class="font-g13">
					應修系級 Major:<b class="font-c15">統計學系3 ,統計學系4 ,資本市場鑑識學士學分學程 &nbsp;,商業智慧與大數據分析學士學分學程 &nbsp;,商業資料分析學士學分學程 &nbsp;,資料拓析學士學分學程 &nbsp;,金融科技與量化金融學士學分學程 &nbsp;,調查方法與資料分析學士學分學程 &nbsp;,經濟資料科學學士微學程 &nbsp;,資料拓析學士學分微學程 &nbsp;,金融科技與量化金融學士微學程 &nbsp;,商業人工智慧學士微學程 &nbsp;,</b>
				</td>
			</tr></table></body></html>`,
			wantCount: 10,
			wantPrograms: []string{
				"資本市場鑑識學士學分學程",
				"商業智慧與大數據分析學士學分學程",
				"商業資料分析學士學分學程",
				"資料拓析學士學分學程",
				"金融科技與量化金融學士學分學程",
				"調查方法與資料分析學士學分學程",
				"經濟資料科學學士微學程",
				"資料拓析學士學分微學程",
				"金融科技與量化金融學士微學程",
				"商業人工智慧學士微學程",
			},
			wantNotIncl: []string{"統計學系3", "統計學系4"}, // departments should be excluded
		},
		{
			name: "programs with HTML entities",
			html: `<html><body><table><tr>
				<td class="font-g13">
					應修系級 Major:<b class="font-c15">商業資料分析學士學分學程&nbsp;,資料拓析學士微學程&nbsp;</b>
				</td>
			</tr></table></body></html>`,
			wantCount: 2,
			wantPrograms: []string{
				"商業資料分析學士學分學程",
				"資料拓析學士微學程",
			},
		},
		{
			name: "no programs - only departments",
			html: `<html><body><table><tr>
				<td class="font-g13">
					應修系級 Major:<b class="font-c15">資訊工程學系1 ,資訊工程學系2 ,電機工程學系3</b>
				</td>
			</tr></table></body></html>`,
			wantCount: 0,
		},
		{
			name: "programs with embedded HTML",
			html: `<html><body><table><tr>
				<td class="font-g13">
					應修系級 Major:<b class="font-c15">統計學系3 <a href="#">有擋修</a>,統計學系4 <a href="#">有擋修</a>,資本市場鑑識學士學分學程 &nbsp;</b>
				</td>
			</tr></table></body></html>`,
			wantCount:    1,
			wantPrograms: []string{"資本市場鑑識學士學分學程"},
			wantNotIncl:  []string{"統計學系", "有擋修"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			programs := parseProgramNamesFromDetailPage(doc)

			if len(programs) != tt.wantCount {
				t.Errorf("Got %d programs, want %d", len(programs), tt.wantCount)
				for i, p := range programs {
					t.Logf("  Program %d: %s", i+1, p)
				}
			}

			// Check expected programs are included
			for _, want := range tt.wantPrograms {
				found := false
				for _, p := range programs {
					if p == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected program %q not found", want)
				}
			}

			// Check excluded items are not included
			for _, notWant := range tt.wantNotIncl {
				for _, p := range programs {
					if strings.Contains(p, notWant) {
						t.Errorf("Program %q should not be included (contains %q)", p, notWant)
					}
				}
			}
		})
	}
}
