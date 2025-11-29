package storage

import "errors"

// Common errors
var (
	// ErrNotFound is returned when a resource is not found in the database
	ErrNotFound = errors.New("resource not found")
)

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
	NameEn       string `json:"name_en,omitempty"` // English name (if different from Chinese)
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

// Syllabus represents a course syllabus record for semantic search
type Syllabus struct {
	UID         string   `json:"uid"`          // Course unique identifier
	Year        int      `json:"year"`         // Academic year
	Term        int      `json:"term"`         // Semester (1 or 2)
	Title       string   `json:"title"`        // Course title
	Teachers    []string `json:"teachers"`     // Course instructors
	Content     string   `json:"content"`      // Merged syllabus content for embedding
	ContentHash string   `json:"content_hash"` // SHA256 hash for change detection
	CachedAt    int64    `json:"cached_at"`    // Unix timestamp when cached
}
