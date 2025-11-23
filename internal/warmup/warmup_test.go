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
