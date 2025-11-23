package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection
type DB struct {
	conn     *sql.DB
	path     string
	cacheTTL time.Duration   // Cache time-to-live for all data
	metrics  MetricsRecorder // Optional metrics recorder for data integrity checks
}

// MetricsRecorder defines the interface for recording data integrity metrics
type MetricsRecorder interface {
	RecordCourseIntegrityIssue(issueType string)
}

// New creates a new database connection and initializes the schema
// cacheTTL specifies how long cached data remains valid before expiring
func New(dbPath string, cacheTTL time.Duration) (*DB, error) {
	// Ensure directory exists (skip for in-memory database)
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		// Only create directory if it's not empty and not current directory
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create database directory: %w", err)
			}
		}
	}

	// Open database connection
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)

	// Enable WAL mode for better concurrency
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout to 5 seconds
	if _, err := conn.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set synchronous mode to NORMAL for better performance
	if _, err := conn.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to set synchronous mode: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		conn:     conn,
		path:     dbPath,
		cacheTTL: cacheTTL,
	}

	// Initialize schema
	if err := InitSchema(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Conn returns the underlying *sql.DB connection
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// Path returns the database file path
func (db *DB) Path() string {
	return db.path
}

// Begin starts a new transaction
func (db *DB) Begin() (*sql.Tx, error) {
	return db.conn.Begin()
}

// Exec executes a query without returning any rows
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(query, args...)
}

// Query executes a query that returns rows
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// QueryRow executes a query that returns at most one row
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// SetMetrics sets the metrics recorder for data integrity monitoring
func (db *DB) SetMetrics(recorder MetricsRecorder) {
	db.metrics = recorder
}

// NewTestDB creates an in-memory database for testing.
// This ensures consistent test data isolation across all test files.
// The database is automatically cleaned up when closed.
// Uses default 7-day TTL for tests.
func NewTestDB() (*DB, error) {
	return New(":memory:", 168*time.Hour) // 7 days
}
