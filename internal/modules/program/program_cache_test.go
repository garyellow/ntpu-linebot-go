package program

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

func setupProgramCacheTestDB(t *testing.T) *storage.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.New(context.Background(), dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db
}

func TestProgramListCacheLoadsFromDB(t *testing.T) {
	t.Parallel()

	db := setupProgramCacheTestDB(t)
	ctx := context.Background()
	cache := NewProgramListCache(time.Minute)

	// Seed one program
	if err := db.SyncPrograms(ctx, []struct{ Name, Category, URL string }{
		{Name: "資訊管理學程", Category: "學程", URL: ""},
	}); err != nil {
		t.Fatalf("SyncPrograms failed: %v", err)
	}

	programs, err := cache.Get(ctx, db, nil, nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}
	if programs[0].Name != "資訊管理學程" {
		t.Fatalf("expected program name 資訊管理學程, got %q", programs[0].Name)
	}
}

func TestProgramListCacheReturnsCachedValueBeforeTTL(t *testing.T) {
	t.Parallel()

	db := setupProgramCacheTestDB(t)
	ctx := context.Background()
	cache := NewProgramListCache(time.Minute)

	if err := db.SyncPrograms(ctx, []struct{ Name, Category, URL string }{
		{Name: "資訊管理學程", Category: "學程", URL: ""},
	}); err != nil {
		t.Fatalf("SyncPrograms failed: %v", err)
	}

	first, err := cache.Get(ctx, db, nil, nil)
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected 1 program on first load, got %d", len(first))
	}

	// Add a second program after the cache is warm
	if err := db.SyncPrograms(ctx, []struct{ Name, Category, URL string }{
		{Name: "法律學程", Category: "學程", URL: ""},
	}); err != nil {
		t.Fatalf("SyncPrograms failed: %v", err)
	}

	second, err := cache.Get(ctx, db, nil, nil)
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("expected cached result to stay at 1 program before TTL, got %d", len(second))
	}
}

func TestProgramListCacheRefreshesAfterTTL(t *testing.T) {
	t.Parallel()

	db := setupProgramCacheTestDB(t)
	ctx := context.Background()
	cache := NewProgramListCache(20 * time.Millisecond)

	if err := db.SyncPrograms(ctx, []struct{ Name, Category, URL string }{
		{Name: "資訊管理學程", Category: "學程", URL: ""},
	}); err != nil {
		t.Fatalf("SyncPrograms failed: %v", err)
	}

	if _, err := cache.Get(ctx, db, nil, nil); err != nil {
		t.Fatalf("initial Get failed: %v", err)
	}

	if err := db.SyncPrograms(ctx, []struct{ Name, Category, URL string }{
		{Name: "法律學程", Category: "學程", URL: ""},
	}); err != nil {
		t.Fatalf("SyncPrograms failed: %v", err)
	}

	time.Sleep(40 * time.Millisecond)

	refreshed, err := cache.Get(ctx, db, nil, nil)
	if err != nil {
		t.Fatalf("refreshed Get failed: %v", err)
	}
	if len(refreshed) != 2 {
		t.Fatalf("expected refreshed result to include 2 programs after TTL, got %d", len(refreshed))
	}
}

func TestProgramListCacheNilDB(t *testing.T) {
	t.Parallel()

	cache := NewProgramListCache(time.Minute)
	_, err := cache.Get(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when db is nil")
	}
}

func TestProgramCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		years []int
		terms []int
		want  string
	}{
		{"nil slices", nil, nil, "[]:[]"},
		{"single semester", []int{114}, []int{2}, "[114]:[2]"},
		{"two semesters", []int{114, 113}, []int{2, 1}, "[114 113]:[2 1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := programCacheKey(tt.years, tt.terms)
			if got != tt.want {
				t.Errorf("programCacheKey(%v, %v) = %q, want %q", tt.years, tt.terms, got, tt.want)
			}
		})
	}
}
