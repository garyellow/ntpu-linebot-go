// Package lineutil provides LINE message building utilities.
package lineutil

// 4-Point Grid Spacing System
const (
	SpacingNone = "none" // 0px
	SpacingXS   = "4px"  // Extra small
	SpacingS    = "8px"  // Small
	SpacingM    = "12px" // Medium
	SpacingL    = "16px" // Large
	SpacingXL   = "20px" // Extra large
	SpacingXXL  = "24px" // 2X large
)

// Line Spacing for multi-line text
const (
	LineSpacingNormal = "6px" // Standard
	LineSpacingLarge  = "8px" // Enhanced readability
)

// LINE Design System Colors
// Reference: https://designsystem.line.me/LDSM/foundation/color/line-color-guide-ex-en
// WCAG AA requires 4.5:1 contrast ratio for normal text, 3:1 for large text
const (
	// Brand & Semantic Colors
	ColorPrimary = "#06C755" // LINE Green - hero backgrounds, primary buttons
	ColorDanger  = "#FF334B" // Errors, warnings, urgent info

	// Text Colors (all WCAG AA compliant on white background)
	ColorText    = "#111111" // Primary text - contrast 18.9:1
	ColorLabel   = "#666666" // Labels, captions - contrast 5.74:1
	ColorSubtext = "#6B6B6B" // Secondary text, descriptions - contrast 5.36:1 (was #777777)
	ColorNote    = "#888888" // Notes, less important info - contrast 3.54:1 (large text only)
	ColorGray400 = "#B7B7B7" // Disabled/muted text, timestamps - contrast 2.24:1

	// Component Colors
	ColorHeroBg   = ColorPrimary // Hero section background
	ColorHeroText = "#FFFFFF"    // Hero section text (white on green)

	// Button Colors (LINE Design System)
	// Primary buttons should use LINE Green for brand consistency
	// Secondary buttons use default gray (no color override needed)
	// Danger buttons use ColorDanger for urgent/destructive actions
	ColorButtonPrimary = ColorPrimary // #06C755 - LINE Green for primary CTA

	// Semantic Button Colors (UX Enhancement)
	// External: Blue color for external website links (opens browser)
	// Internal: Purple color for internal commands/postback actions
	// These colors help users distinguish action types at a glance
	ColorButtonExternal = "#469FD6" // Blue - external links (課程大綱, Dcard, 選課大全)
	ColorButtonInternal = "#8B5CF6" // Purple - internal commands (教師課程, 查看成員)
)
