package stringutil

import "testing"

func TestIsNumeric(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			got := IsNumeric(tt.input)
			if got != tt.want {
				t.Errorf("IsNumeric(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestContainsAllRunes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		s        string
		chars    string
		expected bool
	}{
		{"Empty chars", "hello", "", true},
		{"Empty s", "", "hello", false},
		{"Both empty", "", "", true},
		{"Exact match", "abc", "abc", true},
		{"Contains all", "abcdef", "ace", true},
		{"Missing char", "abc", "abd", false},
		{"CJK - contains all", "資訊工程學系", "資工系", true},
		{"CJK - missing char", "資訊工程學系", "資工學", true}, // 資, 工, 學 all exist
		{"CJK - actually missing", "資訊工程", "系", false},
		{"Case insensitive - ASCII", "HelloWorld", "hw", true},
		{"Case insensitive - exact", "HELLO", "hello", true},
		{"Non-contiguous - CJK", "王小明", "王明", true},     // 非連續字元
		{"Non-contiguous - reverse", "王小明", "明王", true}, // 順序不同也能匹配
		{"Duplicate char - enough", "程程式設計", "程程", true},
		{"Duplicate char - not enough", "aabb", "aaab", false},
		{"Duplicate char - exact", "aabb", "aabb", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ContainsAllRunes(tt.s, tt.chars)
			if got != tt.expected {
				t.Errorf("ContainsAllRunes(%q, %q) = %v, want %v",
					tt.s, tt.chars, got, tt.expected)
			}
		})
	}
}
