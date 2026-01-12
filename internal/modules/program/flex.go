package program

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/data"
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
	// 色系設計：碩士類偏紫色系、學士類偏藍色系，形成清晰漸層
	// Emoji 設計：🎓 學分學程、📚 跨域微學程、📌 單一領域微學程
	ColorCategoryMasterCredit   = lineutil.ColorHeaderProgramMasterCredit   // 🎓 碩士學分學程
	ColorCategoryMixedCredit    = lineutil.ColorHeaderProgramMixedCredit    // 🎓 學士暨碩士學分學程
	ColorCategoryBachelorCredit = lineutil.ColorHeaderProgramBachelorCredit // 🎓 學士學分學程
	ColorCategoryMasterCross    = lineutil.ColorHeaderProgramMasterCross    // 📚 碩士跨域微學程
	ColorCategoryMixedCross     = lineutil.ColorHeaderProgramMixedCross     // 📚 學士暨碩士跨域微學程
	ColorCategoryBachelorCross  = lineutil.ColorHeaderProgramBachelorCross  // 📚 學士跨域微學程
	ColorCategoryMasterSingle   = lineutil.ColorHeaderProgramMasterSingle   // 📌 碩士單一領域微學程
	ColorCategoryMixedSingle    = lineutil.ColorHeaderProgramMixedSingle    // 📌 學士暨碩士單一領域微學程
	ColorCategoryBachelorSingle = lineutil.ColorHeaderProgramBachelorSingle // 📌 學士單一領域微學程
	ColorCategoryDefault        = lineutil.ColorHeaderProgramDefault        // 🎓 學程 (fallback)
)

// getCategoryLabel returns a BodyLabelInfo based on the program category.
// Maps program categories to appropriate emoji, label text, and color.
// Categories (from LMS folders):
//   - "碩士學分學程" - Master's credit program
//   - "學士學分學程" - Bachelor's credit program
//   - "學士暨碩士學分學程" - Joint bachelor/master credit program
//   - "碩士跨域微學程" - Master's cross-domain micro-program
//   - "學士跨域微學程" - Bachelor's cross-domain micro-program
//   - "學士暨碩士跨域微學程" - Joint cross-domain micro-program
//   - "碩士單一領域微學程" - Master's single-domain micro-program
//   - "學士單一領域微學程" - Bachelor's single-domain micro-program
//   - "學士暨碩士單一領域微學程" - Joint bachelor/master single-domain micro-program
//
// Design rationale:
//   - Color gradient: 碩士 (purple/violet) → 混合 (indigo/blue) → 學士 (blue/cyan)
//   - Each program type (學分/跨域/單一領域) has its own gradient for visual hierarchy
//   - 跨域微學程 uses 📚 emoji (cross-connection)
//   - 單一領域微學程 uses 📌 emoji (focused, specialized)
func getCategoryLabel(category string) lineutil.BodyLabelInfo {
	switch category {
	case "碩士學分學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🎓",
			Label: "碩士學分學程",
			Color: ColorCategoryMasterCredit,
		}
	case "學士暨碩士學分學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🎓",
			Label: "學士暨碩士學分學程",
			Color: ColorCategoryMixedCredit,
		}
	case "學士學分學程":
		return lineutil.BodyLabelInfo{
			Emoji: "🎓",
			Label: "學士學分學程",
			Color: ColorCategoryBachelorCredit,
		}
	case "碩士跨域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📚",
			Label: "碩士跨域微學程",
			Color: ColorCategoryMasterCross,
		}
	case "學士暨碩士跨域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📚",
			Label: "學士暨碩士跨域微學程",
			Color: ColorCategoryMixedCross,
		}
	case "學士跨域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📚",
			Label: "學士跨域微學程",
			Color: ColorCategoryBachelorCross,
		}
	case "碩士單一領域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📌",
			Label: "碩士單一領域微學程",
			Color: ColorCategoryMasterSingle,
		}
	case "學士暨碩士單一領域微學程":
		return lineutil.BodyLabelInfo{
			Emoji: "📌",
			Label: "學士暨碩士單一領域微學程",
			Color: ColorCategoryMixedSingle,
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
// Uses text-based display to handle large lists.
// Consolidates all programs into a single message if possible (limit 5000 chars).
func (h *Handler) formatProgramListResponse(programs []storage.Program, headerTitle, footerText string) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)
	var messages []messaging_api.MessageInterface

	// Track rune count of sb (LINE limit is 5000 characters)
	sbRunes := 0
	var sb strings.Builder

	// Track items in current message for batching
	itemsInCurrentMsg := 0

	// Use provided title (allows differentiation between "學程列表" and "Search Results")
	sb.WriteString(headerTitle + "\n")
	sbRunes += utf8.RuneCountInString(headerTitle + "\n")

	separator := "━━━━━━━━━━━━━━━━\n\n"
	sb.WriteString(separator)
	sbRunes += utf8.RuneCountInString(separator)

	for i, prog := range programs {
		// Global index
		idx := i + 1

		// Proposed entry
		var entry strings.Builder
		// Add newline before each program except the first one in the batch
		if itemsInCurrentMsg > 0 {
			entry.WriteString("\n")
		}

		fmt.Fprintf(&entry, "%d. %s", idx, prog.Name)

		// Show course counts if available
		if prog.RequiredCount > 0 || prog.ElectiveCount > 0 {
			entry.WriteString(fmt.Sprintf(" | 必修 %d 門 · 選修 %d 門", prog.RequiredCount, prog.ElectiveCount))
		}
		entry.WriteString("\n")

		// Add URL if available (LINE will auto-link), remove https:// prefix to save chars
		if prog.URL != "" {
			entry.WriteString(fmt.Sprintf("📎 %s\n", strings.TrimPrefix(prog.URL, "https://")))
		}

		entryStr := entry.String()
		entryRunes := utf8.RuneCountInString(entryStr)

		// Check limits:
		// 1. Character limit (TextListSafeBuffer buffer / MaxTextMessageLength max)
		// 2. Batch size limit (TextListBatchSize items/message)
		if sbRunes+entryRunes > lineutil.TextListSafeBuffer || itemsInCurrentMsg >= TextListBatchSize {
			// Remove trailing newline from the last item of the current batch if present
			currentContent := strings.TrimSuffix(sb.String(), "\n")

			// Finalize current message
			messages = append(messages, lineutil.NewTextMessageWithConsistentSender(currentContent, sender))
			sb.Reset()
			sbRunes = 0
			itemsInCurrentMsg = 0

			// Continuation: no separator or header, just continue the list
			// For the first item of a new batch, we don't want the leading newline
			if strings.HasPrefix(entryStr, "\n") {
				entryStr = strings.TrimPrefix(entryStr, "\n")
				entryRunes = utf8.RuneCountInString(entryStr)
			}
		}

		sb.WriteString(entryStr)
		sbRunes += entryRunes
		itemsInCurrentMsg++
	}

	// Remove trailing newline from the last item of the final batch
	finalContent := strings.TrimSuffix(sb.String(), "\n")

	// Add footer
	// Ensure there is a newline before separator if content doesn't end with one (it shouldn't now)
	sb.Reset()
	sb.WriteString(finalContent)
	sb.WriteString("\n━━━━━━━━━━━━━━━━\n")
	// Use provided footer text (allows different hints for list vs search)
	sb.WriteString(footerText)

	// Note: Removed update time display as requested

	// Create the final (or only) message
	msg := lineutil.NewTextMessageWithConsistentSender(sb.String(), sender)
	msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
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
		lineutil.AddQuickReplyToMessages(messages, lineutil.QuickReplyProgramNav()...)
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
//	│ ⚠️ 請參閱學程網頁...      │  <- Warning (if 0 courses)
//	├──────────────────────────┤
//	│ [📋 學程資訊]            │  <- Footer button (row 1)
//	├──────────────────────────┤
//	│ [📚 查看課程]            │  <- Footer button (row 2, only if >0 courses)
//	└──────────────────────────┘
func (h *Handler) buildProgramBubble(program storage.Program) *lineutil.FlexBubble {
	// Get category label info (emoji, label, color based on category)
	labelInfo := getCategoryLabel(program.Category)

	// Header: Program name with category-based colored background
	// WARNING: Do NOT truncate program name here. It's the final info page, user needs full name.
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: program.Name,
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

	// 0 courses warning
	if totalCourses == 0 {
		// Only show warning if there really are 0 courses (both required and elective)
		// Enable wrapping for this warning message to prevent truncation
		warningStyle := lineutil.DefaultInfoRowStyle()
		warningStyle.Wrap = true
		body.AddInfoRow("⚠️", "注意", "近 2 學期無課程資料，請點擊「學程資訊」至網頁確認", warningStyle)
	}

	// Build footer buttons - using rows for vertical stacking
	var footerRows []*lineutil.FlexButton

	// Row 1: Add LMS detail page button
	// Uses program-specific URL if available, otherwise falls back to main program list page
	programURL := program.URL
	if programURL == "" {
		programURL = data.LMSBaseURL // Fallback to main program list page
	}
	detailBtn := lineutil.NewFlexButton(
		lineutil.NewURIAction("📋 學程資訊", programURL),
	).WithStyle("secondary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm")
	footerRows = append(footerRows, detailBtn)

	// Row 2: View courses button (internal) - only if courses exist
	// Stacked vertically: distinct row
	if totalCourses > 0 {
		viewCoursesBtn := lineutil.NewFlexButton(
			lineutil.NewPostbackActionWithDisplayText(
				"📚 "+PostbackViewCoursesLabel,
				lineutil.FormatLabel("查看課程", program.Name, 40),
				PostbackPrefix+"courses"+bot.PostbackSplitChar+program.Name,
			),
		).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm")
		footerRows = append(footerRows, viewCoursesBtn)
	}

	// Pass as slice of slices to NewButtonFooter
	// Each element in footerRows becomes a separate row with one button
	rows := make([][]*lineutil.FlexButton, 0, len(footerRows))
	for _, btn := range footerRows {
		rows = append(rows, []*lineutil.FlexButton{btn})
	}

	footer := lineutil.NewButtonFooter(rows...)

	return lineutil.NewFlexBubble(header, nil, body.Build(), footer)
}

// formatProgramCoursesResponse formats program courses as carousel Flex Messages.
// Required courses are displayed first, followed by elective courses.
// originalRequiredCount and originalElectiveCount are the actual counts before any truncation.
func (h *Handler) formatProgramCoursesResponse(programName string, requiredCourses, electiveCourses []storage.ProgramCourse, originalRequiredCount, originalElectiveCount int) []messaging_api.MessageInterface {
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
		msg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
		return []messaging_api.MessageInterface{msg}
	}

	// Add header message with program info (show original counts, no "必修優先" text)
	// Include disclaimer about referring to official program info page
	headerMsg := lineutil.NewTextMessageWithConsistentSender(
		fmt.Sprintf("🎓 %s\n\n⚠️ 請同步參閱學程資訊頁面，各課程及其必選修別以學程科目規劃表所列為準\n\n📊 課程統計\n• 必修：%d 門\n• 選修：%d 門\n• 共計：%d 門\n\n⬇️ 以下為課程列表",
			programName,
			originalRequiredCount,
			originalElectiveCount,
			originalRequiredCount+originalElectiveCount),
		sender,
	)

	// Build carousel messages
	carouselMessages := lineutil.BuildCarouselMessages(
		lineutil.FormatLabel("課程列表", programName, 400),
		bubbles,
		sender,
	)

	messages := append([]messaging_api.MessageInterface{headerMsg}, carouselMessages...)

	// Add quick reply to last message
	if len(messages) > 0 {
		lineutil.AddQuickReplyToMessages(messages, lineutil.QuickReplyProgramNav()...)
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
//	│     [詳細資訊]           │  <- Footer button (external to course)
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
	// WARNING: Do NOT truncate course title here.
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: lineutil.FormatCourseTitleWithUID(pc.Course.Title, pc.Course.UID),
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

	// Teacher info - use multi-line style for better readability
	if len(pc.Course.Teachers) > 0 {
		teacherNames := strings.Join(pc.Course.Teachers, "、")
		body.AddInfoRow("👨‍🏫", "授課教師", teacherNames, lineutil.CarouselInfoRowStyleMultiLine())
	}

	// Time info - use multi-line style for better readability
	if len(pc.Course.Times) > 0 {
		formattedTimes := lineutil.FormatCourseTimes(pc.Course.Times)
		timeStr := strings.Join(formattedTimes, "、")
		body.AddInfoRow("⏰", "上課時間", timeStr, lineutil.CarouselInfoRowStyleMultiLine())
	}

	// Note: Location info is omitted for program course bubbles to keep display compact
	// Users can view full details by clicking "詳細資訊"

	// Footer: View course detail button (display course title, not UID)
	viewDetailBtn := lineutil.NewFlexButton(
		lineutil.NewPostbackActionWithDisplayText(
			"📄 詳細資訊",
			lineutil.FormatLabel("查詢課程", pc.Course.Title, 40),
			"course:"+pc.Course.UID,
		),
	).WithStyle("primary").WithColor(headerColor).WithHeight("sm")

	footer := lineutil.NewButtonFooter([]*lineutil.FlexButton{viewDetailBtn})

	return lineutil.NewFlexBubble(header, nil, body.Build(), footer)
}

// formatProgramCoursesAsTextList formats program courses as text messages when count exceeds carousel limit.
// Each course is displayed as: {序號}. {課程編號} {課程名}
func (h *Handler) formatProgramCoursesAsTextList(programName string, requiredCourses, electiveCourses []storage.ProgramCourse, originalRequiredCount, originalElectiveCount int) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)
	var messages []messaging_api.MessageInterface

	// Header message with program info and disclaimer
	headerMsg := lineutil.NewTextMessageWithConsistentSender(
		fmt.Sprintf("🎓 %s\n\n⚠️ 請同步參閱學程資訊頁面，各課程及其必選修別以學程科目規劃表所列為準\n\n📊 課程統計\n• 必修：%d 門\n• 選修：%d 門\n• 共計：%d 門\n\n⬇️ 以下為課程列表",
			programName,
			originalRequiredCount,
			originalElectiveCount,
			originalRequiredCount+originalElectiveCount),
		sender,
	)
	messages = append(messages, headerMsg)

	// Build course list text
	var sb strings.Builder
	idx := 0

	// Add required courses
	if len(requiredCourses) > 0 {
		sb.WriteString("【必修課程】\n")
		for _, pc := range requiredCourses {
			idx++
			fmt.Fprintf(&sb, "%d. %s %s\n", idx, pc.Course.UID, pc.Course.Title)
		}
	}

	// Add elective courses
	if len(electiveCourses) > 0 {
		if len(requiredCourses) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("【選修課程】\n")
		for _, pc := range electiveCourses {
			idx++
			fmt.Fprintf(&sb, "%d. %s %s\n", idx, pc.Course.UID, pc.Course.Title)
		}
	}

	// Split into multiple messages if needed
	content := sb.String()
	if utf8.RuneCountInString(content) <= lineutil.TextListSafeBuffer {
		listMsg := lineutil.NewTextMessageWithConsistentSender(content, sender)
		messages = append(messages, listMsg)
	} else {
		// Split by lines
		lines := strings.Split(content, "\n")
		var currentSb strings.Builder
		currentRunes := 0

		for _, line := range lines {
			lineWithNewline := line + "\n"
			lineRunes := utf8.RuneCountInString(lineWithNewline)

			if currentRunes+lineRunes > lineutil.TextListSafeBuffer {
				// Finalize current message
				messages = append(messages, lineutil.NewTextMessageWithConsistentSender(currentSb.String(), sender))
				currentSb.Reset()
				currentRunes = 0
			}

			currentSb.WriteString(lineWithNewline)
			currentRunes += lineRunes
		}

		// Add remaining content
		if currentSb.Len() > 0 {
			messages = append(messages, lineutil.NewTextMessageWithConsistentSender(currentSb.String(), sender))
		}
	}

	// Add footer message with search hint
	footerMsg := lineutil.NewTextMessageWithConsistentSender(
		"💡 可利用課程編號或課程名稱查詢相關課程詳細資訊",
		sender,
	)
	footerMsg.QuickReply = lineutil.NewQuickReply(lineutil.QuickReplyProgramNav())
	messages = append(messages, footerMsg)

	return messages
}

// buildNoProgramsFoundBubble creates a Flex bubble for when no programs are found for a course.
// Provides a link to the LMS program listing page for manual lookup.
//
// Layout (Colored Header pattern):
//
//	┌──────────────────────────┐
//	│   📭 查無相關學程        │  <- Colored Header (warning amber)
//	├──────────────────────────┤
//	│ ⚠️ 注意                   │  <- Body label
//	│ 課程：XX                 │
//	│ 目前沒有相關學程資料      │
//	│ 💡 可至學程列表查詢       │
//	├──────────────────────────┤
//	│ [📋 學程列表]            │  <- Footer button (link to LMS)
//	└──────────────────────────┘
func (h *Handler) buildNoProgramsFoundBubble(courseName string) *lineutil.FlexBubble {
	// Header: Warning style with amber color
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: "📭 查無相關學程",
		Color: lineutil.ColorWarning, // Amber for warning
	})

	// Build body contents
	body := lineutil.NewBodyContentBuilder()

	// Body label: warning tag
	body.AddComponent(lineutil.NewBodyLabel(lineutil.BodyLabelInfo{
		Emoji: "⚠️",
		Label: "注意",
		Color: lineutil.ColorWarning,
	}).FlexBox)

	// Course name info
	body.AddInfoRow("📚", "課程", courseName, lineutil.DefaultInfoRowStyle())

	// Message with wrapping
	msgStyle := lineutil.DefaultInfoRowStyle()
	msgStyle.Wrap = true
	body.AddInfoRow("📝", "說明", "目前沒有相關學程資料，可能是因為該課程尚未被任何學程認列", msgStyle)

	// Hint
	body.AddInfoRow("💡", "提示", "可至學程列表頁面查詢最新學程資訊", msgStyle)

	// Footer: Link to LMS program list
	detailBtn := lineutil.NewFlexButton(
		lineutil.NewURIAction("📋 學程列表", data.LMSBaseURL),
	).WithStyle("primary").WithColor(lineutil.ColorButtonExternal).WithHeight("sm")

	footer := lineutil.NewButtonFooter([]*lineutil.FlexButton{detailBtn})

	return lineutil.NewFlexBubble(header, nil, body.Build(), footer)
}
