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
	Organization string `json:"organization,omitempty"`
	Title        string `json:"title,omitempty"`
	Extension    string `json:"extension,omitempty"`
	Email        string `json:"email,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Website      string `json:"website,omitempty"`
	Location     string `json:"location,omitempty"`
	Superior     string `json:"superior,omitempty"`
	CachedAt     int64  `json:"cached_at"`
}

// Course represents a course record
// Matches Python version with all fields from Course class
type Course struct {
	UID         string   `json:"uid"`
	Year        int      `json:"year"`
	Term        int      `json:"term"`
	No          string   `json:"no"`
	Title       string   `json:"title"`
	Teachers    []string `json:"teachers"`
	TeacherURLs []string `json:"teacher_urls,omitempty"` // Teacher course table URLs
	Times       []string `json:"times"`
	Locations   []string `json:"locations"`
	DetailURL   string   `json:"detail_url,omitempty"`
	Note        string   `json:"note,omitempty"`
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
