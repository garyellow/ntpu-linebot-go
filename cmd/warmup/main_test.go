package main

import (
	"testing"
)

func TestParseModules(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"all modules", "id,contact,course", []string{"id", "contact", "course"}},
		{"single module", "id", []string{"id"}},
		{"with spaces", "id, contact , course", []string{"id", "contact", "course"}},
		{"empty string", "", []string{}},
		{"only commas", ",,,", []string{}},
		{"mixed case", "ID,Contact,COURSE", []string{"id", "contact", "course"}},
		{"duplicate modules", "id,id,contact", []string{"id", "id", "contact"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModules(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseModules(%q) length = %d, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseModules(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
