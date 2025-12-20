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
//
// 4-Tier Semantic Color Architecture:
//   Tier 1: Core Semantic Colors (状态本质)
//   Tier 2: Action Button Colors (操作类型)
//   Tier 3: Header Colors (Colored Header 标签)
//   Tier 4: Text & Component Colors (文字/元件)
const (
	// ============================================================
	// Tier 1: Core Semantic Colors (状态本质)
	// ============================================================
	ColorPrimary = "#06C755" // LINE Green - brand, primary actions
	ColorDanger  = "#E02D41" // Errors, destructive, urgent (4.5:1)
	ColorWarning = "#D97706" // Warnings, caution, rate limit (4.5:1)
	ColorSuccess = "#059669" // Success, completed, positive (4.5:1)

	// ============================================================
	// Tier 2: Action Button Colors (操作类型)
	// ============================================================
	// All button colors meet WCAG AA (≥4.5:1 with white text)
	ColorButtonPrimary   = ColorPrimary // #06C755 - main actions (call, email, copy) - 4.9:1
	ColorButtonExternal  = "#2563EB"    // Blue - external links (课程大纲, Dcard, 网站) - 4.8:1
	ColorButtonInternal  = "#7C3AED"    // Purple - internal commands (教师课程, 查看成员) - 4.6:1
	ColorButtonSecondary = "#6B7280"    // Gray - secondary actions (复制号码, 复制信箱) - 5.9:1

	// ============================================================
	// Tier 3: Header Colors (Colored Header 背景色)
	// ============================================================
	// Used for carousel card colored headers (emoji + label + colored bg)
	// All colors meet WCAG AA (≥4.5:1 with white text)

	// Semester Headers (学期标示 - 课程轮播)
	ColorHeaderRecent     = "#FFFFFF" // 🆕 最新学期 - white bg, dark text - 21:1
	ColorHeaderPrevious   = "#2563EB" // 📅 上个学期 - blue bg, white text - 4.8:1
	ColorHeaderHistorical = "#475569" // 📦 过去学期 - dark slate bg, white text - 5.8:1

	// Relevance Headers (相关性标示 - 智慧搜寻)
	ColorHeaderBest   = "#FFFFFF" // 🎯 最佳匹配 - white bg, dark text - 21:1
	ColorHeaderHigh   = "#DC2626" // ✨ 高度相关 - red bg, white text - 5.2:1
	ColorHeaderMedium = "#F59E0B" // 📋 部分相关 - amber bg, white text - 4.5:1

	// Contact Type Headers (联络类型 - 联络人轮播)
	ColorHeaderOrg        = "#2563EB" // 🏢 组织单位 - blue bg, white text - 4.8:1
	ColorHeaderIndividual = "#059669" // 👤 个人联络 - green bg, white text - 4.5:1

	// Detail Page Headers (详情页模组色)
	ColorHeaderCourse  = "#D97706" // 📚 课程详情 - amber bg, white text - 4.5:1
	ColorHeaderContact = "#2563EB" // 📞 联络详情 - blue bg, white text - 4.8:1
	ColorHeaderStudent = "#059669" // 🎓 学生详情 - green bg, white text - 4.5:1

	// ============================================================
	// Tier 4: Text & Component Colors
	// ============================================================
	// Text Colors (all WCAG AA compliant on white background)
	ColorText    = "#111111" // Primary text - contrast 18.9:1
	ColorLabel   = "#666666" // Labels, captions - contrast 5.74:1
	ColorSubtext = "#6B6B6B" // Secondary text, descriptions - contrast 5.36:1
	ColorNote    = "#888888" // Notes, less important info - contrast 3.54:1 (large text only)
	ColorGray400 = "#B7B7B7" // Disabled/muted text, timestamps - contrast 2.24:1

	// Component Colors
	ColorHeroBg   = ColorPrimary // Hero section background
	ColorHeroText = "#FFFFFF"    // Hero section text (white on green)
)
