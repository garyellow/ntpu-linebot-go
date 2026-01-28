package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNew_FileSystemDatabase tests database creation with file system persistence
func TestNew_FileSystemDatabase(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir() // Automatically cleaned up after test
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := New(ctx, dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify database files exist
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file not created: %s", dbPath)
	}

	// Verify WAL file exists (created by PRAGMA journal_mode=WAL)
	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Logf("WAL file not found (expected after write operations): %s", walPath)
	}

	// Test write operation
	student := &Student{
		ID:         "41247001",
		Name:       "測試學生",
		Department: "資訊工程學系",
		Year:       112,
	}

	if err := db.SaveStudent(ctx, student); err != nil {
		t.Fatalf("SaveStudent failed: %v", err)
	}

	// Verify WAL file created after write
	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Errorf("WAL file not created after write: %s", walPath)
	}

	// Test read operation
	retrieved, err := db.GetStudentByID(ctx, student.ID)
	if err != nil {
		t.Fatalf("GetStudentByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected student, got nil")
		return
	}

	if retrieved.ID != student.ID {
		t.Errorf("Expected ID %s, got %s", student.ID, retrieved.ID)
	}
}

// TestNew_NestedDirectory tests database creation with nested directory path
func TestNew_NestedDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sub1", "sub2", "test.db")

	ctx := context.Background()
	db, err := New(ctx, dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create database with nested path: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify directory created
	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Errorf("Nested directory not created: %s", filepath.Dir(dbPath))
	}

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file not created in nested directory: %s", dbPath)
	}
}

// TestPing_DatabaseConnectivity tests database connectivity check
func TestPing_DatabaseConnectivity(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping failed on healthy database: %v", err)
	}
}

// TestClose_CleanShutdown tests clean database shutdown
func TestClose_CleanShutdown(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	db, err := New(ctx, dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Write some data
	student := &Student{
		ID:         "41247001",
		Name:       "測試學生",
		Department: "資訊工程學系",
		Year:       112,
	}

	if err := db.SaveStudent(ctx, student); err != nil {
		t.Fatalf("SaveStudent failed: %v", err)
	}

	// Close database
	if err := db.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Verify no corruption: reopen and read
	db2, err := New(ctx, dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to reopen database after close: %v", err)
	}
	defer func() { _ = db2.Close() }()

	retrieved, err := db2.GetStudentByID(ctx, student.ID)
	if err != nil {
		t.Fatalf("GetStudentByID failed after reopen: %v", err)
	}

	if retrieved == nil || retrieved.ID != student.ID {
		t.Error("Data lost after close and reopen")
	}
}

// setupTestDB helper is defined in repository_test.go
