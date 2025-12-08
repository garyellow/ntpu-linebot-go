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
