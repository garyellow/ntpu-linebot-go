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
const (
	// Brand & Semantic Colors
	ColorPrimary = "#06C755" // LINE Green - hero backgrounds, primary buttons
	ColorDanger  = "#FF334B" // Errors, warnings, urgent info

	// Text Colors
	ColorText    = "#111111" // Primary text (body, headings)
	ColorLabel   = "#666666" // Labels, captions (WCAG AA compliant)
	ColorSubtext = "#777777" // Secondary text, descriptions
	ColorGray400 = "#B7B7B7" // Disabled/muted text

	// Component Colors
	ColorHeroBg   = ColorPrimary // Hero section background
	ColorHeroText = "#FFFFFF"    // Hero section text (white)
)
