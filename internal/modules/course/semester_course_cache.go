package course

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

const defaultSemesterCourseCacheTTL = 30 * time.Second

type semesterCourseCacheEntry struct {
	courses   []storage.Course
	fetchedAt time.Time
}

// SemesterCourseCache holds recent semester course lists in memory for a short time.
// It reduces repeated SQLite reads when multiple users issue similar queries in bursts.
type SemesterCourseCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[Semester]semesterCourseCacheEntry
}

// NewSemesterCourseCache creates a short-lived in-memory cache for semester course lists.
func NewSemesterCourseCache(ttl time.Duration) *SemesterCourseCache {
	if ttl <= 0 {
		ttl = defaultSemesterCourseCacheTTL
	}

	return &SemesterCourseCache{
		ttl:     ttl,
		entries: make(map[Semester]semesterCourseCacheEntry),
	}
}

// Get returns courses for a semester from memory when fresh, otherwise reloads from SQLite.
func (c *SemesterCourseCache) Get(ctx context.Context, db *storage.DB, year, term int) ([]storage.Course, error) {
	if db == nil {
		return nil, fmt.Errorf("semester course cache: db is nil")
	}

	key := Semester{Year: year, Term: term}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if ok && time.Since(entry.fetchedAt) < c.ttl {
		return cloneCourses(entry.courses), nil
	}

	courses, err := db.GetCoursesByYearTerm(ctx, year, term)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = semesterCourseCacheEntry{
		courses:   cloneCourses(courses),
		fetchedAt: time.Now(),
	}
	c.mu.Unlock()

	return cloneCourses(courses), nil
}

func cloneCourses(courses []storage.Course) []storage.Course {
	if len(courses) == 0 {
		return nil
	}

	cloned := make([]storage.Course, len(courses))
	copy(cloned, courses)
	return cloned
}
