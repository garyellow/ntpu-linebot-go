// Package storage provides repository interfaces for data access abstraction.
// These interfaces enable dependency inversion and facilitate testing by
// decoupling bot handlers from concrete storage implementations.
package storage

import (
	"context"
)

// StudentRepository defines the interface for student data operations.
type StudentRepository interface {
	GetStudentByID(ctx context.Context, id string) (*Student, error)
	SearchStudentsByName(ctx context.Context, name string) ([]Student, error)
	GetStudentsByDepartment(ctx context.Context, dept string, year int) ([]Student, error)
	GetAllStudents(ctx context.Context) ([]Student, error)
	SaveStudent(ctx context.Context, student *Student) error
	SaveStudentsBatch(ctx context.Context, students []*Student) error
	CountStudents(ctx context.Context) (int, error)
}

// CourseRepository defines the interface for course data operations.
type CourseRepository interface {
	GetCourseByUID(ctx context.Context, uid string) (*Course, error)
	GetCoursesByYearTerm(ctx context.Context, year, term int) ([]Course, error)
	GetCoursesByRecentSemesters(ctx context.Context) ([]Course, error)
	SearchCoursesByTitle(ctx context.Context, title string) ([]Course, error)
	SearchCoursesByTeacher(ctx context.Context, teacher string) ([]Course, error)
	SaveCourse(ctx context.Context, course *Course) error
	SaveCoursesBatch(ctx context.Context, courses []*Course) error
	CountCourses(ctx context.Context) (int, error)
}

// ContactRepository defines the interface for contact data operations.
type ContactRepository interface {
	GetContactByUID(ctx context.Context, uid string) (*Contact, error)
	SearchContactsByName(ctx context.Context, name string) ([]Contact, error)
	GetContactsByOrganization(ctx context.Context, org string) ([]Contact, error)
	GetAllContacts(ctx context.Context) ([]Contact, error)
	SaveContact(ctx context.Context, contact *Contact) error
	SaveContactsBatch(ctx context.Context, contacts []*Contact) error
	CountContacts(ctx context.Context) (int, error)
}

// SyllabusRepository defines the interface for syllabus data operations.
type SyllabusRepository interface {
	GetSyllabusByUID(ctx context.Context, uid string) (*Syllabus, error)
	GetSyllabiByYearTerm(ctx context.Context, year, term int) ([]*Syllabus, error)
	GetAllSyllabi(ctx context.Context) ([]*Syllabus, error)
	SaveSyllabus(ctx context.Context, syllabus *Syllabus) error
	SaveSyllabusBatch(ctx context.Context, syllabi []*Syllabus) error
	CountSyllabi(ctx context.Context) (int, error)
}

// StickerRepository defines the interface for sticker data operations.
type StickerRepository interface {
	// GetRandomSticker retrieves a random sticker URL from cache.
	// Returns empty string if no stickers available.
	GetRandomSticker(ctx context.Context) (string, error)

	// GetStickersBySource retrieves all stickers from a specific source.
	// Returns empty slice if no stickers found for the source.
	GetStickersBySource(ctx context.Context, source string) ([]Sticker, error)

	// SaveSticker persists a sticker record to cache.
	SaveSticker(ctx context.Context, sticker *Sticker) error

	// SaveStickers persists multiple sticker records in a batch.
	SaveStickers(ctx context.Context, stickers []*Sticker) error

	// UpdateStickerStats updates the success/failure count for a sticker.
	UpdateStickerStats(ctx context.Context, url string, success bool) error

	// CountStickers returns the total number of cached stickers.
	CountStickers(ctx context.Context) (int, error)

	// CountStickersBySource returns the number of stickers grouped by source.
	CountStickersBySource(ctx context.Context) (map[string]int, error)
}

// HealthRepository defines the interface for health check operations.
type HealthRepository interface {
	// Ping verifies database connection is alive.
	Ping(ctx context.Context) error

	// Ready checks if database is ready to serve queries.
	// Performs more thorough checks than Ping.
	Ready(ctx context.Context) error
}

// TransactionRepository defines the interface for transaction management.
type TransactionRepository interface {
	// BeginTx starts a new database transaction.
	BeginTx(ctx context.Context) (Transaction, error)
}

// Transaction represents a database transaction.
type Transaction interface {
	// Commit commits the transaction.
	Commit() error

	// Rollback rolls back the transaction.
	Rollback() error

	// StudentRepository returns a student repository scoped to this transaction.
	StudentRepository() StudentRepository

	// CourseRepository returns a course repository scoped to this transaction.
	CourseRepository() CourseRepository

	// ContactRepository returns a contact repository scoped to this transaction.
	ContactRepository() ContactRepository
}

// Repository is the aggregate interface that combines all repository interfaces.
// The DB type implements this interface, providing a single entry point for
// all data operations.
type Repository interface {
	StudentRepository
	CourseRepository
	ContactRepository
	SyllabusRepository
	StickerRepository
	HealthRepository
	Close() error
}

// Ensure DB implements all repository interfaces at compile time.
// This provides early detection of interface implementation issues.
var (
	_ StudentRepository  = (*DB)(nil)
	_ CourseRepository   = (*DB)(nil)
	_ ContactRepository  = (*DB)(nil)
	_ SyllabusRepository = (*DB)(nil)
	_ StickerRepository  = (*DB)(nil)
	_ HealthRepository   = (*DB)(nil)
	_ Repository         = (*DB)(nil)
)
