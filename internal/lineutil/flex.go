package lineutil

import (
	"fmt"
	"math"

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

// MaxBubblesPerCarousel is the LINE API limit for Flex Carousel
const MaxBubblesPerCarousel = 10

// NewFlexCarousel creates a Flex Carousel from a slice of bubbles.
// LINE API limits carousels to 10 bubbles maximum.
// For larger sets, use BuildCarouselMessages which automatically splits into multiple messages.
func NewFlexCarousel(bubbles []messaging_api.FlexBubble) *messaging_api.FlexCarousel {
	return &messaging_api.FlexCarousel{
		Contents: bubbles,
	}
}

// BuildCarouselMessages creates Flex Messages from bubbles, automatically splitting into
// multiple carousels (10 bubbles max per carousel) and applying consistent sender.
//
// Parameters:
//   - altText: Alt text for the Flex Messages (will append page numbers for multi-page)
//   - bubbles: Slice of FlexBubbles to include
//   - sender: Sender to apply to all messages (can be nil)
//
// Returns: Slice of messaging_api.MessageInterface ready for reply
//
// Example:
//
//	bubbles := []messaging_api.FlexBubble{...}
//	sender := lineutil.GetSender("課程小幫手", stickerManager)
//	messages := lineutil.BuildCarouselMessages("課程列表", bubbles, sender)
func BuildCarouselMessages(altText string, bubbles []messaging_api.FlexBubble, sender *messaging_api.Sender) []messaging_api.MessageInterface {
	if len(bubbles) == 0 {
		return nil
	}

	var messages []messaging_api.MessageInterface

	for i := 0; i < len(bubbles); i += MaxBubblesPerCarousel {
		end := i + MaxBubblesPerCarousel
		if end > len(bubbles) {
			end = len(bubbles)
		}

		carousel := NewFlexCarousel(bubbles[i:end])

		// Add page indicator for multi-page results
		msgAltText := altText
		if len(bubbles) > MaxBubblesPerCarousel && i > 0 {
			msgAltText = fmt.Sprintf("%s (%d-%d)", altText, i+1, end)
		}

		msg := NewFlexMessage(msgAltText, carousel)
		if sender != nil {
			msg.Sender = sender
		}
		messages = append(messages, msg)
	}

	return messages
}

// FlexBox wrapper for messaging_api.FlexBox with fluent API.
type FlexBox struct {
	*messaging_api.FlexBox
}

// NewFlexBox creates a new FlexBox with the specified layout and contents.
func NewFlexBox(layout string, contents ...messaging_api.FlexComponentInterface) *FlexBox {
	return &FlexBox{&messaging_api.FlexBox{
		Layout:   messaging_api.FlexBoxLAYOUT(layout),
		Contents: contents,
	}}
}

// WithSpacing sets the spacing between components.
func (b *FlexBox) WithSpacing(spacing string) *FlexBox {
	b.Spacing = spacing
	return b
}

// WithMargin sets the margin of the box.
func (b *FlexBox) WithMargin(margin string) *FlexBox {
	b.Margin = margin
	return b
}

// WithPaddingBottom sets the bottom padding of the box.
func (b *FlexBox) WithPaddingBottom(padding string) *FlexBox {
	b.PaddingBottom = padding
	return b
}

// WithPaddingAll sets the padding for all sides of the box.
func (b *FlexBox) WithPaddingAll(padding string) *FlexBox {
	b.PaddingAll = padding
	return b
}

// WithBackgroundColor sets the background color of the box.
func (b *FlexBox) WithBackgroundColor(color string) *FlexBox {
	b.BackgroundColor = color
	return b
}

// WithCornerRadius sets the corner radius of the box.
func (b *FlexBox) WithCornerRadius(radius string) *FlexBox {
	b.CornerRadius = radius
	return b
}

// FlexText wrapper for messaging_api.FlexText with fluent API.
type FlexText struct {
	*messaging_api.FlexText
}

// NewFlexText creates a new FlexText with the specified text.
func NewFlexText(text string) *FlexText {
	return &FlexText{&messaging_api.FlexText{
		Text: text,
	}}
}

// WithWeight sets the font weight (regular/bold).
func (t *FlexText) WithWeight(weight string) *FlexText {
	t.Weight = messaging_api.FlexTextWEIGHT(weight)
	return t
}

// WithSize sets the font size.
func (t *FlexText) WithSize(size string) *FlexText {
	t.Size = size
	return t
}

// WithColor sets the text color.
func (t *FlexText) WithColor(color string) *FlexText {
	t.Color = color
	return t
}

// WithWrap enables or disables text wrapping.
func (t *FlexText) WithWrap(wrap bool) *FlexText {
	t.Wrap = wrap
	return t
}

// WithFlex sets the flex factor for the text component.
func (t *FlexText) WithFlex(flex int) *FlexText {
	if flex < 0 {
		flex = 0
	}
	// Clamp to int32 range to prevent overflow
	if flex > math.MaxInt32 {
		flex = math.MaxInt32
	}
	t.Flex = int32(flex)
	return t
}

// WithAlign sets the text alignment (start/end/center).
func (t *FlexText) WithAlign(align string) *FlexText {
	t.Align = messaging_api.FlexTextALIGN(align)
	return t
}

// WithMargin sets the margin of the text component.
func (t *FlexText) WithMargin(margin string) *FlexText {
	t.Margin = margin
	return t
}

// WithMaxLines sets the maximum number of lines to display.
func (t *FlexText) WithMaxLines(lines int) *FlexText {
	if lines < 0 {
		lines = 0
	}
	// Clamp to int32 range to prevent overflow
	if lines > math.MaxInt32 {
		lines = math.MaxInt32
	}
	t.MaxLines = int32(lines)
	return t
}

// WithLineSpacing sets the spacing between lines.
func (t *FlexText) WithLineSpacing(spacing string) *FlexText {
	t.LineSpacing = spacing
	return t
}

// WithAdjustMode sets the text adjust mode (shrink-to-fit).
func (t *FlexText) WithAdjustMode(mode string) *FlexText {
	t.AdjustMode = messaging_api.FlexTextADJUST_MODE(mode)
	return t
}

// FlexButton wrapper for messaging_api.FlexButton with fluent API.
type FlexButton struct {
	*messaging_api.FlexButton
}

// NewFlexButton creates a new FlexButton with the specified action.
func NewFlexButton(action messaging_api.ActionInterface) *FlexButton {
	return &FlexButton{&messaging_api.FlexButton{
		Action: action,
	}}
}

// WithStyle sets the button style (link/primary/secondary).
func (b *FlexButton) WithStyle(style string) *FlexButton {
	b.Style = messaging_api.FlexButtonSTYLE(style)
	return b
}

// WithColor sets the button color.
func (b *FlexButton) WithColor(color string) *FlexButton {
	b.Color = color
	return b
}

// WithHeight sets the button height (sm/md).
func (b *FlexButton) WithHeight(height string) *FlexButton {
	b.Height = messaging_api.FlexButtonHEIGHT(height)
	return b
}

// WithMargin sets the margin of the button.
func (b *FlexButton) WithMargin(margin string) *FlexButton {
	b.Margin = margin
	return b
}

// FlexSeparator wrapper for messaging_api.FlexSeparator with fluent API.
type FlexSeparator struct {
	*messaging_api.FlexSeparator
}

// NewFlexSeparator creates a new FlexSeparator.
func NewFlexSeparator() *FlexSeparator {
	return &FlexSeparator{&messaging_api.FlexSeparator{}}
}

// WithMargin sets the margin of the separator.
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

// NewHeroBox creates a standardized Hero box with LINE green background
// Provides consistent styling across all modules:
// - Background: ColorHeroBg (LINE Green #06C755)
// - Padding: 24px all, 20px bottom (4-point grid aligned, visual balance)
// - Title: Bold, XL size, white color, full wrap for complete display
// - Subtitle: XS size, white color, md margin top (omitted if empty)
func NewHeroBox(title, subtitle string) *FlexBox {
	contents := []messaging_api.FlexComponentInterface{
		NewFlexText(title).WithWeight("bold").WithSize("xl").WithColor(ColorHeroText).WithWrap(true).WithLineSpacing(LineSpacingLarge).FlexText,
	}
	// Only add subtitle if not empty (LINE API rejects empty text)
	if subtitle != "" {
		contents = append(contents, NewFlexText(subtitle).WithSize("xs").WithColor(ColorHeroText).WithMargin("md").WithWrap(true).FlexText)
	}
	box := NewFlexBox("vertical", contents...)
	box.BackgroundColor = ColorHeroBg
	box.PaddingAll = SpacingXXL
	box.PaddingBottom = SpacingXL
	return box
}

// NewCompactHeroBox creates a compact Hero box for carousel/list views
// Uses smaller padding (16px, 4-point grid aligned) to fit more content
// Max 3 lines for carousel to balance visibility
func NewCompactHeroBox(title string) *FlexBox {
	box := NewFlexBox("vertical",
		NewFlexText(title).WithWeight("bold").WithSize("md").WithColor(ColorHeroText).WithWrap(true).WithMaxLines(3).WithLineSpacing(LineSpacingNormal).FlexText,
	)
	box.BackgroundColor = ColorHeroBg
	box.PaddingAll = SpacingL
	return box
}

// NewHeaderBadge creates a consistent header badge for Flex Messages
// Format: [emoji] [label] with LINE green color
func NewHeaderBadge(emoji, label string) *FlexBox {
	return NewFlexBox("vertical",
		NewFlexBox("baseline",
			NewFlexText(emoji).WithSize("lg").FlexText,
			NewFlexText(label).WithWeight("bold").WithColor(ColorPrimary).WithSize("sm").WithMargin("sm").FlexText,
		).FlexBox,
	)
}

// InfoRowStyle defines the visual style for an info row
type InfoRowStyle struct {
	ValueSize   string // Value text size: "xs", "sm", "md" (default: "sm")
	ValueWeight string // Value text weight: "regular", "bold" (default: "regular")
	ValueColor  string // Value text color (default: "#333333")
	Wrap        bool   // Whether to wrap long text (default: true)
}

// DefaultInfoRowStyle returns the standard info row style
func DefaultInfoRowStyle() InfoRowStyle {
	return InfoRowStyle{
		ValueSize:   "sm",
		ValueWeight: "regular",
		ValueColor:  ColorText,
		Wrap:        true,
	}
}

// BoldInfoRowStyle returns a style with bold value text (for important data like phone/ID)
func BoldInfoRowStyle() InfoRowStyle {
	return InfoRowStyle{
		ValueSize:   "md",
		ValueWeight: "bold",
		ValueColor:  ColorText,
		Wrap:        false,
	}
}

// NewInfoRow creates a vertical info row with icon + label on top, value below
// This is a standardized pattern used across all modules for Flex Message body content
//
// Layout:
//
//	┌─────────────────────────────┐
//	│ [emoji] [label]             │  <- icon + label (horizontal, gray)
//	│ [value text with wrap]      │  <- value (full width, wrappable)
//	└─────────────────────────────┘
//
// Example usage:
//
//	NewInfoRow("👨‍🏫", "授課教師", "王教授、李教授", DefaultInfoRowStyle())
//	NewInfoRow("☎️", "分機號碼", "12345", BoldInfoRowStyle())
func NewInfoRow(emoji, label, value string, style InfoRowStyle) *FlexBox {
	valueText := NewFlexText(value).WithColor(style.ValueColor).WithSize(style.ValueSize).WithMargin("sm")
	if style.ValueWeight == "bold" {
		valueText = valueText.WithWeight("bold")
	}
	if style.Wrap {
		valueText = valueText.WithWrap(true).WithLineSpacing(SpacingXS)
	}

	return NewFlexBox("vertical",
		NewFlexBox("horizontal",
			NewFlexText(emoji).WithSize("sm").WithFlex(0).FlexText,
			NewFlexText(label).WithColor(ColorLabel).WithSize("xs").WithFlex(0).WithMargin("sm").FlexText,
		).WithSpacing("sm").FlexBox,
		valueText.FlexText,
	)
}

// NewInfoRowWithMargin creates an info row with specified margin (convenience wrapper)
func NewInfoRowWithMargin(emoji, label, value string, style InfoRowStyle, margin string) messaging_api.FlexComponentInterface {
	return NewInfoRow(emoji, label, value, style).WithMargin(margin).FlexBox
}

// NewButtonRow creates a horizontal box containing buttons with equal width distribution.
// Each button gets flex:1 to share space equally.
// Use this for creating a row of action buttons in Flex Message footer.
//
// Parameters:
//   - buttons: Variable number of FlexButton to include in the row
//
// Returns: FlexBox with horizontal layout containing the buttons
func NewButtonRow(buttons ...*FlexButton) *FlexBox {
	contents := make([]messaging_api.FlexComponentInterface, 0, len(buttons))
	for _, btn := range buttons {
		if btn != nil {
			// Wrap button in a box with flex:1 for equal distribution
			btnBox := NewFlexBox("vertical", btn.FlexButton)
			btnBox.Flex = 1
			contents = append(contents, btnBox.FlexBox)
		}
	}
	return NewFlexBox("horizontal", contents...).WithSpacing("sm")
}

// NewButtonFooter creates a footer with multiple rows of buttons.
// Each row is rendered horizontally, rows are stacked vertically.
// Empty rows are automatically filtered out.
//
// Layout:
//
//	┌─────────────────────────────────────────┐
//	│ [btn1]  [btn2]                          │ <- row 1 (e.g., phone)
//	│ [btn3]  [btn4]                          │ <- row 2 (e.g., email)
//	│ [btn5]                                  │ <- row 3 (e.g., website)
//	└─────────────────────────────────────────┘
//
// Parameters:
//   - rows: Variable number of button slices, each slice becomes one row
//
// Returns: FlexBox suitable for Flex Bubble footer
func NewButtonFooter(rows ...[]*FlexButton) *FlexBox {
	var contents []messaging_api.FlexComponentInterface

	for _, row := range rows {
		// Filter nil buttons from row
		var validButtons []*FlexButton
		for _, btn := range row {
			if btn != nil {
				validButtons = append(validButtons, btn)
			}
		}

		// Add row if not empty
		if len(validButtons) > 0 {
			contents = append(contents, NewButtonRow(validButtons...).FlexBox)
		}
	}

	return NewFlexBox("vertical", contents...).WithSpacing("sm")
}

// ================================================
// Body Content Builders (for consistent info display)
// ================================================

// BodyContentBuilder helps build Flex Message body contents with automatic separators
type BodyContentBuilder struct {
	contents []messaging_api.FlexComponentInterface
}

// NewBodyContentBuilder creates a new body content builder
func NewBodyContentBuilder() *BodyContentBuilder {
	return &BodyContentBuilder{
		contents: make([]messaging_api.FlexComponentInterface, 0),
	}
}

// AddInfoRow adds an info row with automatic separator (except for first item)
func (b *BodyContentBuilder) AddInfoRow(emoji, label, value string, style InfoRowStyle) *BodyContentBuilder {
	if len(b.contents) > 0 {
		b.contents = append(b.contents, NewFlexSeparator().WithMargin("sm").FlexSeparator)
	}
	b.contents = append(b.contents, NewInfoRowWithMargin(emoji, label, value, style, "sm"))
	return b
}

// AddInfoRowIf adds an info row only if value is not empty
func (b *BodyContentBuilder) AddInfoRowIf(emoji, label, value string, style InfoRowStyle) *BodyContentBuilder {
	if value != "" {
		return b.AddInfoRow(emoji, label, value, style)
	}
	return b
}

// AddComponent adds a raw component with automatic separator
func (b *BodyContentBuilder) AddComponent(component messaging_api.FlexComponentInterface) *BodyContentBuilder {
	if len(b.contents) > 0 {
		b.contents = append(b.contents, NewFlexSeparator().WithMargin("sm").FlexSeparator)
	}
	b.contents = append(b.contents, component)
	return b
}

// Build returns the FlexBox with all contents
func (b *BodyContentBuilder) Build() *FlexBox {
	return NewFlexBox("vertical", b.contents...).WithSpacing("sm")
}

// Contents returns the raw contents slice (for manual FlexBox creation)
func (b *BodyContentBuilder) Contents() []messaging_api.FlexComponentInterface {
	return b.contents
}
