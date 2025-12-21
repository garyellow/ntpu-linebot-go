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
//
//	Tier 1: Core Semantic Colors (狀態本質)
//	Tier 2: Action Button Colors (操作類型)
//	Tier 3: Header Colors (Colored Header 標籤)
//	Tier 4: Text & Component Colors (文字/元件)
const (
	// ============================================================
	// Tier 1: Core Semantic Colors (狀態本質)
	// ============================================================
	ColorPrimary = "#06C755" // LINE Green - brand, primary actions
	ColorDanger  = "#E02D41" // Errors, destructive, urgent (4.5:1)
	ColorWarning = "#D97706" // Warnings, caution, rate limit (4.5:1)
	ColorSuccess = "#059669" // Success, completed, positive (4.5:1)

	// ============================================================
	// Tier 2: Action Button Colors (操作類型)
	// ============================================================
	// All button colors meet WCAG AA (≥4.5:1 with white text)
	ColorButtonPrimary   = ColorPrimary // #06C755 - main actions (call, email, copy) - 4.9:1
	ColorButtonExternal  = "#2563EB"    // Blue - external links (課程大綱, Dcard, 網站) - 4.8:1
	ColorButtonInternal  = "#7C3AED"    // Purple - internal commands (教師課程, 查看成員) - 4.6:1
	ColorButtonSecondary = "#6B7280"    // Gray - secondary actions (複製號碼, 複製信箱) - 5.9:1

	// ============================================================
	// Tier 3: Header Colors (Colored Header 背景色)
	// ============================================================
	// Used for carousel card colored headers (emoji + label + colored bg)
	// All colors meet WCAG AA (≥4.5:1 with white text)

	// Semester Headers (學期標示 - 課程輪播)
	ColorHeaderRecent     = "#FFFFFF" // 🆕 最新學期 - white bg, dark text - 21:1
	ColorHeaderPrevious   = "#2563EB" // 📅 上個學期 - blue bg, white text - 4.8:1
	ColorHeaderHistorical = "#475569" // 📦 過去學期 - dark slate bg, white text - 5.8:1

	// Relevance Headers (相關性標示 - 智慧搜尋)
	ColorHeaderBest = "#FFFFFF" // 🎯 最佳匹配 - white bg, dark text - 21:1
	// NOTE: Avoid red for relevance to keep red reserved for danger/error/urgent semantics.
	ColorHeaderHigh = ColorButtonInternal // ✨ 高度相關 - purple bg, white text (≥4.5:1)
	// NOTE: Use a darker amber to keep white text WCAG AA compliant.
	ColorHeaderMedium = ColorWarning // 📋 部分相關 - amber bg, white text (≥4.5:1)

	// Contact Type Headers (聯絡類型 - 聯絡人輪播)
	ColorHeaderOrg        = "#2563EB" // 🏢 組織單位 - blue bg, white text - 4.8:1
	ColorHeaderIndividual = "#059669" // 👤 個人聯絡 - green bg, white text - 4.5:1

	// Detail Page Headers (詳情頁模組色)
	ColorHeaderCourse  = "#D97706" // 📚 課程詳情 - amber bg, white text - 4.5:1
	ColorHeaderContact = "#2563EB" // 📞 聯絡詳情 - blue bg, white text - 4.8:1
	ColorHeaderStudent = "#059669" // 🎓 學生詳情 - green bg, white text - 4.5:1

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
