package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// InitSchema creates all necessary tables and indexes.
// Note: WAL mode is configured in db.go's configureConnection function.
func InitSchema(ctx context.Context, db *sql.DB) error {
	// Create students table
	if err := createStudentsTable(ctx, db); err != nil {
		return err
	}

	// Create contacts table
	if err := createContactsTable(ctx, db); err != nil {
		return err
	}

	// Create courses table
	if err := createCoursesTable(ctx, db); err != nil {
		return err
	}

	// Create stickers table
	if err := createStickersTable(ctx, db); err != nil {
		return err
	}

	// Create historical_courses table for on-demand historical course queries
	if err := createHistoricalCoursesTable(ctx, db); err != nil {
		return err
	}

	// Create course_programs table for course-program relationships (學程)
	if err := createCourseProgramsTable(ctx, db); err != nil {
		return err
	}

	// Create syllabi table for course syllabus smart search (BM25 index)
	return createSyllabiTable(ctx, db)
}

func createStudentsTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS students (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		year INTEGER,
		department TEXT,
		cached_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_students_name ON students(name);
	CREATE INDEX IF NOT EXISTS idx_students_year_dept ON students(year, department);
	CREATE INDEX IF NOT EXISTS idx_students_cached_at ON students(cached_at);
	`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create students table: %w", err)
	}

	return nil
}

func createContactsTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS contacts (
		uid TEXT PRIMARY KEY,
		type TEXT CHECK(type IN ('individual', 'organization')) NOT NULL,
		name TEXT NOT NULL,
		name_en TEXT,
		organization TEXT,
		title TEXT,
		extension TEXT,
		email TEXT,
		phone TEXT,
		website TEXT,
		location TEXT,
		superior TEXT,
		cached_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name);
	CREATE INDEX IF NOT EXISTS idx_contacts_type ON contacts(type);
	CREATE INDEX IF NOT EXISTS idx_contacts_organization ON contacts(organization);
	CREATE INDEX IF NOT EXISTS idx_contacts_cached_at ON contacts(cached_at);
	`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create contacts table: %w", err)
	}

	return nil
}

func createCoursesTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS courses (
		uid TEXT PRIMARY KEY,
		year INTEGER NOT NULL,
		term INTEGER NOT NULL,
		no TEXT,
		title TEXT NOT NULL,
		teachers TEXT,
		teacher_urls TEXT,
		times TEXT,
		locations TEXT,
		detail_url TEXT,
		note TEXT,
		cached_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_courses_title ON courses(title);
	CREATE INDEX IF NOT EXISTS idx_courses_year_term ON courses(year, term);
	CREATE INDEX IF NOT EXISTS idx_courses_teachers ON courses(teachers);
	CREATE INDEX IF NOT EXISTS idx_courses_cached_at ON courses(cached_at);
	`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create courses table: %w", err)
	}

	return nil
}

func createStickersTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS stickers (
		url TEXT PRIMARY KEY,
		source TEXT NOT NULL CHECK(source IN ('spy_family', 'ichigo', 'fallback')),
		cached_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_stickers_source ON stickers(source);
	CREATE INDEX IF NOT EXISTS idx_stickers_cached_at ON stickers(cached_at);
	`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create stickers table: %w", err)
	}

	return nil
}

// createHistoricalCoursesTable creates table for historical course queries
// This table stores courses from semesters older than the regular warmup range (4 semesters)
// Uses 7-day TTL for cache management, same structure as regular courses table
func createHistoricalCoursesTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS historical_courses (
		uid TEXT PRIMARY KEY,
		year INTEGER NOT NULL,
		term INTEGER NOT NULL,
		no TEXT,
		title TEXT NOT NULL,
		teachers TEXT,
		teacher_urls TEXT,
		times TEXT,
		locations TEXT,
		detail_url TEXT,
		note TEXT,
		cached_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_historical_courses_title ON historical_courses(title);
	CREATE INDEX IF NOT EXISTS idx_historical_courses_year_term ON historical_courses(year, term);
	CREATE INDEX IF NOT EXISTS idx_historical_courses_teachers ON historical_courses(teachers);
	CREATE INDEX IF NOT EXISTS idx_historical_courses_cached_at ON historical_courses(cached_at);
	`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create historical_courses table: %w", err)
	}

	return nil
}

// createSyllabiTable creates table for course syllabus search content.
// Stores unified CN+EN text for BM25 indexing with SHA256 hash for change detection.
func createSyllabiTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS syllabi (
		uid TEXT PRIMARY KEY,
		year INTEGER NOT NULL,
		term INTEGER NOT NULL,
		title TEXT NOT NULL,
		teachers TEXT,
		objectives TEXT,
		outline TEXT,
		schedule TEXT,
		content_hash TEXT NOT NULL,
		cached_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_syllabi_year_term ON syllabi(year, term);
	CREATE INDEX IF NOT EXISTS idx_syllabi_content_hash ON syllabi(content_hash);
	CREATE INDEX IF NOT EXISTS idx_syllabi_cached_at ON syllabi(cached_at);
	`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create syllabi table: %w", err)
	}

	return nil
}

// createCourseProgramsTable creates table for course-program relationships (學程).
// A course can belong to multiple programs with different requirement types (必修/選修).
// This table enables bidirectional queries: courses by program, programs by course.
func createCourseProgramsTable(ctx context.Context, db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS course_programs (
		course_uid TEXT NOT NULL,
		program_name TEXT NOT NULL,
		course_type TEXT NOT NULL,
		cached_at INTEGER NOT NULL,
		PRIMARY KEY (course_uid, program_name)
	);
	CREATE INDEX IF NOT EXISTS idx_course_programs_program ON course_programs(program_name);
	CREATE INDEX IF NOT EXISTS idx_course_programs_course ON course_programs(course_uid);
	CREATE INDEX IF NOT EXISTS idx_course_programs_type ON course_programs(course_type);
	CREATE INDEX IF NOT EXISTS idx_course_programs_cached_at ON course_programs(cached_at);
	`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create course_programs table: %w", err)
	}

	return nil
}
