package course

import (
	"context"
	"errors"
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
// Expired entries are lazily deleted on read to keep the map bounded.
func (c *SemesterCourseCache) Get(ctx context.Context, db *storage.DB, year, term int) ([]storage.Course, error) {
	if db == nil {
		return nil, errors.New("semester course cache: db is nil")
	}

	key := Semester{Year: year, Term: term}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if ok {
		if time.Since(entry.fetchedAt) < c.ttl {
			return cloneCourses(entry.courses), nil
		}
		// Lazy eviction: remove stale entry.
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
	}

	courses, err := db.GetCoursesByYearTerm(ctx, year, term)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = semesterCourseCacheEntry{
		courses:   courses, // store original; Get always returns a defensive copy
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
