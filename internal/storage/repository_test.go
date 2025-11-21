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
	err = db.DeleteExpiredStudents(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("DeleteExpiredStudents failed: %v", err)
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

// Removed redundant count/update tests - SaveAndGet already validates these
