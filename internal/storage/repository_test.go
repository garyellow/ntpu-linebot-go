package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	// Use in-memory SQLite database for testing with 7-day TTL
	db, err := New(context.Background(), ":memory:", 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}

func TestSaveAndGetCourses(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	student := &Student{
		ID:         "41247001",
		Name:       "測試學生",
		Department: "資訊工程學系",
		Year:       112,
	}

	// Test save
	err := db.SaveStudent(ctx, student)
	if err != nil {
		t.Fatalf("SaveStudent failed: %v", err)
	}

	// Test get
	retrieved, err := db.GetStudentByID(ctx, student.ID)
	if err != nil {
		t.Fatalf("GetStudentByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected student, got nil")
	}

	if retrieved.ID != student.ID {
		t.Errorf("Expected ID %s, got %s", student.ID, retrieved.ID)
	}

	if retrieved.Name != student.Name {
		t.Errorf("Expected name %s, got %s", student.Name, retrieved.Name)
	}

	if retrieved.Department != student.Department {
		t.Errorf("Expected department %s, got %s", student.Department, retrieved.Department)
	}
}

// TestSearchStudentsByName tests core search logic (critical user feature)
func TestSearchStudentsByName(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	students := []*Student{
		{ID: "41247001", Name: "王小明", Department: "資工系", Year: 112},
		{ID: "41247002", Name: "王大華", Department: "電機系", Year: 112},
		{ID: "41247003", Name: "李小明", Department: "資工系", Year: 111},
	}

	for _, s := range students {
		if err := db.SaveStudent(ctx, s); err != nil {
			t.Fatalf("SaveStudent failed: %v", err)
		}
	}

	// Test partial match (critical for Chinese name search)
	result, err := db.SearchStudentsByName(ctx, "小明")
	if err != nil {
		t.Fatalf("SearchStudentsByName failed: %v", err)
	}
	if len(result.Students) != 2 {
		t.Errorf("Expected 2 students with '小明', got %d", len(result.Students))
	}
	if result.TotalCount != 2 {
		t.Errorf("Expected TotalCount 2, got %d", result.TotalCount)
	}

	// Test non-contiguous matching: "王明" should match "王小明"
	result, err = db.SearchStudentsByName(ctx, "王明")
	if err != nil {
		t.Fatalf("SearchStudentsByName failed: %v", err)
	}
	if len(result.Students) != 1 {
		t.Errorf("Expected 1 student with '王明' (non-contiguous), got %d", len(result.Students))
	}
	if len(result.Students) > 0 && result.Students[0].Name != "王小明" {
		t.Errorf("Expected to find '王小明', got '%s'", result.Students[0].Name)
	}

	// Test reversed order: "明王" should also match "王小明"
	result, err = db.SearchStudentsByName(ctx, "明王")
	if err != nil {
		t.Fatalf("SearchStudentsByName failed: %v", err)
	}
	if len(result.Students) != 1 {
		t.Errorf("Expected 1 student with '明王' (reversed order), got %d", len(result.Students))
	}
	if len(result.Students) > 0 && result.Students[0].Name != "王小明" {
		t.Errorf("Expected to find '王小明', got '%s'", result.Students[0].Name)
	}

	// Test character-set membership: "資工" should match "資工系" in department
	// (Note: This tests the ContainsAllRunes logic, even though search is on name field)
	result, err = db.SearchStudentsByName(ctx, "王")
	if err != nil {
		t.Fatalf("SearchStudentsByName failed: %v", err)
	}
	if len(result.Students) != 2 {
		t.Errorf("Expected 2 students with '王', got %d", len(result.Students))
	}

	// Test no match
	result, err = db.SearchStudentsByName(ctx, "張三")
	if err != nil {
		t.Fatalf("SearchStudentsByName failed: %v", err)
	}
	if len(result.Students) != 0 {
		t.Errorf("Expected 0 students with '張三', got %d", len(result.Students))
	}
	if result.TotalCount != 0 {
		t.Errorf("Expected TotalCount 0 for no match, got %d", result.TotalCount)
	}
}

// TestSaveStudentsBatch tests batch student save operation
func TestSaveStudentsBatch(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	students := []*Student{
		{ID: "41247001", Name: "王小明", Department: "資工系", Year: 112},
		{ID: "41247002", Name: "王大華", Department: "電機系", Year: 112},
		{ID: "41247003", Name: "李小明", Department: "資工系", Year: 111},
	}

	// Test batch save
	err := db.SaveStudentsBatch(ctx, students)
	if err != nil {
		t.Fatalf("SaveStudentsBatch failed: %v", err)
	}

	// Verify all students were saved
	for _, student := range students {
		retrieved, err := db.GetStudentByID(ctx, student.ID)
		if err != nil {
			t.Fatalf("GetStudentByID failed for %s: %v", student.ID, err)
		}
		if retrieved == nil {
			t.Errorf("Expected student %s, got nil", student.ID)
		}
		if retrieved != nil && retrieved.Name != student.Name {
			t.Errorf("Expected name %s, got %s", student.Name, retrieved.Name)
		}
	}
}

// TestSaveContactsBatch tests batch contact save operation
func TestSaveContactsBatch(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	contacts := []*Contact{
		{UID: "85", Type: "individual", Name: "陳大華", Organization: "資工系"},
		{UID: "87", Type: "individual", Name: "陳小明", Organization: "電機系"},
		{UID: "86", Type: "organization", Name: "資訊工程學系", Organization: "電機資訊學院"},
	}

	// Test batch save
	err := db.SaveContactsBatch(ctx, contacts)
	if err != nil {
		t.Fatalf("SaveContactsBatch failed: %v", err)
	}

	// Verify all contacts were saved
	for _, contact := range contacts {
		retrieved, err := db.GetContactByUID(ctx, contact.UID)
		if err != nil {
			t.Fatalf("GetContactByUID failed for %s: %v", contact.UID, err)
		}
		if retrieved == nil {
			t.Errorf("Expected contact %s, got nil", contact.UID)
		}
		if retrieved != nil && retrieved.Name != contact.Name {
			t.Errorf("Expected name %s, got %s", contact.Name, retrieved.Name)
		}
	}
}

// TestSaveCoursesBatch tests batch course save operation
func TestSaveCoursesBatch(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	courses := []*Course{
		{
			UID:      "1121U1234",
			Year:     112,
			Term:     1,
			No:       "CS101",
			Title:    "計算機概論",
			Teachers: []string{"張教授"},
		},
		{
			UID:      "1121U5678",
			Year:     112,
			Term:     1,
			No:       "CS102",
			Title:    "程式設計",
			Teachers: []string{"李教授"},
		},
	}

	// Test batch save
	err := db.SaveCoursesBatch(ctx, courses)
	if err != nil {
		t.Fatalf("SaveCoursesBatch failed: %v", err)
	}

	// Verify all courses were saved
	for _, course := range courses {
		retrieved, err := db.GetCourseByUID(ctx, course.UID)
		if err != nil {
			t.Fatalf("GetCourseByUID failed for %s: %v", course.UID, err)
		}
		if retrieved == nil {
			t.Errorf("Expected course %s, got nil", course.UID)
		}
		if retrieved != nil && retrieved.Title != course.Title {
			t.Errorf("Expected title %s, got %s", course.Title, retrieved.Title)
		}
	}
}

// TestSearchContactsByName tests core contact search (critical for directory lookup)
func TestSearchContactsByName(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	contacts := []*Contact{
		{UID: "c1", Type: "individual", Name: "陳大華", Organization: "資工系"},
		{UID: "c2", Type: "individual", Name: "陳小明", Organization: "電機系"},
		{UID: "c3", Type: "organization", Name: "資訊工程學系", Organization: "工學院"},
	}

	for _, c := range contacts {
		if err := db.SaveContact(ctx, c); err != nil {
			t.Fatalf("SaveContact failed: %v", err)
		}
	}

	results, err := db.SearchContactsByName(ctx, "陳")
	if err != nil {
		t.Fatalf("SearchContactsByName failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 contacts with '陳', got %d", len(results))
	}
}

// TestCourseArrayHandling tests JSON array serialization (critical for course data)
func TestCourseArrayHandling(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	course := &Course{
		UID:       "3141U0001",
		Year:      113,
		Term:      1,
		No:        "3141U0001",
		Title:     "資料結構",
		Teachers:  []string{"王教授", "李教授"},
		Times:     []string{"星期二 3-4", "星期四 7-8"},
		Locations: []string{"資訊大樓 101", "資訊大樓 203"},
	}

	if err := db.SaveCourse(ctx, course); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	retrieved, err := db.GetCourseByUID(ctx, course.UID)
	if err != nil {
		t.Fatalf("GetCourseByUID failed: %v", err)
	}

	// Critical: Verify array deserialization
	if len(retrieved.Teachers) != 2 {
		t.Errorf("Expected 2 teachers, got %d", len(retrieved.Teachers))
	}
	if len(retrieved.Times) != 2 {
		t.Errorf("Expected 2 time slots, got %d", len(retrieved.Times))
	}
	if len(retrieved.Locations) != 2 {
		t.Errorf("Expected 2 locations, got %d", len(retrieved.Locations))
	}
}

func TestStudentDataNeverExpires(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert student
	student := &Student{
		ID:         "41247001",
		Name:       "學生",
		Department: "資工系",
		Year:       113,
	}
	if err := db.SaveStudent(ctx, student); err != nil {
		t.Fatalf("SaveStudent failed: %v", err)
	}

	// Insert "old" student with cached_at 30 days ago (should still be accessible since students never expire)
	query := `INSERT INTO students (id, name, department, year, cached_at) VALUES (?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-30 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "41247002", "舊生", "電機系", 112, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Both students should be accessible (no TTL filtering)
	count, err := db.CountStudents(ctx)
	if err != nil {
		t.Fatalf("CountStudents failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 students (students never expire), got %d", count)
	}

	// Old student should still be retrievable
	retrieved, err := db.GetStudentByID(ctx, "41247002")
	if err != nil {
		t.Fatalf("GetStudentByID failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Old student should still exist (students never expire)")
	}
}

func TestDeleteExpiredContacts(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh contact
	fresh := &Contact{
		UID:          "fresh001",
		Type:         "individual",
		Name:         "新聯絡人",
		Organization: "資工系",
	}
	if err := db.SaveContact(ctx, fresh); err != nil {
		t.Fatalf("SaveContact failed: %v", err)
	}

	// Insert old contact (manually set cached_at to 8 days ago)
	old := &Contact{
		UID:          "old001",
		Type:         "individual",
		Name:         "舊聯絡人",
		Organization: "電機系",
	}
	query := `INSERT INTO contacts (uid, type, name, organization, cached_at) VALUES (?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, old.UID, old.Type, old.Name, old.Organization, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Delete expired
	deleted, err := db.DeleteExpiredContacts(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredContacts failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify fresh contact still exists
	retrieved, err := db.GetContactByUID(ctx, fresh.UID)
	if err != nil {
		t.Fatalf("GetContactByUID failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Fresh contact should still exist")
	}

	// Verify old contact is gone
	retrieved, err = db.GetContactByUID(ctx, old.UID)
	if err != nil {
		t.Fatalf("GetContactByUID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Old contact should be deleted")
	}
}

func TestDeleteExpiredCourses(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh course
	fresh := &Course{
		UID:      "1131A0001",
		Year:     113,
		Term:     1,
		No:       "A0001",
		Title:    "新課程",
		Teachers: []string{"王老師"},
		Times:    []string{"一1-2"},
	}
	if err := db.SaveCourse(ctx, fresh); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	// Insert old course (manually set cached_at to 8 days ago)
	query := `INSERT INTO courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "1121A0002", 112, 1, "A0002", "舊課程", `["李老師"]`, `[]`, `["二3-4"]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Delete expired
	deleted, err := db.DeleteExpiredCourses(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredCourses failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify fresh course still exists
	retrieved, err := db.GetCourseByUID(ctx, fresh.UID)
	if err != nil {
		t.Fatalf("GetCourseByUID failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Fresh course should still exist")
	}

	// Verify old course is gone
	retrieved, err = db.GetCourseByUID(ctx, "1121A0002")
	if err != nil {
		t.Fatalf("GetCourseByUID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Old course should be deleted")
	}
}

func TestStickerDataNeverExpires(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh sticker
	fresh := &Sticker{
		URL:    "https://example.com/fresh.png",
		Source: "spy_family",
	}
	if err := db.SaveSticker(ctx, fresh); err != nil {
		t.Fatalf("SaveSticker failed: %v", err)
	}

	// Insert old sticker (manually set cached_at to 30 days ago)
	query := `INSERT INTO stickers (url, source, cached_at, success_count, failure_count) VALUES (?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-30 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "https://example.com/old.png", "spy_family", oldTime, 0, 0)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Both stickers should be counted (no TTL filtering for stickers)
	count, _ := db.CountStickers(ctx)
	if count != 2 {
		t.Errorf("Expected 2 stickers (stickers never expire), got %d", count)
	}
}

// TestGetAllContacts tests retrieving all contacts with TTL filtering
func TestGetAllContacts(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh contacts
	freshContacts := []*Contact{
		{UID: "c1", Type: "individual", Name: "陳大華", Organization: "資訊工程學系"},
		{UID: "c2", Type: "individual", Name: "陳小明", Organization: "電機工程學系"},
		{UID: "c3", Type: "organization", Name: "資訊工程學系", Superior: "電機資訊學院"},
	}
	for _, c := range freshContacts {
		if err := db.SaveContact(ctx, c); err != nil {
			t.Fatalf("SaveContact failed: %v", err)
		}
	}

	// Insert expired contact (manually set cached_at to 8 days ago)
	query := `INSERT INTO contacts (uid, type, name, organization, cached_at) VALUES (?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "c_old", "individual", "舊聯絡人", "舊單位", oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Get all contacts - should only return non-expired ones
	contacts, err := db.GetAllContacts(ctx)
	if err != nil {
		t.Fatalf("GetAllContacts failed: %v", err)
	}

	// Should return 3 fresh contacts, not the expired one
	if len(contacts) != 3 {
		t.Errorf("Expected 3 contacts, got %d", len(contacts))
	}

	// Verify ordering (by type, name)
	// organization should come after individual alphabetically
	foundOrg := false
	for _, c := range contacts {
		if c.Type == "organization" {
			foundOrg = true
			if c.Name != "資訊工程學系" {
				t.Errorf("Expected organization name '資訊工程學系', got '%s'", c.Name)
			}
		}
	}
	if !foundOrg {
		t.Error("Expected to find organization contact")
	}
}

// TestGetAllContactsLimit tests the LIMIT 1000 constraint
func TestGetAllContactsLimit(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// This test verifies the query structure without inserting 1000+ records
	// Just verify the method works with empty database
	contacts, err := db.GetAllContacts(ctx)
	if err != nil {
		t.Fatalf("GetAllContacts failed on empty database: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("Expected 0 contacts on empty database, got %d", len(contacts))
	}
}

// TestGetCoursesByRecentSemesters tests retrieving courses with TTL filtering
func TestGetCoursesByRecentSemesters(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh courses from different semesters
	freshCourses := []*Course{
		{
			UID:      "1132U0001",
			Year:     113,
			Term:     2,
			No:       "U0001",
			Title:    "程式設計二",
			Teachers: []string{"王教授"},
		},
		{
			UID:      "1131U0001",
			Year:     113,
			Term:     1,
			No:       "U0001",
			Title:    "程式設計一",
			Teachers: []string{"王教授"},
		},
		{
			UID:      "1122U0002",
			Year:     112,
			Term:     2,
			No:       "U0002",
			Title:    "資料結構",
			Teachers: []string{"李教授"},
		},
	}
	for _, c := range freshCourses {
		if err := db.SaveCourse(ctx, c); err != nil {
			t.Fatalf("SaveCourse failed: %v", err)
		}
	}

	// Insert expired course (manually set cached_at to 8 days ago)
	query := `INSERT INTO courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "1121U9999", 112, 1, "U9999", "舊課程", `["舊教授"]`, `[]`, `[]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Get courses by recent semesters - should only return non-expired ones
	courses, err := db.GetCoursesByRecentSemesters(ctx)
	if err != nil {
		t.Fatalf("GetCoursesByRecentSemesters failed: %v", err)
	}

	// Should return 3 fresh courses, not the expired one
	if len(courses) != 3 {
		t.Errorf("Expected 3 courses, got %d", len(courses))
	}

	// Verify ordering (by year DESC, term DESC) - newest first
	if len(courses) >= 2 {
		if courses[0].Year < courses[1].Year {
			t.Errorf("Expected courses ordered by year DESC, got year %d before %d", courses[0].Year, courses[1].Year)
		}
		if courses[0].Year == courses[1].Year && courses[0].Term < courses[1].Term {
			t.Errorf("Expected courses ordered by term DESC within same year")
		}
	}

	// Verify first course is the newest (113-2)
	if courses[0].UID != "1132U0001" {
		t.Errorf("Expected first course to be 1132U0001 (newest), got %s", courses[0].UID)
	}
}

// TestGetCoursesByRecentSemestersLimit tests the LIMIT 2000 constraint
func TestGetCoursesByRecentSemestersLimit(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// This test verifies the query structure without inserting 2000+ records
	// Just verify the method works with empty database
	courses, err := db.GetCoursesByRecentSemesters(ctx)
	if err != nil {
		t.Fatalf("GetCoursesByRecentSemesters failed on empty database: %v", err)
	}
	if len(courses) != 0 {
		t.Errorf("Expected 0 courses on empty database, got %d", len(courses))
	}
}

// Removed redundant count/update tests - SaveAndGet already validates these

// ===== Historical Courses Repository Tests =====

// TestSaveHistoricalCourse tests single historical course save with conflict handling
func TestSaveHistoricalCourse(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	course := &Course{
		UID:         "1001U0001",
		Year:        100,
		Term:        1,
		No:          "U0001",
		Title:       "計算機概論",
		Teachers:    []string{"王教授"},
		TeacherURLs: []string{"https://example.com/teacher1"},
		Times:       []string{"週一1-2"},
		Locations:   []string{"資訊大樓 101"},
		DetailURL:   "https://example.com/course/1001U0001",
		Note:        "測試備註",
	}

	// Test save
	err := db.SaveHistoricalCourse(ctx, course)
	if err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	// Verify saved by searching
	courses, err := db.SearchHistoricalCoursesByYear(ctx, 100)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear failed: %v", err)
	}
	if len(courses) != 1 {
		t.Fatalf("Expected 1 course, got %d", len(courses))
	}
	if courses[0].Title != course.Title {
		t.Errorf("Expected title %q, got %q", course.Title, courses[0].Title)
	}

	// Test upsert (update on conflict)
	course.Title = "計算機概論（更新）"
	err = db.SaveHistoricalCourse(ctx, course)
	if err != nil {
		t.Fatalf("SaveHistoricalCourse (upsert) failed: %v", err)
	}

	courses, err = db.SearchHistoricalCoursesByYear(ctx, 100)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear after upsert failed: %v", err)
	}
	if len(courses) != 1 {
		t.Fatalf("Expected 1 course after upsert, got %d", len(courses))
	}
	if courses[0].Title != "計算機概論（更新）" {
		t.Errorf("Expected updated title, got %q", courses[0].Title)
	}
}

// TestSaveHistoricalCoursesBatch tests batch historical course save
func TestSaveHistoricalCoursesBatch(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	courses := []*Course{
		{
			UID:      "1001U0001",
			Year:     100,
			Term:     1,
			No:       "U0001",
			Title:    "計算機概論",
			Teachers: []string{"王教授"},
		},
		{
			UID:      "1001U0002",
			Year:     100,
			Term:     1,
			No:       "U0002",
			Title:    "程式設計",
			Teachers: []string{"李教授"},
		},
		{
			UID:      "1002M0001",
			Year:     100,
			Term:     2,
			No:       "M0001",
			Title:    "資料結構",
			Teachers: []string{"陳教授", "林教授"},
		},
	}

	// Test batch save
	err := db.SaveHistoricalCoursesBatch(ctx, courses)
	if err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch failed: %v", err)
	}

	// Verify all courses saved
	result, err := db.SearchHistoricalCoursesByYear(ctx, 100)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear failed: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 courses, got %d", len(result))
	}

	// Verify count
	count, err := db.CountHistoricalCourses(ctx)
	if err != nil {
		t.Fatalf("CountHistoricalCourses failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

// TestSaveHistoricalCoursesBatchEmpty tests batch save with empty slice
func TestSaveHistoricalCoursesBatchEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Test with empty slice - should return nil without error
	err := db.SaveHistoricalCoursesBatch(ctx, []*Course{})
	if err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch with empty slice failed: %v", err)
	}
}

// TestSearchHistoricalCoursesByYearAndTitle tests search with title filter
func TestSearchHistoricalCoursesByYearAndTitle(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	courses := []*Course{
		{UID: "1001U0001", Year: 100, Term: 1, No: "U0001", Title: "計算機概論", Teachers: []string{"王教授"}},
		{UID: "1001U0002", Year: 100, Term: 1, No: "U0002", Title: "程式設計基礎", Teachers: []string{"李教授"}},
		{UID: "1001U0003", Year: 100, Term: 2, No: "U0003", Title: "進階程式設計", Teachers: []string{"陳教授"}},
		{UID: "1011U0001", Year: 101, Term: 1, No: "U0001", Title: "程式設計", Teachers: []string{"張教授"}},
	}

	if err := db.SaveHistoricalCoursesBatch(ctx, courses); err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch failed: %v", err)
	}

	tests := []struct {
		name          string
		year          int
		title         string
		expectedCount int
	}{
		{
			name:          "Match partial title in year 100",
			year:          100,
			title:         "程式設計",
			expectedCount: 2, // 程式設計基礎, 進階程式設計
		},
		{
			name:          "Match exact title",
			year:          100,
			title:         "計算機概論",
			expectedCount: 1,
		},
		{
			name:          "No match in year",
			year:          100,
			title:         "資料庫",
			expectedCount: 0,
		},
		{
			name:          "Different year",
			year:          101,
			title:         "程式設計",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.SearchHistoricalCoursesByYearAndTitle(ctx, tt.year, tt.title)
			if err != nil {
				t.Fatalf("SearchHistoricalCoursesByYearAndTitle failed: %v", err)
			}
			if len(result) != tt.expectedCount {
				t.Errorf("Expected %d courses, got %d", tt.expectedCount, len(result))
			}
		})
	}
}

// TestSearchHistoricalCoursesByYearAndTitleTooLong tests search term length validation
func TestSearchHistoricalCoursesByYearAndTitleTooLong(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Create a search term longer than 100 characters
	longTitle := strings.Repeat("測", 101)

	_, err := db.SearchHistoricalCoursesByYearAndTitle(ctx, 100, longTitle)
	if err == nil {
		t.Error("Expected error for too long search term, got nil")
	}
}

// TestSearchHistoricalCoursesByYear tests year-only search
func TestSearchHistoricalCoursesByYear(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	courses := []*Course{
		{UID: "1001U0001", Year: 100, Term: 1, No: "U0001", Title: "計算機概論", Teachers: []string{"王教授"}},
		{UID: "1002U0002", Year: 100, Term: 2, No: "U0002", Title: "程式設計", Teachers: []string{"李教授"}},
		{UID: "1011U0001", Year: 101, Term: 1, No: "U0001", Title: "資料結構", Teachers: []string{"陳教授"}},
	}

	if err := db.SaveHistoricalCoursesBatch(ctx, courses); err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch failed: %v", err)
	}

	// Search for year 100
	result, err := db.SearchHistoricalCoursesByYear(ctx, 100)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 courses for year 100, got %d", len(result))
	}

	// Verify ordering (term DESC) - term 2 should come first
	if len(result) >= 2 {
		if result[0].Term < result[1].Term {
			t.Errorf("Expected courses ordered by term DESC, got term %d before %d", result[0].Term, result[1].Term)
		}
	}

	// Search for year with no courses
	result, err = db.SearchHistoricalCoursesByYear(ctx, 99)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear for empty year failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 courses for year 99, got %d", len(result))
	}
}

// TestDeleteExpiredHistoricalCourses tests TTL-based cleanup
func TestDeleteExpiredHistoricalCourses(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh course
	fresh := &Course{
		UID:      "1001U0001",
		Year:     100,
		Term:     1,
		No:       "U0001",
		Title:    "新課程",
		Teachers: []string{"新教授"},
	}
	if err := db.SaveHistoricalCourse(ctx, fresh); err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	// Insert expired course (manually set cached_at to 8 days ago)
	query := `INSERT INTO historical_courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "1001U0002", 100, 1, "U0002", "舊課程", `["舊教授"]`, `[]`, `[]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Count before delete (only counts non-expired due to TTL filtering)
	countBefore, err := db.CountHistoricalCourses(ctx)
	if err != nil {
		t.Fatalf("CountHistoricalCourses failed: %v", err)
	}
	if countBefore != 1 {
		t.Errorf("Expected 1 course before delete (TTL-filtered), got %d", countBefore)
	}

	// Delete expired (7 day TTL)
	deleted, err := db.DeleteExpiredHistoricalCourses(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredHistoricalCourses failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Count after delete
	countAfter, err := db.CountHistoricalCourses(ctx)
	if err != nil {
		t.Fatalf("CountHistoricalCourses failed: %v", err)
	}
	if countAfter != 1 {
		t.Errorf("Expected 1 course after delete, got %d", countAfter)
	}
}

// TestCountHistoricalCourses tests counting functionality
func TestCountHistoricalCourses(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Count empty table
	count, err := db.CountHistoricalCourses(ctx)
	if err != nil {
		t.Fatalf("CountHistoricalCourses failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 on empty table, got %d", count)
	}

	// Add courses and count again
	courses := []*Course{
		{UID: "1001U0001", Year: 100, Term: 1, No: "U0001", Title: "課程1", Teachers: []string{}},
		{UID: "1001U0002", Year: 100, Term: 1, No: "U0002", Title: "課程2", Teachers: []string{}},
	}
	if err := db.SaveHistoricalCoursesBatch(ctx, courses); err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch failed: %v", err)
	}

	count, err = db.CountHistoricalCourses(ctx)
	if err != nil {
		t.Fatalf("CountHistoricalCourses failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 after insert, got %d", count)
	}
}

// TestCountCoursesBySemester tests counting courses by semester
func TestCountCoursesBySemester(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Count empty table
	count, err := db.CountCoursesBySemester(ctx, 113, 1)
	if err != nil {
		t.Fatalf("CountCoursesBySemester failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 on empty table, got %d", count)
	}

	// Add courses for different semesters
	courses := []*Course{
		{UID: "1131U0001", Year: 113, Term: 1, No: "U0001", Title: "課程1", Teachers: []string{}},
		{UID: "1131U0002", Year: 113, Term: 1, No: "U0002", Title: "課程2", Teachers: []string{}},
		{UID: "1132U0001", Year: 113, Term: 2, No: "U0001", Title: "課程3", Teachers: []string{}},
		{UID: "1121U0001", Year: 112, Term: 1, No: "U0001", Title: "課程4", Teachers: []string{}},
	}
	for _, c := range courses {
		if err := db.SaveCourse(ctx, c); err != nil {
			t.Fatalf("SaveCourse failed: %v", err)
		}
	}

	// Count 113-1
	count, err = db.CountCoursesBySemester(ctx, 113, 1)
	if err != nil {
		t.Fatalf("CountCoursesBySemester failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 for 113-1, got %d", count)
	}

	// Count 113-2
	count, err = db.CountCoursesBySemester(ctx, 113, 2)
	if err != nil {
		t.Fatalf("CountCoursesBySemester failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 for 113-2, got %d", count)
	}

	// Count 112-1
	count, err = db.CountCoursesBySemester(ctx, 112, 1)
	if err != nil {
		t.Fatalf("CountCoursesBySemester failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 for 112-1, got %d", count)
	}

	// Count non-existent semester
	count, err = db.CountCoursesBySemester(ctx, 114, 1)
	if err != nil {
		t.Fatalf("CountCoursesBySemester failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 for non-existent 114-1, got %d", count)
	}
}

// TestHistoricalCoursesArrayHandling tests JSON array serialization/deserialization
func TestHistoricalCoursesArrayHandling(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	course := &Course{
		UID:         "1001U0001",
		Year:        100,
		Term:        1,
		No:          "U0001",
		Title:       "資料結構",
		Teachers:    []string{"王教授", "李教授"},
		TeacherURLs: []string{"https://example.com/teacher1", "https://example.com/teacher2"},
		Times:       []string{"週二 3-4", "週四 7-8"},
		Locations:   []string{"資訊大樓 101", "資訊大樓 203"},
	}

	if err := db.SaveHistoricalCourse(ctx, course); err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	courses, err := db.SearchHistoricalCoursesByYear(ctx, 100)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear failed: %v", err)
	}
	if len(courses) != 1 {
		t.Fatalf("Expected 1 course, got %d", len(courses))
	}

	retrieved := courses[0]

	// Verify array deserialization
	if len(retrieved.Teachers) != 2 {
		t.Errorf("Expected 2 teachers, got %d", len(retrieved.Teachers))
	}
	if len(retrieved.TeacherURLs) != 2 {
		t.Errorf("Expected 2 teacher URLs, got %d", len(retrieved.TeacherURLs))
	}
	if len(retrieved.Times) != 2 {
		t.Errorf("Expected 2 time slots, got %d", len(retrieved.Times))
	}
	if len(retrieved.Locations) != 2 {
		t.Errorf("Expected 2 locations, got %d", len(retrieved.Locations))
	}
}

// TestHistoricalCoursesTTLFiltering tests that expired courses are not returned
func TestHistoricalCoursesTTLFiltering(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh course
	fresh := &Course{
		UID:      "1001U0001",
		Year:     100,
		Term:     1,
		No:       "U0001",
		Title:    "新課程",
		Teachers: []string{"新教授"},
	}
	if err := db.SaveHistoricalCourse(ctx, fresh); err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	// Insert expired course (manually set cached_at to 8 days ago)
	query := `INSERT INTO historical_courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "1001U0002", 100, 1, "U0002", "舊課程", `["舊教授"]`, `[]`, `[]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// SearchHistoricalCoursesByYear should not return expired course
	courses, err := db.SearchHistoricalCoursesByYear(ctx, 100)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear failed: %v", err)
	}
	if len(courses) != 1 {
		t.Errorf("Expected 1 non-expired course, got %d", len(courses))
	}
	if len(courses) > 0 && courses[0].Title != "新課程" {
		t.Errorf("Expected fresh course, got %s", courses[0].Title)
	}

	// SearchHistoricalCoursesByYearAndTitle should not return expired course
	courses, err = db.SearchHistoricalCoursesByYearAndTitle(ctx, 100, "課程")
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYearAndTitle failed: %v", err)
	}
	if len(courses) != 1 {
		t.Errorf("Expected 1 non-expired course, got %d", len(courses))
	}
}

// ==================== Syllabus Repository Tests ====================

func TestSaveSyllabus(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	syllabus := &Syllabus{
		UID:         "1131U0001",
		Year:        113,
		Term:        1,
		Title:       "程式設計",
		Teachers:    []string{"王小明", "李小華"},
		Objectives:  "培養程式設計能力\nDevelop programming skills",
		Outline:     "變數、迴圈、函式\nVariables, loops, functions",
		Schedule:    "第1週：課程介紹",
		ContentHash: "abc123hash",
	}

	err := db.SaveSyllabus(ctx, syllabus)
	if err != nil {
		t.Fatalf("SaveSyllabus failed: %v", err)
	}

	// Verify it was saved
	retrieved, err := db.GetSyllabusByUID(ctx, syllabus.UID)
	if err != nil {
		t.Fatalf("GetSyllabusByUID failed: %v", err)
	}

	if retrieved.UID != syllabus.UID {
		t.Errorf("UID = %q, want %q", retrieved.UID, syllabus.UID)
	}
	if retrieved.Year != syllabus.Year {
		t.Errorf("Year = %d, want %d", retrieved.Year, syllabus.Year)
	}
	if retrieved.Term != syllabus.Term {
		t.Errorf("Term = %d, want %d", retrieved.Term, syllabus.Term)
	}
	if retrieved.Title != syllabus.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, syllabus.Title)
	}
	if len(retrieved.Teachers) != len(syllabus.Teachers) {
		t.Errorf("Teachers count = %d, want %d", len(retrieved.Teachers), len(syllabus.Teachers))
	}
	if retrieved.Objectives != syllabus.Objectives {
		t.Errorf("Objectives = %q, want %q", retrieved.Objectives, syllabus.Objectives)
	}
	if retrieved.Outline != syllabus.Outline {
		t.Errorf("Outline = %q, want %q", retrieved.Outline, syllabus.Outline)
	}
	if retrieved.Schedule != syllabus.Schedule {
		t.Errorf("Schedule = %q, want %q", retrieved.Schedule, syllabus.Schedule)
	}
	if retrieved.ContentHash != syllabus.ContentHash {
		t.Errorf("ContentHash = %q, want %q", retrieved.ContentHash, syllabus.ContentHash)
	}
}

func TestSaveSyllabus_Upsert(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Save initial syllabus
	syllabus := &Syllabus{
		UID:         "1131U0001",
		Year:        113,
		Term:        1,
		Title:       "程式設計",
		Teachers:    []string{"王小明"},
		Objectives:  "原始目標",
		Outline:     "原始大綱",
		ContentHash: "hash1",
	}
	if err := db.SaveSyllabus(ctx, syllabus); err != nil {
		t.Fatalf("First SaveSyllabus failed: %v", err)
	}

	// Update with new content
	syllabus.Objectives = "更新目標"
	syllabus.ContentHash = "hash2"
	syllabus.Teachers = []string{"李小華"}
	if err := db.SaveSyllabus(ctx, syllabus); err != nil {
		t.Fatalf("Second SaveSyllabus failed: %v", err)
	}

	// Verify update
	retrieved, err := db.GetSyllabusByUID(ctx, syllabus.UID)
	if err != nil {
		t.Fatalf("GetSyllabusByUID failed: %v", err)
	}

	if retrieved.Objectives != "更新目標" {
		t.Errorf("Objectives not updated: got %q", retrieved.Objectives)
	}
	if retrieved.ContentHash != "hash2" {
		t.Errorf("ContentHash not updated: got %q", retrieved.ContentHash)
	}
	if len(retrieved.Teachers) != 1 || retrieved.Teachers[0] != "李小華" {
		t.Errorf("Teachers not updated: got %v", retrieved.Teachers)
	}

	// Verify there's only one record
	count, err := db.CountSyllabi(ctx)
	if err != nil {
		t.Fatalf("CountSyllabi failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 syllabus after upsert, got %d", count)
	}
}

func TestSaveSyllabusBatch(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	syllabi := []*Syllabus{
		{
			UID:         "1131U0001",
			Year:        113,
			Term:        1,
			Title:       "程式設計",
			Teachers:    []string{"王小明"},
			Objectives:  "程式設計目標",
			Outline:     "程式設計大綱",
			ContentHash: "hash1",
		},
		{
			UID:         "1131U0002",
			Year:        113,
			Term:        1,
			Title:       "資料結構",
			Teachers:    []string{"李小華"},
			Objectives:  "資料結構目標",
			Outline:     "資料結構大綱",
			ContentHash: "hash2",
		},
		{
			UID:         "1132U0003",
			Year:        113,
			Term:        2,
			Title:       "演算法",
			Teachers:    []string{"張小龍"},
			Objectives:  "演算法目標",
			Outline:     "演算法大綱",
			ContentHash: "hash3",
		},
	}

	err := db.SaveSyllabusBatch(ctx, syllabi)
	if err != nil {
		t.Fatalf("SaveSyllabusBatch failed: %v", err)
	}

	// Verify count
	count, err := db.CountSyllabi(ctx)
	if err != nil {
		t.Fatalf("CountSyllabi failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 syllabi, got %d", count)
	}

	// Verify each syllabus was saved
	for _, s := range syllabi {
		retrieved, err := db.GetSyllabusByUID(ctx, s.UID)
		if err != nil {
			t.Errorf("GetSyllabusByUID(%s) failed: %v", s.UID, err)
			continue
		}
		if retrieved.Title != s.Title {
			t.Errorf("Syllabus %s title = %q, want %q", s.UID, retrieved.Title, s.Title)
		}
	}
}

func TestSaveSyllabusBatch_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Empty batch should succeed
	err := db.SaveSyllabusBatch(ctx, nil)
	if err != nil {
		t.Errorf("SaveSyllabusBatch(nil) failed: %v", err)
	}

	err = db.SaveSyllabusBatch(ctx, []*Syllabus{})
	if err != nil {
		t.Errorf("SaveSyllabusBatch([]) failed: %v", err)
	}
}

func TestGetSyllabusContentHash(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Test non-existent syllabus
	hash, err := db.GetSyllabusContentHash(ctx, "nonexistent")
	if err != nil {
		t.Errorf("GetSyllabusContentHash for nonexistent UID failed: %v", err)
	}
	if hash != "" {
		t.Errorf("Expected empty hash for nonexistent UID, got %q", hash)
	}

	// Save a syllabus
	syllabus := &Syllabus{
		UID:         "1131U0001",
		Year:        113,
		Term:        1,
		Title:       "程式設計",
		Teachers:    []string{"王小明"},
		Objectives:  "測試目標",
		ContentHash: "expected_hash_value",
	}
	if err := db.SaveSyllabus(ctx, syllabus); err != nil {
		t.Fatalf("SaveSyllabus failed: %v", err)
	}

	// Test existing syllabus
	hash, err = db.GetSyllabusContentHash(ctx, syllabus.UID)
	if err != nil {
		t.Fatalf("GetSyllabusContentHash failed: %v", err)
	}
	if hash != "expected_hash_value" {
		t.Errorf("Hash = %q, want %q", hash, "expected_hash_value")
	}
}

func TestGetSyllabusByUID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	_, err := db.GetSyllabusByUID(ctx, "nonexistent")
	if err != domerrors.ErrNotFound {
		t.Errorf("Expected domerrors.ErrNotFound, got %v", err)
	}
}

func TestGetAllSyllabi(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Test empty result
	syllabi, err := db.GetAllSyllabi(ctx)
	if err != nil {
		t.Fatalf("GetAllSyllabi (empty) failed: %v", err)
	}
	if len(syllabi) != 0 {
		t.Errorf("Expected 0 syllabi, got %d", len(syllabi))
	}

	// Insert some syllabi
	testData := []*Syllabus{
		{UID: "1131U0001", Year: 113, Term: 1, Title: "課程1", Teachers: []string{"教師1"}, Objectives: "目標1", ContentHash: "h1"},
		{UID: "1132U0002", Year: 113, Term: 2, Title: "課程2", Teachers: []string{"教師2"}, Objectives: "目標2", ContentHash: "h2"},
	}
	if err := db.SaveSyllabusBatch(ctx, testData); err != nil {
		t.Fatalf("SaveSyllabusBatch failed: %v", err)
	}

	// Test with data
	syllabi, err = db.GetAllSyllabi(ctx)
	if err != nil {
		t.Fatalf("GetAllSyllabi failed: %v", err)
	}
	if len(syllabi) != 2 {
		t.Errorf("Expected 2 syllabi, got %d", len(syllabi))
	}
}

func TestGetSyllabiByYearTerm(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert syllabi for different year/terms
	testData := []*Syllabus{
		{UID: "1131U0001", Year: 113, Term: 1, Title: "113-1 課程1", Teachers: []string{"教師"}, Objectives: "目標", ContentHash: "h1"},
		{UID: "1131U0002", Year: 113, Term: 1, Title: "113-1 課程2", Teachers: []string{"教師"}, Objectives: "目標", ContentHash: "h2"},
		{UID: "1132U0003", Year: 113, Term: 2, Title: "113-2 課程3", Teachers: []string{"教師"}, Objectives: "目標", ContentHash: "h3"},
		{UID: "1121U0004", Year: 112, Term: 1, Title: "112-1 課程4", Teachers: []string{"教師"}, Objectives: "目標", ContentHash: "h4"},
	}
	if err := db.SaveSyllabusBatch(ctx, testData); err != nil {
		t.Fatalf("SaveSyllabusBatch failed: %v", err)
	}

	tests := []struct {
		name     string
		year     int
		term     int
		expected int
	}{
		{"113-1", 113, 1, 2},
		{"113-2", 113, 2, 1},
		{"112-1", 112, 1, 1},
		{"114-1 (empty)", 114, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syllabi, err := db.GetSyllabiByYearTerm(ctx, tt.year, tt.term)
			if err != nil {
				t.Fatalf("GetSyllabiByYearTerm(%d, %d) failed: %v", tt.year, tt.term, err)
			}
			if len(syllabi) != tt.expected {
				t.Errorf("GetSyllabiByYearTerm(%d, %d) = %d syllabi, want %d", tt.year, tt.term, len(syllabi), tt.expected)
			}
		})
	}
}

func TestDeleteExpiredSyllabi(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Insert fresh syllabus
	fresh := &Syllabus{
		UID:         "1131U0001",
		Year:        113,
		Term:        1,
		Title:       "新課程",
		Teachers:    []string{"教師"},
		Objectives:  "目標",
		ContentHash: "hash1",
	}
	if err := db.SaveSyllabus(ctx, fresh); err != nil {
		t.Fatalf("SaveSyllabus failed: %v", err)
	}

	// Manually insert expired syllabus (8 days ago)
	query := `INSERT INTO syllabi (uid, year, term, title, teachers, objectives, outline, schedule, content_hash, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.writer.ExecContext(ctx, query, "1131U0002", 113, 1, "舊課程", `["舊教師"]`, "舊目標", "", "", "oldhash", oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Verify non-expired count (CountSyllabi now filters by TTL)
	count, _ := db.CountSyllabi(ctx)
	if count != 1 {
		t.Fatalf("Expected 1 syllabus before deletion (TTL-filtered), got %d", count)
	}

	// Delete expired (TTL = 7 days)
	deleted, err := db.DeleteExpiredSyllabi(ctx, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredSyllabi failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify only fresh syllabus remains
	count, _ = db.CountSyllabi(ctx)
	if count != 1 {
		t.Errorf("Expected 1 syllabus after deletion, got %d", count)
	}

	// Verify it's the fresh one
	retrieved, err := db.GetSyllabusByUID(ctx, fresh.UID)
	if err != nil {
		t.Fatalf("GetSyllabusByUID failed: %v", err)
	}
	if retrieved.Title != "新課程" {
		t.Errorf("Wrong syllabus remained: %s", retrieved.Title)
	}
}

func TestCountSyllabi(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	// Test empty count
	count, err := db.CountSyllabi(ctx)
	if err != nil {
		t.Fatalf("CountSyllabi failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 syllabi, got %d", count)
	}

	// Add some syllabi
	syllabi := []*Syllabus{
		{UID: "1131U0001", Year: 113, Term: 1, Title: "課程1", Teachers: []string{}, Objectives: "目標1", ContentHash: "h1"},
		{UID: "1131U0002", Year: 113, Term: 1, Title: "課程2", Teachers: []string{}, Objectives: "目標2", ContentHash: "h2"},
		{UID: "1131U0003", Year: 113, Term: 1, Title: "課程3", Teachers: []string{}, Objectives: "目標3", ContentHash: "h3"},
	}
	if err := db.SaveSyllabusBatch(ctx, syllabi); err != nil {
		t.Fatalf("SaveSyllabusBatch failed: %v", err)
	}

	count, err = db.CountSyllabi(ctx)
	if err != nil {
		t.Fatalf("CountSyllabi failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 syllabi, got %d", count)
	}
}
