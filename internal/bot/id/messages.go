package id

// Message constants for ID module
// Centralized management of user-facing messages
const (
	// Year validation messages
	MsgYearTooEarly = "🏫 學校都還沒蓋好啦\n\n臺北大學於民國 89 年成立\n請輸入 90 學年度以後的年份"
	MsgYearTooLate  = "🔮 你是未來人嗎？\n\n請輸入有效的學年度"
	MsgYearNoData   = "📒 數位學苑還沒出生喔\n\n請輸入 95 學年度以後的年份"
	MsgYear113Plus  = "💔 數位學苑 2.0 已停止使用，無法取得資料"

	// Department messages
	MsgDeptNotFound     = "🔍 查無此系所\n\n請檢查系所代碼是否正確"
	MsgDeptCodeInvalid  = "❌ 系所代碼格式錯誤\n\n系所代碼應為 1-3 位數字"
	MsgSelectCollege    = "🏛️ 請選擇學院群"
	MsgSelectDepartment = "🎓 請選擇科系"

	// Student search messages
	MsgStudentNotFound  = "🔍 查無此學生\n\n請確認學號或姓名是否正確"
	MsgNoStudentsInDept = "🤔 %d 學年度%s%s好像沒有人耶"
	MsgSearchLimit      = "⚠️ 搜尋結果過多\n\n已顯示前 %d 筆結果\n建議使用更精確的搜尋條件"

	// Help messages
	MsgIDHelp = "🎓 學號查詢說明\n\n" +
		"支援以下查詢方式：\n" +
		"1️⃣ 直接輸入學號（8-9位）\n" +
		"2️⃣ 姓名搜尋：學生 王小明\n" +
		"3️⃣ 年度查詢：學年 112\n" +
		"4️⃣ 系所代碼：系代碼 85\n\n" +
		"💡 提示：僅提供 101-112 學年度資料"

	// Error messages
	MsgErrorGeneric = "❌ 系統錯誤\n\n請稍後再試或聯絡管理員"
	MsgErrorTimeout = "⏱️ 查詢逾時\n\n請稍後再試"
	MsgErrorScrape  = "🌐 資料來源暫時無法存取\n\n請稍後再試"

	// Input validation messages
	MsgInvalidYear      = "📅 請輸入正確的學年度\n\n例如：學年 112"
	MsgInvalidStudentID = "🔢 學號格式不正確\n\n學號應為 8-9 位數字"
	MsgInvalidInput     = "❓ 輸入格式不正確\n\n輸入「使用說明」查看正確格式"

	// Image URLs
	ImageRIP113 = "https://raw.githubusercontent.com/garyellow/ntpu-linebot/main/assets/rip.png"
)
