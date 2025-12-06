package rag

import (
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
			UID:          "1131U0001",
			Title:        "雲端運算 Cloud Computing",
			Teachers:     []string{"王大明"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "本課程介紹雲端運算基礎概念，包含 AWS EC2, S3, Lambda 等服務",
			ObjectivesEN: "Introduction to cloud computing with AWS services",
			OutlineCN:    "1. 雲端運算概論 2. AWS 架構 3. EC2 虛擬機器 4. S3 儲存服務",
			OutlineEN:    "1. Cloud Computing Overview 2. AWS Architecture 3. EC2 4. S3",
			Schedule:     "Week 1: 課程介紹 Week 2: AWS Academy",
		},
		{
			UID:          "1131U0002",
			Title:        "資料結構 Data Structures",
			Teachers:     []string{"李小華"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "學習基礎資料結構，包含陣列、鏈結串列、樹、圖",
			ObjectivesEN: "Learn fundamental data structures",
			OutlineCN:    "陣列 鏈結串列 堆疊 佇列 樹 圖 排序演算法",
			OutlineEN:    "Array, Linked List, Stack, Queue, Tree, Graph, Sorting",
			Schedule:     "Week 1-4: 基礎結構 Week 5-8: 進階結構",
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
			UID:          "1131U0001",
			Title:        "雲端運算 Cloud Computing",
			Teachers:     []string{"王大明"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "本課程介紹雲端運算基礎概念，包含 AWS EC2, S3, Lambda 等服務",
			ObjectivesEN: "Introduction to cloud computing with AWS services",
			OutlineCN:    "1. 雲端運算概論 2. AWS 架構 3. EC2 虛擬機器 4. S3 儲存服務",
			OutlineEN:    "1. Cloud Computing Overview 2. AWS Architecture 3. EC2 4. S3",
			Schedule:     "Week 1: 課程介紹 Week 2: AWS Academy",
		},
		{
			UID:          "1131U0002",
			Title:        "資料結構 Data Structures",
			Teachers:     []string{"李小華"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "學習基礎資料結構，包含陣列、鏈結串列、樹、圖",
			ObjectivesEN: "Learn fundamental data structures",
			OutlineCN:    "陣列 鏈結串列 堆疊 佇列 樹 圖 排序演算法",
			OutlineEN:    "Array, Linked List, Stack, Queue, Tree, Graph, Sorting",
			Schedule:     "Week 1-4: 基礎結構 Week 5-8: 進階結構",
		},
		{
			UID:          "1131U0003",
			Title:        "機器學習 Machine Learning",
			Teachers:     []string{"陳小明"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "介紹機器學習基礎，包含監督式學習、非監督式學習",
			ObjectivesEN: "Introduction to machine learning, supervised and unsupervised learning",
			OutlineCN:    "線性迴歸 邏輯迴歸 決策樹 神經網路",
			OutlineEN:    "Linear Regression, Logistic Regression, Decision Trees, Neural Networks",
			Schedule:     "Week 1-8: 基礎 Week 9-16: 進階",
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
			UID:          "1131U0001",
			Title:        "Test Course",
			ObjectivesCN: "Test content",
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
			UID:          "1131U0001",
			Title:        "雲端運算 Cloud Computing",
			Teachers:     []string{"王大明"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "本課程介紹雲端運算基礎概念，包含 AWS EC2, S3, Lambda 等服務",
			ObjectivesEN: "Introduction to cloud computing with AWS services",
			OutlineCN:    "1. 雲端運算概論 2. AWS 架構 3. EC2 虛擬機器 4. S3 儲存服務",
			OutlineEN:    "1. Cloud Computing Overview 2. AWS Architecture 3. EC2 4. S3",
			Schedule:     "Week 1: 課程介紹 Week 2: AWS Academy",
		},
		{
			UID:          "1131U0002",
			Title:        "資料結構 Data Structures",
			Teachers:     []string{"李小華"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "學習基礎資料結構，包含陣列、鏈結串列、樹、圖",
			ObjectivesEN: "Learn fundamental data structures",
			OutlineCN:    "陣列 鏈結串列 堆疊 佇列 樹 圖 排序演算法",
			OutlineEN:    "Array, Linked List, Stack, Queue, Tree, Graph, Sorting",
			Schedule:     "Week 1-4: 基礎結構 Week 5-8: 進階結構",
		},
		{
			UID:          "1131U0003",
			Title:        "機器學習 Machine Learning",
			Teachers:     []string{"陳小明"},
			Year:         113,
			Term:         1,
			ObjectivesCN: "介紹機器學習基礎，包含監督式學習、非監督式學習",
			ObjectivesEN: "Introduction to machine learning, supervised and unsupervised learning",
			OutlineCN:    "線性迴歸 邏輯迴歸 決策樹 神經網路",
			OutlineEN:    "Linear Regression, Logistic Regression, Decision Trees, Neural Networks",
			Schedule:     "Week 1-8: 基礎 Week 9-16: 進階",
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
		UID:          "1131U0004",
		Title:        "人工智慧 Artificial Intelligence",
		Teachers:     []string{"張大華"},
		Year:         113,
		Term:         1,
		ObjectivesCN: "探索人工智慧的理論與應用，包含深度學習框架",
		ObjectivesEN: "Explore AI theory and applications with deep learning frameworks",
		OutlineCN:    "卷積神經網路 循環神經網路 強化學習 自然語言處理",
		OutlineEN:    "CNN, RNN, Reinforcement Learning, NLP",
		Schedule:     "Week 1-8: 理論 Week 9-16: 實作",
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
		UID:          "1131U0001",
		Title:        "雲端運算",
		Teachers:     []string{"王大明"},
		Year:         113,
		Term:         1,
		ObjectivesCN: "雲端運算課程",
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
