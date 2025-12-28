package program

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Color constants for program module (referencing lineutil design system)
const (
	// Program module header color (使用與課程相同的藍色系，表示學術相關)
	ColorHeaderProgram = lineutil.ColorHeaderCourse // #3B82F6 - bright blue

	// Course type colors for program courses carousel (引用 lineutil 設計系統)
	ColorHeaderRequired = lineutil.ColorHeaderRequired // ✅ 必修 - deep teal
	ColorHeaderElective = lineutil.ColorHeaderElective // 📝 選修 - cyan

	// Category-based colors for program bubbles (引用 lineutil 設計系統)
	// Gradient: 碩士類偏紫色系、學士類偏藍色系
	ColorCategoryMasterCredit   = lineutil.ColorHeaderProgramMasterCredit   // 🎓 碩士學分學程
	ColorCategoryBachelorCredit = lineutil.ColorHeaderProgramBachelorCredit // 📚 學士學分學程
	ColorCategoryMixedCredit    = lineutil.ColorHeaderProgramMixedCredit    // 🎓 學士暨碩士學分學程
	ColorCategoryMasterCross    = lineutil.ColorHeaderProgramMasterCross    // 🔗 碩士跨域微學程
	ColorCategoryBachelorCross  = lineutil.ColorHeaderProgramBachelorCross  // 🔗 學士跨域微學程
	ColorCategoryMixedCross     = lineutil.ColorHeaderProgramMixedCross     // 🔗 學士暨碩士跨域微學程
	ColorCategoryMasterSingle   = lineutil.ColorHeaderProgramMasterSingle   // 📌 碩士單一領域微學程
	ColorCategoryBachelorSingle = lineutil.ColorHeaderProgramBachelorSingle // 📌 學士單一領域微學程
	ColorCategoryDefault        = lineutil.ColorHeaderProgramDefault        // 🎓 學程 (fallback)
)

// getCategoryLabel returns a BodyLabelInfo based on the program category.
// Maps program categories to appropriate emoji, label text, and color.
//
// Categories (from LMS folders):
//   - "碩士學分學程" - Master's credit program
//   - "學士學分學程" - Bachelor's credit program
//   - "學士暨碩士學分學程" - Joint bachelor/master credit program
//   - "碩士跨域微學程" - Master's cross-domain micro-program
//   - "學士跨域微學程" - Bachelor's cross-domain micro-program
//   - "學士暨碩士跨域微學程" - Joint cross-domain micro-program
//   - "碩士單一領域微學程" - Master's single-domain micro-program
//   - "學士單一領域微學程" - Bachelor's single-domain micro-program
//
// Design rationale:
//   - 碩士類 uses violet/purple gradient (academic prestige)
//   - 學士類 uses blue/cyan gradient (fresh, approachable)
//   - 跨域類 uses 🔗 emoji (cross-connection)
//   - 單一領域 uses 📌 emoji (focused, specialized)
func getCategoryLabel(category string) lineutil.BodyLabelInfo {
	switch category {
	case "碩士學分學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🎓",
			Label: "碩士學分學程",
			Color: ColorCategoryMasterCredit,
		}
	case "學士學分學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📚",
			Label: "學士學分學程",
			Color: ColorCategoryBachelorCredit,
		}
	case "學士暨碩士學分學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🎓",
			Label: "學士暨碩士學分學程",
			Color: ColorCategoryMixedCredit,
		}
	case "碩士跨域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🔗",
			Label: "碩士跨域微學程",
			Color: ColorCategoryMasterCross,
		}
	case "學士跨域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🔗",
			Label: "學士跨域微學程",
			Color: ColorCategoryBachelorCross,
		}
	case "學士暨碩士跨域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🔗",
			Label: "學士暨碩士跨域微學程",
			Color: ColorCategoryMixedCross,
		}
	case "碩士單一領域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📌",
			Label: "碩士單一領域微學程",
			Color: ColorCategoryMasterSingle,
		}
	case "學士單一領域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📌",
			Label: "學士單一領域微學程",
			Color: ColorCategoryBachelorSingle,
		}
	default:
		// Fallback for unknown category or empty string
		return lineutil.BodyLabelInfo{
			Emoji: "🎓",
			Label: "學程",
			Color: ColorCategoryDefault,
		}
	}
}

// formatProgramListResponse formats a list of programs as a text message.
// Uses text-based display to handle large lists (>50 programs).
// Format:
// 🎓 學程列表 (共 N 個)
// ────────────────
//  1. 學程名稱 (必X/選Y)
//     https://lms.ntpu.edu.tw/...
//
// 2. 學程名稱...
// formatProgramListResponse formats a list of programs as a text message.
// Uses text-based display to handle large lists.
// Consolidates all programs into a single message if possible (limit 5000 chars).
func (h *Handler) formatProgramListResponse(programs []storage.Program, totalCount int) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)
	var messages []messaging_api.MessageInterface

	// Track rune count of sb (LINE limit is 5000 characters)
	sbRunes := 0
	var sb strings.Builder

	header := fmt.Sprintf("🎓 學程列表 (共 %d 個)\n", totalCount)
	sb.WriteString(header)
	sbRunes += utf8.RuneCountInString(header)

	separator := "━━━━━━━━━━━━━━━━\n\n"
	sb.WriteString(separator)
	sbRunes += utf8.RuneCountInString(separator)

	for i, prog := range programs {
		// Global index
		idx := i + 1

		// Proposed entry
		var entry strings.Builder
		fmt.Fprintf(&entry, "%d. %s", idx, prog.Name)

		// Show course counts if available
		if prog.RequiredCount > 0 || prog.ElectiveCount > 0 {
			entry.WriteString(fmt.Sprintf(" | 必修 %d 門 · 選修 %d 門", prog.RequiredCount, prog.ElectiveCount))
		}
		entry.WriteString("\n")

		// Add URL if available (LINE will auto-link)
		if prog.URL != "" {
			entry.WriteString(fmt.Sprintf("   📎 %s\n", prog.URL))
		}

		// Add spacing between items
		entry.WriteString("\n")

		entryStr := entry.String()
		entryRunes := utf8.RuneCountInString(entryStr)

		// Check if adding this entry would exceed the limit (using 4800 characters as safety margin)
		// Max Text Message length is 5000 characters.
		if sbRunes+entryRunes > 4800 {
			// Finalize current message
			messages = append(messages, lineutil.NewTextMessageWithConsistentSender(sb.String(), sender))
			sb.Reset()
			sbRunes = 0

			separator := "━━━━━━━━━━━━━━━━\n"
			sb.WriteString(separator)
			sbRunes += utf8.RuneCountInString(separator)

			headerCont := fmt.Sprintf("🎓 學程列表 (續 - %d...)\n\n", idx)
			sb.WriteString(headerCont)
			sbRunes += utf8.RuneCountInString(headerCont)
		}

		sb.WriteString(entryStr)
		sbRunes += entryRunes
	}

	// Add footer
	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	sb.WriteString("💡 輸入「學程 關鍵字」搜尋特定學程")

	// Create the final (or only) message
	msg := lineutil.NewTextMessageWithConsistentSender(sb.String(), sender)
	msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
	messages = append(messages, msg)

	return messages
}

// formatProgramSearchResponse formats search results as carousel Flex Messages.
// Used when results are few enough to be displayed in a carousel (<= MaxSearchResultsWithCard).
func (h *Handler) formatProgramSearchResponse(programs []storage.Program) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Build carousel bubbles
	bubbles := make([]messaging_api.FlexBubble, 0, len(programs))
	for _, program := range programs {
		bubble := h.buildProgramBubble(program)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Build carousel messages
	messages := lineutil.BuildCarouselMessages("學程搜尋結果", bubbles, sender)

	// Add quick reply to last message
	if len(messages) > 0 {
		lineutil.AddQuickReplyToMessages(messages, QuickReplyProgramNav()...)
	}

	return messages
}

// buildProgramBubble creates a Flex Bubble for a single program in the list.
//
// Layout:
//
//	┌──────────────────────────┐
//	│      學程名稱             │  <- Colored header (category-based)
//	├──────────────────────────┤
//	│ 🎓 碩士學分學程          │  <- Body label (dynamic category)
//	│ 📚 課程數量：15 門       │
//	│ ✅ 必修：8 門            │
//	│ 📝 選修：7 門            │
//	├──────────────────────────┤
//	│ [📋 查看學程詳細]        │  <- Footer button (external URL, if available)
//	│ [📚 查看課程]            │  <- Footer button (internal)
//	└──────────────────────────┘
func (h *Handler) buildProgramBubble(program storage.Program) *lineutil.FlexBubble {
	// Get category label info (emoji, label, color based on category)
	labelInfo := getCategoryLabel(program.Category)

	// Header: Program name with category-based colored background
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: lineutil.TruncateRunes(program.Name, MaxTitleDisplayChars),
		Color: labelInfo.Color,
	})

	// Build body contents
	body := lineutil.NewBodyContentBuilder()

	// Body label: dynamic category tag
	body.AddComponent(lineutil.NewBodyLabel(labelInfo).FlexBox)

	// Course count info
	totalCourses := program.RequiredCount + program.ElectiveCount
	body.AddComponent(lineutil.NewInfoRow("📚", "課程數量", fmt.Sprintf("%d 門", totalCourses), lineutil.DefaultInfoRowStyle()).FlexBox)

	// Required courses count
	if program.RequiredCount > 0 {
		body.AddInfoRow("✅", "必修", fmt.Sprintf("%d 門", program.RequiredCount), lineutil.DefaultInfoRowStyle())
	}

	// Elective courses count
	if program.ElectiveCount > 0 {
		body.AddInfoRow("📝", "選修", fmt.Sprintf("%d 門", program.ElectiveCount), lineutil.DefaultInfoRowStyle())
	}

	// Build footer buttons
	var footerButtons []*lineutil.FlexButton

	// Add LMS detail page button if URL is available
	if program.URL != "" {
		detailBtn := lineutil.NewFlexButton(
			lineutil.NewURIAction("📋 查看學程詳細", program.URL),
		).WithStyle("secondary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm")
		footerButtons = append(footerButtons, detailBtn)
	}

	// View courses button (internal)
	viewCoursesBtn := lineutil.NewFlexButton(
		lineutil.NewPostbackActionWithDisplayText(
			"📚 "+PostbackViewCoursesLabel,
			lineutil.TruncateRunes(fmt.Sprintf("查看「%s」的課程", program.Name), 40),
			PostbackPrefix+"courses"+bot.PostbackSplitChar+program.Name,
		),
	).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm")
	footerButtons = append(footerButtons, viewCoursesBtn)

	footer := lineutil.NewButtonFooter(footerButtons)

	return lineutil.NewFlexBubble(header, nil, body.Build(), footer)
}

// formatProgramCoursesResponse formats program courses as carousel Flex Messages.
// Required courses are displayed first, followed by elective courses.
func (h *Handler) formatProgramCoursesResponse(programName string, requiredCourses, electiveCourses []storage.ProgramCourse) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Build carousel bubbles
	bubbles := make([]messaging_api.FlexBubble, 0, len(requiredCourses)+len(electiveCourses))

	// Add required courses first
	for _, pc := range requiredCourses {
		bubble := h.buildProgramCourseBubble(pc, true)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Add elective courses
	for _, pc := range electiveCourses {
		bubble := h.buildProgramCourseBubble(pc, false)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	if len(bubbles) == 0 {
		msg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("📭 「%s」目前沒有課程資料", programName),
			sender,
		)
		msg.QuickReply = lineutil.NewQuickReply(QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	// Add header message with program info
	headerMsg := lineutil.NewTextMessageWithConsistentSender(
		fmt.Sprintf("🎓 %s\n\n📊 課程統計\n• 必修：%d 門\n• 選修：%d 門\n• 共計：%d 門\n\n⬇️ 以下為課程列表（必修優先）",
			programName,
			len(requiredCourses),
			len(electiveCourses),
			len(requiredCourses)+len(electiveCourses)),
		sender,
	)

	// Build carousel messages
	carouselMessages := lineutil.BuildCarouselMessages(
		lineutil.TruncateRunes(programName+"課程", 400),
		bubbles,
		sender,
	)

	messages := append([]messaging_api.MessageInterface{headerMsg}, carouselMessages...)

	// Add quick reply to last message
	if len(messages) > 0 {
		lineutil.AddQuickReplyToMessages(messages, QuickReplyProgramNav()...)
	}

	return messages
}

// buildProgramCourseBubble creates a Flex Bubble for a single course in the program.
//
// Layout:
//
//	┌──────────────────────────┐
//	│      課程名稱             │  <- Colored header (green=必修, cyan=選修)
//	├──────────────────────────┤
//	│ ✅ 必修 / 📝 選修        │  <- Body label
//	│ 📅 開課學期：113-1       │
//	│ 👨‍🏫 授課教師：王教授     │
//	│ ⏰ 上課時間：一1-2       │
//	├──────────────────────────┤
//	│     [查看詳細]           │  <- Footer button (external to course)
//	└──────────────────────────┘
func (h *Handler) buildProgramCourseBubble(pc storage.ProgramCourse, isRequired bool) *lineutil.FlexBubble {
	// Determine colors and labels based on course type
	var headerColor string
	var labelEmoji, labelText string
	if isRequired {
		headerColor = ColorHeaderRequired
		labelEmoji = "✅"
		labelText = "必修課程"
	} else {
		headerColor = ColorHeaderElective
		labelEmoji = "📝"
		labelText = "選修課程"
	}

	// Header: Course title with colored background
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: lineutil.TruncateRunes(lineutil.FormatCourseTitleWithUID(pc.Course.Title, pc.Course.UID), MaxTitleDisplayChars),
		Color: headerColor,
	})

	// Build body contents
	body := lineutil.NewBodyContentBuilder()

	// Body label (course type indicator)
	body.AddComponent(lineutil.NewBodyLabel(lineutil.BodyLabelInfo{
		Emoji: labelEmoji,
		Label: labelText,
		Color: headerColor,
	}).FlexBox)

	// Semester info - first row (no separator between label and first row)
	semesterText := lineutil.FormatSemester(pc.Course.Year, pc.Course.Term)
	firstInfoRow := lineutil.NewInfoRow("📅", "開課學期", semesterText, lineutil.DefaultInfoRowStyle())
	body.AddComponent(firstInfoRow.FlexBox)

	// Teacher info
	if len(pc.Course.Teachers) > 0 {
		teacherNames := strings.Join(pc.Course.Teachers, "、")
		body.AddInfoRow("👨‍🏫", "授課教師", teacherNames, lineutil.DefaultInfoRowStyle())
	}

	// Time info
	if len(pc.Course.Times) > 0 {
		formattedTimes := lineutil.FormatCourseTimes(pc.Course.Times)
		timeStr := strings.Join(formattedTimes, "、")
		body.AddInfoRow("⏰", "上課時間", timeStr, lineutil.DefaultInfoRowStyle())
	}

	// Location info
	if len(pc.Course.Locations) > 0 {
		locationStr := strings.Join(pc.Course.Locations, "、")
		body.AddInfoRow("📍", "上課地點", locationStr, lineutil.DefaultInfoRowStyle())
	}

	// Footer: View course detail button
	viewDetailBtn := lineutil.NewFlexButton(
		lineutil.NewPostbackActionWithDisplayText(
			"📄 查看詳細",
			lineutil.TruncateRunes(fmt.Sprintf("查詢課程 %s", pc.Course.UID), 40),
			"course:"+pc.Course.UID,
		),
	).WithStyle("primary").WithColor(headerColor).WithHeight("sm")

	footer := lineutil.NewButtonFooter([]*lineutil.FlexButton{viewDetailBtn})

	return lineutil.NewFlexBubble(header, nil, body.Build(), footer)
}
