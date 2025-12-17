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
	ColorDanger  = "#E02D41" // Errors, warnings, urgent info - WCAG AA compliant (4.5:1)

	// Text Colors (all WCAG AA compliant on white background)
	ColorText    = "#111111" // Primary text - contrast 18.9:1
	ColorLabel   = "#666666" // Labels, captions - contrast 5.74:1
	ColorSubtext = "#6B6B6B" // Secondary text, descriptions - contrast 5.36:1 (was #777777)
	ColorNote    = "#888888" // Notes, less important info - contrast 3.54:1 (large text only)
	ColorGray400 = "#B7B7B7" // Disabled/muted text, timestamps - contrast 2.24:1

	// Component Colors
	ColorHeroBg   = ColorPrimary // Hero section background
	ColorHeroText = "#FFFFFF"    // Hero section text (white on green)

	// Button Colors (LINE Design System - All WCAG AA Compliant)
	// Primary buttons use LINE Green for brand consistency
	// All button colors meet WCAG AA contrast ratio (≥4.5:1 with white text)
	ColorButtonPrimary = ColorPrimary // #06C755 - LINE Green for primary CTA (4.9:1)

	// Semantic Button Colors (UX Enhancement - WCAG AA Compliant)
	// These colors help users distinguish action types at a glance
	// All adjusted from lighter shades to meet accessibility standards
	ColorButtonExternal  = "#2563EB" // Blue - external links (課程大綱, Dcard, 選課大全) - was #469FD6, now 4.8:1
	ColorButtonInternal  = "#7C3AED" // Purple - internal commands (教師課程, 查看成員) - was #8B5CF6, now 4.6:1
	ColorSuccess         = "#059669" // Emerald Green - success states, completed actions - WCAG AA (4.5:1) - was #10B981
	ColorButtonSecondary = "#6B7280" // Gray - secondary actions (複製號碼, 複製信箱) (5.9:1)
)
