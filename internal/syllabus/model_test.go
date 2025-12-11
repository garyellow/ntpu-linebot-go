package syllabus

import (
	"fmt"
	"strings"
	"testing"
)

func TestComputeContentHash(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantLen int // SHA256 produces 64 hex characters
	}{
		{
			name:    "empty content",
			content: "",
			wantLen: 64,
		},
		{
			name:    "simple content",
			content: "教學目標：學習程式設計基礎",
			wantLen: 64,
		},
		{
			name:    "multiline content",
			content: "教學目標：學習程式設計\n內容綱要：變數、迴圈、函數",
			wantLen: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := ComputeContentHash(tt.content)
			if len(hash) != tt.wantLen {
				t.Errorf("ComputeContentHash() hash length = %d, want %d", len(hash), tt.wantLen)
			}
		})
	}
}

func TestComputeContentHash_Deterministic(t *testing.T) {
	content := "教學目標：本課程旨在培養學生程式設計能力"

	hash1 := ComputeContentHash(content)
	hash2 := ComputeContentHash(content)

	if hash1 != hash2 {
		t.Errorf("ComputeContentHash() not deterministic: %s != %s", hash1, hash2)
	}
}

func TestComputeContentHash_DifferentContent(t *testing.T) {
	content1 := "教學目標：學習程式設計"
	content2 := "教學目標：學習資料結構"

	hash1 := ComputeContentHash(content1)
	hash2 := ComputeContentHash(content2)

	if hash1 == hash2 {
		t.Errorf("ComputeContentHash() same hash for different content")
	}
}

func TestContentNeedsUpdate(t *testing.T) {
	// Test the concept of detecting content changes using hash comparison
	content := "教學目標：學習程式設計基礎"
	currentHash := ComputeContentHash(content)

	tests := []struct {
		name       string
		newContent string
		wantUpdate bool
	}{
		{
			name:       "same content",
			newContent: content,
			wantUpdate: false,
		},
		{
			name:       "different content",
			newContent: "教學目標：學習資料結構",
			wantUpdate: true,
		},
		{
			name:       "empty new content",
			newContent: "",
			wantUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newHash := ComputeContentHash(tt.newContent)
			needsUpdate := currentHash != newHash
			if needsUpdate != tt.wantUpdate {
				t.Errorf("hash comparison for %q: got needsUpdate=%v, want %v", tt.newContent, needsUpdate, tt.wantUpdate)
			}
		})
	}
}

func TestFields_IsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		fields Fields
		want   bool
	}{
		{
			name:   "all empty",
			fields: Fields{},
			want:   true,
		},
		{
			name: "objectives_cn set",
			fields: Fields{
				ObjectivesCN: "培養能力",
			},
			want: false,
		},
		{
			name: "objectives_en set",
			fields: Fields{
				ObjectivesEN: "Learn skills",
			},
			want: false,
		},
		{
			name: "outline_cn set",
			fields: Fields{
				OutlineCN: "課程內容",
			},
			want: false,
		},
		{
			name: "outline_en set",
			fields: Fields{
				OutlineEN: "Course content",
			},
			want: false,
		},
		{
			name: "schedule set",
			fields: Fields{
				Schedule: "進度表",
			},
			want: false,
		},
		{
			name: "all whitespace only",
			fields: Fields{
				ObjectivesCN: "   ",
				ObjectivesEN: "   ",
				OutlineCN:    "\n\n",
				OutlineEN:    "\n\n",
				Schedule:     "\t\t",
			},
			want: true,
		},
		{
			name: "mixed whitespace and content",
			fields: Fields{
				ObjectivesCN: "   ",
				ObjectivesEN: "   ",
				OutlineCN:    "Valid content",
				OutlineEN:    "",
				Schedule:     "\t\t",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fields.IsEmpty()
			if got != tt.want {
				t.Errorf("Fields.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFields_ContentForIndexing(t *testing.T) {
	tests := []struct {
		name         string
		fields       Fields
		courseTitle  string
		wantEmpty    bool
		wantContains []string // Strings that must appear in content
	}{
		{
			name:        "empty fields",
			fields:      Fields{},
			courseTitle: "測試課程",
			wantEmpty:   true,
		},
		{
			name: "objectives_cn only",
			fields: Fields{
				ObjectivesCN: "培養程式設計能力",
			},
			courseTitle:  "程式設計",
			wantEmpty:    false,
			wantContains: []string{"【程式設計】", "教學目標：", "培養程式設計能力"},
		},
		{
			name: "objectives_cn and objectives_en",
			fields: Fields{
				ObjectivesCN: "培養程式設計能力",
				ObjectivesEN: "Develop programming skills",
			},
			courseTitle:  "程式設計",
			wantEmpty:    false,
			wantContains: []string{"【程式設計】", "教學目標：", "培養程式設計能力", "Course Objectives:", "Develop programming skills"},
		},
		{
			name: "all fields (CN only)",
			fields: Fields{
				ObjectivesCN: "培養程式設計能力",
				OutlineCN:    "變數、迴圈、函數",
				Schedule:     "第1週：課程介紹",
			},
			courseTitle:  "程式設計",
			wantEmpty:    false,
			wantContains: []string{"教學目標：", "培養程式設計能力", "內容綱要：", "變數、迴圈、函數", "教學進度：", "第1週：課程介紹"},
		},
		{
			name: "all fields (CN + EN)",
			fields: Fields{
				ObjectivesCN: "培養程式設計能力",
				ObjectivesEN: "Develop programming skills",
				OutlineCN:    "變數、迴圈、函數",
				OutlineEN:    "Variables, loops, functions",
				Schedule:     "第1週：課程介紹",
			},
			courseTitle: "程式設計",
			wantEmpty:   false,
			wantContains: []string{
				"【程式設計】",
				"教學目標：", "培養程式設計能力",
				"Course Objectives:", "Develop programming skills",
				"內容綱要：", "變數、迴圈、函數",
				"Course Outline:", "Variables, loops, functions",
				"教學進度：", "第1週：課程介紹",
			},
		},
		{
			name: "empty course title",
			fields: Fields{
				ObjectivesCN: "培養能力",
			},
			courseTitle:  "",
			wantEmpty:    false,
			wantContains: []string{"教學目標：", "培養能力"},
		},
		{
			name: "whitespace only fields are skipped",
			fields: Fields{
				ObjectivesCN: "   ",    // whitespace only - should be skipped
				ObjectivesEN: "   ",    // whitespace only - should be skipped
				OutlineCN:    "\n\t\n", // whitespace only - should be skipped
				OutlineEN:    "\n\t\n", // whitespace only - should be skipped
				Schedule:     "第1週導論",  // valid content
			},
			courseTitle:  "測試課程",
			wantEmpty:    false,
			wantContains: []string{"教學進度：", "第1週導論"},
		},
		{
			name: "mixed whitespace and content",
			fields: Fields{
				ObjectivesCN: "   有效的教學目標   ", // has content with leading/trailing whitespace
				ObjectivesEN: "",              // empty - should be skipped
				OutlineCN:    "",              // empty - should be skipped
				OutlineEN:    "",              // empty - should be skipped
				Schedule:     "\t",            // whitespace only - should be skipped
			},
			courseTitle:  "測試課程",
			wantEmpty:    false,
			wantContains: []string{"教學目標：", "有效的教學目標"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := tt.fields.ContentForIndexing(tt.courseTitle)
			isEmpty := content == ""

			if isEmpty != tt.wantEmpty {
				t.Errorf("ContentForIndexing() empty = %v, want %v", isEmpty, tt.wantEmpty)
			}

			if !tt.wantEmpty {
				for _, want := range tt.wantContains {
					if !containsStr(content, want) {
						t.Errorf("ContentForIndexing() should contain %q, got:\n%s", want, content)
					}
				}
			}
		})
	}
}

func TestFields_ContentForIndexing_FullContent(t *testing.T) {
	// Create a very long schedule - should NOT be truncated
	var scheduleBuilder strings.Builder
	for i := 0; i < 30; i++ {
		scheduleBuilder.WriteString("第")
		scheduleBuilder.WriteRune(rune('0' + i%10))
		scheduleBuilder.WriteString("週：課程內容說明與實作練習\n")
	}
	longSchedule := scheduleBuilder.String()

	fields := Fields{
		ObjectivesCN: "目標",
		Schedule:     longSchedule,
	}

	content := fields.ContentForIndexing("測試課程")

	// Content should contain the schedule content (TrimSpace is applied)
	if !containsStr(content, "課程內容說明與實作練習") {
		t.Error("Content should contain schedule content")
	}

	// Should NOT end with "..." (no truncation)
	if containsStr(content, "...") {
		t.Error("Content should not be truncated")
	}

	// Verify it contains the full range (week 0-9 should appear multiple times)
	for i := 0; i < 10; i++ {
		weekStr := fmt.Sprintf("第%d週", i)
		if !containsStr(content, weekStr) {
			t.Errorf("Content should contain %s", weekStr)
		}
	}
}

func TestFields_ContentForIndexing_SingleDocument(t *testing.T) {
	// Test that all content is combined into a single document
	fields := Fields{
		ObjectivesCN: "培養程式設計能力",
		ObjectivesEN: "Develop programming skills",
		OutlineCN:    "變數、迴圈、函數",
		OutlineEN:    "Variables, loops, functions",
	}

	content := fields.ContentForIndexing("測試課程")

	// All content should be in a single string
	if content == "" {
		t.Fatal("Expected non-empty content")
	}

	// Verify all sections are present
	expectedParts := []string{
		"【測試課程】",
		"教學目標：培養程式設計能力",
		"Course Objectives: Develop programming skills",
		"內容綱要：變數、迴圈、函數",
		"Course Outline: Variables, loops, functions",
	}

	for _, part := range expectedParts {
		if !containsStr(content, part) {
			t.Errorf("Content should contain %q", part)
		}
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsStrHelper(s, substr)))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
