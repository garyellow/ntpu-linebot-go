package sliceutil

import (
	"strconv"
	"testing"
)

type testItem struct {
	ID   string
	Name string
}

func TestDeduplicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		items   []testItem
		keyFunc func(testItem) string
		want    []testItem
	}{
		{
			name: "No duplicates",
			items: []testItem{
				{ID: "1", Name: "A"},
				{ID: "2", Name: "B"},
				{ID: "3", Name: "C"},
			},
			keyFunc: func(t testItem) string { return t.ID },
			want: []testItem{
				{ID: "1", Name: "A"},
				{ID: "2", Name: "B"},
				{ID: "3", Name: "C"},
			},
		},
		{
			name: "With duplicates - preserve first",
			items: []testItem{
				{ID: "1", Name: "A"},
				{ID: "2", Name: "B"},
				{ID: "1", Name: "C"}, // Duplicate ID
				{ID: "3", Name: "D"},
			},
			keyFunc: func(t testItem) string { return t.ID },
			want: []testItem{
				{ID: "1", Name: "A"}, // First occurrence kept
				{ID: "2", Name: "B"},
				{ID: "3", Name: "D"},
			},
		},
		{
			name: "All duplicates",
			items: []testItem{
				{ID: "1", Name: "A"},
				{ID: "1", Name: "B"},
				{ID: "1", Name: "C"},
			},
			keyFunc: func(t testItem) string { return t.ID },
			want: []testItem{
				{ID: "1", Name: "A"},
			},
		},
		{
			name:    "Empty slice",
			items:   []testItem{},
			keyFunc: func(t testItem) string { return t.ID },
			want:    []testItem{},
		},
		{
			name: "Single item",
			items: []testItem{
				{ID: "1", Name: "A"},
			},
			keyFunc: func(t testItem) string { return t.ID },
			want: []testItem{
				{ID: "1", Name: "A"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Deduplicate(tt.items, tt.keyFunc)
			if len(got) != len(tt.want) {
				t.Errorf("Deduplicate() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].ID != tt.want[i].ID || got[i].Name != tt.want[i].Name {
					t.Errorf("Deduplicate()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestDeduplicatePreservesOrder ensures that deduplication preserves the original order
func TestDeduplicatePreservesOrder(t *testing.T) {
	t.Parallel()
	items := []testItem{
		{ID: "3", Name: "C"},
		{ID: "1", Name: "A"},
		{ID: "2", Name: "B"},
		{ID: "3", Name: "C2"}, // Duplicate
		{ID: "1", Name: "A2"}, // Duplicate
	}

	got := Deduplicate(items, func(t testItem) string { return t.ID })

	// Should preserve order: 3, 1, 2 (first occurrences)
	want := []testItem{
		{ID: "3", Name: "C"},
		{ID: "1", Name: "A"},
		{ID: "2", Name: "B"},
	}

	if len(got) != len(want) {
		t.Fatalf("Deduplicate() length = %d, want %d", len(got), len(want))
	}

	for i := range got {
		if got[i].ID != want[i].ID || got[i].Name != want[i].Name {
			t.Errorf("Deduplicate()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// BenchmarkDeduplicate measures performance
func BenchmarkDeduplicate(b *testing.B) {
	items := make([]testItem, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = testItem{ID: strconv.Itoa(i % 100), Name: "test"}
	}

	keyFunc := func(t testItem) string { return t.ID }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Deduplicate(items, keyFunc)
	}
}
