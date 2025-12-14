package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
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

// Global cache registry to ensure URL caches are shared across all usages
// Key: "client_ptr_domain" format to support multiple clients
var (
	urlCacheRegistry = make(map[string]*URLCache)
	urlCacheMutex    sync.RWMutex
)

// NewURLCache returns a shared URL cache for a specific domain.
// If a cache for this client+domain combination already exists, it returns the existing one.
// This ensures URL detection results are shared across all scrapers.
// Domain must be registered in Client.baseURLs (e.g., "lms", "sea").
func NewURLCache(client *Client, domain string) *URLCache {
	// Create a unique key based on client pointer and domain
	key := fmt.Sprintf("%p_%s", client, domain)

	// Fast path: try read lock first
	urlCacheMutex.RLock()
	if cache, exists := urlCacheRegistry[key]; exists {
		urlCacheMutex.RUnlock()
		return cache
	}
	urlCacheMutex.RUnlock()

	// Slow path: acquire write lock and create new cache
	urlCacheMutex.Lock()
	defer urlCacheMutex.Unlock()

	// Double-check after acquiring write lock
	if cache, exists := urlCacheRegistry[key]; exists {
		return cache
	}

	cache := &URLCache{
		client: client,
		domain: domain,
	}
	urlCacheRegistry[key] = cache
	return cache
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
	oldURL := c.GetCached()
	start := time.Now()
	baseURL, err := c.client.TryFailoverURLs(ctx, c.domain)
	if err != nil {
		slog.WarnContext(ctx, "URL failover detection failed",
			"domain", c.domain,
			"error", err,
			"duration_ms", time.Since(start).Milliseconds())

		// Fallback to first configured URL if failover detection fails
		urls := c.client.GetBaseURLs(c.domain)
		if len(urls) > 0 {
			baseURL = urls[0]
		} else {
			return "", fmt.Errorf("no URLs available for domain %s: %w", c.domain, err)
		}
	}

	// Log failover event if URL changed
	if oldURL != "" && oldURL != baseURL {
		slog.InfoContext(ctx, "URL failover successful",
			"domain", c.domain,
			"old_url", oldURL,
			"new_url", baseURL,
			"detection_duration_ms", time.Since(start).Milliseconds())
	} else if oldURL == "" {
		slog.DebugContext(ctx, "initial URL detected",
			"domain", c.domain,
			"url", baseURL,
			"detection_duration_ms", time.Since(start).Milliseconds())
	}

	// Cache the working URL for future requests (lock-free write)
	c.cache.Store(baseURL)
	return baseURL, nil
}

// Clear invalidates the cached URL, forcing re-detection on next Get().
// Call this when a scrape operation fails to trigger automatic failover.
func (c *URLCache) Clear() {
	previousURL := c.GetCached()
	c.cache.Store("")
	if previousURL != "" {
		slog.Info("URL cache cleared due to scrape failure",
			"domain", c.domain,
			"previous_url", previousURL)
	}
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

// ClearURLCacheRegistry clears all entries from the global URL cache registry.
// This is intended for testing purposes to ensure test isolation.
// In production, Client instances should be long-lived singletons.
func ClearURLCacheRegistry() {
	urlCacheMutex.Lock()
	defer urlCacheMutex.Unlock()
	urlCacheRegistry = make(map[string]*URLCache)
}
