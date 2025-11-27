package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/timeouts"
	_ "modernc.org/sqlite" // SQLite driver for database/sql
)

// DB wraps SQLite database connections with read/write separation.
//
// SQLite with WAL mode allows concurrent reads but only one writer at a time.
// By separating read and write connections:
//   - Writer: Single connection to avoid SQLITE_BUSY errors
//   - Readers: Multiple connections for parallel read queries
//
// This pattern is recommended for Go applications using database/sql with SQLite.
// See: https://github.com/mattn/go-sqlite3/issues/274
type DB struct {
	writer   *sql.DB         // Single connection for writes (MaxOpenConns=1)
	reader   *sql.DB         // Multiple connections for reads
	path     string          // Database file path
	cacheTTL time.Duration   // Cache time-to-live for all data
	metrics  MetricsRecorder // Optional metrics recorder for data integrity checks
}

// MetricsRecorder defines the interface for recording data integrity metrics
type MetricsRecorder interface {
	RecordCourseIntegrityIssue(issueType string)
}

// New creates a new database with read/write separation and initializes the schema.
// cacheTTL specifies how long cached data remains valid before expiring.
//
// Connection architecture:
//   - Writer: 1 connection with immediate transaction lock
//   - Reader: 10 connections in read-only mode for parallel queries
//
// Note: In-memory databases use shared cache mode to allow read/write separation
func New(dbPath string, cacheTTL time.Duration) (*DB, error) {
	// Ensure directory exists (skip for in-memory database)
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		// Only create directory if it's not empty and not current directory
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create database directory: %w", err)
			}
		}
	}

	// For in-memory databases, use shared cache mode so multiple connections
	// can access the same database. Without this, each connection gets its
	// own private database instance.
	isMemory := dbPath == ":memory:"

	// Open writer connection (single connection for all writes)
	// Using _txlock=immediate ensures write transactions acquire lock immediately
	var writerDSN string
	if isMemory {
		// file::memory:?cache=shared creates a shared in-memory database
		writerDSN = "file::memory:?cache=shared&_txlock=immediate"
	} else {
		writerDSN = dbPath + "?_txlock=immediate"
	}
	writer, err := sql.Open("sqlite", writerDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open writer connection: %w", err)
	}

	// CRITICAL: Writer must have MaxOpenConns=1 to prevent SQLITE_BUSY
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(timeouts.DatabaseConnMaxLifetime)

	// Configure writer connection
	if err := configureConnection(writer, false); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("failed to configure writer: %w", err)
	}

	// Open reader connection pool (multiple connections for parallel reads)
	var readerDSN string
	if isMemory {
		// Same shared cache for in-memory database, but in read-only mode
		readerDSN = "file::memory:?cache=shared&mode=ro"
	} else {
		readerDSN = dbPath + "?mode=ro"
	}
	reader, err := sql.Open("sqlite", readerDSN)
	if err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("failed to open reader connection: %w", err)
	}

	// Reader can have multiple connections for parallel queries
	reader.SetMaxOpenConns(10)
	reader.SetMaxIdleConns(5)
	reader.SetConnMaxLifetime(timeouts.DatabaseConnMaxLifetime)

	// Configure reader connection
	if err := configureConnection(reader, true); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return nil, fmt.Errorf("failed to configure reader: %w", err)
	}

	// Test connections
	if err := writer.Ping(); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return nil, fmt.Errorf("failed to ping writer: %w", err)
	}
	if err := reader.Ping(); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return nil, fmt.Errorf("failed to ping reader: %w", err)
	}

	db := &DB{
		writer:   writer,
		reader:   reader,
		path:     dbPath,
		cacheTTL: cacheTTL,
	}

	// Initialize schema using writer connection
	if err := InitSchema(writer); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// configureConnection sets up SQLite pragmas for optimal performance
func configureConnection(conn *sql.DB, readOnly bool) error {
	// Enable WAL mode for better concurrency (readers don't block writer)
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout to handle concurrent access during warmup
	busyTimeoutMs := int(timeouts.DatabaseBusyTimeout.Milliseconds())
	if _, err := conn.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMs)); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set synchronous mode to NORMAL for better write performance
	// (WAL mode makes this safe - data is still durable)
	if !readOnly {
		if _, err := conn.Exec("PRAGMA synchronous=NORMAL"); err != nil {
			return fmt.Errorf("failed to set synchronous mode: %w", err)
		}
	}

	return nil
}

// Close closes both reader and writer database connections
func (db *DB) Close() error {
	var errs []error
	if db.reader != nil {
		if err := db.reader.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close reader: %w", err))
		}
	}
	if db.writer != nil {
		if err := db.writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close writer: %w", err))
		}
	}
	if len(errs) > 0 {
		return errs[0] // Return first error
	}
	return nil
}

// Writer returns the writer connection for write operations.
// Use this for INSERT, UPDATE, DELETE operations.
func (db *DB) Writer() *sql.DB {
	return db.writer
}

// Reader returns the reader connection pool for read operations.
// Use this for SELECT queries.
func (db *DB) Reader() *sql.DB {
	return db.reader
}

// Conn returns the writer connection for backward compatibility.
// Deprecated: Use Writer() for writes or Reader() for reads.
func (db *DB) Conn() *sql.DB {
	return db.writer
}

// Path returns the database file path
func (db *DB) Path() string {
	return db.path
}

// Begin starts a new write transaction on the writer connection
func (db *DB) Begin() (*sql.Tx, error) {
	return db.writer.Begin()
}

// Exec executes a write query (INSERT, UPDATE, DELETE) on the writer connection
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.writer.Exec(query, args...)
}

// Query executes a read query on the reader connection pool
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.reader.Query(query, args...)
}

// QueryRow executes a read query that returns at most one row on the reader connection
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.reader.QueryRow(query, args...)
}

// SetMetrics sets the metrics recorder for data integrity monitoring
func (db *DB) SetMetrics(recorder MetricsRecorder) {
	db.metrics = recorder
}

// GetCacheTTL returns the configured cache TTL
func (db *DB) GetCacheTTL() time.Duration {
	return db.cacheTTL
}

// getTTLTimestamp returns the Unix timestamp for TTL cutoff (entries older than this are expired)
// This is a helper method to avoid repeating the same calculation across repository methods
func (db *DB) getTTLTimestamp() int64 {
	return time.Now().Unix() - int64(db.cacheTTL.Seconds())
}

// CountExpiringStudents counts students that will expire within the given duration
// Used by warmup scheduler to determine if proactive refresh is needed
func (db *DB) CountExpiringStudents(softTTL time.Duration) (int, error) {
	// Count entries where: softTTL <= age < hardTTL
	// These are entries that should be refreshed proactively
	softTimestamp := time.Now().Unix() - int64(softTTL.Seconds())
	hardTimestamp := time.Now().Unix() - int64(db.cacheTTL.Seconds())

	query := `SELECT COUNT(*) FROM students WHERE cached_at <= ? AND cached_at > ?`
	var count int
	err := db.reader.QueryRow(query, softTimestamp, hardTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count expiring students: %w", err)
	}
	return count, nil
}

// CountExpiringCourses counts courses that will expire within the given duration
func (db *DB) CountExpiringCourses(softTTL time.Duration) (int, error) {
	softTimestamp := time.Now().Unix() - int64(softTTL.Seconds())
	hardTimestamp := time.Now().Unix() - int64(db.cacheTTL.Seconds())

	query := `SELECT COUNT(*) FROM courses WHERE cached_at <= ? AND cached_at > ?`
	var count int
	err := db.reader.QueryRow(query, softTimestamp, hardTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count expiring courses: %w", err)
	}
	return count, nil
}

// CountExpiringContacts counts contacts that will expire within the given duration
func (db *DB) CountExpiringContacts(softTTL time.Duration) (int, error) {
	softTimestamp := time.Now().Unix() - int64(softTTL.Seconds())
	hardTimestamp := time.Now().Unix() - int64(db.cacheTTL.Seconds())

	query := `SELECT COUNT(*) FROM contacts WHERE cached_at <= ? AND cached_at > ?`
	var count int
	err := db.reader.QueryRow(query, softTimestamp, hardTimestamp).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count expiring contacts: %w", err)
	}
	return count, nil
}

// NewTestDB creates an in-memory database for testing.
// Note: In-memory databases don't support read/write separation as they use
// a single shared connection. Both reader and writer point to the same connection.
// This ensures consistent test data isolation across all test files.
// Uses default 7-day TTL for tests.
func NewTestDB() (*DB, error) {
	return New(":memory:", 168*time.Hour) // 7 days
}
