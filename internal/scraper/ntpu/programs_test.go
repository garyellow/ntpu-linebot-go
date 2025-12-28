package ntpu

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestCleanProgramName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal name",
			input:    "金融科技學士學分學程",
			expected: "金融科技學士學分學程",
		},
		{
			name:     "Rename annotation",
			input:    "金融科技與量化金融學士學分學程（112-1更名，原名：金融科技學士學分學程)",
			expected: "金融科技與量化金融學士學分學程",
		},
		{
			name:     "Cross-school annotation",
			input:    "創新創業學士學分學程 ★跨校（北醫、北科大）★",
			expected: "創新創業學士學分學程",
		},
		{
			name:     "Mixed annotations",
			input:    "創新創業學士學分學程(104學年度更名，原名：創新產業管理學士學分學程) ★跨校（北醫、北科大）★",
			expected: "創新創業學士學分學程",
		},
		{
			name:     "Trailing whitespace",
			input:    "金融科技學士學分學程  ",
			expected: "金融科技學士學分學程",
		},
		{
			name:     "Leading whitespace",
			input:    "  金融科技學士學分學程",
			expected: "金融科技學士學分學程",
		},
		{
			name:     "Garbage after 學程",
			input:    "金融科技學士學分學程abc",
			expected: "金融科技學士學分學程",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanProgramName(tt.input)
			if got != tt.expected {
				t.Errorf("cleanProgramName() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExtractProgramsFromPage_Uniqueness(t *testing.T) {
	// Simulate HTML with a program link
	html := `
	<html>
		<body>
			<a href="board.php?courseID=28286&f=doc&cid=123" target="_blank">Duplicate Program 學程</a>
		</body>
	</html>
	`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	seen := make(map[string]bool)

	// First extraction with Category A
	resultsA, _ := extractProgramsFromPage(doc, seen, "Category A")
	if len(resultsA) != 1 {
		t.Fatalf("Expected 1 result for Category A, got %d", len(resultsA))
	}
	if resultsA[0].Name != "Duplicate Program 學程" {
		t.Errorf("Expected name 'Duplicate Program 學程', got '%s'", resultsA[0].Name)
	}
	if resultsA[0].Category != "Category A" {
		t.Errorf("Expected category 'Category A', got '%s'", resultsA[0].Category)
	}

	// Reset seen map to simulate extracting from a different folder context
	// In reality, 'seen' avoids duplicates WITHIN the same crawl if CIDs match.
	// However, different categories usually imply different folders, and thus different CIDs if the links differ.
	// If the CIDs are identical, they are the same underlying resource.
	// If the CIDs are different but names are same, we need to ensure they are distinct.

	// Let's simulate a second document with a different CID but same raw name
	html2 := `
	<html>
		<body>
			<a href="board.php?courseID=28286&f=doc&cid=456" target="_blank">Duplicate Program 學程</a>
		</body>
	</html>
	`
	doc2, _ := goquery.NewDocumentFromReader(strings.NewReader(html2))

	// Use SAME seen map, but new CID should be fresh
	resultsB, _ := extractProgramsFromPage(doc2, seen, "Category B")

	if len(resultsB) != 1 {
		t.Fatalf("Expected 1 result for Category B, got %d", len(resultsB))
	}
	if resultsB[0].Name != "Duplicate Program 學程" {
		t.Errorf("Expected name 'Duplicate Program 學程', got '%s'", resultsB[0].Name)
	}
	if resultsB[0].Category != "Category B" {
		t.Errorf("Expected category 'Category B', got '%s'", resultsB[0].Category)
	}
}

func TestPagination(t *testing.T) {
	seen := make(map[string]bool)

	// Test logic of `extractProgramsFromPage` for detecting "Next".

	htmlPage1 := `
		<a href="board.php?courseID=28286&f=doc&cid=111">Program 1</a>
		<a href="board.php?page=2">Next</a>
	`
	doc1, _ := goquery.NewDocumentFromReader(strings.NewReader(htmlPage1))
	_, hasNext1 := extractProgramsFromPage(doc1, seen, "")
	if !hasNext1 {
		t.Error("Expected hasNext=true for page 1")
	}

	htmlPage2 := `
		<a href="board.php?courseID=28286&f=doc&cid=222">Program 2</a>
	`
	doc2, _ := goquery.NewDocumentFromReader(strings.NewReader(htmlPage2))
	_, hasNext2 := extractProgramsFromPage(doc2, seen, "")
	if hasNext2 {
		t.Error("Expected hasNext=false for page 2")
	}
}
