// Package data provides static data definitions for the application.
// These data are maintained manually and updated periodically.
package data

// ProgramInfo contains static information about an academic program (學程).
// Names must exactly match those in the course system (選課系統) for correct matching.
type ProgramInfo struct {
	Name string // 學程名稱 (e.g., "智慧財產權學士學分學程")
	URL  string // LMS 詳細頁面 URL
}

// LMSBaseURL is the base URL for LMS program pages.
const LMSBaseURL = "https://lms.ntpu.edu.tw/board.php?courseID=28286"

// AllPrograms contains all program definitions with their LMS URLs.
// Names must exactly match those in the course system (選課系統).
// Source: https://lms.ntpu.edu.tw/board.php?courseID=28286
//
// Categories:
// - 碩士學分學程 (folderID=115531)
// - 學士學分學程 (folderID=115532)
// - 學士暨碩士學分學程 (folderID=115533)
// - 碩士跨域微學程 (folderID=198807)
// - 學士跨域微學程 (folderID=198808)
// - 學士暨碩士跨域微學程 (folderID=198809)
// - 碩士單一領域微學程 (folderID=198811)
// - 學士單一領域微學程 (folderID=198812)
var AllPrograms = []ProgramInfo{
	// ============================================
	// 碩士學分學程 (folderID=115531)
	// ============================================
	{"企業併購碩士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906250"},
	{"英語授課商學碩士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906246"},
	{"財務金融碩士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906243"},
	{"電子商務碩士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906237"},

	// ============================================
	// 學士學分學程 (folderID=115532) - 第 1 頁
	// ============================================
	{"不動產估價師學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3858905"},
	{"資訊安全與應用學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752477"},
	{"永續發展與治理英語學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752475"},
	{"環境、社會與企業治理學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3628899"},
	{"數位多媒體學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3489202"},
	{"晶片系統學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3489201"},
	{"消費者保護與消費行為學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3489200"},
	{"商業資料分析學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3156363"},
	{"全球化公共事務英語學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396119"},
	{"文官人才學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396069"},
	{"法律暨企業社會責任學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396043"},
	{"人工智慧英語授課學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396014"},
	{"社會創新與政策研究英語學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2395993"},
	{"美學、文化與藝術學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221516"},
	{"人文與資訊學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221515"},
	{"基礎法學與社會學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221514"},
	{"商業智慧與大數據分析學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221512"},
	{"傳播學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037354"},
	{"公共事務數據分析學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037350"},
	{"ESP專業英文學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037341"},

	// 學士學分學程 - 第 2 頁
	{"A-EGP進階英文學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037336"},
	{"職涯探索及創新創業學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899302"},
	{"性別與人權學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899300"},
	{"調查方法與資料分析學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899299"},
	{"人工智慧學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1741586"},
	{"文化與觀光學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1741580"},
	{"金融科技與量化金融學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1458778"},
	{"賽會活動籌組專業人力就業學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1335140"},
	{"企業永續發展學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1335136"},
	{"智慧財產權學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1198674"},
	{"文化與溝通外語授課學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1198668"},
	{"全球經濟分析與領袖人才英語學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=959469"},
	{"資料拓析學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=945373"},
	{"通訊系統實作學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906271"},
	{"嵌入式系統學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906270"},
	{"行動裝置軟體學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906269"},
	{"國際法暨外交事務學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906266"},
	{"文創產業管理行銷學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906265"},

	// 學士學分學程 - 第 3 頁
	{"會計師事務所專業實務養成學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906260"},
	{"創新創業學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906259"},
	{"資本市場鑑識學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906258"},
	{"英語授課商學學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906257"},
	{"財務金融學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906256"},
	{"電子商務學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906255"},
	{"公部門與公民社會人力資源發展學士學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906254"},

	// ============================================
	// 學士暨碩士學分學程 (folderID=115533)
	// ============================================
	{"人工智慧視覺技術學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939269"},
	{"人工智慧自然語言技術學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939268"},
	{"人工智慧工業應用學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939267"},
	{"人工智慧探索應用學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939266"},
	{"都市更新學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939263"},
	{"海山人文與創意實踐學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3937967"},
	{"高齡社區照顧學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752476"},
	{"歷史實踐與文化資產學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2870089"},
	{"勞動法與人力資源管理學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221513"},
	{"跨國法律實務英語學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037362"},
	{"國際經貿談判與訴訟人才學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1741595"},
	{"日本研究學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1335145"},
	{"家事法與社會工作學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1198671"},
	{"氣候變遷與淨零策略學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906273"},
	{"醫療科技暨法律學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906267"},
	{"華語文教學學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906261"},
	{"租稅行政救濟學分學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=906247"},

	// ============================================
	// 碩士跨域微學程 (folderID=198807)
	// ============================================
	{"投資管理碩士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899259"},

	// ============================================
	// 學士跨域微學程 (folderID=198808) - 第 1 頁
	// ============================================
	{"人工智慧英語授課學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939366"},
	{"商業人工智慧學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3938003"},
	{"智慧永續發展與管理跨領域英語學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3858910"},
	{"智慧運算系統英語授課學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3858908"},
	{"資訊安全與應用學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752483"},
	{"斯拉夫語言文化學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752482"},
	{"行銷管理英語學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752481"},
	{"學術英文(EAP)/專業學術英文(ESAP)學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752479"},
	{"社會創新與政策研究英語學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3628889"},
	{"經濟資料科學學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3489216"},
	{"環境、社會與企業治理學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3489213"},
	{"永續發展與治理英語學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3489211"},
	{"全球化公共事務英語學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3489209"},
	{"貨幣與金融市場學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3156361"},
	{"公共部門經濟學學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3156340"},
	{"法政策學—永續發展學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2591745"},
	{"法政策學—憲政法制學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2591744"},
	{"法律暨企業社會責任學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2591739"},
	{"跨文化與國際移動學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396162"},
	{"文官人才學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396160"},

	// 學士跨域微學程 - 第 2 頁
	{"智慧財產權學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396155"},
	{"地理經濟數量分析學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396152"},
	{"租稅規劃學士學分微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396146"},
	{"金融科技與量化金融學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396143"},
	{"領隊導遊人才培育學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396140"},
	{"商務溝通學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396134"},
	{"美學、文化與藝術學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221526"},
	{"人文與資訊學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221525"},
	{"基礎法學與社會學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221524"},
	{"刑事犯罪學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221522"},
	{"消費者保護與消費行為學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221521"},
	{"資料拓析學士學分微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221520"},
	{"傳播學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037496"},
	{"戶外體驗與青少年服務學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037450"},
	{"賽會活動企劃學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037439"},
	{"公共事務數據分析學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037437"},
	{"ESP專業英文學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037433"},
	{"A-EGP進階英文學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037428"},
	{"職涯探索及創新創業學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899295"},
	{"人工智慧學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899292"},

	// 學士跨域微學程 - 第 3 頁
	{"性別學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899290"},
	{"人權與正義學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899289"},
	{"文化與觀光學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899266"},

	// ============================================
	// 學士暨碩士跨域微學程 (folderID=198809)
	// ============================================
	{"醫療科技暨法律微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939368"},
	{"都市更新微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3939264"},
	{"海山人文與創意實踐微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3938004"},
	{"全球勞動法英語微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3752480"},
	{"高齡社會與法微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2591746"},
	{"歷史實踐與文化資產微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396169"},
	{"日本研究微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221527"},
	{"勞動法與人力資源管理微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221523"},
	{"社會共融與障礙微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2221519"},
	{"跨國法律實務英語微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037502"},
	{"華語文教學微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2037452"},
	{"高齡社區照顧微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899291"},
	{"氣候變遷與淨零策略微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899269"},

	// ============================================
	// 碩士單一領域微學程 (folderID=198811)
	// ============================================
	{"犯罪學碩士學分微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899395"},

	// ============================================
	// 學士單一領域微學程 (folderID=198812) - 第 1 頁
	// ============================================
	{"華語文增能學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3858917"},
	{"國際華語教學學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=3858912"},
	{"公共支出政策分析學士學分微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=2396174"},
	{"賽會活動專業人力學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899423"},
	{"休閒管理學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899422"},
	{"資料分析學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899421"},
	{"會計實務學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899420"},
	{"永續金融學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899414"},
	{"創新創業學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899413"},
	{"消費者保護學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899406"},
	{"基礎法學學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899400"},
	{"調查方法學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899393"},
	{"社會關懷學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899392"},
	{"經濟分析學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899390"},
	{"臺灣史學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899376"},
	{"現代文學學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899374"},
	{"古典文學學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899371"},
	{"中國思想學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899369"},
	{"人文與數位學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899365"},
	{"文化與溝通英語授課學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899363"},

	// 學士單一領域微學程 - 第 2 頁
	{"電路與系統學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899358"},
	{"通訊學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899356"},
	{"程式設計學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899354"},
	{"國際租稅學士學分微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899350"},
	{"政策分析與行政管理學士微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899346"},
	{"不動產與城鄉環境學士學分微學程", "https://lms.ntpu.edu.tw/board.php?courseID=28286&f=doc&cid=1899338"},
}

// programURLMap is a lookup map for O(1) URL retrieval.
// Initialized lazily on first GetProgramURL call.
var programURLMap map[string]string

// GetProgramURL returns the LMS URL for a program name.
// Returns empty string if program not found in the static list.
func GetProgramURL(name string) string {
	if programURLMap == nil {
		programURLMap = make(map[string]string, len(AllPrograms))
		for _, p := range AllPrograms {
			programURLMap[p.Name] = p.URL
		}
	}
	return programURLMap[name]
}

// GetProgramCount returns the total number of programs in the static list.
func GetProgramCount() int {
	return len(AllPrograms)
}
