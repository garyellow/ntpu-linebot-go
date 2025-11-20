package scraper

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheWrapperSingleExecution(t *testing.T) {
	wrapper := NewCacheWrapper()
	ctx := context.Background()

	var execCount int32
	key := "test-key"

	// Simulate 10 concurrent requests for the same key
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			result, err := wrapper.DoScrape(ctx, key, func() (interface{}, error) {
				atomic.AddInt32(&execCount, 1)
				time.Sleep(100 * time.Millisecond) // Simulate slow operation
				return "result", nil
			})

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != "result" {
				t.Errorf("Expected 'result', got %v", result)
			}
		}()
	}

	wg.Wait()

	// Verify function was executed only once despite 10 concurrent requests
	if execCount != 1 {
		t.Errorf("Expected function to execute once, but executed %d times", execCount)
	}
}

func TestCacheWrapperDifferentKeys(t *testing.T) {
	wrapper := NewCacheWrapper()
	ctx := context.Background()

	var execCount int32

	// Execute with different keys - should execute separately
	var wg sync.WaitGroup
	keys := []string{"key1", "key2", "key3"}

	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()

			_, err := wrapper.DoScrape(ctx, k, func() (interface{}, error) {
				atomic.AddInt32(&execCount, 1)
				time.Sleep(50 * time.Millisecond)
				return k + "-result", nil
			})

			if err != nil {
				t.Errorf("Unexpected error for key %s: %v", k, err)
			}
		}(key)
	}

	wg.Wait()

	// Should execute once per unique key
	if execCount != int32(len(keys)) {
		t.Errorf("Expected %d executions, got %d", len(keys), execCount)
	}
}

func TestCacheWrapperError(t *testing.T) {
	wrapper := NewCacheWrapper()
	ctx := context.Background()

	expectedErr := errors.New("scraping failed")

	result, err := wrapper.DoScrape(ctx, "error-key", func() (interface{}, error) {
		return nil, expectedErr
	})

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
}

func TestCacheWrapperContextCancellation(t *testing.T) {
	wrapper := NewCacheWrapper()
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context immediately
	cancel()

	_, err := wrapper.DoScrape(ctx, "cancelled-key", func() (interface{}, error) {
		t.Error("Function should not execute when context is cancelled")
		return nil, nil
	})

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestCacheWrapperForget(t *testing.T) {
	wrapper := NewCacheWrapper()
	ctx := context.Background()

	var execCount int32
	key := "forget-key"

	// First execution
	_, err := wrapper.DoScrape(ctx, key, func() (interface{}, error) {
		atomic.AddInt32(&execCount, 1)
		return "first", nil
	})
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}

	// Forget the key
	wrapper.Forget(key)

	// Second execution should run again
	_, err = wrapper.DoScrape(ctx, key, func() (interface{}, error) {
		atomic.AddInt32(&execCount, 1)
		return "second", nil
	})
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}

	// Should have executed twice (once before forget, once after)
	if execCount != 2 {
		t.Errorf("Expected 2 executions after forget, got %d", execCount)
	}
}

func TestCacheWrapperConcurrentDifferentKeys(t *testing.T) {
	wrapper := NewCacheWrapper()
	ctx := context.Background()

	execCounts := make(map[string]*int32)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create 5 different keys, each with 5 concurrent requests
	for i := 0; i < 5; i++ {
		key := string(rune('A' + i))
		var count int32
		execCounts[key] = &count

		for j := 0; j < 5; j++ {
			wg.Add(1)
			go func(k string, c *int32) {
				defer wg.Done()

				_, err := wrapper.DoScrape(ctx, k, func() (interface{}, error) {
					atomic.AddInt32(c, 1)
					time.Sleep(50 * time.Millisecond)
					return k, nil
				})

				if err != nil {
					t.Errorf("Error for key %s: %v", k, err)
				}
			}(key, &count)
		}
	}

	wg.Wait()

	// Each key should execute exactly once
	mu.Lock()
	defer mu.Unlock()
	for key, count := range execCounts {
		if *count != 1 {
			t.Errorf("Key %s: expected 1 execution, got %d", key, *count)
		}
	}
}
