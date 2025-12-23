package program

import (
	"fmt"
	"strings"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Color constants for program module
const (
	// Program module header color (使用與課程相同的藍色系，表示學術相關)
	ColorHeaderProgram = lineutil.ColorHeaderCourse // #3B82F6 - bright blue

	// Course type colors for program courses carousel
	ColorHeaderRequired = "#059669" // 必修 - deep teal (重要、必要)
	ColorHeaderElective = "#0891B2" // 選修 - cyan (選擇、靈活)
)

// formatProgramListResponse formats a list of programs as carousel Flex Messages.
func (h *Handler) formatProgramListResponse(programs []storage.Program, totalCount int) []messaging_api.MessageInterface {
	sender := lineutil.GetSender(senderName, h.stickerManager)

	// Build carousel bubbles
	bubbles := make([]messaging_api.FlexBubble, 0, len(programs))
	for _, program := range programs {
		bubble := h.buildProgramBubble(program)
		bubbles = append(bubbles, *bubble.FlexBubble)
	}

	// Build carousel messages
	messages := lineutil.BuildCarouselMessages("學程列表", bubbles, sender)

	// Add result count message if needed
	if totalCount > MaxProgramsPerSearch {
		countMsg := lineutil.NewTextMessageWithConsistentSender(
			fmt.Sprintf("📊 找到 %d 個學程，顯示前 %d 個\n\n💡 可使用「學程 關鍵字」縮小搜尋範圍", totalCount, MaxProgramsPerSearch),
			sender,
		)
		messages = append([]messaging_api.MessageInterface{countMsg}, messages...)
	}

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
//	│      學程名稱             │  <- Colored header (blue)
//	├──────────────────────────┤
//	│ 🎓 學程資訊              │  <- Body label
//	│ 📚 課程數量：15 門       │
//	│ ✅ 必修：8 門            │
//	│ 📝 選修：7 門            │
//	├──────────────────────────┤
//	│     [查看課程]           │  <- Footer button (internal)
//	└──────────────────────────┘
func (h *Handler) buildProgramBubble(program storage.Program) *lineutil.FlexBubble {
	// Header: Program name with colored background
	header := lineutil.NewColoredHeader(lineutil.ColoredHeaderInfo{
		Title: lineutil.TruncateRunes(program.Name, MaxTitleDisplayChars),
		Color: ColorHeaderProgram,
	})

	// Build body contents
	body := lineutil.NewBodyContentBuilder()

	// Body label
	body.AddComponent(lineutil.NewBodyLabel(lineutil.BodyLabelInfo{
		Emoji: "🎓",
		Label: "學程資訊",
		Color: ColorHeaderProgram,
	}).FlexBox)

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

	// Footer: View courses button
	viewCoursesBtn := lineutil.NewFlexButton(
		lineutil.NewPostbackActionWithDisplayText(
			"📚 "+PostbackViewCoursesLabel,
			lineutil.TruncateRunes(fmt.Sprintf("查看「%s」的課程", program.Name), 40),
			PostbackPrefix+"courses"+bot.PostbackSplitChar+program.Name,
		),
	).WithStyle("primary").WithColor(lineutil.ColorButtonInternal).WithHeight("sm")

	footer := lineutil.NewButtonFooter([]*lineutil.FlexButton{viewCoursesBtn})

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
