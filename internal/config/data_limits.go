// Package config provides data availability and limitation constants.
// These constants define the boundaries of available data and user-facing messages
// explaining data limitations, following UX best practices.
//
// # Data Availability Context
//
// The NTPU LMS (數位學苑 2.0) was deprecated in 2024, but existing data remains accessible:
//   - Student ID data: LMS has data from year 94-113 (earlier years have sparse data)
//   - New enrollments (114+): No longer tracked in LMS 2.0
//
// Query capabilities (verified 2024-12):
//   - Year-based query (學年度查詢): 94-113 (live scraping)
//   - Name-based query (姓名查詢): 101-113 (cached data only, warmup range)
//
// These constants ensure consistent messaging across all modules about data limitations.
package config

// ================================================
// Student ID Data Availability Constants
// ================================================

const (
	// IDDataYearStart is the earliest academic year included in cache warmup.
	// Academic year 101 = 2012.
	// Note: This is a warmup optimization choice, not a hard data limit.
	// LMS actually has data from year 90, but warmup only fetches 101+ for efficiency.
	// Users can still query years 90-100 via live scraping (slower, not cached).
	IDDataYearStart = 101

	// IDDataYearEnd is the latest academic year with complete student data.
	// Academic year 113 = 2024 (verified 2024-12: LMS 2.0 still has 113 data).
	IDDataYearEnd = 113

	// IDDataCutoffYear is the first year WITHOUT available data.
	// Academic year 114 = 2025 (LMS 2.0 deprecated, no new data).
	IDDataCutoffYear = 114

	// LMSLaunchYear is the earliest year with complete data in LMS (數位學苑).
	// Academic year 94 = 2005 (verified 2024-12: earlier years have sparse/incomplete data).
	// Note: LMS has some data from year 90, but 94+ has more complete coverage.
	LMSLaunchYear = 94

	// NTPUFoundedYear is the year when NTPU was established.
	// Academic year 89 = 2000 (民國 89 年 2 月 1 日正式成立).
	// Note: year < 89 means school doesn't exist yet.
	NTPUFoundedYear = 89
)

// ================================================
// User-Facing Messages: Data Limitation Explanations
// ================================================
//
// These messages follow UX best practices:
//   1. Acknowledge the limitation clearly
//   2. Explain WHY the limitation exists
//   3. Provide actionable alternatives
//   4. Use friendly, non-technical language
//
// Message structure:
//   - Emoji + Clear statement of the problem
//   - Brief explanation of the reason
//   - Suggested next steps

const (
	// IDDataCutoffNotice is the main notice for the 114+ year cutoff.
	// Used when users query years >= 114.
	IDDataCutoffNotice = "📅 數位學苑 2.0 已於 114 學年度起停用\n\n" +
		"因此無法提供 114 學年度以後的學號資料。\n\n" +
		"💡 您可以查詢：\n" +
		"• 94-113 學年度的學生資料"

	// IDDataRangeHint is a brief hint about available data range.
	// Used in search results and error messages.
	IDDataRangeHint = "📊 資料範圍：94-113 學年度（數位學苑 2.0 資料）"

	// IDDataCutoffReason is the technical reason for data cutoff.
	// Used when more detail is needed.
	IDDataCutoffReason = "數位學苑 2.0 已於 114 學年度起停用"

	// IDNotFoundWithCutoffHint is the message when student is not found,
	// with a hint about data availability.
	IDNotFoundWithCutoffHint = "🔍 查無「%s」的學號資料\n\n" +
		"📊 資料僅涵蓋 94-113 學年度\n" +
		"（數位學苑 2.0 已停用，114+ 無資料）\n\n" +
		"💡 建議：\n" +
		"• 確認姓名拼寫是否正確\n" +
		"• 嘗試輸入完整姓名或部分姓名\n" +
		"• 如為 114 學年度以後入學，抱歉無法查詢"

	// IDYear114PlusMessage is the message shown for 114+ year queries.
	// Includes image reference and emotional acknowledgment.
	IDYear114PlusMessage = "😢 數位學苑 2.0 已於 114 學年度起停止更新\n\n" +
		"很抱歉，無法取得 114 學年度以後的學號資料。\n\n" +
		"📅 可查詢的資料範圍：\n" +
		"• 學年度查詢：94-113 學年度\n" +
		"• 姓名查詢：101-113 學年度"

	// IDYearTooOldMessage is the message for years before LMS existed.
	// Friendly message with historical context.
	IDYearTooOldMessage = "📚 這個年份的資料不完整喔\n\n" +
		"數位學苑資料從民國 94 年起較完整，\n" +
		"請輸入 94-113 學年度的年份。"

	// IDYearBeforeNTPUMessage is the message for years before NTPU existed.
	IDYearBeforeNTPUMessage = "🏫 學校都還沒蓋好啦\n\n" +
		"臺北大學於民國 89 年成立。"

	// IDYearFutureMessage is the message for future years.
	IDYearFutureMessage = "🔮 哎呀～你是未來人嗎？"
)

// ================================================
// Format Functions for Data Limitation Messages
// ================================================

// FormatIDDataRangeFooter returns a small footer text for data range info.
// This can be appended to search results to remind users of the data scope.
func FormatIDDataRangeFooter() string {
	return "\n\n📊 資料範圍：94-113 學年度"
}

// FormatIDCutoffExplanation returns a brief explanation of the cutoff.
// Used in Flex Message footers or info boxes.
func FormatIDCutoffExplanation() string {
	return "數位學苑 2.0 已停用，僅提供 94-113 學年度資料"
}
