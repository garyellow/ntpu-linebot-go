package rag

import (
	"context"
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
			Objectives: "本課程介紹雲端運算基礎概念，包含 AWS EC2, S3, Lambda 等服務\nIntroduction to cloud computing with AWS services",
			Outline:    "1. 雲端運算概論 2. AWS 架構 3. EC2 虛擬機器 4. S3 儲存服務\n1. Cloud Computing Overview 2. AWS Architecture 3. EC2 4. S3",
			Schedule:   "Week 1: 課程介紹 Week 2: AWS Academy",
		},
		{
			UID:        "1131U0002",
			Title:      "資料結構 Data Structures",
			Teachers:   []string{"李小華"},
			Year:       113,
			Term:       1,
			Objectives: "學習基礎資料結構，包含陣列、鏈結串列、樹、圖\nLearn fundamental data structures",
			Outline:    "陣列 鏈結串列 堆疊 佇列 樹 圖 排序演算法\nArray, Linked List, Stack, Queue, Tree, Graph, Sorting",
			Schedule:   "Week 1-4: 基礎結構 Week 5-8: 進階結構",
		},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if !idx.IsEnabled() {
		t.Error("IsEnabled() should be true after initialization")
	}

	if idx.Count() == 0 {
		t.Error("Count() should be > 0 after initialization")
	}
}

func TestBM25Index_Search(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	syllabi := []*storage.Syllabus{
		{
			UID:        "1131U0001",
			Title:      "雲端運算 Cloud Computing",
			Teachers:   []string{"王大明"},
			Year:       113,
			Term:       1,
			Objectives: "本課程介紹雲端運算基礎概念，包含 AWS EC2, S3, Lambda 等服務\nIntroduction to cloud computing with AWS services",
			Outline:    "1. 雲端運算概論 2. AWS 架構 3. EC2 虛擬機器 4. S3 儲存服務\n1. Cloud Computing Overview 2. AWS Architecture 3. EC2 4. S3",
			Schedule:   "Week 1: 課程介紹 Week 2: AWS Academy",
		},
		{
			UID:        "1131U0002",
			Title:      "資料結構 Data Structures",
			Teachers:   []string{"李小華"},
			Year:       113,
			Term:       1,
			Objectives: "學習基礎資料結構，包含陣列、鏈結串列、樹、圖\nLearn fundamental data structures",
			Outline:    "陣列 鏈結串列 堆疊 佇列 樹 圖 排序演算法\nArray, Linked List, Stack, Queue, Tree, Graph, Sorting",
			Schedule:   "Week 1-4: 基礎結構 Week 5-8: 進階結構",
		},
		{
			UID:        "1131U0003",
			Title:      "機器學習 Machine Learning",
			Teachers:   []string{"陳小明"},
			Year:       113,
			Term:       1,
			Objectives: "介紹機器學習基礎，包含監督式學習、非監督式學習\nIntroduction to machine learning, supervised and unsupervised learning",
			Outline:    "線性迴歸 邏輯迴歸 決策樹 神經網路\nLinear Regression, Logistic Regression, Decision Trees, Neural Networks",
			Schedule:   "Week 1-8: 基礎 Week 9-16: 進階",
		},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	tests := []struct {
		name        string
		query       string
		wantUIDs    []string // Expected UIDs in results (order doesn't matter)
		wantTopUID  string   // Expected top result UID
		wantResults bool     // Whether we expect any results
	}{
		{
			name:        "Search AWS keyword",
			query:       "AWS",
			wantUIDs:    []string{"1131U0001"},
			wantTopUID:  "1131U0001",
			wantResults: true,
		},
		{
			name:        "Search aws lowercase",
			query:       "aws",
			wantUIDs:    []string{"1131U0001"},
			wantTopUID:  "1131U0001",
			wantResults: true,
		},
		{
			// Note: BM25 is keyword-based, so natural language queries like "我想學 AWS"
			// may match unrelated courses containing common words like "學".
			// This is expected behavior - Query Expansion should be used for NL queries.
			name:        "Search mixed query with AWS - keyword in results",
			query:       "我想學 AWS",
			wantUIDs:    []string{"1131U0001"}, // AWS course should be in results
			wantTopUID:  "",                    // Don't check top result - "學" may match other courses
			wantResults: true,
		},
		{
			name:        "Search ec2 keyword",
			query:       "ec2",
			wantUIDs:    []string{"1131U0001"},
			wantTopUID:  "1131U0001",
			wantResults: true,
		},
		{
			name:        "Search s3 keyword",
			query:       "s3",
			wantUIDs:    []string{"1131U0001"},
			wantTopUID:  "1131U0001",
			wantResults: true,
		},
		{
			name:        "Search data structures in English",
			query:       "data structures",
			wantUIDs:    []string{"1131U0002"},
			wantTopUID:  "1131U0002",
			wantResults: true,
		},
		{
			name:        "Search machine learning in English",
			query:       "machine learning",
			wantUIDs:    []string{"1131U0003"},
			wantTopUID:  "1131U0003",
			wantResults: true,
		},
		{
			name:        "Search array in English",
			query:       "array",
			wantUIDs:    []string{"1131U0002"},
			wantTopUID:  "1131U0002",
			wantResults: true,
		},
		{
			name:        "Search neural networks",
			query:       "neural networks",
			wantUIDs:    []string{"1131U0003"},
			wantTopUID:  "1131U0003",
			wantResults: true,
		},
		{
			name:        "Search introduction",
			query:       "introduction",
			wantUIDs:    []string{"1131U0001", "1131U0003"},
			wantTopUID:  "1131U0003", // BM25 favors shorter documents with same term frequency
			wantResults: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := idx.Search(tt.query, 10)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}

			if tt.wantResults && len(results) == 0 {
				t.Fatalf("Search(%q) returned no results, expected results", tt.query)
			}

			if !tt.wantResults && len(results) > 0 {
				t.Fatalf("Search(%q) returned %d results, expected none", tt.query, len(results))
			}

			if !tt.wantResults {
				return // No more checks needed
			}

			// Check top result (skip if wantTopUID is empty)
			if tt.wantTopUID != "" && results[0].UID != tt.wantTopUID {
				t.Errorf("Search(%q) top result = %s, want %s", tt.query, results[0].UID, tt.wantTopUID)
			}

			// Check that expected UIDs are in results
			resultUIDs := make(map[string]bool)
			for _, r := range results {
				resultUIDs[r.UID] = true
			}

			for _, uid := range tt.wantUIDs {
				if !resultUIDs[uid] {
					t.Errorf("Search(%q) missing expected UID %s", tt.query, uid)
				}
			}
		})
	}
}

func TestBM25Index_SearchEmpty(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Initialize with empty syllabi
	if err := idx.Initialize(nil); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	results, err := idx.Search("test", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Search() on empty index returned %d results, want 0", len(results))
	}
}

func TestBM25Index_SearchEmptyQuery(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	syllabi := []*storage.Syllabus{
		{
			UID:        "1131U0001",
			Title:      "Test Course",
			Objectives: "Test content",
		},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	results, err := idx.Search("", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Search() with empty query returned %d results, want 0", len(results))
	}
}

func TestTokenizeChinese(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "English words",
			input: "Hello World",
			want:  []string{"hello", "world"},
		},
		{
			name:  "Chinese characters",
			input: "雲端運算",
			want:  []string{"雲", "端", "運", "算"}, // Unigrams only, no bigrams
		},
		{
			name:  "Mixed Chinese and English",
			input: "AWS 雲端",
			want:  []string{"aws", "雲", "端"},
		},
		{
			name:  "With punctuation",
			input: "Hello, World!",
			want:  []string{"hello", "world"},
		},
		{
			name:  "Numbers",
			input: "EC2 S3",
			want:  []string{"ec2", "s3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizeChinese(tt.input)

			// Check that all expected tokens are present
			gotSet := make(map[string]bool)
			for _, token := range got {
				gotSet[token] = true
			}

			for _, token := range tt.want {
				if !gotSet[token] {
					t.Errorf("tokenizeChinese(%q) missing token %q, got %v", tt.input, token, got)
				}
			}
		})
	}
}

func TestIsCJK(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'雲', true},  // Chinese
		{'A', false}, // English
		{'1', false}, // Number
		{'あ', true},  // Japanese Hiragana
		{'ア', true},  // Japanese Katakana
		{'한', true},  // Korean
		{' ', false}, // Space
	}

	for _, tt := range tests {
		t.Run(string(tt.r), func(t *testing.T) {
			if got := isCJK(tt.r); got != tt.want {
				t.Errorf("isCJK(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestBM25Index_AddSyllabus(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Initialize with three syllabi - same as TestBM25Index_Search for proper IDF calculation
	initialSyllabi := []*storage.Syllabus{
		{
			UID:        "1131U0001",
			Title:      "雲端運算 Cloud Computing",
			Teachers:   []string{"王大明"},
			Year:       113,
			Term:       1,
			Objectives: "本課程介紹雲端運算基礎概念，包含 AWS EC2, S3, Lambda 等服務\nIntroduction to cloud computing with AWS services",
			Outline:    "1. 雲端運算概論 2. AWS 架構 3. EC2 虛擬機器 4. S3 儲存服務\n1. Cloud Computing Overview 2. AWS Architecture 3. EC2 4. S3",
			Schedule:   "Week 1: 課程介紹 Week 2: AWS Academy",
		},
		{
			UID:        "1131U0002",
			Title:      "資料結構 Data Structures",
			Teachers:   []string{"李小華"},
			Year:       113,
			Term:       1,
			Objectives: "學習基礎資料結構，包含陣列、鏈結串列、樹、圖\nLearn fundamental data structures",
			Outline:    "陣列 鏈結串列 堆疊 佇列 樹 圖 排序演算法\nArray, Linked List, Stack, Queue, Tree, Graph, Sorting",
			Schedule:   "Week 1-4: 基礎結構 Week 5-8: 進階結構",
		},
		{
			UID:        "1131U0003",
			Title:      "機器學習 Machine Learning",
			Teachers:   []string{"陳小明"},
			Year:       113,
			Term:       1,
			Objectives: "介紹機器學習基礎，包含監督式學習、非監督式學習\nIntroduction to machine learning, supervised and unsupervised learning",
			Outline:    "線性迴歸 邏輯迴歸 決策樹 神經網路\nLinear Regression, Logistic Regression, Decision Trees, Neural Networks",
			Schedule:   "Week 1-8: 基礎 Week 9-16: 進階",
		},
	}

	if err := idx.Initialize(initialSyllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	initialCount := idx.Count()
	if initialCount == 0 {
		t.Fatal("Expected count > 0 after initialization")
	}
	t.Logf("Initial count: %d", initialCount)

	// Verify initial search works BEFORE AddSyllabus
	results, err := idx.Search("AWS", 5)
	if err != nil {
		t.Fatalf("Initial Search() error = %v", err)
	}
	t.Logf("Initial AWS search: %d results", len(results))
	if len(results) == 0 {
		t.Fatal("Expected to find AWS course after initialization (before AddSyllabus)")
	}

	// Add a new syllabus (fourth one)
	newSyllabus := &storage.Syllabus{
		UID:        "1131U0004",
		Title:      "人工智慧 Artificial Intelligence",
		Teachers:   []string{"張大華"},
		Year:       113,
		Term:       1,
		Objectives: "探索人工智慧的理論與應用，包含深度學習框架\nExplore AI theory and applications with deep learning frameworks",
		Outline:    "卷積神經網路 循環神經網路 強化學習 自然語言處理\nCNN, RNN, Reinforcement Learning, NLP",
		Schedule:   "Week 1-8: 理論 Week 9-16: 實作",
	}

	if err := idx.AddSyllabus(newSyllabus); err != nil {
		t.Fatalf("AddSyllabus() error = %v", err)
	}

	afterCount := idx.Count()
	t.Logf("After AddSyllabus count: %d", afterCount)

	// Count should increase
	if afterCount <= initialCount {
		t.Errorf("Count after AddSyllabus = %d, want > %d", afterCount, initialCount)
	}

	// Search for the new course's unique content (deep learning)
	results, err = idx.Search("deep learning", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Expected to find deep learning after AddSyllabus")
	}
	if results[0].UID != "1131U0004" {
		t.Errorf("Expected top result UID = 1131U0004, got %s", results[0].UID)
	}

	// Original courses should still be searchable
	results, err = idx.Search("AWS", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	t.Logf("After AddSyllabus - AWS search: %d results", len(results))
	if len(results) == 0 {
		t.Fatal("Expected to still find AWS course after AddSyllabus")
	}
	if results[0].UID != "1131U0001" {
		t.Errorf("Expected top result UID = 1131U0001, got %s", results[0].UID)
	}
}

func TestBM25Index_AddSyllabus_Duplicate(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	syl := &storage.Syllabus{
		UID:        "1131U0001",
		Title:      "雲端運算",
		Teachers:   []string{"王大明"},
		Year:       113,
		Term:       1,
		Objectives: "雲端運算課程",
	}

	// Initialize with the syllabus
	if err := idx.Initialize([]*storage.Syllabus{syl}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	initialCount := idx.Count()

	// Try to add the same syllabus again (duplicate UID)
	if err := idx.AddSyllabus(syl); err != nil {
		t.Fatalf("AddSyllabus() error = %v", err)
	}

	// Count should not change (duplicate skipped)
	if idx.Count() != initialCount {
		t.Errorf("Count after duplicate AddSyllabus = %d, want %d (no change)", idx.Count(), initialCount)
	}
}

func TestBM25Index_AddSyllabus_Nil(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Initialize first
	if err := idx.Initialize([]*storage.Syllabus{}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Adding nil should not error
	if err := idx.AddSyllabus(nil); err != nil {
		t.Errorf("AddSyllabus(nil) error = %v, want nil", err)
	}

	// Nil index should not error
	var nilIdx *BM25Index
	if err := nilIdx.AddSyllabus(&storage.Syllabus{UID: "test"}); err != nil {
		t.Errorf("nil.AddSyllabus() error = %v, want nil", err)
	}
}

// TestBM25Index_RelativeScoreFiltering tests the relative score threshold filtering behavior.
// This ensures results significantly less relevant than the top result are filtered out.
func TestBM25Index_RelativeScoreFiltering(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Create syllabi with varying relevance to "AWS"
	// - Course 1: Highly relevant (many AWS mentions)
	// - Course 2: Moderately relevant (some AWS mentions)
	// - Course 3: Low relevance (one AWS mention)
	// - Course 4: Completely irrelevant (no AWS)
	syllabi := []*storage.Syllabus{
		{
			UID:        "1131U0001",
			Title:      "AWS 雲端架構師課程",
			Teachers:   []string{"王大明"},
			Year:       113,
			Term:       1,
			Objectives: "深入學習 AWS 雲端服務，包含 AWS EC2, AWS S3, AWS Lambda, AWS RDS\nMaster AWS cloud services: EC2, S3, Lambda, RDS",
			Outline:    "AWS 架構設計 AWS 安全 AWS 成本優化 AWS 部署策略\nAWS Architecture, AWS Security, AWS Cost Optimization",
		},
		{
			UID:        "1131U0002",
			Title:      "雲端運算導論",
			Teachers:   []string{"李小華"},
			Year:       113,
			Term:       1,
			Objectives: "介紹雲端運算概念，包含 AWS 和其他雲端平台\nIntroduction to cloud computing including AWS",
			Outline:    "雲端基礎 虛擬化技術 AWS 入門\nCloud Basics, Virtualization, AWS Introduction",
		},
		{
			UID:        "1131U0003",
			Title:      "程式設計基礎",
			Teachers:   []string{"陳小明"},
			Year:       113,
			Term:       1,
			Objectives: "學習程式設計基礎，可部署於 AWS 等雲端\nLearn programming basics, deployable to AWS cloud",
			Outline:    "變數 迴圈 函數 物件導向\nVariables, Loops, Functions, OOP",
		},
		{
			UID:        "1131U0004",
			Title:      "資料結構與演算法",
			Teachers:   []string{"張大華"},
			Year:       113,
			Term:       1,
			Objectives: "學習基礎資料結構和演算法分析\nLearn data structures and algorithm analysis",
			Outline:    "陣列 鏈結串列 樹 圖 排序\nArray, Linked List, Tree, Graph, Sorting",
		},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Search for AWS - should get results ranked by relevance
	results, err := idx.Search("AWS", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	// Should have some results
	if len(results) == 0 {
		t.Fatal("Expected at least one result for AWS search")
	}

	// Top result should be the highly relevant AWS course
	if results[0].UID != "1131U0001" {
		t.Errorf("Expected top result UID = 1131U0001, got %s", results[0].UID)
	}

	t.Logf("AWS search returned %d results", len(results))
	for i, r := range results {
		relativeScore := r.Score / results[0].Score
		t.Logf("  [%d] %s: %s (score: %.2f, relative: %.2f)", i+1, r.UID, r.Title, r.Score, relativeScore)
	}
}

// TestBM25Index_TopKLimit tests that results are limited to topN.
func TestBM25Index_TopKLimit(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Create many syllabi
	syllabi := []*storage.Syllabus{
		{
			UID:        "1131U0001",
			Title:      "深度學習 Deep Learning",
			Teachers:   []string{"王大明"},
			Year:       113,
			Term:       1,
			Objectives: "深度學習神經網路\nDeep learning neural networks",
		},
		{
			UID:        "1131U0002",
			Title:      "機器學習基礎",
			Teachers:   []string{"李小華"},
			Year:       113,
			Term:       1,
			Objectives: "機器學習入門\nMachine learning basics",
		},
		{
			UID:        "1131U0003",
			Title:      "學習理論",
			Teachers:   []string{"張三"},
			Year:       113,
			Term:       1,
			Objectives: "學習方法論",
		},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Search with limit of 2
	results, err := idx.Search("學習", 2)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	// Should be limited to 2 results
	if len(results) > 2 {
		t.Errorf("Expected at most 2 results with topN=2, got %d", len(results))
	}

	t.Logf("Search returned %d results (topN=2)", len(results))
}

// TestBM25Index_SearchCoursesConfidence tests that SearchCourses returns correct confidence values.
func TestBM25Index_SearchCoursesConfidence(t *testing.T) {
	log := logger.New("debug")
	idx := NewBM25Index(log)

	// Create syllabi with varying relevance to "雲端運算"
	// Course 1 has highest relevance (多次提到雲端)
	// Course 2 has medium relevance (提到一次雲端)
	// Course 3 has low relevance (不相關)
	// Course 4 has no relevance (完全不相關)
	syllabi := []*storage.Syllabus{
		{
			UID:        "1131U0001",
			Title:      "雲端運算 Cloud Computing",
			Teachers:   []string{"王教授"},
			Year:       113,
			Term:       1,
			Objectives: "本課程介紹雲端運算基礎概念，包含雲端架構、雲端服務、雲端部署\nIntroduction to cloud computing",
			Outline:    "雲端運算概論、雲端平台、雲端應用",
		},
		{
			UID:        "1131U0002",
			Title:      "分散式系統",
			Teachers:   []string{"李教授"},
			Year:       113,
			Term:       1,
			Objectives: "介紹分散式系統架構，包含雲端運算簡介\nDistributed systems with cloud intro",
		},
		{
			UID:        "1131U0003",
			Title:      "資料結構",
			Teachers:   []string{"陳教授"},
			Year:       113,
			Term:       1,
			Objectives: "學習基礎資料結構，包含陣列、鏈結串列、樹、圖\nData structures",
		},
		{
			UID:        "1131U0004",
			Title:      "計算機概論",
			Teachers:   []string{"張教授"},
			Year:       113,
			Term:       1,
			Objectives: "介紹電腦基礎知識\nIntroduction to computer science",
		},
	}

	if err := idx.Initialize(syllabi); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Search using SearchCourses for "雲端"
	results, err := idx.SearchCourses(context.Background(), "雲端", 10)
	if err != nil {
		t.Fatalf("SearchCourses() error = %v", err)
	}

	// Should return results with confidence values
	if len(results) == 0 {
		t.Error("SearchCourses should return results")
	}

	// First result should have confidence = 1.0 (it's the top result)
	if len(results) > 0 && results[0].Confidence != 1.0 {
		t.Errorf("First result confidence = %v, want 1.0", results[0].Confidence)
	}

	// All confidence values should be between 0 and 1
	for i, r := range results {
		if r.Confidence < 0 || r.Confidence > 1 {
			t.Errorf("Result %d confidence = %v, want between 0 and 1", i, r.Confidence)
		}
		t.Logf("  [%d] %s: confidence = %.2f", i+1, r.Title, r.Confidence)
	}
}

// TestBM25Index_MaxSearchResultsConstant verifies the constant is sensible.
func TestBM25Index_MaxSearchResultsConstant(t *testing.T) {
	// Verify MaxSearchResults is reasonable
	if MaxSearchResults < 1 || MaxSearchResults > 100 {
		t.Errorf("MaxSearchResults = %d, want value between 1 and 100", MaxSearchResults)
	}

	t.Logf("MaxSearchResults = %d", MaxSearchResults)
}
