package storage

// Student represents a student record
type Student struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Year       int    `json:"year"`
	Department string `json:"department"`
	CachedAt   int64  `json:"cached_at"`
}

// StudentSearchResult represents the result of a student search with total count
type StudentSearchResult struct {
	Students   []Student // Limited results (up to 400)
	TotalCount int       // Total number of matches (may exceed 400)
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

// ProgramRequirement represents a course's requirement for an academic program (學程).
// A course can belong to multiple programs with different requirement types.
type ProgramRequirement struct {
	ProgramName string `json:"program_name"` // Program name (e.g., "智慧財產權學士學分學程")
	CourseType  string `json:"course_type"`  // Requirement type: "必" (required), "選" (elective), etc.
}

// Course represents a course record
type Course struct {
	UID         string               `json:"uid"`
	Year        int                  `json:"year"`
	Term        int                  `json:"term"`
	No          string               `json:"no"`
	Title       string               `json:"title"`
	Teachers    []string             `json:"teachers"`
	TeacherURLs []string             `json:"teacher_urls,omitzero"`
	Times       []string             `json:"times"`
	Locations   []string             `json:"locations"`
	DetailURL   string               `json:"detail_url,omitzero"`
	Note        string               `json:"note,omitzero"`
	Programs    []ProgramRequirement `json:"programs,omitzero"` // Academic programs this course belongs to
	CachedAt    int64                `json:"cached_at"`
}

// Program represents an academic program (學程) with course statistics.
// Used for displaying program list with course counts.
type Program struct {
	Name          string `json:"name"`           // Program name (e.g., "智慧財產權學士學分學程")
	RequiredCount int    `json:"required_count"` // Number of required courses
	ElectiveCount int    `json:"elective_count"` // Number of elective courses
	TotalCount    int    `json:"total_count"`    // Total number of courses
}

// ProgramCourse represents a course within a program with its requirement type.
// Used when listing courses for a specific program.
type ProgramCourse struct {
	Course     Course `json:"course"`      // Full course information
	CourseType string `json:"course_type"` // Requirement type for this program: "必", "選", etc.
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
// Syllabus represents a course syllabus record for BM25 smart search.
// All content fields store unified CN+EN text extracted from NTPU course pages.
// Used by internal/rag for building search index.
type Syllabus struct {
	UID         string   `json:"uid"`          // Course unique identifier (e.g., "1132U3009")
	Year        int      `json:"year"`         // Academic year
	Term        int      `json:"term"`         // Semester (1 or 2)
	Title       string   `json:"title"`        // Course title
	Teachers    []string `json:"teachers"`     // Course instructors
	Objectives  string   `json:"objectives"`   // Teaching objectives (教學目標)
	Outline     string   `json:"outline"`      // Course outline (內容綱要)
	Schedule    string   `json:"schedule"`     // Weekly schedule (教學預定進度)
	ContentHash string   `json:"content_hash"` // SHA256 hash for change detection
	CachedAt    int64    `json:"cached_at"`    // Unix timestamp when cached
}
