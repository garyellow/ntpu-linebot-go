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

func TestFields_MergeForEmbedding(t *testing.T) {
	tests := []struct {
		name   string
		fields Fields
		want   string
	}{
		{
			name:   "empty fields",
			fields: Fields{},
			want:   "",
		},
		{
			name: "objectives only",
			fields: Fields{
				Objectives: "培養程式設計能力",
			},
			want: "教學目標：培養程式設計能力",
		},
		{
			name: "outline only",
			fields: Fields{
				Outline: "變數、迴圈、函數",
			},
			want: "內容綱要：變數、迴圈、函數",
		},
		{
			name: "schedule only",
			fields: Fields{
				Schedule: "第1週：課程介紹",
			},
			want: "教學進度：第1週：課程介紹",
		},
		{
			name: "all fields",
			fields: Fields{
				Objectives: "培養程式設計能力",
				Outline:    "變數、迴圈、函數",
				Schedule:   "第1週：課程介紹",
			},
			want: "教學目標：培養程式設計能力\n\n內容綱要：變數、迴圈、函數\n\n教學進度：第1週：課程介紹",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fields.MergeForEmbedding()
			if got != tt.want {
				t.Errorf("Fields.MergeForEmbedding() = %q, want %q", got, tt.want)
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
			name: "objectives set",
			fields: Fields{
				Objectives: "培養能力",
			},
			want: false,
		},
		{
			name: "outline set",
			fields: Fields{
				Outline: "課程內容",
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
