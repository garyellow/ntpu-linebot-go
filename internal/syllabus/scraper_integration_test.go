package syllabus

import (
	"context"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// TestScrapeSyllabus_RealPage tests scraping against a real NTPU course page
// This is an integration test that requires network access
func TestScrapeSyllabus_RealPage(t *testing.T) {
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
		DetailURL: "https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query.queryGuide?g_serial=U3556&g_year=114&g_term=1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fields, err := s.ScrapeSyllabus(ctx, course)
	if err != nil {
		t.Fatalf("ScrapeSyllabus failed: %v", err)
	}

	t.Logf("=== Scraped Syllabus Fields ===")
	t.Logf("ObjectivesCN (%d chars): %s", len(fields.ObjectivesCN), truncateForLog(fields.ObjectivesCN, 200))
	t.Logf("ObjectivesEN (%d chars): %s", len(fields.ObjectivesEN), truncateForLog(fields.ObjectivesEN, 200))
	t.Logf("OutlineCN (%d chars): %s", len(fields.OutlineCN), truncateForLog(fields.OutlineCN, 200))
	t.Logf("OutlineEN (%d chars): %s", len(fields.OutlineEN), truncateForLog(fields.OutlineEN, 200))
	t.Logf("Schedule (%d chars): %s", len(fields.Schedule), truncateForLog(fields.Schedule, 200))

	// Validate that we got content for at least CN or EN objectives
	if fields.ObjectivesCN == "" && fields.ObjectivesEN == "" {
		t.Error("Expected non-empty Objectives (教學目標 or Course Objectives)")
	} else {
		// Verify it contains expected content for 演算法 course
		combinedObjectives := fields.ObjectivesCN + " " + fields.ObjectivesEN
		if !containsAny(combinedObjectives, []string{"演算法", "程式", "複雜度", "algorithm"}) {
			t.Errorf("Objectives doesn't seem to contain expected content: CN=%s EN=%s", truncateForLog(fields.ObjectivesCN, 100), truncateForLog(fields.ObjectivesEN, 100))
		}
	}

	// Validate outline
	if fields.OutlineCN == "" && fields.OutlineEN == "" {
		t.Error("Expected non-empty Outline (內容綱要 or Course Outline)")
	} else {
		// Verify it contains expected content
		combinedOutline := fields.OutlineCN + " " + fields.OutlineEN
		if !containsAny(combinedOutline, []string{"Algorithm", "Dynamic", "Sorting", "Greedy", "NP"}) {
			t.Errorf("Outline doesn't seem to contain expected content: CN=%s EN=%s", truncateForLog(fields.OutlineCN, 100), truncateForLog(fields.OutlineEN, 100))
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

	// Verify the CN and EN sections are distinct (not duplicated)
	if fields.ObjectivesCN != "" && fields.ObjectivesEN != "" {
		if fields.ObjectivesCN == fields.ObjectivesEN {
			t.Error("ObjectivesCN and ObjectivesEN are identical - possible parsing issue")
		}
	}
}

// TestScrapeSyllabus_DistinctSections verifies that each section is parsed independently
func TestScrapeSyllabus_DistinctSections(t *testing.T) {
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
		DetailURL: "https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query.queryGuide?g_serial=U3556&g_year=114&g_term=1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fields, err := s.ScrapeSyllabus(ctx, course)
	if err != nil {
		t.Fatalf("ScrapeSyllabus failed: %v", err)
	}

	// Create a map to check for duplicates
	sections := map[string]string{
		"ObjectivesCN": fields.ObjectivesCN,
		"ObjectivesEN": fields.ObjectivesEN,
		"OutlineCN":    fields.OutlineCN,
		"OutlineEN":    fields.OutlineEN,
		"Schedule":     fields.Schedule,
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
	t.Logf("  - ObjectivesCN: %d chars", len(fields.ObjectivesCN))
	t.Logf("  - ObjectivesEN: %d chars", len(fields.ObjectivesEN))
	t.Logf("  - OutlineCN: %d chars", len(fields.OutlineCN))
	t.Logf("  - OutlineEN: %d chars", len(fields.OutlineEN))
	t.Logf("  - Schedule: %d chars", len(fields.Schedule))
}

// Helper functions

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
