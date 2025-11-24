package contact

// Message constants for Contact module
// Centralized management of user-facing messages
const (
	// Search messages
	MsgContactNotFound = "🔍 查無符合條件的聯絡資訊\n\n建議：\n• 使用單位全名或簡稱\n• 嘗試只輸入姓氏\n• 確認是否拼寫錯誤"
	MsgMultipleResults = "🔍 找到 %d 筆符合的聯絡資訊"

	// Input prompt messages
	MsgEnterSearchTerm = "📞 請輸入查詢內容\n\n例如：\n• 聯絡 資工系\n• 電話 圖書館\n• 分機 學務處\n\n💡 也可直接輸入「緊急」查看緊急聯絡電話"

	// Emergency messages
	MsgEmergencyTitle = "🚨 緊急聯絡電話"

	// Help messages
	MsgContactHelp = "📞 通訊錄查詢說明\n\n" +
		"支援以下查詢方式：\n" +
		"1️⃣ 單位查詢：聯絡 資工系\n" +
		"2️⃣ 人員查詢：電話 王教授\n" +
		"3️⃣ 緊急電話：輸入「緊急」\n\n" +
		"💡 資料即時從校園聯絡簿抓取"

	// Error messages
	MsgErrorGeneric = "❌ 系統錯誤\n\n請稍後再試或聯絡管理員"
	MsgErrorTimeout = "⏱️ 查詢逾時\n\n請稍後再試"
	MsgErrorScrape  = "🌐 資料來源暫時無法存取\n\n請稍後再試"

	// Input validation messages
	MsgInvalidInput = "輸入格式不正確\n\n輸入「使用說明」查看正確格式"
)
