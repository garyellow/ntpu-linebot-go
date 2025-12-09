package warmup

import (
	"testing"
)

func TestParseModules(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", []string{}},
		{"single module", "id", []string{"id"}},
		{"multiple modules", "id,contact,course", []string{"id", "contact", "course"}},
		{"with spaces", "id, contact , course", []string{"id", "contact", "course"}},
		{"with empty items", "id,,contact", []string{"id", "contact"}},
		{"all modules", "id,contact,course,sticker", []string{"id", "contact", "course", "sticker"}},
		{"all modules with syllabus", "sticker,id,contact,course,syllabus", []string{"sticker", "id", "contact", "course", "syllabus"}},
		// Note: ParseModules does NOT convert to lowercase - removed this test
		{"duplicate modules", "id,id,contact", []string{"id", "id", "contact"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseModules(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("ParseModules() got %v modules, want %v", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseModules()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestMin tests the min helper function
func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"a smaller", 1, 2, 1},
		{"b smaller", 5, 3, 3},
		{"equal", 4, 4, 4},
		{"negative", -1, -5, -5},
		{"zero", 0, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestStats tests the Stats struct atomic operations
func TestStats(t *testing.T) {
	stats := &Stats{}

	// Test initial values are zero
	if stats.Students.Load() != 0 {
		t.Errorf("Students should be 0 initially, got %d", stats.Students.Load())
	}
	if stats.Contacts.Load() != 0 {
		t.Errorf("Contacts should be 0 initially, got %d", stats.Contacts.Load())
	}
	if stats.Courses.Load() != 0 {
		t.Errorf("Courses should be 0 initially, got %d", stats.Courses.Load())
	}
	if stats.Stickers.Load() != 0 {
		t.Errorf("Stickers should be 0 initially, got %d", stats.Stickers.Load())
	}
	if stats.Syllabi.Load() != 0 {
		t.Errorf("Syllabi should be 0 initially, got %d", stats.Syllabi.Load())
	}

	// Test Add operations
	stats.Students.Add(100)
	stats.Contacts.Add(50)
	stats.Courses.Add(200)
	stats.Stickers.Store(10)
	stats.Syllabi.Add(30)

	if stats.Students.Load() != 100 {
		t.Errorf("Students should be 100, got %d", stats.Students.Load())
	}
	if stats.Contacts.Load() != 50 {
		t.Errorf("Contacts should be 50, got %d", stats.Contacts.Load())
	}
	if stats.Courses.Load() != 200 {
		t.Errorf("Courses should be 200, got %d", stats.Courses.Load())
	}
	if stats.Stickers.Load() != 10 {
		t.Errorf("Stickers should be 10, got %d", stats.Stickers.Load())
	}
	if stats.Syllabi.Load() != 30 {
		t.Errorf("Syllabi should be 30, got %d", stats.Syllabi.Load())
	}
}

// TestStatsConcurrent tests concurrent access to Stats
func TestStatsConcurrent(t *testing.T) {
	stats := &Stats{}
	const goroutines = 100
	const incrementsPerGoroutine = 1000

	done := make(chan struct{})
	for range goroutines {
		go func() {
			for range incrementsPerGoroutine {
				stats.Students.Add(1)
				stats.Contacts.Add(1)
				stats.Courses.Add(1)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for range goroutines {
		<-done
	}

	expected := int64(goroutines * incrementsPerGoroutine)
	if stats.Students.Load() != expected {
		t.Errorf("Students should be %d, got %d", expected, stats.Students.Load())
	}
	if stats.Contacts.Load() != expected {
		t.Errorf("Contacts should be %d, got %d", expected, stats.Contacts.Load())
	}
	if stats.Courses.Load() != expected {
		t.Errorf("Courses should be %d, got %d", expected, stats.Courses.Load())
	}
}

// TestOptions tests the Options struct
func TestOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		modules []string
		reset   bool
	}{
		{
			name: "default options",
			opts: Options{
				Modules: []string{"id", "contact", "course"},
				Reset:   false,
			},
			modules: []string{"id", "contact", "course"},
			reset:   false,
		},
		{
			name: "with reset",
			opts: Options{
				Modules: []string{"sticker"},
				Reset:   true,
			},
			modules: []string{"sticker"},
			reset:   true,
		},
		{
			name: "empty modules",
			opts: Options{
				Modules: []string{},
				Reset:   false,
			},
			modules: []string{},
			reset:   false,
		},
		{
			name: "all modules with syllabus",
			opts: Options{
				Modules: []string{"id", "contact", "course", "sticker", "syllabus"},
				Reset:   false,
			},
			modules: []string{"id", "contact", "course", "sticker", "syllabus"},
			reset:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.opts.Modules) != len(tt.modules) {
				t.Errorf("Modules length mismatch: got %d, want %d", len(tt.opts.Modules), len(tt.modules))
			}
			if tt.opts.Reset != tt.reset {
				t.Errorf("Reset mismatch: got %v, want %v", tt.opts.Reset, tt.reset)
			}
		})
	}
}

// TestParseModulesEdgeCases tests edge cases for ParseModules
func TestParseModulesEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		contains []string
	}{
		{
			name:     "whitespace only",
			input:    "   ",
			wantLen:  0,
			contains: []string{},
		},
		{
			name:     "commas only",
			input:    ",,,",
			wantLen:  0,
			contains: []string{},
		},
		{
			name:     "mixed empty and valid",
			input:    ",id,,contact,",
			wantLen:  2,
			contains: []string{"id", "contact"},
		},
		{
			name:     "tabs and spaces",
			input:    "id\t,\tcontact",
			wantLen:  2,
			contains: []string{"id", "contact"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseModules(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("ParseModules(%q) returned %d modules, want %d", tt.input, len(got), tt.wantLen)
			}
			for _, want := range tt.contains {
				found := false
				for _, g := range got {
					if g == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ParseModules(%q) should contain %q", tt.input, want)
				}
			}
		})
	}
}
