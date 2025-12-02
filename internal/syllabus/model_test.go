package syllabus

import (
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

func TestFields_ChunksForEmbedding(t *testing.T) {
	tests := []struct {
		name        string
		fields      Fields
		courseTitle string
		wantCount   int
		wantTypes   []ChunkType
	}{
		{
			name:        "empty fields",
			fields:      Fields{},
			courseTitle: "測試課程",
			wantCount:   0,
			wantTypes:   nil,
		},
		{
			name: "objectives_cn only",
			fields: Fields{
				ObjectivesCN: "培養程式設計能力",
			},
			courseTitle: "程式設計",
			wantCount:   1,
			wantTypes:   []ChunkType{ChunkTypeObjectives},
		},
		{
			name: "objectives_cn and objectives_en (merged)",
			fields: Fields{
				ObjectivesCN: "培養程式設計能力",
				ObjectivesEN: "Develop programming skills",
			},
			courseTitle: "程式設計",
			wantCount:   1,
			wantTypes:   []ChunkType{ChunkTypeObjectives},
		},
		{
			name: "all fields (CN only)",
			fields: Fields{
				ObjectivesCN: "培養程式設計能力",
				OutlineCN:    "變數、迴圈、函數",
				Schedule:     "第1週：課程介紹",
			},
			courseTitle: "程式設計",
			wantCount:   3,
			wantTypes:   []ChunkType{ChunkTypeObjectives, ChunkTypeOutline, ChunkTypeSchedule},
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
			wantCount:   3,
			wantTypes:   []ChunkType{ChunkTypeObjectives, ChunkTypeOutline, ChunkTypeSchedule},
		},
		{
			name: "empty course title",
			fields: Fields{
				ObjectivesCN: "培養能力",
			},
			courseTitle: "",
			wantCount:   1,
			wantTypes:   []ChunkType{ChunkTypeObjectives},
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
			courseTitle: "測試課程",
			wantCount:   1,
			wantTypes:   []ChunkType{ChunkTypeSchedule},
		},
		{
			name: "mixed whitespace and content",
			fields: Fields{
				ObjectivesCN: "   有效的教學目標   ", // has content with leading/trailing whitespace
				ObjectivesEN: "",              // empty - should be merged with CN
				OutlineCN:    "",              // empty - should be skipped
				OutlineEN:    "",              // empty - should be skipped
				Schedule:     "\t",            // whitespace only - should be skipped
			},
			courseTitle: "測試課程",
			wantCount:   1,
			wantTypes:   []ChunkType{ChunkTypeObjectives},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := tt.fields.ChunksForEmbedding(tt.courseTitle)
			if len(chunks) != tt.wantCount {
				t.Errorf("ChunksForEmbedding() count = %d, want %d", len(chunks), tt.wantCount)
			}
			for i, chunk := range chunks {
				if i < len(tt.wantTypes) && chunk.Type != tt.wantTypes[i] {
					t.Errorf("ChunksForEmbedding()[%d].Type = %s, want %s", i, chunk.Type, tt.wantTypes[i])
				}
				// Verify course title is included in content
				if tt.courseTitle != "" && len(chunk.Content) > 0 {
					if !containsStr(chunk.Content, tt.courseTitle) {
						t.Errorf("Chunk content should contain course title %q", tt.courseTitle)
					}
				}
			}
		})
	}
}

func TestFields_ChunksForEmbedding_FullContent(t *testing.T) {
	// Create a very long schedule - should NOT be truncated (2025 best practice)
	longSchedule := ""
	for i := 0; i < 100; i++ {
		longSchedule += "第" + string(rune('0'+i%10)) + "週：課程內容說明與實作練習\n"
	}

	fields := Fields{
		ObjectivesCN: "目標",
		Schedule:     longSchedule,
	}

	chunks := fields.ChunksForEmbedding("測試課程")

	// Find schedule chunk
	var scheduleChunk *Chunk
	for i := range chunks {
		if chunks[i].Type == ChunkTypeSchedule {
			scheduleChunk = &chunks[i]
			break
		}
	}

	if scheduleChunk == nil {
		t.Fatal("Expected schedule chunk")
	}

	// Schedule should contain full content (no truncation)
	// Full content: prefix + "教學進度：" + longSchedule
	expectedContent := "【測試課程】\n教學進度：" + longSchedule
	if scheduleChunk.Content != expectedContent {
		t.Error("Schedule chunk should contain full content without truncation")
	}

	// Should NOT end with "..." (no truncation)
	if containsStr(scheduleChunk.Content, "...") {
		t.Error("Schedule should not be truncated")
	}
}

func TestFields_ChunksForEmbedding_MergeContent(t *testing.T) {
	// Test that CN and EN content are properly merged
	fields := Fields{
		ObjectivesCN: "培養程式設計能力",
		ObjectivesEN: "Develop programming skills",
		OutlineCN:    "變數、迴圈、函數",
		OutlineEN:    "Variables, loops, functions",
	}

	chunks := fields.ChunksForEmbedding("測試課程")

	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks (objectives, outline), got %d", len(chunks))
	}

	// Check objectives chunk contains both CN and EN
	objChunk := chunks[0]
	if !containsStr(objChunk.Content, "培養程式設計能力") {
		t.Error("Objectives chunk should contain Chinese content")
	}
	if !containsStr(objChunk.Content, "Develop programming skills") {
		t.Error("Objectives chunk should contain English content")
	}

	// Check outline chunk contains both CN and EN
	outChunk := chunks[1]
	if !containsStr(outChunk.Content, "變數、迴圈、函數") {
		t.Error("Outline chunk should contain Chinese content")
	}
	if !containsStr(outChunk.Content, "Variables, loops, functions") {
		t.Error("Outline chunk should contain English content")
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
