// Package lineutil provides LINE message building utilities.
package lineutil

// 4-Point Grid Spacing System
// All spacing values follow the 4-point grid for consistent visual rhythm.
// Reference: Material Design spacing guidelines
const (
	SpacingNone = "none" // 0px
	SpacingXS   = "4px"  // Extra small
	SpacingS    = "8px"  // Small
	SpacingM    = "12px" // Medium
	SpacingL    = "16px" // Large
	SpacingXL   = "20px" // Extra large
	SpacingXXL  = "24px" // 2X large

	// Line Spacing for multi-line text readability
	LineSpacingNormal = "6px" // Standard line spacing
	LineSpacingLarge  = "8px" // Enhanced readability for dense content
)

// LINE Design System Colors
// Reference: https://designsystem.line.me/LDSM/foundation/color/line-color-guide-ex-en
//
// These colors follow LINE's official design guidelines for consistent branding
// and accessibility (minimum 3.0:1 contrast ratio for text).

const (
	// Brand Colors - LINE Green
	// Use for primary actions, hero backgrounds, and brand emphasis
	ColorLineGreen    = "#06C755" // LINE Green (iOS) - Primary brand color
	ColorLineGreenAnd = "#4CC764" // LINE Green (Android) - Slightly lighter variant

	// Gray Scale - For text, labels, and UI elements
	ColorWhite   = "#FFFFFF" // Pure white
	ColorGray100 = "#FCFCFC" // Lightest gray
	ColorGray150 = "#F5F5F5" // Very light gray
	ColorGray200 = "#EFEFEF" // Light gray
	ColorGray250 = "#E8E8E8" // Light gray
	ColorGray300 = "#DFDFDF" // Separator, divider
	ColorGray350 = "#C8C8C8" // Light border
	ColorGray400 = "#B7B7B7" // Disabled text (use sparingly)
	ColorGray500 = "#949494" // Label text (3.0:1 contrast ratio)
	ColorGray600 = "#777777" // Secondary text
	ColorGray650 = "#616161" // Medium gray
	ColorGray700 = "#555555" // Dark gray
	ColorGray750 = "#3F3F3F" // Very dark gray
	ColorGray800 = "#2A2A2A" // Near black
	ColorGray850 = "#1F1F1F" // Near black
	ColorGray900 = "#111111" // Primary text (highest contrast)
	ColorBlack   = "#000000" // Pure black

	// Accent Colors - For emphasis and interactive elements
	ColorBlue400 = "#96B2FF" // Light blue
	ColorBlue500 = "#638DFF" // Button background, tooltip, secondary actions
	ColorBlue600 = "#4270ED" // Darker blue
	ColorBlue700 = "#2F59CC" // Darkest blue

	// Alert Colors - For warnings, errors, and urgent information
	ColorRed400 = "#FF334B" // Error, warning (iOS)

	// Semantic Colors - Use these for consistent meaning across the app
	ColorPrimary   = ColorLineGreen // Primary brand color for hero, buttons
	ColorSecondary = ColorBlue500   // Secondary actions, links
	ColorDanger    = ColorRed400    // Errors, warnings, urgent info

	// Text Colors - For typography hierarchy
	ColorText    = ColorGray900 // Primary text (body, headings)
	ColorLabel   = "#666666"    // Labels, captions (5.7:1 contrast ratio, WCAG AA compliant)
	ColorSubtext = ColorGray600 // Secondary text, descriptions

	// Component Colors - For specific UI components
	ColorHeroBg          = ColorLineGreen // Hero section background
	ColorHeroText        = ColorWhite     // Hero section text
	ColorSeparator       = ColorGray300   // Divider lines
	ColorButtonPrimary   = ColorLineGreen // Primary button background
	ColorButtonSecondary = ColorBlue500   // Secondary button background
)
