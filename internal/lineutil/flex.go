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
// Key is aligned baseline, value wraps with MaxLines to prevent overflow
// Designed for Flex Message body content (not hero/header)
func NewKeyValueRow(key, value string) *FlexBox {
	return NewFlexBox("baseline",
		NewFlexText(key).WithColor("#aaaaaa").WithSize("sm").WithFlex(1).WithAlign("start").FlexText,
		NewFlexText(value).WithWrap(true).WithMaxLines(3).WithColor("#666666").WithSize("sm").WithFlex(5).FlexText,
	)
}
