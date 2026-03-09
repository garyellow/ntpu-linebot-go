package course

import (
	"context"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

func TestSemesterCourseCacheLoadsFromDB(t *testing.T) {
	t.Parallel()

	db := setupSemesterTestDB(t)
	ctx := context.Background()
	cache := NewSemesterCourseCache(time.Minute)

	if err := db.SaveCourse(ctx, &storage.Course{UID: "1142U0001", Year: 114, Term: 2, No: "U0001", Title: "資料結構"}); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	courses, err := cache.Get(ctx, db, 114, 2)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(courses) != 1 {
		t.Fatalf("expected 1 course, got %d", len(courses))
	}
	if courses[0].Title != "資料結構" {
		t.Fatalf("expected course title 資料結構, got %q", courses[0].Title)
	}
}

func TestSemesterCourseCacheReturnsCachedValueBeforeTTL(t *testing.T) {
	t.Parallel()

	db := setupSemesterTestDB(t)
	ctx := context.Background()
	cache := NewSemesterCourseCache(time.Minute)

	if err := db.SaveCourse(ctx, &storage.Course{UID: "1142U0001", Year: 114, Term: 2, No: "U0001", Title: "資料結構"}); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	first, err := cache.Get(ctx, db, 114, 2)
	if err != nil {
		t.Fatalf("first Get failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected 1 course on first load, got %d", len(first))
	}

	if err := db.SaveCourse(ctx, &storage.Course{UID: "1142U0002", Year: 114, Term: 2, No: "U0002", Title: "演算法"}); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	second, err := cache.Get(ctx, db, 114, 2)
	if err != nil {
		t.Fatalf("second Get failed: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("expected cached result to stay at 1 course before TTL, got %d", len(second))
	}
}

func TestSemesterCourseCacheRefreshesAfterTTL(t *testing.T) {
	t.Parallel()

	db := setupSemesterTestDB(t)
	ctx := context.Background()
	cache := NewSemesterCourseCache(20 * time.Millisecond)

	if err := db.SaveCourse(ctx, &storage.Course{UID: "1142U0001", Year: 114, Term: 2, No: "U0001", Title: "資料結構"}); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	if _, err := cache.Get(ctx, db, 114, 2); err != nil {
		t.Fatalf("initial Get failed: %v", err)
	}

	if err := db.SaveCourse(ctx, &storage.Course{UID: "1142U0002", Year: 114, Term: 2, No: "U0002", Title: "演算法"}); err != nil {
		t.Fatalf("SaveCourse failed: %v", err)
	}

	time.Sleep(40 * time.Millisecond)

	refreshed, err := cache.Get(ctx, db, 114, 2)
	if err != nil {
		t.Fatalf("refreshed Get failed: %v", err)
	}
	if len(refreshed) != 2 {
		t.Fatalf("expected refreshed result to include 2 courses after TTL, got %d", len(refreshed))
	}
}

func TestSemesterCourseCacheNilDB(t *testing.T) {
	t.Parallel()

	cache := NewSemesterCourseCache(time.Minute)
	_, err := cache.Get(context.Background(), nil, 114, 2)
	if err == nil {
		t.Fatal("expected error when db is nil")
	}
}
