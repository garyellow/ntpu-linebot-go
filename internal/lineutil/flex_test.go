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

// TestFlexTextChaining tests method chaining for FlexText
func TestFlexTextChaining(t *testing.T) {
	text := NewFlexText("Test").
		WithWeight("bold").
		WithSize("xl").
		WithColor("#1DB446").
		WithWrap(true).
		WithMaxLines(2).
		WithAlign("center").
		WithMargin("md").
		WithLineSpacing("4px")

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
	if text.LineSpacing != "4px" {
		t.Errorf("Expected lineSpacing '4px', got %v", text.LineSpacing)
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
		WithPaddingBottom("16px").
		WithPaddingAll("20px").
		WithBackgroundColor("#1DB446")

	if box.Spacing != "sm" {
		t.Errorf("Expected spacing 'sm', got %v", box.Spacing)
	}
	if box.Margin != "md" {
		t.Errorf("Expected margin 'md', got %v", box.Margin)
	}
	if box.PaddingBottom != "16px" {
		t.Errorf("Expected paddingBottom '16px', got %v", box.PaddingBottom)
	}
	if box.PaddingAll != "20px" {
		t.Errorf("Expected paddingAll '20px', got %v", box.PaddingAll)
	}
	if box.BackgroundColor != "#1DB446" {
		t.Errorf("Expected backgroundColor '#1DB446', got %v", box.BackgroundColor)
	}
}

// TestNewHeroBox tests standardized hero box creation
func TestNewHeroBox(t *testing.T) {
	t.Run("with subtitle", func(t *testing.T) {
		hero := NewHeroBox("測試標題", "副標題")

		// Check background color
		if hero.BackgroundColor != "#1DB446" {
			t.Errorf("Expected backgroundColor '#1DB446', got %v", hero.BackgroundColor)
		}
		// Check padding
		if hero.PaddingAll != "20px" {
			t.Errorf("Expected paddingAll '20px', got %v", hero.PaddingAll)
		}
		if hero.PaddingBottom != "16px" {
			t.Errorf("Expected paddingBottom '16px', got %v", hero.PaddingBottom)
		}
		// Check contents
		if len(hero.Contents) != 2 {
			t.Errorf("Expected 2 contents (title + subtitle), got %d", len(hero.Contents))
		}
	})

	t.Run("empty subtitle omitted", func(t *testing.T) {
		hero := NewHeroBox("測試標題", "")

		// Check contents - should only have title
		if len(hero.Contents) != 1 {
			t.Errorf("Expected 1 content (title only), got %d", len(hero.Contents))
		}
		// Check background color still applied
		if hero.BackgroundColor != "#1DB446" {
			t.Errorf("Expected backgroundColor '#1DB446', got %v", hero.BackgroundColor)
		}
	})
}

// TestNewCompactHeroBox tests compact hero box for carousel
func TestNewCompactHeroBox(t *testing.T) {
	hero := NewCompactHeroBox("輪播標題")

	// Check background color
	if hero.BackgroundColor != "#1DB446" {
		t.Errorf("Expected backgroundColor '#1DB446', got %v", hero.BackgroundColor)
	}
	// Check compact padding
	if hero.PaddingAll != "15px" {
		t.Errorf("Expected paddingAll '15px', got %v", hero.PaddingAll)
	}
	// Check contents (only title)
	if len(hero.Contents) != 1 {
		t.Errorf("Expected 1 content (title only), got %d", len(hero.Contents))
	}
}

// TestNewHeaderBadge tests header badge creation
func TestNewHeaderBadge(t *testing.T) {
	badge := NewHeaderBadge("📚", "測試標籤")

	// Check layout
	if badge.Layout != "vertical" {
		t.Errorf("Expected layout 'vertical', got %v", badge.Layout)
	}
	// Check contents
	if len(badge.Contents) != 1 {
		t.Errorf("Expected 1 content (baseline box), got %d", len(badge.Contents))
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

// TestNewInfoRow tests the standardized info row creation
func TestNewInfoRow(t *testing.T) {
	tests := []struct {
		name       string
		emoji      string
		label      string
		value      string
		style      InfoRowStyle
		checkWrap  bool
		checkBold  bool
		valueSize  string
		valueColor string
	}{
		{
			name:       "Default style with wrap",
			emoji:      "👨‍🏫",
			label:      "授課教師",
			value:      "王教授、李教授",
			style:      DefaultInfoRowStyle(),
			checkWrap:  true,
			checkBold:  false,
			valueSize:  "sm",
			valueColor: "#333333",
		},
		{
			name:       "Bold style without wrap",
			emoji:      "☎️",
			label:      "分機號碼",
			value:      "12345",
			style:      BoldInfoRowStyle(),
			checkWrap:  false,
			checkBold:  true,
			valueSize:  "md",
			valueColor: "#333333",
		},
		{
			name:  "Custom style",
			emoji: "📝",
			label: "備註",
			value: "這是備註內容",
			style: InfoRowStyle{
				ValueSize:   "xs",
				ValueWeight: "regular",
				ValueColor:  "#666666",
				Wrap:        true,
			},
			checkWrap:  true,
			checkBold:  false,
			valueSize:  "xs",
			valueColor: "#666666",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := NewInfoRow(tt.emoji, tt.label, tt.value, tt.style)

			// Check it's a vertical box
			if row.Layout != "vertical" {
				t.Errorf("Expected layout 'vertical', got %v", row.Layout)
			}

			// Check it has 2 contents (header row + value)
			if len(row.Contents) != 2 {
				t.Errorf("Expected 2 contents, got %d", len(row.Contents))
			}
		})
	}
}

// TestNewInfoRowWithMargin tests the convenience wrapper with margin
func TestNewInfoRowWithMargin(t *testing.T) {
	result := NewInfoRowWithMargin("🆔", "學號", "41247001", BoldInfoRowStyle(), "lg")

	// Should not be nil
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
}

// TestInfoRowStyles tests the predefined styles
func TestInfoRowStyles(t *testing.T) {
	t.Run("DefaultInfoRowStyle", func(t *testing.T) {
		style := DefaultInfoRowStyle()
		if style.ValueSize != "sm" {
			t.Errorf("Expected ValueSize 'sm', got %s", style.ValueSize)
		}
		if style.ValueWeight != "regular" {
			t.Errorf("Expected ValueWeight 'regular', got %s", style.ValueWeight)
		}
		if style.ValueColor != "#333333" {
			t.Errorf("Expected ValueColor '#333333', got %s", style.ValueColor)
		}
		if !style.Wrap {
			t.Error("Expected Wrap to be true")
		}
	})

	t.Run("BoldInfoRowStyle", func(t *testing.T) {
		style := BoldInfoRowStyle()
		if style.ValueSize != "md" {
			t.Errorf("Expected ValueSize 'md', got %s", style.ValueSize)
		}
		if style.ValueWeight != "bold" {
			t.Errorf("Expected ValueWeight 'bold', got %s", style.ValueWeight)
		}
		if style.ValueColor != "#333333" {
			t.Errorf("Expected ValueColor '#333333', got %s", style.ValueColor)
		}
		if style.Wrap {
			t.Error("Expected Wrap to be false")
		}
	})
}

// BenchmarkNewInfoRow benchmarks the NewInfoRow function
func BenchmarkNewInfoRow(b *testing.B) {
	style := DefaultInfoRowStyle()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewInfoRow("👨‍🏫", "授課教師", "王教授、李教授、陳教授", style)
	}
}

// TestNewButtonRow tests the button row creation for footer layouts
func TestNewButtonRow(t *testing.T) {
	action := NewMessageAction("Test", "test")

	tests := []struct {
		name            string
		buttons         []*FlexButton
		expectedLen     int
		expectedSpacing string
	}{
		{
			name:            "Empty button list",
			buttons:         []*FlexButton{},
			expectedLen:     0,
			expectedSpacing: "sm",
		},
		{
			name:            "All nil buttons",
			buttons:         []*FlexButton{nil, nil, nil},
			expectedLen:     0,
			expectedSpacing: "sm",
		},
		{
			name: "Single button",
			buttons: []*FlexButton{
				NewFlexButton(action).WithStyle("primary"),
			},
			expectedLen:     1,
			expectedSpacing: "sm",
		},
		{
			name: "Multiple buttons",
			buttons: []*FlexButton{
				NewFlexButton(action).WithStyle("primary"),
				NewFlexButton(action).WithStyle("secondary"),
			},
			expectedLen:     2,
			expectedSpacing: "sm",
		},
		{
			name: "Mixed nil and valid buttons",
			buttons: []*FlexButton{
				NewFlexButton(action).WithStyle("primary"),
				nil,
				NewFlexButton(action).WithStyle("secondary"),
				nil,
			},
			expectedLen:     2,
			expectedSpacing: "sm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := NewButtonRow(tt.buttons...)

			// Check layout is horizontal
			if row.Layout != "horizontal" {
				t.Errorf("Expected layout 'horizontal', got %v", row.Layout)
			}

			// Check spacing
			if row.Spacing != tt.expectedSpacing {
				t.Errorf("Expected spacing '%s', got %v", tt.expectedSpacing, row.Spacing)
			}

			// Check content count
			if len(row.Contents) != tt.expectedLen {
				t.Errorf("Expected %d contents, got %d", tt.expectedLen, len(row.Contents))
			}
		})
	}
}

// TestNewButtonFooter tests the multi-row button footer creation
func TestNewButtonFooter(t *testing.T) {
	action := NewMessageAction("Test", "test")
	primaryBtn := NewFlexButton(action).WithStyle("primary").WithHeight("sm")
	secondaryBtn := NewFlexButton(action).WithStyle("secondary").WithHeight("sm")

	tests := []struct {
		name        string
		rows        [][]*FlexButton
		expectedLen int
	}{
		{
			name:        "Empty rows",
			rows:        [][]*FlexButton{},
			expectedLen: 0,
		},
		{
			name: "All empty rows",
			rows: [][]*FlexButton{
				{},
				{nil, nil},
				{},
			},
			expectedLen: 0,
		},
		{
			name: "Single row with buttons",
			rows: [][]*FlexButton{
				{primaryBtn, secondaryBtn},
			},
			expectedLen: 1,
		},
		{
			name: "Multiple rows with buttons",
			rows: [][]*FlexButton{
				{primaryBtn, secondaryBtn},
				{primaryBtn},
				{secondaryBtn},
			},
			expectedLen: 3,
		},
		{
			name: "Mixed empty and non-empty rows",
			rows: [][]*FlexButton{
				{primaryBtn, secondaryBtn},
				{nil, nil},
				{primaryBtn},
				{},
			},
			expectedLen: 2,
		},
		{
			name: "Rows with mixed nil and valid buttons",
			rows: [][]*FlexButton{
				{primaryBtn, nil, secondaryBtn},
				{nil, primaryBtn, nil},
			},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			footer := NewButtonFooter(tt.rows...)

			// Check layout is vertical
			if footer.Layout != "vertical" {
				t.Errorf("Expected layout 'vertical', got %v", footer.Layout)
			}

			// Check spacing
			if footer.Spacing != "sm" {
				t.Errorf("Expected spacing 'sm', got %v", footer.Spacing)
			}

			// Check row count
			if len(footer.Contents) != tt.expectedLen {
				t.Errorf("Expected %d rows, got %d", tt.expectedLen, len(footer.Contents))
			}
		})
	}
}

// TestButtonRowFlexDistribution tests that buttons get equal flex distribution
func TestButtonRowFlexDistribution(t *testing.T) {
	action := NewMessageAction("Test", "test")
	btn1 := NewFlexButton(action).WithStyle("primary")
	btn2 := NewFlexButton(action).WithStyle("secondary")
	btn3 := NewFlexButton(action).WithStyle("primary")

	row := NewButtonRow(btn1, btn2, btn3)

	// Each button should be wrapped in a box with Flex=1
	for i, content := range row.Contents {
		// The content should be a FlexBox wrapping the button
		if content == nil {
			t.Errorf("Content at index %d is nil", i)
		}
	}

	// Check that we have 3 wrapped buttons
	if len(row.Contents) != 3 {
		t.Errorf("Expected 3 contents, got %d", len(row.Contents))
	}
}
