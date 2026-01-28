package storage

import (
	"context"
	"testing"
)

// TestGetProgramByName_SemesterFilters reproduces the bug where SQL arguments are passed in wrong order
// when semester filtering is enabled.
func TestGetProgramByName_SemesterFilters(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// 1. Setup Data
	programName := "資訊工程學系學士班"
	program := Program{
		Name:     programName,
		Category: "學士學分學程",
		URL:      "https://example.com",
	}

	// Insert program metadata via SyncPrograms (which inserts to programs table)
	err := db.SyncPrograms(ctx, []struct{ Name, Category, URL string }{
		{Name: program.Name, Category: program.Category, URL: program.URL},
	})
	if err != nil {
		t.Fatalf("SyncPrograms failed: %v", err)
	}

	// Insert a course for this program in 113-1
	course := &Course{
		UID:      "1131U0001",
		Year:     113,
		Term:     1,
		No:       "U0001",
		Title:    "程式設計",
		Teachers: []string{"王老師"},
	}
	if err := db.SaveCourse(ctx, course); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	// Link course to program
	err = db.SaveCoursePrograms(ctx, course.UID, []ProgramRequirement{
		{ProgramName: programName, CourseType: "必"},
	})
	if err != nil {
		t.Fatalf("SaveCoursePrograms failed: %v", err)
	}

	// 2. Test GetProgramByName WITH semester filter
	// This should return the program with correct counts
	// The bug is that "name" is passed BEFORE semester args, but SQL expects semester args (in SELECT) then name (in WHERE)
	years := []int{113}
	terms := []int{1}

	result, err := db.GetProgramByName(ctx, programName, years, terms)
	if err != nil {
		t.Fatalf("GetProgramByName failed: %v", err)
	}

	// If the bug exists, the query might return no rows (if args mismatch causes WHERE to fail)
	// or return incorrect counts.
	// In SQLite, mixing up int (113) and string ("資訊工程學系學士班") might cause errors or just no matches.
	// The WHERE clause expects `p.name = ?`. If we pass 113 (int) to it, it won't match the program name.

	if result == nil {
		t.Fatal("Expected program result, got nil")
		return
	}

	if result.Name != programName {
		t.Errorf("Expected program name %s, got %s", programName, result.Name)
	}

	if result.RequiredCount != 1 {
		t.Errorf("Expected 1 required course, got %d. (This likely indicates semester filter failed due to argument mismatch)", result.RequiredCount)
	}
}

// TestSearchPrograms_SemesterFilters reproduces the bug in SearchPrograms
func TestSearchPrograms_SemesterFilters(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	programName := "資訊工程學系"
	err := db.SyncPrograms(ctx, []struct{ Name, Category, URL string }{
		{Name: programName, Category: "學士", URL: "http://example.com"},
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Add course to ensure verification logic (optional but good)
	course := &Course{UID: "1131A001", Year: 113, Term: 1, No: "A001", Title: "Test", Teachers: []string{"T"}}
	_ = db.SaveCourse(ctx, course)
	_ = db.SaveCoursePrograms(ctx, course.UID, []ProgramRequirement{{ProgramName: programName, CourseType: "必"}})

	// Search with filter
	years := []int{113}
	terms := []int{1}

	// Search term "資訊" matches "資訊工程學系".
	// Args order bug: passed "資訊" then 113, 1.
	// Query expects: 113, 1 (in SELECT) ... then "資訊" (in WHERE LIKE ?).
	// So params will be shifted. WHERE LIKE ? will get 113 (or last param).

	results, err := db.SearchPrograms(ctx, "資訊", years, terms)
	if err != nil {
		t.Fatalf("SearchPrograms failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find program '資訊工程學系', got 0 results. (Argument order mismatch?)")
	}
}
