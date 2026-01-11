// Package config provides data availability and limitation constants.
// Defines data boundaries and user-facing messages for explaining data limitations.
//
// NTPU LMS (數位學苑 2.0) deprecated in 2024:
//   - Student ID data: 94-112 (complete data), 113 (sparse, incomplete)
//   - Cached warmup: 101-112 for all student types (undergrad/master's/PhD)
//   - Year 113: Only students who manually created LMS 2.0 accounts
//   - Year 114+: No data available (LMS 3.0 only)
package config

// ================================================
// Student ID Data Availability Constants
// ================================================

const (
	// IDDataYearStart is the earliest academic year included in cache warmup (101 = 2012).
	// LMS has data from year 90, but warmup only fetches 101+ for efficiency.
	IDDataYearStart = 101

	// IDDataYearEnd is the latest academic year with complete student data (112 = 2023).
	// Year 113 has sparse, incomplete data (only manual LMS 2.0 account creations).
	IDDataYearEnd = 112

	// IDDataCutoffYear is the first year with incomplete/no data (113 = 2024, LMS 2.0 deprecation started).
	// Year 113: Sparse data (only students who manually created accounts)
	// Year 114+: No data (LMS 3.0 only)
	IDDataCutoffYear = 113

	// LMSLaunchYear is the earliest year with complete data in LMS (94 = 2005).
	LMSLaunchYear = 94

	// NTPUFoundedYear is when NTPU was established (89 = 2000).
	// Note: Used for ID module validation only (before LMS existed).
	NTPUFoundedYear = 89

	// CourseSystemLaunchYear is the earliest year with course data available (90 = 2001).
	// Course query system started from year 90.
	CourseSystemLaunchYear = 90
)

// ================================================
// User-facing Messages for LMS 2.0 Deprecation
// ================================================
//
// All messages use consistent terminology:
//   - 「數位學苑 2.0 已於 113 學年度起停用」 (113 年資料不完整，114 年起完全無資料)
//   - Data ranges: 學年度查詢 94-112 (完整), 113 (不完整); 姓名查詢 101-112 (完整), 113 (不完整)
//
// Message structure: Emoji + Clear statement + Brief explanation + Actionable alternatives.
const (
	// IDLMSDeprecatedMessage is the core message for LMS 2.0 deprecation.
	// Used for year-based queries (學年 114+) and student ID queries (學號 414xxxxxx+).
	IDLMSDeprecatedMessage = "😢 數位學苑 2.0 已於 113 學年度起停用\n\n" +
		"113 學年度起新生使用數位學苑 3.0，僅少數學生有建立數位學苑 2.0 帳號。\n\n" +
		"📅 完整資料範圍：\n" +
		"• 學年度/學號查詢：94-112 學年度\n" +
		"• 姓名查詢：101-112 學年度\n\n" +
		"⚠️ 113 學年度資料不完整"

	// IDNotFoundWithCutoffHint is the message when student name is not found,
	// with a hint about data availability.
	IDNotFoundWithCutoffHint = "🔍 查無「%s」的學號資料\n\n" +
		"📊 姓名查詢範圍\n" +
		"• 學士班/碩博士班：101-112 學年度（完整）\n" +
		"• 113 學年度資料不完整（僅極少數學生）\n" +
		"• 114 學年度起無資料（數位學苑 2.0 停用）\n\n" +
		"💡 建議：\n" +
		"• 確認姓名拼寫是否正確\n" +
		"• 使用「學年」功能按年度查詢"

	// ID113YearEmptyMessage is shown when year 113 query returns no results.
	// Explains why data is missing without using deprecated RIP image.
	ID113YearEmptyMessage = "🔍 查無 113 學年度「%s」的學生資料\n\n" +
		"⚠️ 113 學年度資料不完整\n" +
		"僅極少數手動建立數位學苑 2.0 帳號的學生有資料。\n\n" +
		"📅 完整資料範圍：94-112 學年度"

	// IDYearTooOldMessage is the message for years before LMS has complete data (90-93).
	// Friendly message with historical context.
	IDYearTooOldMessage = "📚 這個年份的資料不完整喔\n\n" +
		"數位學苑資料從民國 94 年起較完整，\n" +
		"請輸入 94-112 學年度的年份。"

	// IDYearBeforeNTPUMessage is the message for years before NTPU existed (< 89).
	IDYearBeforeNTPUMessage = "🏫 學校都還沒蓋好啦\n\n" +
		"臺北大學於民國 89 年成立。"

	// IDYearFutureMessage is the message for future years (> current year).
	IDYearFutureMessage = "🔮 哎呀～你是未來人嗎？"
)
