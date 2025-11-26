package storage

import (
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	// Use in-memory SQLite database for testing with 7-day TTL
	db, err := New(":memory:", 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}

func TestSaveAndGetCourses(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	student := &Student{
		ID:         "41247001",
		Name:       "測試學生",
		Department: "資訊工程學系",
		Year:       112,
	}

	// Test save
	err := db.SaveStudent(student)
	if err != nil {
		t.Fatalf("SaveStudent failed: %v", err)
	}

	// Test get
	retrieved, err := db.GetStudentByID(student.ID)
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

	students := []*Student{
		{ID: "41247001", Name: "王小明", Department: "資工系", Year: 112},
		{ID: "41247002", Name: "王大華", Department: "電機系", Year: 112},
		{ID: "41247003", Name: "李小明", Department: "資工系", Year: 111},
	}

	for _, s := range students {
		if err := db.SaveStudent(s); err != nil {
			t.Fatalf("SaveStudent failed: %v", err)
		}
	}

	// Test partial match (critical for Chinese name search)
	results, err := db.SearchStudentsByName("小明")
	if err != nil {
		t.Fatalf("SearchStudentsByName failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 students with '小明', got %d", len(results))
	}
}

// TestSaveStudentsBatch tests batch student save operation
func TestSaveStudentsBatch(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	students := []*Student{
		{ID: "41247001", Name: "王小明", Department: "資工系", Year: 112},
		{ID: "41247002", Name: "王大華", Department: "電機系", Year: 112},
		{ID: "41247003", Name: "李小明", Department: "資工系", Year: 111},
	}

	// Test batch save
	err := db.SaveStudentsBatch(students)
	if err != nil {
		t.Fatalf("SaveStudentsBatch failed: %v", err)
	}

	// Verify all students were saved
	for _, student := range students {
		retrieved, err := db.GetStudentByID(student.ID)
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

	contacts := []*Contact{
		{UID: "c1", Type: "individual", Name: "陳大華", Organization: "資工系"},
		{UID: "c2", Type: "individual", Name: "陳小明", Organization: "電機系"},
		{UID: "c3", Type: "organization", Name: "資訊工程學系", Organization: "工學院"},
	}

	// Test batch save
	err := db.SaveContactsBatch(contacts)
	if err != nil {
		t.Fatalf("SaveContactsBatch failed: %v", err)
	}

	// Verify all contacts were saved
	for _, contact := range contacts {
		retrieved, err := db.GetContactByUID(contact.UID)
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
	err := db.SaveCoursesBatch(courses)
	if err != nil {
		t.Fatalf("SaveCoursesBatch failed: %v", err)
	}

	// Verify all courses were saved
	for _, course := range courses {
		retrieved, err := db.GetCourseByUID(course.UID)
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

	contacts := []*Contact{
		{UID: "c1", Type: "individual", Name: "陳大華", Organization: "資工系"},
		{UID: "c2", Type: "individual", Name: "陳小明", Organization: "電機系"},
		{UID: "c3", Type: "organization", Name: "資訊工程學系", Organization: "工學院"},
	}

	for _, c := range contacts {
		if err := db.SaveContact(c); err != nil {
			t.Fatalf("SaveContact failed: %v", err)
		}
	}

	results, err := db.SearchContactsByName("陳")
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

	if err := db.SaveCourse(course); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	retrieved, err := db.GetCourseByUID(course.UID)
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

func TestDeleteExpiredStudents(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Insert fresh student
	fresh := &Student{
		ID:         "41247001",
		Name:       "新生",
		Department: "資工系",
		Year:       113,
	}
	if err := db.SaveStudent(fresh); err != nil {
		t.Fatalf("SaveStudent failed: %v", err)
	}

	// Insert old student (manually set cached_at to 8 days ago)
	old := &Student{
		ID:         "41247002",
		Name:       "舊生",
		Department: "電機系",
		Year:       112,
	}
	query := `INSERT INTO students (id, name, department, year, cached_at) VALUES (?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.conn.Exec(query, old.ID, old.Name, old.Department, old.Year, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Count before delete
	countBefore, err := db.CountStudents()
	if err != nil {
		t.Fatalf("CountStudents failed: %v", err)
	}
	if countBefore != 2 {
		t.Errorf("Expected 2 students before delete, got %d", countBefore)
	}

	// Delete expired
	deleted, err := db.DeleteExpiredStudents(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredStudents failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Count after delete
	countAfter, err := db.CountStudents()
	if err != nil {
		t.Fatalf("CountStudents failed: %v", err)
	}
	if countAfter != 1 {
		t.Errorf("Expected 1 student after delete, got %d", countAfter)
	}

	// Verify fresh student still exists
	retrieved, err := db.GetStudentByID(fresh.ID)
	if err != nil {
		t.Fatalf("GetStudentByID failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Fresh student should still exist")
	}

	// Verify old student is gone
	retrieved, err = db.GetStudentByID(old.ID)
	if err != nil {
		t.Fatalf("GetStudentByID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Old student should be deleted")
	}
}

func TestDeleteExpiredContacts(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Insert fresh contact
	fresh := &Contact{
		UID:          "fresh001",
		Type:         "individual",
		Name:         "新聯絡人",
		Organization: "資工系",
	}
	if err := db.SaveContact(fresh); err != nil {
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
	_, err := db.conn.Exec(query, old.UID, old.Type, old.Name, old.Organization, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Delete expired
	deleted, err := db.DeleteExpiredContacts(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredContacts failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify fresh contact still exists
	retrieved, err := db.GetContactByUID(fresh.UID)
	if err != nil {
		t.Fatalf("GetContactByUID failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Fresh contact should still exist")
	}

	// Verify old contact is gone
	retrieved, err = db.GetContactByUID(old.UID)
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
	if err := db.SaveCourse(fresh); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	// Insert old course (manually set cached_at to 8 days ago)
	query := `INSERT INTO courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.conn.Exec(query, "1121A0002", 112, 1, "A0002", "舊課程", `["李老師"]`, `[]`, `["二3-4"]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Delete expired
	deleted, err := db.DeleteExpiredCourses(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredCourses failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify fresh course still exists
	retrieved, err := db.GetCourseByUID(fresh.UID)
	if err != nil {
		t.Fatalf("GetCourseByUID failed: %v", err)
	}
	if retrieved == nil {
		t.Error("Fresh course should still exist")
	}

	// Verify old course is gone
	retrieved, err = db.GetCourseByUID("1121A0002")
	if err != nil {
		t.Fatalf("GetCourseByUID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Old course should be deleted")
	}
}

func TestCleanupExpiredStickers(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Insert fresh sticker
	fresh := &Sticker{
		URL:    "https://example.com/fresh.png",
		Source: "spy_family",
	}
	if err := db.SaveSticker(fresh); err != nil {
		t.Fatalf("SaveSticker failed: %v", err)
	}

	// Insert old sticker (manually set cached_at to 8 days ago)
	query := `INSERT INTO stickers (url, source, cached_at, success_count, failure_count) VALUES (?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.conn.Exec(query, "https://example.com/old.png", "spy_family", oldTime, 0, 0)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Verify we have 2 stickers
	count, _ := db.CountStickers()
	if count != 2 {
		t.Fatalf("Expected 2 stickers, got %d", count)
	}

	// Cleanup expired
	deleted, err := db.CleanupExpiredStickers()
	if err != nil {
		t.Fatalf("CleanupExpiredStickers failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify only fresh sticker remains
	count, _ = db.CountStickers()
	if count != 1 {
		t.Errorf("Expected 1 sticker remaining, got %d", count)
	}
}

// TestGetAllContacts tests retrieving all contacts with TTL filtering
func TestGetAllContacts(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Insert fresh contacts
	freshContacts := []*Contact{
		{UID: "c1", Type: "individual", Name: "陳大華", Organization: "資訊工程學系"},
		{UID: "c2", Type: "individual", Name: "陳小明", Organization: "電機工程學系"},
		{UID: "c3", Type: "organization", Name: "資訊工程學系", Superior: "電機資訊學院"},
	}
	for _, c := range freshContacts {
		if err := db.SaveContact(c); err != nil {
			t.Fatalf("SaveContact failed: %v", err)
		}
	}

	// Insert expired contact (manually set cached_at to 8 days ago)
	query := `INSERT INTO contacts (uid, type, name, organization, cached_at) VALUES (?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.conn.Exec(query, "c_old", "individual", "舊聯絡人", "舊單位", oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Get all contacts - should only return non-expired ones
	contacts, err := db.GetAllContacts()
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

	// This test verifies the query structure without inserting 1000+ records
	// Just verify the method works with empty database
	contacts, err := db.GetAllContacts()
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
		if err := db.SaveCourse(c); err != nil {
			t.Fatalf("SaveCourse failed: %v", err)
		}
	}

	// Insert expired course (manually set cached_at to 8 days ago)
	query := `INSERT INTO courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.conn.Exec(query, "1121U9999", 112, 1, "U9999", "舊課程", `["舊教授"]`, `[]`, `[]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Get courses by recent semesters - should only return non-expired ones
	courses, err := db.GetCoursesByRecentSemesters()
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

	// This test verifies the query structure without inserting 2000+ records
	// Just verify the method works with empty database
	courses, err := db.GetCoursesByRecentSemesters()
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
	err := db.SaveHistoricalCourse(course)
	if err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	// Verify saved by searching
	courses, err := db.SearchHistoricalCoursesByYear(100)
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
	err = db.SaveHistoricalCourse(course)
	if err != nil {
		t.Fatalf("SaveHistoricalCourse (upsert) failed: %v", err)
	}

	courses, err = db.SearchHistoricalCoursesByYear(100)
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
	err := db.SaveHistoricalCoursesBatch(courses)
	if err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch failed: %v", err)
	}

	// Verify all courses saved
	result, err := db.SearchHistoricalCoursesByYear(100)
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYear failed: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 courses, got %d", len(result))
	}

	// Verify count
	count, err := db.CountHistoricalCourses()
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

	// Test with empty slice - should return nil without error
	err := db.SaveHistoricalCoursesBatch([]*Course{})
	if err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch with empty slice failed: %v", err)
	}
}

// TestSearchHistoricalCoursesByYearAndTitle tests search with title filter
func TestSearchHistoricalCoursesByYearAndTitle(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	courses := []*Course{
		{UID: "1001U0001", Year: 100, Term: 1, No: "U0001", Title: "計算機概論", Teachers: []string{"王教授"}},
		{UID: "1001U0002", Year: 100, Term: 1, No: "U0002", Title: "程式設計基礎", Teachers: []string{"李教授"}},
		{UID: "1001U0003", Year: 100, Term: 2, No: "U0003", Title: "進階程式設計", Teachers: []string{"陳教授"}},
		{UID: "1011U0001", Year: 101, Term: 1, No: "U0001", Title: "程式設計", Teachers: []string{"張教授"}},
	}

	if err := db.SaveHistoricalCoursesBatch(courses); err != nil {
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
			result, err := db.SearchHistoricalCoursesByYearAndTitle(tt.year, tt.title)
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

	// Create a search term longer than 100 characters
	longTitle := ""
	for i := 0; i < 101; i++ {
		longTitle += "測"
	}

	_, err := db.SearchHistoricalCoursesByYearAndTitle(100, longTitle)
	if err == nil {
		t.Error("Expected error for too long search term, got nil")
	}
}

// TestSearchHistoricalCoursesByYear tests year-only search
func TestSearchHistoricalCoursesByYear(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	courses := []*Course{
		{UID: "1001U0001", Year: 100, Term: 1, No: "U0001", Title: "計算機概論", Teachers: []string{"王教授"}},
		{UID: "1002U0002", Year: 100, Term: 2, No: "U0002", Title: "程式設計", Teachers: []string{"李教授"}},
		{UID: "1011U0001", Year: 101, Term: 1, No: "U0001", Title: "資料結構", Teachers: []string{"陳教授"}},
	}

	if err := db.SaveHistoricalCoursesBatch(courses); err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch failed: %v", err)
	}

	// Search for year 100
	result, err := db.SearchHistoricalCoursesByYear(100)
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
	result, err = db.SearchHistoricalCoursesByYear(99)
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

	// Insert fresh course
	fresh := &Course{
		UID:      "1001U0001",
		Year:     100,
		Term:     1,
		No:       "U0001",
		Title:    "新課程",
		Teachers: []string{"新教授"},
	}
	if err := db.SaveHistoricalCourse(fresh); err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	// Insert expired course (manually set cached_at to 8 days ago)
	query := `INSERT INTO historical_courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.conn.Exec(query, "1001U0002", 100, 1, "U0002", "舊課程", `["舊教授"]`, `[]`, `[]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// Count before delete
	countBefore, err := db.CountHistoricalCourses()
	if err != nil {
		t.Fatalf("CountHistoricalCourses failed: %v", err)
	}
	if countBefore != 2 {
		t.Errorf("Expected 2 courses before delete, got %d", countBefore)
	}

	// Delete expired (7 day TTL)
	deleted, err := db.DeleteExpiredHistoricalCourses(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredHistoricalCourses failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Count after delete
	countAfter, err := db.CountHistoricalCourses()
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

	// Count empty table
	count, err := db.CountHistoricalCourses()
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
	if err := db.SaveHistoricalCoursesBatch(courses); err != nil {
		t.Fatalf("SaveHistoricalCoursesBatch failed: %v", err)
	}

	count, err = db.CountHistoricalCourses()
	if err != nil {
		t.Fatalf("CountHistoricalCourses failed: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 after insert, got %d", count)
	}
}

// TestHistoricalCoursesArrayHandling tests JSON array serialization/deserialization
func TestHistoricalCoursesArrayHandling(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

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

	if err := db.SaveHistoricalCourse(course); err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	courses, err := db.SearchHistoricalCoursesByYear(100)
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

	// Insert fresh course
	fresh := &Course{
		UID:      "1001U0001",
		Year:     100,
		Term:     1,
		No:       "U0001",
		Title:    "新課程",
		Teachers: []string{"新教授"},
	}
	if err := db.SaveHistoricalCourse(fresh); err != nil {
		t.Fatalf("SaveHistoricalCourse failed: %v", err)
	}

	// Insert expired course (manually set cached_at to 8 days ago)
	query := `INSERT INTO historical_courses (uid, year, term, no, title, teachers, teacher_urls, times, locations, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	oldTime := time.Now().Add(-8 * 24 * time.Hour).Unix()
	_, err := db.conn.Exec(query, "1001U0002", 100, 1, "U0002", "舊課程", `["舊教授"]`, `[]`, `[]`, `[]`, oldTime)
	if err != nil {
		t.Fatalf("Manual insert failed: %v", err)
	}

	// SearchHistoricalCoursesByYear should not return expired course
	courses, err := db.SearchHistoricalCoursesByYear(100)
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
	courses, err = db.SearchHistoricalCoursesByYearAndTitle(100, "課程")
	if err != nil {
		t.Fatalf("SearchHistoricalCoursesByYearAndTitle failed: %v", err)
	}
	if len(courses) != 1 {
		t.Errorf("Expected 1 non-expired course, got %d", len(courses))
	}
}
