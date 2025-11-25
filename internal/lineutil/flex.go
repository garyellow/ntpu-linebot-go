package lineutil

import (
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// FlexBubble wrapper
type FlexBubble struct {
	*messaging_api.FlexBubble
}

// NewFlexBubble creates a new Flex Bubble container
// Note: header, body, footer must be FlexBox or nil
func NewFlexBubble(header *FlexBox, hero messaging_api.FlexComponentInterface, body *FlexBox, footer *FlexBox) *FlexBubble {
	bubble := &messaging_api.FlexBubble{}
	if header != nil {
		bubble.Header = header.FlexBox
	}
	if hero != nil {
		bubble.Hero = hero
	}
	if body != nil {
		bubble.Body = body.FlexBox
	}
	if footer != nil {
		bubble.Footer = footer.FlexBox
	}
	return &FlexBubble{bubble}
}

// FlexBox wrapper
type FlexBox struct {
	*messaging_api.FlexBox
}

func NewFlexBox(layout string, contents ...messaging_api.FlexComponentInterface) *FlexBox {
	return &FlexBox{&messaging_api.FlexBox{
		Layout:   messaging_api.FlexBoxLAYOUT(layout),
		Contents: contents,
	}}
}

func (b *FlexBox) WithSpacing(spacing string) *FlexBox {
	b.Spacing = spacing
	return b
}

func (b *FlexBox) WithMargin(margin string) *FlexBox {
	b.Margin = margin
	return b
}

func (b *FlexBox) WithPaddingBottom(padding string) *FlexBox {
	b.PaddingBottom = padding
	return b
}

func (b *FlexBox) WithPaddingAll(padding string) *FlexBox {
	b.PaddingAll = padding
	return b
}

func (b *FlexBox) WithBackgroundColor(color string) *FlexBox {
	b.BackgroundColor = color
	return b
}

// FlexText wrapper
type FlexText struct {
	*messaging_api.FlexText
}

func NewFlexText(text string) *FlexText {
	return &FlexText{&messaging_api.FlexText{
		Text: text,
	}}
}

func (t *FlexText) WithWeight(weight string) *FlexText {
	t.Weight = messaging_api.FlexTextWEIGHT(weight)
	return t
}

func (t *FlexText) WithSize(size string) *FlexText {
	t.Size = size
	return t
}

func (t *FlexText) WithColor(color string) *FlexText {
	t.Color = color
	return t
}

func (t *FlexText) WithWrap(wrap bool) *FlexText {
	t.Wrap = wrap
	return t
}

func (t *FlexText) WithFlex(flex int) *FlexText {
	t.Flex = int32(flex)
	return t
}

func (t *FlexText) WithAlign(align string) *FlexText {
	t.Align = messaging_api.FlexTextALIGN(align)
	return t
}

func (t *FlexText) WithMargin(margin string) *FlexText {
	t.Margin = margin
	return t
}

func (t *FlexText) WithMaxLines(lines int) *FlexText {
	t.MaxLines = int32(lines)
	return t
}

func (t *FlexText) WithLineSpacing(spacing string) *FlexText {
	t.LineSpacing = spacing
	return t
}

func (t *FlexText) WithAdjustMode(mode string) *FlexText {
	t.AdjustMode = messaging_api.FlexTextADJUST_MODE(mode)
	return t
}

// FlexButton wrapper
type FlexButton struct {
	*messaging_api.FlexButton
}

func NewFlexButton(action messaging_api.ActionInterface) *FlexButton {
	return &FlexButton{&messaging_api.FlexButton{
		Action: action,
	}}
}

func (b *FlexButton) WithStyle(style string) *FlexButton {
	b.Style = messaging_api.FlexButtonSTYLE(style)
	return b
}

func (b *FlexButton) WithColor(color string) *FlexButton {
	b.Color = color
	return b
}

func (b *FlexButton) WithHeight(height string) *FlexButton {
	b.Height = messaging_api.FlexButtonHEIGHT(height)
	return b
}

func (b *FlexButton) WithMargin(margin string) *FlexButton {
	b.Margin = margin
	return b
}

// FlexSeparator wrapper
type FlexSeparator struct {
	*messaging_api.FlexSeparator
}

func NewFlexSeparator() *FlexSeparator {
	return &FlexSeparator{&messaging_api.FlexSeparator{}}
}

func (s *FlexSeparator) WithMargin(margin string) *FlexSeparator {
	s.Margin = margin
	return s
}

// TruncateRunes truncates text by rune count (not byte count) to properly handle UTF-8.
// Returns truncated string with "..." if exceeds maxRunes.
func TruncateRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

// NewKeyValueRow creates a key-value row for Flex Box with consistent styling
// Key uses flex:0 (auto-width based on content) to prevent truncation
// Value uses flex:1 (fill remaining space) with alignment end for better visibility
// This ensures keys like "🆔 學號" are never truncated
// Designed for Flex Message body content (not hero/header)
// Value supports full wrapping to show complete information
func NewKeyValueRow(key, value string) *FlexBox {
	return NewFlexBox("horizontal",
		NewFlexText(key).WithColor("#555555").WithSize("sm").WithFlex(0).WithWeight("bold").FlexText,
		NewFlexText(value).WithWrap(true).WithColor("#333333").WithSize("sm").WithFlex(1).WithAlign("end").WithLineSpacing("4px").FlexText,
	).WithSpacing("md")
}

// NewHeroBox creates a standardized Hero box with NTPU green background
// Provides consistent styling across all modules:
// - Background: #1DB446 (NTPU green)
// - Padding: 20px all, 16px bottom (for visual balance)
// - Title: Bold, XL size, white color, full wrap for complete display
// - Subtitle: XS size, white color, md margin top
func NewHeroBox(title, subtitle string) *FlexBox {
	box := NewFlexBox("vertical",
		NewFlexText(title).WithWeight("bold").WithSize("xl").WithColor("#ffffff").WithWrap(true).WithLineSpacing("6px").FlexText,
		NewFlexText(subtitle).WithSize("xs").WithColor("#ffffff").WithMargin("md").WithWrap(true).FlexText,
	)
	box.BackgroundColor = "#1DB446"
	box.PaddingAll = "20px"
	box.PaddingBottom = "16px"
	return box
}

// NewCompactHeroBox creates a compact Hero box for carousel/list views
// Uses smaller padding (15px) to fit more content
// Max 3 lines for carousel to balance visibility
func NewCompactHeroBox(title string) *FlexBox {
	box := NewFlexBox("vertical",
		NewFlexText(title).WithWeight("bold").WithSize("md").WithColor("#ffffff").WithWrap(true).WithMaxLines(3).WithLineSpacing("4px").FlexText,
	)
	box.BackgroundColor = "#1DB446"
	box.PaddingAll = "15px"
	return box
}

// NewHeaderBadge creates a consistent header badge for Flex Messages
// Format: [emoji] [label] with NTPU green color
func NewHeaderBadge(emoji, label string) *FlexBox {
	return NewFlexBox("vertical",
		NewFlexBox("baseline",
			NewFlexText(emoji).WithSize("lg").FlexText,
			NewFlexText(label).WithWeight("bold").WithColor("#1DB446").WithSize("sm").WithMargin("sm").FlexText,
		).FlexBox,
	)
}
