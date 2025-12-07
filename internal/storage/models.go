package storage

import (
	domerrors "github.com/garyellow/ntpu-linebot-go/internal/errors"
)

// ErrNotFound is an alias to the domain-level error for backward compatibility.
// Prefer using domerrors.ErrNotFound directly in new code.
var ErrNotFound = domerrors.ErrNotFound

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

// Syllabus represents a course syllabus record for smart search
// Supports both separated (5 fields) and merged (3 fields) formats:
// - Separated: ObjectivesCN, ObjectivesEN, OutlineCN, OutlineEN, Schedule
// - Merged: ObjectivesCN contains merged content, *EN fields are empty
type Syllabus struct {
	UID          string   `json:"uid"`           // Course unique identifier
	Year         int      `json:"year"`          // Academic year
	Term         int      `json:"term"`          // Semester (1 or 2)
	Title        string   `json:"title"`         // Course title
	Teachers     []string `json:"teachers"`      // Course instructors
	ObjectivesCN string   `json:"objectives_cn"` // 教學目標 (Chinese)
	ObjectivesEN string   `json:"objectives_en"` // Course Objectives (English, may be empty)
	OutlineCN    string   `json:"outline_cn"`    // 內容綱要 (Chinese)
	OutlineEN    string   `json:"outline_en"`    // Course Outline (English, may be empty)
	Schedule     string   `json:"schedule"`      // 教學進度 (schedule content only)
	ContentHash  string   `json:"content_hash"`  // SHA256 hash for change detection
	CachedAt     int64    `json:"cached_at"`     // Unix timestamp when cached
}
