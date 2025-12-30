package warmup

import (
	"testing"
)

// TestStats tests the Stats struct atomic operations
func TestStats(t *testing.T) {
	t.Parallel()
	stats := &Stats{}

	// Test initial values are zero
	if stats.Contacts.Load() != 0 {
		t.Errorf("Contacts should be 0 initially, got %d", stats.Contacts.Load())
	}
	if stats.Courses.Load() != 0 {
		t.Errorf("Courses should be 0 initially, got %d", stats.Courses.Load())
	}
	if stats.Syllabi.Load() != 0 {
		t.Errorf("Syllabi should be 0 initially, got %d", stats.Syllabi.Load())
	}

	// Test Add operations
	stats.Contacts.Add(50)
	stats.Courses.Add(200)
	stats.Syllabi.Add(30)

	if stats.Contacts.Load() != 50 {
		t.Errorf("Contacts should be 50, got %d", stats.Contacts.Load())
	}
	if stats.Courses.Load() != 200 {
		t.Errorf("Courses should be 200, got %d", stats.Courses.Load())
	}
	if stats.Syllabi.Load() != 30 {
		t.Errorf("Syllabi should be 30, got %d", stats.Syllabi.Load())
	}
}

// TestStatsConcurrent tests concurrent access to Stats
func TestStatsConcurrent(t *testing.T) {
	t.Parallel()
	stats := &Stats{}
	const goroutines = 100
	const incrementsPerGoroutine = 1000

	done := make(chan struct{})
	for range goroutines {
		go func() {
			for range incrementsPerGoroutine {
				stats.Contacts.Add(1)
				stats.Courses.Add(1)
				stats.Syllabi.Add(1)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for range goroutines {
		<-done
	}

	expected := int64(goroutines * incrementsPerGoroutine)
	if stats.Contacts.Load() != expected {
		t.Errorf("Contacts should be %d, got %d", expected, stats.Contacts.Load())
	}
	if stats.Courses.Load() != expected {
		t.Errorf("Courses should be %d, got %d", expected, stats.Courses.Load())
	}
	if stats.Syllabi.Load() != expected {
		t.Errorf("Syllabi should be %d, got %d", expected, stats.Syllabi.Load())
	}
}

// TestOptions tests the Options struct
func TestOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		opts      Options
		hasLLMKey bool
		warmID    bool
		reset     bool
	}{
		{
			name: "default options",
			opts: Options{
				HasLLMKey: false,
				WarmID:    false,
				Reset:     false,
			},
			hasLLMKey: false,
			warmID:    false,
			reset:     false,
		},
		{
			name: "with LLM key",
			opts: Options{
				HasLLMKey: true,
				WarmID:    false,
				Reset:     false,
			},
			hasLLMKey: true,
			warmID:    false,
			reset:     false,
		},
		{
			name: "with reset",
			opts: Options{
				HasLLMKey: false,
				WarmID:    false,
				Reset:     true,
			},
			hasLLMKey: false,
			warmID:    false,
			reset:     true,
		},
		{
			name: "with LLM key and reset",
			opts: Options{
				HasLLMKey: true,
				WarmID:    false,
				Reset:     true,
			},
			hasLLMKey: true,
			warmID:    false,
			reset:     true,
		},
		{
			name: "with warm ID",
			opts: Options{
				HasLLMKey: false,
				WarmID:    true,
				Reset:     false,
			},
			hasLLMKey: false,
			warmID:    true,
			reset:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.opts.HasLLMKey != tt.hasLLMKey {
				t.Errorf("HasLLMKey mismatch: got %v, want %v", tt.opts.HasLLMKey, tt.hasLLMKey)
			}
			if tt.opts.WarmID != tt.warmID {
				t.Errorf("WarmID mismatch: got %v, want %v", tt.opts.WarmID, tt.warmID)
			}
			if tt.opts.Reset != tt.reset {
				t.Errorf("Reset mismatch: got %v, want %v", tt.opts.Reset, tt.reset)
			}
		})
	}
}
