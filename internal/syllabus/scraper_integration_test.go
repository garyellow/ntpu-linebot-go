package syllabus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// TestScrapeCourseDetail_RealPage tests scraping against a real NTPU course page
// This is an integration test that requires network access
func TestScrapeCourseDetail_RealPage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a real scraper client
	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw"},
		"sea": {"https://sea.cc.ntpu.edu.tw"},
	}
	client := scraper.NewClient(30*time.Second, 3, baseURLs)
	s := NewScraper(client)

	// Test with a known course URL (演算法, 114-1)
	course := &storage.Course{
		UID:       "1141U3556",
		DetailURL: "https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query.queryguide?g_serial=U3556&g_year=114&g_term=1&show_info=all",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := s.ScrapeCourseDetail(ctx, course)
	if err != nil {
		t.Fatalf("ScrapeCourseDetail failed: %v", err)
	}
	fields := result.Fields

	t.Logf("=== Scraped Syllabus Fields ===")
	t.Logf("Objectives (%d chars): %s", len(fields.Objectives), truncateForLog(fields.Objectives, 200))
	t.Logf("Outline (%d chars): %s", len(fields.Outline), truncateForLog(fields.Outline, 200))
	t.Logf("Schedule (%d chars): %s", len(fields.Schedule), truncateForLog(fields.Schedule, 200))

	// Validate that we got content for objectives
	if fields.Objectives == "" {
		t.Error("Expected non-empty Objectives (教學目標)")
	} else {
		// Verify it contains expected content for 演算法 course
		if !containsAny(fields.Objectives, []string{"演算法", "程式", "複雜度", "algorithm"}) {
			t.Errorf("Objectives doesn't seem to contain expected content: %s", truncateForLog(fields.Objectives, 100))
		}
	}

	// Validate outline
	if fields.Outline == "" {
		t.Error("Expected non-empty Outline (內容綱要)")
	} else {
		// Verify it contains expected content
		if !containsAny(fields.Outline, []string{"Algorithm", "Dynamic", "Sorting", "Greedy", "NP"}) {
			t.Errorf("Outline doesn't seem to contain expected content: %s", truncateForLog(fields.Outline, 100))
		}
	}

	// Schedule might be empty for some courses, so just log it
	if fields.Schedule == "" {
		t.Log("Schedule (教學進度) is empty - this may be normal for some courses")
	}

	// Test that ContentForIndexing works
	content := fields.ContentForIndexing("測試課程")
	if content == "" && !fields.IsEmpty() {
		t.Error("ContentForIndexing() returned empty for non-empty fields")
	}
	t.Logf("Generated content length: %d characters", len(content))
}

// TestScrapeCourseDetail_DistinctSections verifies that each section is parsed independently
func TestScrapeCourseDetail_DistinctSections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw"},
		"sea": {"https://sea.cc.ntpu.edu.tw"},
	}
	client := scraper.NewClient(30*time.Second, 3, baseURLs)
	s := NewScraper(client)

	// Test with the algorithms course
	course := &storage.Course{
		UID:       "1141U3556",
		DetailURL: "https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query.queryguide?g_serial=U3556&g_year=114&g_term=1&show_info=all",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := s.ScrapeCourseDetail(ctx, course)
	if err != nil {
		t.Fatalf("ScrapeCourseDetail failed: %v", err)
	}
	fields := result.Fields

	// Create a map to check for duplicates
	sections := map[string]string{
		"Objectives": fields.Objectives,
		"Outline":    fields.Outline,
		"Schedule":   fields.Schedule,
	}

	// Check that non-empty sections are distinct
	for name1, content1 := range sections {
		if content1 == "" {
			continue
		}
		for name2, content2 := range sections {
			if name1 >= name2 || content2 == "" {
				continue
			}
			if content1 == content2 {
				t.Errorf("%s and %s have identical content - parsing may be broken", name1, name2)
			}
			// Also check if one is a substring of another (shouldn't happen)
			if len(content1) > 50 && len(content2) > 50 {
				if contains(content1, content2[:50]) || contains(content2, content1[:50]) {
					t.Logf("Warning: %s and %s may have overlapping content", name1, name2)
				}
			}
		}
	}

	t.Logf("Distinct sections verified:")
	t.Logf("  - Objectives: %d chars", len(fields.Objectives))
	t.Logf("  - Outline: %d chars", len(fields.Outline))
	t.Logf("  - Schedule: %d chars", len(fields.Schedule))
}

// Helper functions

// TestScrapeCourseDetail_Programs tests that programs are correctly extracted from course detail pages
func TestScrapeCourseDetail_Programs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw"},
		"sea": {"https://sea.cc.ntpu.edu.tw"},
	}
	client := scraper.NewClient(30*time.Second, 3, baseURLs)
	s := NewScraper(client)

	// Test with the algorithms course (演算法) which has multiple programs
	course := &storage.Course{
		UID:       "1141U3556",
		DetailURL: "https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query.queryguide?g_serial=U3556&g_year=114&g_term=1&show_info=all",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := s.ScrapeCourseDetail(ctx, course)
	if err != nil {
		t.Fatalf("ScrapeCourseDetail failed: %v", err)
	}

	t.Logf("=== Extracted Programs ===")
	t.Logf("Total programs found: %d", len(result.Programs))

	for i, prog := range result.Programs {
		t.Logf("  Program %d: %s (type: %s)", i+1, prog.ProgramName, prog.CourseType)
	}

	// Validate that we found programs
	if len(result.Programs) == 0 {
		t.Error("Expected to find programs, but got none")
	}

	// Expected programs for 演算法 course (may change over time, but some should be present)
	expectedPrograms := []string{
		"商業智慧與大數據分析學士學分學程",
		"資料拓析學士學分學程",
		"人工智慧學士學分學程",
	}

	for _, expected := range expectedPrograms {
		found := false
		for _, prog := range result.Programs {
			if prog.ProgramName == expected {
				found = true
				break
			}
		}
		if !found {
			t.Logf("Warning: Expected program %q not found (may have changed)", expected)
		}
	}

	// Verify no departments are included (should only have items ending with "學程")
	for _, prog := range result.Programs {
		if !strings.HasSuffix(prog.ProgramName, "學程") {
			t.Errorf("Program %q does not end with '學程' - parsing may be broken", prog.ProgramName)
		}
	}
}

func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0))
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
