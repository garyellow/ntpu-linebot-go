package program

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

const defaultProgramListCacheTTL = 30 * time.Second

type programListCacheEntry struct {
	programs  []storage.Program
	fetchedAt time.Time
}

// ProgramListCache holds the full program list in memory for a short time.
// It reduces repeated complex JOIN queries when multiple users browse or search
// programs concurrently within the same burst window.
type ProgramListCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]programListCacheEntry
}

// NewProgramListCache creates a short-lived in-memory cache for program lists.
func NewProgramListCache(ttl time.Duration) *ProgramListCache {
	if ttl <= 0 {
		ttl = defaultProgramListCacheTTL
	}

	return &ProgramListCache{
		ttl:     ttl,
		entries: make(map[string]programListCacheEntry),
	}
}

// Get returns all programs for the given semester filter from memory when fresh,
// otherwise reloads from SQLite.
// Expired entries are lazily deleted on read to keep the map bounded.
func (c *ProgramListCache) Get(ctx context.Context, db *storage.DB, years, terms []int) ([]storage.Program, error) {
	if db == nil {
		return nil, fmt.Errorf("program list cache: db is nil")
	}

	key := programCacheKey(years, terms)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if ok {
		if time.Since(entry.fetchedAt) < c.ttl {
			return clonePrograms(entry.programs), nil
		}
		// Lazy eviction: remove stale entry.
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
	}

	programs, err := db.GetAllPrograms(ctx, years, terms)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = programListCacheEntry{
		programs:  programs, // store original; GetCached always returns a defensive copy
		fetchedAt: time.Now(),
	}
	c.mu.Unlock()

	return clonePrograms(programs), nil
}

// programCacheKey returns a stable string key for a (years, terms) pair.
func programCacheKey(years, terms []int) string {
	return fmt.Sprintf("%v:%v", years, terms)
}

func clonePrograms(programs []storage.Program) []storage.Program {
	if len(programs) == 0 {
		return nil
	}

	cloned := make([]storage.Program, len(programs))
	copy(cloned, programs)
	return cloned
}
