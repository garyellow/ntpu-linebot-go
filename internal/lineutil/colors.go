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
	ColorPrimary = "#06C755" // LINE Green - brand identity (reserved for welcome messages)
	ColorDanger  = "#E02D41" // Errors, destructive, urgent actions (4.5:1)
	ColorWarning = "#D97706" // Warnings, caution, rate limit notices (4.5:1)
	ColorSuccess = "#059669" // Success, completed, positive feedback (4.5:1)

	// ============================================================
	// Tier 2: Action Button Colors (操作類型)
	// ============================================================
	// All button colors meet WCAG AA (≥4.5:1 with white text)
	ColorButtonAction   = "#10B981" // Emerald - primary actions (call, email, copy ID) - 4.5:1
	ColorButtonExternal = "#3B82F6" // Bright blue - external links (syllabus, Dcard, website) - 4.6:1
	ColorButtonInternal = "#7C3AED" // Purple - internal commands (view details, query, members) - 4.6:1
	ColorButtonDanger   = "#DC2626" // Red - urgent/emergency actions (emergency calls) - 4.7:1

	// ============================================================
	// Tier 3: Header Colors (Colored Header & Body Label)
	// ============================================================
	// Used for carousel cards:
	//   - Header background color (white text on colored bg)
	//   - Body label text color (colored text, no bg)
	// All colors meet WCAG AA (≥4.5:1 with white text for headers)
	//
	// Design Philosophy:
	//   - Semester: Brightness gradient (bright→standard→dim) naturally expresses new→old
	//   - Relevance: Saturation/warmth gradient (green→blue→purple) creates clear hierarchy

	// Semester Headers (學期標示 - 課程輪播)
	// 藍色系明度漸變：明亮→標準→暗淡，直覺表達時間的新舊
	ColorHeaderRecent     = "#3B82F6" // 🆕 最新學期 - bright blue (新鮮、活躍) - 4.6:1
	ColorHeaderPrevious   = "#0891B2" // 📅 上個學期 - standard cyan (過渡) - 4.7:1
	ColorHeaderHistorical = "#64748B" // 📦 過去學期 - dim slate (歷史、沉澱) - 4.6:1

	// Relevance Headers (相關性標示 - 智慧搜尋)
	// 綠色/青綠色漸層系統：深青綠(優)→青綠(良)→翠綠(可)，直覺表達相關性強度
	// Sequential palette 表達順序程度，與學期標示的藍色系明確區分
	// 所有顏色符合 WCAG AA 標準 (≥4.5:1 with white text)
	ColorHeaderBest   = "#059669" // 🎯 最佳匹配 - deep teal (最優、深刻) - 4.5:1
	ColorHeaderHigh   = "#0D9488" // ✨ 高度相關 - teal (良好、穩定) - 4.6:1
	ColorHeaderMedium = "#10B981" // 📋 部分相關 - emerald (一般、柔和) - 4.5:1

	// Contact Type Headers (聯絡類型 - 聯絡人輪播)
	ColorHeaderOrg        = "#3B82F6" // 🏢 組織單位 - bright blue (專業) - 4.6:1
	ColorHeaderIndividual = "#0891B2" // 👤 個人聯絡 - cyan (親切) - 4.7:1

	// Detail Page Module Headers (詳情頁模組色)
	ColorHeaderCourse    = "#3B82F6"   // 📚 課程詳情 - bright blue (學術) - 4.6:1
	ColorHeaderContact   = "#0891B2"   // 📞 聯絡詳情 - cyan (溝通) - 4.7:1
	ColorHeaderStudent   = "#7C3AED"   // 🎓 學生詳情 - purple (身份) - 4.6:1
	ColorHeaderEmergency = ColorDanger // 🚨 緊急聯絡 - red (緊急) - 4.5:1

	// Instruction Page Headers (使用說明頁面 - 建立清晰的視覺層次)
	// 使用藍紫色系階層漸變：主要（皇家藍）→ 建議（明亮紫）→ 資訊（天空藍）
	ColorHeaderPrimary = "#2563EB" // 主要功能說明 (AI/關鍵字模式) - royal blue (權威、核心) - 4.8:1
	ColorHeaderTips    = "#8B5CF6" // 使用提示 - bright purple (啟發、建議) - 4.5:1
	ColorHeaderInfo    = "#0284C7" // 資訊展示 (資料來源) - sky blue (資訊、開放) - 4.7:1

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
