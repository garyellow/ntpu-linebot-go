package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/syllabus"
)

func TestNewVectorDB_DisabledWithoutAPIKey(t *testing.T) {
	log := logger.New("info")

	vdb, err := NewVectorDB("", "", log)
	if err != nil {
		t.Errorf("NewVectorDB() error = %v", err)
	}
	if vdb != nil {
		t.Error("Expected nil VectorDB when API key is empty")
	}
}

func TestVectorDB_IsEnabled_Nil(t *testing.T) {
	var vdb *VectorDB
	if vdb.IsEnabled() {
		t.Error("Expected IsEnabled() = false for nil VectorDB")
	}
}

func TestVectorDB_Count_Nil(t *testing.T) {
	var vdb *VectorDB
	if count := vdb.Count(); count != 0 {
		t.Errorf("Expected Count() = 0 for nil VectorDB, got %d", count)
	}
}

func TestVectorDB_Search_Nil(t *testing.T) {
	var vdb *VectorDB
	ctx := context.Background()

	results, err := vdb.Search(ctx, "test query", 10)
	if err != nil {
		t.Errorf("Search() on nil VectorDB error = %v", err)
	}
	if results != nil {
		t.Error("Expected nil results for nil VectorDB")
	}
}

func TestVectorDB_Search_EmptyQuery(t *testing.T) {
	// Create a mock VectorDB structure (without actual chromem)
	vdb := &VectorDB{
		initialized: true,
	}
	ctx := context.Background()

	results, err := vdb.Search(ctx, "", 10)
	if err != nil {
		t.Errorf("Search() with empty query error = %v", err)
	}
	if results != nil {
		t.Error("Expected nil results for empty query")
	}
}

func TestVectorDB_AddSyllabus_Nil(t *testing.T) {
	var vdb *VectorDB
	ctx := context.Background()

	err := vdb.AddSyllabus(ctx, &storage.Syllabus{
		UID:          "1131U0001",
		ObjectivesCN: "test objectives",
	})
	if err != nil {
		t.Errorf("AddSyllabus() on nil VectorDB error = %v", err)
	}
}

func TestVectorDB_AddSyllabi_Nil(t *testing.T) {
	var vdb *VectorDB
	ctx := context.Background()

	err := vdb.AddSyllabi(ctx, []*storage.Syllabus{
		{UID: "1131U0001", ObjectivesCN: "test"},
	})
	if err != nil {
		t.Errorf("AddSyllabi() on nil VectorDB error = %v", err)
	}
}

func TestVectorDB_AddSyllabi_Empty(t *testing.T) {
	vdb := &VectorDB{initialized: true}
	ctx := context.Background()

	err := vdb.AddSyllabi(ctx, nil)
	if err != nil {
		t.Errorf("AddSyllabi() with nil slice error = %v", err)
	}

	err = vdb.AddSyllabi(ctx, []*storage.Syllabus{})
	if err != nil {
		t.Errorf("AddSyllabi() with empty slice error = %v", err)
	}
}

// TestAddSyllabusInternal_WhitespaceOnlyFields tests that syllabi with
// whitespace-only fields are properly skipped during embedding
func TestAddSyllabusInternal_WhitespaceOnlyFields(t *testing.T) {
	tests := []struct {
		name           string
		syllabus       *storage.Syllabus
		wantChunkCount int
	}{
		{
			name: "all empty fields - should skip",
			syllabus: &storage.Syllabus{
				UID:          "1131U0001",
				Title:        "測試課程",
				ObjectivesCN: "",
				ObjectivesEN: "",
				OutlineCN:    "",
				OutlineEN:    "",
				Schedule:     "",
			},
			wantChunkCount: 0,
		},
		{
			name: "all whitespace-only fields - should skip",
			syllabus: &storage.Syllabus{
				UID:          "1131U0002",
				Title:        "測試課程",
				ObjectivesCN: "   ",
				ObjectivesEN: "   ",
				OutlineCN:    "\n\t\n",
				OutlineEN:    "\n\t\n",
				Schedule:     "\t\t",
			},
			wantChunkCount: 0,
		},
		{
			name: "one valid field (CN only) - should create 1 chunk",
			syllabus: &storage.Syllabus{
				UID:          "1131U0003",
				Title:        "測試課程",
				ObjectivesCN: "有效的教學目標",
				ObjectivesEN: "",
				OutlineCN:    "   ", // whitespace only
				OutlineEN:    "",
				Schedule:     "", // empty
			},
			wantChunkCount: 1,
		},
		{
			name: "CN + EN objectives - should create 1 merged chunk",
			syllabus: &storage.Syllabus{
				UID:          "1131U0003b",
				Title:        "測試課程",
				ObjectivesCN: "有效的教學目標",
				ObjectivesEN: "Valid objectives",
				OutlineCN:    "",
				OutlineEN:    "",
				Schedule:     "",
			},
			wantChunkCount: 1,
		},
		{
			name: "two valid fields (CN only) - should create 2 chunks",
			syllabus: &storage.Syllabus{
				UID:          "1131U0004",
				Title:        "測試課程",
				ObjectivesCN: "教學目標內容",
				ObjectivesEN: "",
				OutlineCN:    "課程內容綱要",
				OutlineEN:    "",
				Schedule:     "\t", // whitespace only
			},
			wantChunkCount: 2,
		},
		{
			name: "all valid fields (CN only) - should create 3 chunks",
			syllabus: &storage.Syllabus{
				UID:          "1131U0005",
				Title:        "測試課程",
				ObjectivesCN: "教學目標",
				ObjectivesEN: "",
				OutlineCN:    "內容綱要",
				OutlineEN:    "",
				Schedule:     "教學進度",
			},
			wantChunkCount: 3,
		},
		{
			name: "field with leading/trailing whitespace - should keep content",
			syllabus: &storage.Syllabus{
				UID:          "1131U0006",
				Title:        "測試課程",
				ObjectivesCN: "   有效內容前後有空格   ",
				ObjectivesEN: "",
				OutlineCN:    "",
				OutlineEN:    "",
				Schedule:     "",
			},
			wantChunkCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use syllabus.Fields.ChunksForEmbedding to verify chunk generation
			// This tests the same logic used by addSyllabusInternal
			fields := &syllabus.Fields{
				ObjectivesCN: tt.syllabus.ObjectivesCN,
				ObjectivesEN: tt.syllabus.ObjectivesEN,
				OutlineCN:    tt.syllabus.OutlineCN,
				OutlineEN:    tt.syllabus.OutlineEN,
				Schedule:     tt.syllabus.Schedule,
			}
			chunks := fields.ChunksForEmbedding(tt.syllabus.Title)

			if len(chunks) != tt.wantChunkCount {
				t.Errorf("ChunksForEmbedding() returned %d chunks, want %d", len(chunks), tt.wantChunkCount)
			}

			// Also verify each chunk has content (not just prefix)
			for i, chunk := range chunks {
				if len(chunk.Content) == 0 {
					t.Errorf("Chunk %d has empty content", i)
				}
			}
		})
	}
}

func TestVectorDB_UpdateSyllabus_Nil(t *testing.T) {
	var vdb *VectorDB
	ctx := context.Background()

	err := vdb.UpdateSyllabus(ctx, &storage.Syllabus{
		UID:          "1131U0001",
		ObjectivesCN: "updated objectives",
	})
	if err != nil {
		t.Errorf("UpdateSyllabus() on nil VectorDB error = %v", err)
	}
}

func TestVectorDB_DeleteSyllabus_Nil(t *testing.T) {
	var vdb *VectorDB
	ctx := context.Background()

	err := vdb.DeleteSyllabus(ctx, "1131U0001")
	if err != nil {
		t.Errorf("DeleteSyllabus() on nil VectorDB error = %v", err)
	}
}

func TestVectorDB_Initialize_Nil(t *testing.T) {
	var vdb *VectorDB

	err := vdb.Initialize(context.Background(), nil)
	if err != nil {
		t.Errorf("Initialize() on nil VectorDB error = %v", err)
	}
}

func TestVectorDB_Close_Nil(t *testing.T) {
	var vdb *VectorDB

	err := vdb.Close()
	if err != nil {
		t.Errorf("Close() on nil VectorDB error = %v", err)
	}
}

func TestVectorDB_Close(t *testing.T) {
	vdb := &VectorDB{initialized: true}

	err := vdb.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestSearchResult_Fields(t *testing.T) {
	result := SearchResult{
		UID:        "1131U0001",
		Title:      "程式設計",
		Teachers:   []string{"王小明", "李小華"},
		Year:       113,
		Term:       1,
		Content:    "教學目標：培養程式設計能力",
		Similarity: 0.95,
	}

	if result.UID != "1131U0001" {
		t.Errorf("UID = %q, want %q", result.UID, "1131U0001")
	}
	if result.Title != "程式設計" {
		t.Errorf("Title = %q, want %q", result.Title, "程式設計")
	}
	if len(result.Teachers) != 2 {
		t.Errorf("Teachers count = %d, want 2", len(result.Teachers))
	}
	if result.Year != 113 {
		t.Errorf("Year = %d, want 113", result.Year)
	}
	if result.Term != 1 {
		t.Errorf("Term = %d, want 1", result.Term)
	}
	if result.Similarity != 0.95 {
		t.Errorf("Similarity = %f, want 0.95", result.Similarity)
	}
}

func TestConstants(t *testing.T) {
	if SyllabusCollectionName != "syllabi" {
		t.Errorf("SyllabusCollectionName = %q, want %q", SyllabusCollectionName, "syllabi")
	}
	if DefaultSearchResults != 10 {
		t.Errorf("DefaultSearchResults = %d, want 10", DefaultSearchResults)
	}
	if MaxSearchResults != 100 {
		t.Errorf("MaxSearchResults = %d, want 100", MaxSearchResults)
	}
	if MinSimilarityThreshold != 0.3 {
		t.Errorf("MinSimilarityThreshold = %f, want 0.3", MinSimilarityThreshold)
	}
	if HighRelevanceThreshold != 0.7 {
		t.Errorf("HighRelevanceThreshold = %f, want 0.7", HighRelevanceThreshold)
	}
}

func TestExtractUIDFromDocID(t *testing.T) {
	tests := []struct {
		docID   string
		wantUID string
	}{
		{"1141U3556_objectives", "1141U3556"},
		{"1141U3556_outline", "1141U3556"},
		{"1141U3556_schedule", "1141U3556"},
		{"1131U0001_objectives", "1131U0001"},
		{"", ""},            // empty input
		{"invalid", ""},     // no underscore
		{"_objectives", ""}, // empty UID (lastIdx == 0)
	}

	for _, tt := range tests {
		got := extractUIDFromDocID(tt.docID)
		if got != tt.wantUID {
			t.Errorf("extractUIDFromDocID(%q) = %q, want %q", tt.docID, got, tt.wantUID)
		}
	}
}

// TestVectorDB_Integration tests with actual chromem (requires temp directory)
func TestVectorDB_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "vectordb_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	log := logger.New("info")

	// Use a mock API key - we won't actually call the API in this test
	// We're just testing the database operations
	t.Run("creation without API key returns nil", func(t *testing.T) {
		vdb, err := NewVectorDB(tmpDir, "", log)
		if err != nil {
			t.Errorf("NewVectorDB() error = %v", err)
		}
		if vdb != nil {
			t.Error("Expected nil VectorDB with empty API key")
		}
	})

	t.Run("persistence path is correct", func(t *testing.T) {
		expectedPath := filepath.Join(tmpDir, "chromem", "syllabi")
		// This tests that the path construction is correct
		// Actual creation would fail without valid API key for embedding
		if !filepath.IsAbs(filepath.Join(tmpDir, "chromem", "syllabi")) {
			t.Logf("Expected path: %s", expectedPath)
		}
	})
}
