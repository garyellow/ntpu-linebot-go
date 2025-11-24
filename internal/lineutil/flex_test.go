package lineutil

import (
	"testing"
)

// TestTruncateRunes tests UTF-8 safe rune truncation
func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		expected string
	}{
		{"ASCII within limit", "Hello World", 20, "Hello World"},
		{"ASCII exceeds limit", "Hello World", 5, "He..."},
		{"Chinese within limit", "你好世界", 10, "你好世界"},
		{"Chinese exceeds limit", "你好世界測試", 4, "你..."},
		{"Mixed CJK exceeds", "資料結構Data Structure", 10, "資料結構Dat..."},
		{"Emoji handling", "🎓學生資訊📚", 5, "🎓學..."},
		{"Empty string", "", 10, ""},
		{"Exactly at limit", "測試", 2, "測試"},
		{"Max less than ellipsis", "Hello", 2, "He"},
		{"Single rune", "A", 1, "A"},
		{"Zero limit", "Test", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateRunes(tt.input, tt.maxRunes)
			if result != tt.expected {
				t.Errorf("TruncateRunes(%q, %d) = %q, want %q",
					tt.input, tt.maxRunes, result, tt.expected)
			}
			// Verify result is valid UTF-8
			if len(result) > 0 && !isValidUTF8(result) {
				t.Errorf("Result %q is not valid UTF-8", result)
			}
			// Verify result doesn't exceed maxRunes
			if len([]rune(result)) > tt.maxRunes {
				t.Errorf("Result %q has %d runes, exceeds max %d",
					result, len([]rune(result)), tt.maxRunes)
			}
		})
	}
}

// TestFlexBubbleComponents tests Flex Message component creation
func TestFlexBubbleComponents(t *testing.T) {
	t.Run("Full bubble", func(t *testing.T) {
		header := NewFlexBox("vertical", NewFlexText("Header").FlexText)
		hero := NewFlexBox("vertical", NewFlexText("Hero").FlexText).FlexBox
		body := NewFlexBox("vertical", NewFlexText("Body").FlexText)
		footer := NewFlexBox("vertical", NewFlexText("Footer").FlexText)

		bubble := NewFlexBubble(header, hero, body, footer)

		if bubble.Header == nil {
			t.Error("Expected non-nil header")
		}
		if bubble.Hero == nil {
			t.Error("Expected non-nil hero")
		}
		if bubble.Body == nil {
			t.Error("Expected non-nil body")
		}
		if bubble.Footer == nil {
			t.Error("Expected non-nil footer")
		}
	})

	t.Run("Minimal bubble (no header/footer)", func(t *testing.T) {
		hero := NewFlexBox("vertical", NewFlexText("Hero").FlexText).FlexBox
		body := NewFlexBox("vertical", NewFlexText("Body").FlexText)

		bubble := NewFlexBubble(nil, hero, body, nil)

		if bubble.Header != nil {
			t.Error("Expected nil header")
		}
		if bubble.Hero == nil {
			t.Error("Expected non-nil hero")
		}
		if bubble.Body == nil {
			t.Error("Expected non-nil body")
		}
		if bubble.Footer != nil {
			t.Error("Expected nil footer")
		}
	})
}

// TestKeyValueRow tests key-value row formatting
func TestKeyValueRow(t *testing.T) {
	t.Run("Standard row", func(t *testing.T) {
		row := NewKeyValueRow("Key", "Value")

		if len(row.Contents) != 2 {
			t.Errorf("Expected 2 components, got %d", len(row.Contents))
		}
	})

	t.Run("Long value", func(t *testing.T) {
		longValue := "This is a very long value that should wrap properly in the Flex Message layout without breaking the UI"
		longRow := NewKeyValueRow("Label", longValue)

		if len(longRow.Contents) != 2 {
			t.Errorf("Expected 2 components for long value, got %d", len(longRow.Contents))
		}
	})

	t.Run("Chinese characters", func(t *testing.T) {
		row := NewKeyValueRow("系所", "資訊工程學系資訊科學組")

		if len(row.Contents) != 2 {
			t.Errorf("Expected 2 components for Chinese text, got %d", len(row.Contents))
		}
	})

	t.Run("Empty values", func(t *testing.T) {
		row := NewKeyValueRow("", "")

		if len(row.Contents) != 2 {
			t.Errorf("Expected 2 components even for empty strings, got %d", len(row.Contents))
		}
	})
}

// TestFlexTextChaining tests method chaining for FlexText
func TestFlexTextChaining(t *testing.T) {
	text := NewFlexText("Test").
		WithWeight("bold").
		WithSize("xl").
		WithColor("#1DB446").
		WithWrap(true).
		WithMaxLines(2).
		WithAlign("center").
		WithMargin("md")

	if text.Weight != "bold" {
		t.Errorf("Expected weight 'bold', got %v", text.Weight)
	}
	if text.Size != "xl" {
		t.Errorf("Expected size 'xl', got %v", text.Size)
	}
	if text.Color != "#1DB446" {
		t.Errorf("Expected color '#1DB446', got %v", text.Color)
	}
	if !text.Wrap {
		t.Error("Expected wrap to be true")
	}
	if text.MaxLines != 2 {
		t.Errorf("Expected maxLines 2, got %v", text.MaxLines)
	}
}

// TestFlexButtonChaining tests method chaining for FlexButton
func TestFlexButtonChaining(t *testing.T) {
	action := NewMessageAction("Test", "test")
	button := NewFlexButton(action).
		WithStyle("primary").
		WithColor("#1DB446").
		WithHeight("sm").
		WithMargin("md")

	if button.Style != "primary" {
		t.Errorf("Expected style 'primary', got %v", button.Style)
	}
	if button.Color != "#1DB446" {
		t.Errorf("Expected color '#1DB446', got %v", button.Color)
	}
	if button.Height != "sm" {
		t.Errorf("Expected height 'sm', got %v", button.Height)
	}
}

// TestFlexBoxChaining tests method chaining for FlexBox
func TestFlexBoxChaining(t *testing.T) {
	box := NewFlexBox("vertical", NewFlexText("Test").FlexText).
		WithSpacing("sm").
		WithMargin("md").
		WithPaddingBottom("16px")

	if box.Spacing != "sm" {
		t.Errorf("Expected spacing 'sm', got %v", box.Spacing)
	}
	if box.Margin != "md" {
		t.Errorf("Expected margin 'md', got %v", box.Margin)
	}
	if box.PaddingBottom != "16px" {
		t.Errorf("Expected paddingBottom '16px', got %v", box.PaddingBottom)
	}
}

// TestFlexSeparator tests separator creation and chaining
func TestFlexSeparator(t *testing.T) {
	sep := NewFlexSeparator().WithMargin("md")

	if sep.Margin != "md" {
		t.Errorf("Expected margin 'md', got %v", sep.Margin)
	}
}

// Helper function to check if string is valid UTF-8
func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '\ufffd' { // replacement character indicates invalid UTF-8
			return false
		}
	}
	return true
}

// BenchmarkTruncateRunes benchmarks the TruncateRunes function
func BenchmarkTruncateRunes(b *testing.B) {
	testString := "這是一個測試字串，用來測試 TruncateRunes 函數的效能，包含中英文與數字 123"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TruncateRunes(testString, 20)
	}
}

// BenchmarkNewKeyValueRow benchmarks key-value row creation
func BenchmarkNewKeyValueRow(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewKeyValueRow("測試標籤", "測試數值內容比較長一點的情況")
	}
}
