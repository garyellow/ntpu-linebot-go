package contact

import (
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

const defaultContactOrgCacheTTL = 30 * time.Second

type contactOrgCacheEntry struct {
	contacts  []storage.Contact
	fetchedAt time.Time
}

// ContactOrgCache holds the member list for organization names in memory for a short time.
// It reduces repeated SQLite reads when multiple users browse the same organization's
// member list in a short burst (e.g., via the "查看成員列表" postback button).
//
// Only non-empty results are cached: when the DB has no data (triggering a scrape),
// we skip caching so the next request can re-query the DB after the scrape completes.
type ContactOrgCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]contactOrgCacheEntry
}

// NewContactOrgCache creates a short-lived in-memory cache for organization member lists.
func NewContactOrgCache(ttl time.Duration) *ContactOrgCache {
	if ttl <= 0 {
		ttl = defaultContactOrgCacheTTL
	}

	return &ContactOrgCache{
		ttl:     ttl,
		entries: make(map[string]contactOrgCacheEntry),
	}
}

// GetCached returns cached individual contacts for an organization when the entry is
// still fresh. Returns (nil, false) when the cache is cold or expired.
func (c *ContactOrgCache) GetCached(orgName string) ([]storage.Contact, bool) {
	c.mu.RLock()
	entry, ok := c.entries[orgName]
	c.mu.RUnlock()
	if ok && time.Since(entry.fetchedAt) < c.ttl {
		return cloneContacts(entry.contacts), true
	}

	return nil, false
}

// SetCached stores the individual contacts for an organization.
// Empty slices are silently skipped to avoid caching "not yet scraped" states.
func (c *ContactOrgCache) SetCached(orgName string, contacts []storage.Contact) {
	if len(contacts) == 0 {
		return
	}

	c.mu.Lock()
	c.entries[orgName] = contactOrgCacheEntry{
		contacts:  cloneContacts(contacts),
		fetchedAt: time.Now(),
	}
	c.mu.Unlock()
}

func cloneContacts(contacts []storage.Contact) []storage.Contact {
	if len(contacts) == 0 {
		return nil
	}

	cloned := make([]storage.Contact, len(contacts))
	copy(cloned, contacts)
	return cloned
}
