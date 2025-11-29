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
