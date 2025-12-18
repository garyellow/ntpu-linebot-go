package storage

// Student represents a student record
type Student struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Year       int    `json:"year"`
	Department string `json:"department"`
	CachedAt   int64  `json:"cached_at"`
}

// Contact represents a contact record (individual or organization)
type Contact struct {
	UID          string `json:"uid"`
	Type         string `json:"type"` // "individual" or "organization"
	Name         string `json:"name"`
	NameEn       string `json:"name_en,omitzero"`
	Organization string `json:"organization,omitzero"`
	Title        string `json:"title,omitzero"`
	Extension    string `json:"extension,omitzero"`
	Email        string `json:"email,omitzero"`
	Phone        string `json:"phone,omitzero"`
	Website      string `json:"website,omitzero"`
	Location     string `json:"location,omitzero"`
	Superior     string `json:"superior,omitzero"`
	CachedAt     int64  `json:"cached_at"`
}

// Course represents a course record
type Course struct {
	UID         string   `json:"uid"`
	Year        int      `json:"year"`
	Term        int      `json:"term"`
	No          string   `json:"no"`
	Title       string   `json:"title"`
	Teachers    []string `json:"teachers"`
	TeacherURLs []string `json:"teacher_urls,omitzero"`
	Times       []string `json:"times"`
	Locations   []string `json:"locations"`
	DetailURL   string   `json:"detail_url,omitzero"`
	Note        string   `json:"note,omitzero"`
	CachedAt    int64    `json:"cached_at"`
}

// Sticker represents a sticker URL record
type Sticker struct {
	URL          string `json:"url"`
	Source       string `json:"source"` // "spy_family", "ichigo", or "fallback"
	CachedAt     int64  `json:"cached_at"`
	SuccessCount int    `json:"success_count"`
	FailureCount int    `json:"failure_count"`
}

// Syllabus represents a course syllabus record for smart search
// Syllabus stores unified syllabus content from NTPU course detail pages
// Supports both merged format (show_info=all) and separate CN/EN formats:
//   - Merged: "教學目標 Course Objectives：" (single field with CN+EN)
//   - Separate: "教學目標：" + "Course Objectives：" (two fields merged during scraping)
//
// Each field stores merged CN+EN content for BM25 search indexing
type Syllabus struct {
	UID         string   `json:"uid"`          // Course unique identifier (e.g., "1132U3009")
	Year        int      `json:"year"`         // Academic year
	Term        int      `json:"term"`         // Semester (1 or 2)
	Title       string   `json:"title"`        // Course title
	Teachers    []string `json:"teachers"`     // Course instructors
	Objectives  string   `json:"objectives"`   // 教學目標 + Course Objectives (merged CN+EN)
	Outline     string   `json:"outline"`      // 內容綱要 + Course Outline (merged CN+EN)
	Schedule    string   `json:"schedule"`     // 教學進度 weekly schedule (教學預定進度 column only)
	ContentHash string   `json:"content_hash"` // SHA256 hash for incremental update detection
	CachedAt    int64    `json:"cached_at"`    // Unix timestamp when cached
}
