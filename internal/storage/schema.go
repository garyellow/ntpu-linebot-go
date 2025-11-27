package storage

import (
	"database/sql"
	"fmt"
)

// InitSchema creates all necessary tables and indexes
func InitSchema(db *sql.DB) error {
	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout to avoid SQLITE_BUSY errors
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Create students table
	if err := createStudentsTable(db); err != nil {
		return err
	}

	// Create contacts table
	if err := createContactsTable(db); err != nil {
		return err
	}

	// Create courses table
	if err := createCoursesTable(db); err != nil {
		return err
	}

	// Create stickers table
	if err := createStickersTable(db); err != nil {
		return err
	}

	// Create historical_courses table for on-demand historical course queries
	return createHistoricalCoursesTable(db)
}

func createStudentsTable(db *sql.DB) error {
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

	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create students table: %w", err)
	}

	return nil
}

func createContactsTable(db *sql.DB) error {
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

	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create contacts table: %w", err)
	}

	return nil
}

func createCoursesTable(db *sql.DB) error {
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

	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create courses table: %w", err)
	}

	return nil
}

func createStickersTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS stickers (
		url TEXT PRIMARY KEY,
		source TEXT NOT NULL CHECK(source IN ('spy_family', 'ichigo', 'fallback')),
		cached_at INTEGER NOT NULL,
		success_count INTEGER DEFAULT 0,
		failure_count INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_stickers_source ON stickers(source);
	CREATE INDEX IF NOT EXISTS idx_stickers_cached_at ON stickers(cached_at);
	CREATE INDEX IF NOT EXISTS idx_stickers_success_rate ON stickers(success_count DESC, failure_count ASC);
	`

	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create stickers table: %w", err)
	}

	return nil
}

// createHistoricalCoursesTable creates table for historical course queries
// This table stores courses from years older than the regular warmup range (2 years)
// Uses 7-day hard TTL for cache management, same structure as regular courses table
func createHistoricalCoursesTable(db *sql.DB) error {
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

	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create historical_courses table: %w", err)
	}

	return nil
}
