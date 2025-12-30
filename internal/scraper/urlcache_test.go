package scraper

import (
	"context"
	"sync"
	"testing"
	"time"
)

// testBaseURLs provides standard URLs for testing
var testBaseURLs = map[string][]string{
	"lms": {"https://lms.ntpu.edu.tw"},
	"sea": {"https://sea.cc.ntpu.edu.tw"},
}

func TestURLCache_Get(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	t.Parallel()

	client := NewClient(5*time.Second, 3, testBaseURLs)
	cache := NewURLCache(client, "lms")

	ctx := context.Background()

	// First call should trigger failover detection
	url1, err := cache.Get(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if url1 == "" {
		t.Fatal("Expected non-empty URL")
	}

	// Second call should hit cache (instant)
	url2, err := cache.Get(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if url2 != url1 {
		t.Errorf("Expected cached URL %q, got %q", url1, url2)
	}

	// GetCached should return same URL
	cached := cache.GetCached()
	if cached != url1 {
		t.Errorf("Expected GetCached to return %q, got %q", url1, cached)
	}
}

func TestURLCache_Clear(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	t.Parallel()

	client := NewClient(5*time.Second, 3, testBaseURLs)
	cache := NewURLCache(client, "sea")

	ctx := context.Background()

	// Populate cache
	url1, err := cache.Get(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify cache is populated
	cached := cache.GetCached()
	if cached != url1 {
		t.Errorf("Expected cached URL %q, got %q", url1, cached)
	}

	// Clear cache
	cache.Clear()

	// GetCached should return empty after clear
	cached = cache.GetCached()
	if cached != "" {
		t.Errorf("Expected empty cached URL after clear, got %q", cached)
	}

	// Next Get should trigger re-detection
	url2, err := cache.Get(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if url2 == "" {
		t.Fatal("Expected non-empty URL after re-detection")
	}
}

func TestURLCache_InvalidDomain(t *testing.T) {
	t.Parallel()
	client := NewClient(5*time.Second, 3, testBaseURLs)
	cache := NewURLCache(client, "invalid_domain")

	ctx := context.Background()

	_, err := cache.Get(ctx)
	if err == nil {
		t.Fatal("Expected error for invalid domain, got none")
	}
}

func TestURLCache_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	client := NewClient(5*time.Second, 3, testBaseURLs)
	cache := NewURLCache(client, "lms")

	ctx := context.Background()
	const goroutines = 100
	var wg sync.WaitGroup

	// All goroutines should get the same cached URL without races
	urls := make([]string, goroutines)
	for i := range goroutines {
		wg.Go(func() {
			url, err := cache.Get(ctx)
			if err != nil {
				t.Errorf("Goroutine %d: unexpected error: %v", i, err)
				return
			}
			urls[i] = url
		})
	}

	wg.Wait()

	// Verify all goroutines got the same URL
	firstURL := urls[0]
	for i, url := range urls {
		if url != firstURL {
			t.Errorf("Goroutine %d got different URL: %q vs %q", i, url, firstURL)
		}
	}
}

func BenchmarkURLCache_Get(b *testing.B) {
	client := NewClient(5*time.Second, 3, testBaseURLs)
	cache := NewURLCache(client, "lms")
	ctx := context.Background()

	// Populate cache first
	_, _ = cache.Get(ctx)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = cache.Get(ctx)
		}
	})
}

func BenchmarkURLCache_GetCached(b *testing.B) {
	client := NewClient(5*time.Second, 3, testBaseURLs)
	cache := NewURLCache(client, "sea")
	ctx := context.Background()

	// Populate cache first
	_, _ = cache.Get(ctx)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cache.GetCached()
		}
	})
}
