package scraper

import (
	"context"

	"golang.org/x/sync/singleflight"
)

// CacheWrapper wraps scraping operations with singleflight to prevent cache stampede
type CacheWrapper struct {
	group singleflight.Group
}

// NewCacheWrapper creates a new cache wrapper
func NewCacheWrapper() *CacheWrapper {
	return &CacheWrapper{}
}

// DoScrape executes a scraping operation with singleflight
// Multiple concurrent requests for the same key will only execute the function once
func (c *CacheWrapper) DoScrape(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, error) {
	// Use singleflight to ensure only one goroutine per key executes the function
	result, err, _ := c.group.Do(key, func() (interface{}, error) {
		// Check context before executing
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		return fn()
	})

	return result, err
}

// Forget removes a key from singleflight group, allowing new requests to execute
func (c *CacheWrapper) Forget(key string) {
	c.group.Forget(key)
}
