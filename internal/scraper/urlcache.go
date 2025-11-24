package scraper

import (
	"context"
	"fmt"
	"sync/atomic"
)

// URLCache provides thread-safe caching for working base URLs with automatic failover.
// Uses atomic.Value for lock-free reads (O(1) with zero contention).
//
// Design rationale:
// - 99.9% of requests hit cache (instant, no locks needed)
// - Cache miss triggers failover detection (slow path)
// - Auto-recovery: clears cache on scrape error to re-detect
//
// This pattern is optimal for read-heavy workloads where URL changes are rare.
// References:
// - https://pkg.go.dev/sync/atomic#Value
// - Go best practice: measure first, cache strategically
type URLCache struct {
	client *Client
	domain string
	cache  atomic.Value // stores string
}

// NewURLCache creates a new URL cache for a specific domain.
// Domain must be registered in Client.baseURLs (e.g., "lms", "sea").
func NewURLCache(client *Client, domain string) *URLCache {
	return &URLCache{
		client: client,
		domain: domain,
	}
}

// Get returns the cached working URL or detects a new one if cache is empty.
// Fast path (99.9% of calls): Returns cached URL immediately with O(1) lock-free read.
// Slow path (cache miss): Triggers failover detection and caches result.
func (c *URLCache) Get(ctx context.Context) (string, error) {
	// Fast path: try cached URL first (lock-free read)
	if cached := c.cache.Load(); cached != nil {
		if url, ok := cached.(string); ok && url != "" {
			return url, nil
		}
	}

	// Slow path: cache miss or empty - detect working URL
	baseURL, err := c.client.TryFailoverURLs(ctx, c.domain)
	if err != nil {
		// Fallback to first configured URL if failover detection fails
		urls := c.client.GetBaseURLs(c.domain)
		if len(urls) > 0 {
			baseURL = urls[0]
		} else {
			return "", fmt.Errorf("no URLs available for domain %s: %w", c.domain, err)
		}
	}

	// Cache the working URL for future requests (lock-free write)
	c.cache.Store(baseURL)
	return baseURL, nil
}

// Clear invalidates the cached URL, forcing re-detection on next Get().
// Call this when a scrape operation fails to trigger automatic failover.
func (c *URLCache) Clear() {
	c.cache.Store("")
}

// GetCached returns the cached URL without triggering failover detection.
// Returns empty string if cache is empty or invalid.
// Useful for debugging or metrics collection.
func (c *URLCache) GetCached() string {
	if cached := c.cache.Load(); cached != nil {
		if url, ok := cached.(string); ok {
			return url
		}
	}
	return ""
}
