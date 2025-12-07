package stringutil

import "testing"

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"Valid digits", "123456", true},
		{"Valid student ID", "41247001", true},
		{"Empty string", "", false},
		{"Contains letter", "123a456", false},
		{"Contains space", "123 456", false},
		{"Only letters", "abc", false},
		{"Special chars", "123-456", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNumeric(tt.input)
			if got != tt.want {
				t.Errorf("IsNumeric(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestContainsAllRunes(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		required []rune
		want     bool
	}{
		{"All present", "王小明", []rune("王明"), true},
		{"Missing char", "王小", []rune("王明"), false},
		{"Empty required", "test", []rune(""), true},
		{"Empty string", "", []rune("test"), false},
		{"Exact match", "abc", []rune("abc"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsAllRunes(tt.s, tt.required)
			if got != tt.want {
				t.Errorf("ContainsAllRunes(%q, %v) = %v, want %v", tt.s, tt.required, got, tt.want)
			}
		})
	}
}
