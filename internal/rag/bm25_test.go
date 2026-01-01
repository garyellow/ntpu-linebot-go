package rag

import (
	"context"
	"fmt"
	"testing"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

func TestNewBM25Index(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	if idx == nil {
		t.Fatal("NewBM25Index() returned nil")
	}

	if idx.IsEnabled() {
		t.Error("NewBM25Index() should not be enabled before initialization")
	}
}

func TestBM25Index_Initialize(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	syllabi := []*storage.Syllabus{
		{
			UID:        "1131U0001",
			Title:      "雲端運算 Cloud Computing",
			Teachers:   []string{"王大明"},
			Year:       113,
			Term:       1,
			Objectives: "本課程介紹雲端運算基礎概念",
		},
		{
			UID:        "1132U0002",
			Title:      "資料結構 Data Structures",
			Teachers:   []string{"李小華"},
			Year:       113,
			Term:       2,
			Objectives: "學習基礎資料結構",
		},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if !idx.IsEnabled() {
		t.Error("IsEnabled() should be true after initialization")
	}

	if idx.Count() != 2 {
		t.Errorf("Count() = %d, want 2", idx.Count())
	}

	// Verify per-semester architecture
	if len(idx.semesterIndexes) != 2 {
		t.Errorf("Expected 2 semester indexes, got %d", len(idx.semesterIndexes))
	}

	if len(idx.allSemesters) != 2 {
		t.Errorf("Expected 2 semesters, got %d", len(idx.allSemesters))
	}

	// Verify semester ordering (newest first)
	if idx.allSemesters[0].Year != 113 || idx.allSemesters[0].Term != 2 {
		t.Errorf("First semester should be 113-2, got %d-%d",
			idx.allSemesters[0].Year, idx.allSemesters[0].Term)
	}
}

func TestBM25Index_InitializeEmpty(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	if err := idx.Initialize([]*storage.Syllabus{}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Should be initialized but with no semesters
	if !idx.initialized {
		t.Error("Should be initialized")
	}
	if len(idx.semesterIndexes) != 0 {
		t.Errorf("Expected 0 semester indexes, got %d", len(idx.semesterIndexes))
	}
}

func TestBM25Index_PerSemesterIndexing(t *testing.T) {
	t.Parallel()
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Create syllabi from 3 different semesters
	syllabi := []*storage.Syllabus{
		{UID: "1141U0001", Title: "雲端運算", Teachers: []string{"王教授"}, Year: 114, Term: 1, Objectives: "雲端基礎"},
		{UID: "1141U0002", Title: "資料結構", Teachers: []string{"李教授"}, Year: 114, Term: 1, Objectives: "資料結構"},
		{UID: "1132U0001", Title: "雲端服務", Teachers: []string{"陳教授"}, Year: 113, Term: 2, Objectives: "雲端服務"},
		{UID: "1131U0001", Title: "雲端概論", Teachers: []string{"林教授"}, Year: 113, Term: 1, Objectives: "雲端入門"},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Verify: Should have 3 semester indexes
	if len(idx.semesterIndexes) != 3 {
		t.Errorf("Expected 3 semester indexes, got %d", len(idx.semesterIndexes))
	}

	// Verify: allSemesters should be sorted newest first
	if len(idx.allSemesters) != 3 {
		t.Errorf("Expected 3 semesters, got %d", len(idx.allSemesters))
	}
	if idx.allSemesters[0].Year != 114 || idx.allSemesters[0].Term != 1 {
		t.Errorf("First semester should be 114-1, got %d-%d",
			idx.allSemesters[0].Year, idx.allSemesters[0].Term)
	}
	if idx.allSemesters[1].Year != 113 || idx.allSemesters[1].Term != 2 {
		t.Errorf("Second semester should be 113-2, got %d-%d",
			idx.allSemesters[1].Year, idx.allSemesters[1].Term)
	}

	// Verify: 114-1 should have 2 courses, others should have 1
	sem114_1 := SemesterKey{Year: 114, Term: 1}
	if semIdx := idx.semesterIndexes[sem114_1]; semIdx == nil || len(semIdx.corpus) != 2 {
		t.Errorf("Semester 114-1 should have 2 courses")
	}
}

func TestBM25Index_SearchCourses_PerSemesterTopK(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Create syllabi: 15 courses per semester (114-1 and 113-2), all matching "程式"
	// This ensures we have MORE than 10 courses per semester to verify the limit
	syllabi := []*storage.Syllabus{
		// 114-1 (Newest semester) - 15 courses matching "程式"
		{UID: "1141U0001", Title: "程式設計一", Teachers: []string{"王教授"}, Year: 114, Term: 1, Objectives: "程式設計基礎"},
		{UID: "1141U0002", Title: "程式設計二", Teachers: []string{"李教授"}, Year: 114, Term: 1, Objectives: "進階程式設計"},
		{UID: "1141U0003", Title: "物件導向程式", Teachers: []string{"陳教授"}, Year: 114, Term: 1, Objectives: "OOP程式"},
		{UID: "1141U0004", Title: "系統程式", Teachers: []string{"林教授"}, Year: 114, Term: 1, Objectives: "系統程式設計"},
		{UID: "1141U0005", Title: "網路程式", Teachers: []string{"張教授"}, Year: 114, Term: 1, Objectives: "網路程式設計"},
		{UID: "1141U0006", Title: "程式語言", Teachers: []string{"黃教授"}, Year: 114, Term: 1, Objectives: "程式語言概論"},
		{UID: "1141U0007", Title: "應用程式開發", Teachers: []string{"吳教授"}, Year: 114, Term: 1, Objectives: "應用程式"},
		{UID: "1141U0008", Title: "程式邏輯", Teachers: []string{"周教授"}, Year: 114, Term: 1, Objectives: "程式邏輯思維"},
		{UID: "1141U0009", Title: "組合語言程式", Teachers: []string{"許教授"}, Year: 114, Term: 1, Objectives: "低階程式"},
		{UID: "1141U0010", Title: "嵌入式程式", Teachers: []string{"鄭教授"}, Year: 114, Term: 1, Objectives: "嵌入式程式設計"},
		{UID: "1141U0011", Title: "遊戲程式", Teachers: []string{"趙教授"}, Year: 114, Term: 1, Objectives: "遊戲程式開發"},
		{UID: "1141U0012", Title: "競技程式", Teachers: []string{"孫教授"}, Year: 114, Term: 1, Objectives: "競技程式設計"},
		{UID: "1141U0013", Title: "平行程式", Teachers: []string{"蔡教授"}, Year: 114, Term: 1, Objectives: "平行程式設計"},
		{UID: "1141U0014", Title: "函數程式", Teachers: []string{"葉教授"}, Year: 114, Term: 1, Objectives: "函數式程式"},
		{UID: "1141U0015", Title: "程式測試", Teachers: []string{"謝教授"}, Year: 114, Term: 1, Objectives: "程式測試方法"},
		// 113-2 (Second newest) - 15 courses matching "程式"
		{UID: "1132U0001", Title: "C程式設計", Teachers: []string{"劉教授"}, Year: 113, Term: 2, Objectives: "C程式"},
		{UID: "1132U0002", Title: "Java程式", Teachers: []string{"方教授"}, Year: 113, Term: 2, Objectives: "Java程式設計"},
		{UID: "1132U0003", Title: "Python程式", Teachers: []string{"施教授"}, Year: 113, Term: 2, Objectives: "Python程式"},
		{UID: "1132U0004", Title: "程式實作", Teachers: []string{"洪教授"}, Year: 113, Term: 2, Objectives: "程式實作專題"},
		{UID: "1132U0005", Title: "程式專題", Teachers: []string{"彭教授"}, Year: 113, Term: 2, Objectives: "程式專題研究"},
		{UID: "1132U0006", Title: "競程入門", Teachers: []string{"曾教授"}, Year: 113, Term: 2, Objectives: "競技程式"},
		{UID: "1132U0007", Title: "網頁程式", Teachers: []string{"宋教授"}, Year: 113, Term: 2, Objectives: "網頁程式設計"},
		{UID: "1132U0008", Title: "程式設計三", Teachers: []string{"梁教授"}, Year: 113, Term: 2, Objectives: "高階程式"},
		{UID: "1132U0009", Title: "科學程式", Teachers: []string{"馬教授"}, Year: 113, Term: 2, Objectives: "科學程式設計"},
		{UID: "1132U0010", Title: "程式最佳化", Teachers: []string{"廖教授"}, Year: 113, Term: 2, Objectives: "程式效能"},
		{UID: "1132U0011", Title: "腳本程式", Teachers: []string{"賴教授"}, Year: 113, Term: 2, Objectives: "腳本程式語言"},
		{UID: "1132U0012", Title: "行動程式", Teachers: []string{"蕭教授"}, Year: 113, Term: 2, Objectives: "行動程式開發"},
		{UID: "1132U0013", Title: "程式重構", Teachers: []string{"丁教授"}, Year: 113, Term: 2, Objectives: "程式重構技術"},
		{UID: "1132U0014", Title: "程式安全", Teachers: []string{"沈教授"}, Year: 113, Term: 2, Objectives: "程式安全"},
		{UID: "1132U0015", Title: "程式分析", Teachers: []string{"韓教授"}, Year: 113, Term: 2, Objectives: "程式分析"},
		// 113-1 (Older semester - should be excluded from search)
		{UID: "1131U0001", Title: "程式概論", Teachers: []string{"范教授"}, Year: 113, Term: 1, Objectives: "程式概論"},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Search with topN=10: each semester should get up to 10 results independently
	results, err := idx.SearchCourses(context.Background(), "程式", 10)
	if err != nil {
		t.Fatalf("SearchCourses() error = %v", err)
	}

	// Count results per semester
	semesterCounts := make(map[string]int)
	for _, r := range results {
		semKey := fmt.Sprintf("%d-%d", r.Year, r.Term)
		semesterCounts[semKey]++
	}

	t.Logf("Results: %d total", len(results))
	for sem, count := range semesterCounts {
		t.Logf("  Semester %s: %d results", sem, count)
	}

	// Verify: Each semester should have EXACTLY 10 results (per-semester Top-K)
	if semesterCounts["114-1"] != 10 {
		t.Errorf("Semester 114-1 has %d results, want exactly 10", semesterCounts["114-1"])
	}
	if semesterCounts["113-2"] != 10 {
		t.Errorf("Semester 113-2 has %d results, want exactly 10", semesterCounts["113-2"])
	}

	// Verify: Total should be 20 (10 per semester)
	if len(results) != 20 {
		t.Errorf("Expected exactly 20 results (10 per semester), got %d", len(results))
	}

	// Verify: Should NOT have results from 113-1 (older semester)
	if semesterCounts["113-1"] > 0 {
		t.Errorf("Should not have results from older semester 113-1, got %d", semesterCounts["113-1"])
	}
}

func TestBM25Index_SearchCourses_IndependentConfidence(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Create syllabi: each semester has courses with "雲端"
	syllabi := []*storage.Syllabus{
		// 114-1: 1 course with "雲端" (should get confidence 1.0)
		{UID: "1141U0001", Title: "雲端運算", Teachers: []string{"王教授"}, Year: 114, Term: 1, Objectives: "雲端基礎"},
		// 113-2: 2 courses with "雲端" (best one should get confidence 1.0)
		{UID: "1132U0001", Title: "雲端服務", Teachers: []string{"李教授"}, Year: 113, Term: 2, Objectives: "雲端服務設計"},
		{UID: "1132U0002", Title: "雲端", Teachers: []string{"陳教授"}, Year: 113, Term: 2, Objectives: "雲端"},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	results, err := idx.SearchCourses(context.Background(), "雲端", 10)
	if err != nil {
		t.Fatalf("SearchCourses() error = %v", err)
	}

	// Group by semester and find first result of each
	semesterFirstConfidence := make(map[string]float32)
	for _, r := range results {
		semKey := fmt.Sprintf("%d-%d", r.Year, r.Term)
		if _, exists := semesterFirstConfidence[semKey]; !exists {
			semesterFirstConfidence[semKey] = r.Confidence
		}
	}

	// Verify: Each semester's best result should have confidence 1.0
	for sem, conf := range semesterFirstConfidence {
		if conf != 1.0 {
			t.Errorf("Semester %s best result has confidence %.2f, want 1.0", sem, conf)
		}
	}
}

func TestBM25Index_SearchCourses_NewestTwoSemesters(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Create courses in 4 semesters, all matching "雲端"
	syllabi := []*storage.Syllabus{
		{UID: "1141U0001", Title: "雲端運算", Year: 114, Term: 1, Objectives: "雲端"},
		{UID: "1132U0001", Title: "雲端服務", Year: 113, Term: 2, Objectives: "雲端"},
		{UID: "1131U0001", Title: "雲端概論", Year: 113, Term: 1, Objectives: "雲端"},
		{UID: "1122U0001", Title: "雲端技術", Year: 112, Term: 2, Objectives: "雲端"},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	results, err := idx.SearchCourses(context.Background(), "雲端", 10)
	if err != nil {
		t.Fatalf("SearchCourses() error = %v", err)
	}

	// Verify: Only newest 2 semesters should have results
	allowedSemesters := map[string]bool{"114-1": true, "113-2": true}
	for _, r := range results {
		semKey := fmt.Sprintf("%d-%d", r.Year, r.Term)
		if !allowedSemesters[semKey] {
			t.Errorf("Result from semester %s should not be included (only 114-1 and 113-2)", semKey)
		}
	}

	// Verify: Should have exactly 2 results (one per newest semester)
	if len(results) != 2 {
		t.Errorf("Expected 2 results (one per newest semester), got %d", len(results))
	}
}

func TestBM25Index_SearchCourses_EmptyQuery(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	syllabi := []*storage.Syllabus{
		{UID: "1131U0001", Title: "雲端運算", Year: 113, Term: 1, Objectives: "雲端基礎"},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Empty query should return nil
	results, err := idx.SearchCourses(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("SearchCourses() error = %v", err)
	}
	if results != nil {
		t.Errorf("Expected nil results for empty query, got %d results", len(results))
	}

	// Whitespace-only query should return nil
	results, err = idx.SearchCourses(context.Background(), "   ", 10)
	if err != nil {
		t.Fatalf("SearchCourses() error = %v", err)
	}
	if results != nil {
		t.Errorf("Expected nil results for whitespace query, got %d results", len(results))
	}
}

func TestBM25Index_SearchCourses_NoMatch(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	syllabi := []*storage.Syllabus{
		{UID: "1131U0001", Title: "雲端運算", Year: 113, Term: 1, Objectives: "雲端基礎"},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Query that doesn't match any course
	results, err := idx.SearchCourses(context.Background(), "完全不相關的查詢", 10)
	if err != nil {
		t.Fatalf("SearchCourses() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-matching query, got %d", len(results))
	}
}

func TestBM25Index_AddSyllabus(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Start with one course
	syllabi := []*storage.Syllabus{
		{UID: "1131U0001", Title: "雲端運算", Year: 113, Term: 1, Objectives: "雲端基礎"},
	}
	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if idx.Count() != 1 {
		t.Errorf("Initial count = %d, want 1", idx.Count())
	}

	// Add course to same semester
	newSyl := &storage.Syllabus{
		UID:        "1131U0002",
		Title:      "資料結構",
		Year:       113,
		Term:       1,
		Objectives: "資料結構基礎",
	}
	if err := idx.AddSyllabus(newSyl); err != nil {
		t.Fatalf("AddSyllabus() error = %v", err)
	}

	if idx.Count() != 2 {
		t.Errorf("After add count = %d, want 2", idx.Count())
	}

	// Add course to new semester
	newSemSyl := &storage.Syllabus{
		UID:        "1132U0001",
		Title:      "進階雲端",
		Year:       113,
		Term:       2,
		Objectives: "進階雲端運算",
	}
	if err := idx.AddSyllabus(newSemSyl); err != nil {
		t.Fatalf("AddSyllabus() error = %v", err)
	}

	if idx.Count() != 3 {
		t.Errorf("After add count = %d, want 3", idx.Count())
	}
	if len(idx.semesterIndexes) != 2 {
		t.Errorf("Should have 2 semesters, got %d", len(idx.semesterIndexes))
	}
}

func TestBM25Index_AddSyllabus_Duplicate(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	syllabi := []*storage.Syllabus{
		{UID: "1131U0001", Title: "雲端運算", Year: 113, Term: 1, Objectives: "雲端基礎"},
	}
	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Try to add duplicate
	duplicate := &storage.Syllabus{
		UID:        "1131U0001",
		Title:      "不同標題",
		Year:       113,
		Term:       1,
		Objectives: "不同內容",
	}
	if err := idx.AddSyllabus(duplicate); err != nil {
		t.Fatalf("AddSyllabus() error = %v", err)
	}

	// Count should still be 1
	if idx.Count() != 1 {
		t.Errorf("Count after duplicate = %d, want 1", idx.Count())
	}
}

func TestTokenizeChinese(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "Chinese characters as unigrams",
			input:  "雲端",
			expect: []string{"雲", "端"},
		},
		{
			name:   "English word kept intact",
			input:  "AWS",
			expect: []string{"aws"}, // lowercase
		},
		{
			name:   "Mixed Chinese and English",
			input:  "雲端運算 cloud computing",
			expect: []string{"雲", "端", "運", "算", "cloud", "computing"},
		},
		{
			name:   "Empty string",
			input:  "",
			expect: nil,
		},
		{
			name:   "Punctuation ignored",
			input:  "Hello, 世界!",
			expect: []string{"hello", "世", "界"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tokenizeChinese(tt.input)
			if len(result) != len(tt.expect) {
				t.Errorf("tokenizeChinese(%q) = %v, want %v", tt.input, result, tt.expect)
				return
			}
			for i, token := range result {
				if token != tt.expect[i] {
					t.Errorf("tokenizeChinese(%q)[%d] = %q, want %q", tt.input, i, token, tt.expect[i])
				}
			}
		})
	}
}

func TestIsCJK(t *testing.T) {
	t.Parallel()
	tests := []struct {
		r    rune
		want bool
	}{
		{'雲', true},  // Chinese
		{'あ', true},  // Japanese Hiragana
		{'ア', true},  // Japanese Katakana
		{'한', true},  // Korean
		{'a', false}, // English
		{'1', false}, // Number
		{'!', false}, // Punctuation
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			t.Parallel()
			if got := isCJK(tt.r); got != tt.want {
				t.Errorf("isCJK(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestComputeRelativeConfidence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		score    float64
		maxScore float64
		want     float32
	}{
		{"Max score gets 1.0", 10.0, 10.0, 1.0},
		{"Half score gets 0.5", 5.0, 10.0, 0.5},
		{"Zero max returns 0", 5.0, 0.0, 0.0},
		{"Negative scores - best is 1.0", -7.68, -7.68, 1.0},
		{"Negative scores - worse gets less", -8.21, -7.68, 0.935}, // 7.68/8.21 ≈ 0.935
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := computeRelativeConfidence(tt.score, tt.maxScore)
			// Allow small floating point difference
			diff := got - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("computeRelativeConfidence(%v, %v) = %v, want %v", tt.score, tt.maxScore, got, tt.want)
			}
		})
	}
}

func TestMaxSearchResultsConstant(t *testing.T) {
	t.Parallel()
	// Verify the constant is sensible
	if MaxSearchResults < 5 || MaxSearchResults > 100 {
		t.Errorf("MaxSearchResults = %d, should be between 5 and 100", MaxSearchResults)
	}
}
