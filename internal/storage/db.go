// Package storage provides SQLite database operations for caching
// student, course, contact, and sticker data with TTL management.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
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
			if err := os.MkdirAll(dir, 0o750); err != nil {
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
	writer.SetConnMaxLifetime(config.DatabaseConnMaxLifetime)

	// Configure writer connection
	if err := configureConnection(writer, false); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("failed to configure writer: %w", err)
	}

	// Test writer connection
	if err := writer.PingContext(context.Background()); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("failed to ping writer: %w", err)
	}

	// Initialize schema using writer connection BEFORE opening reader
	// This is critical for in-memory databases: the reader connection in read-only mode
	// cannot access the database until schema exists via the writer connection
	if err := InitSchema(writer); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
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
	reader.SetConnMaxLifetime(config.DatabaseConnMaxLifetime)

	// Configure reader connection
	if err := configureConnection(reader, true); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return nil, fmt.Errorf("failed to configure reader: %w", err)
	}

	// Test reader connection
	if err := reader.PingContext(context.Background()); err != nil {
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

	return db, nil
}

// configureConnection sets up SQLite pragmas for optimal performance
func configureConnection(conn *sql.DB, readOnly bool) error {
	ctx := context.Background()

	// Enable WAL mode for better concurrency (readers don't block writer)
	if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set busy timeout to handle concurrent access during warmup
	busyTimeoutMs := int(config.DatabaseBusyTimeout.Milliseconds())
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMs)); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set synchronous mode to NORMAL for better write performance
	// (WAL mode makes this safe - data is still durable)
	if !readOnly {
		if _, err := conn.ExecContext(ctx, "PRAGMA synchronous=NORMAL"); err != nil {
			return fmt.Errorf("failed to set synchronous mode: %w", err)
		}
	}

	return nil
}

// Close closes both reader and writer database connections.
// Returns all errors joined together (Go 1.20+).
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
	return errors.Join(errs...)
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

// Path returns the database file path
func (db *DB) Path() string {
	return db.path
}

// Begin starts a new write transaction on the writer connection
func (db *DB) Begin() (*sql.Tx, error) {
	return db.writer.BeginTx(context.Background(), nil)
}

// Exec executes a write query (INSERT, UPDATE, DELETE) on the writer connection
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.writer.ExecContext(context.Background(), query, args...)
}

// ExecContext executes a write query with context on the writer connection
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.writer.ExecContext(ctx, query, args...)
}

// Query executes a read query on the reader connection pool
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.reader.QueryContext(context.Background(), query, args...)
}

// QueryRow executes a read query that returns at most one row on the reader connection
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.reader.QueryRowContext(context.Background(), query, args...)
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

// Ready checks if the database is ready to serve requests.
// This performs a ping on both reader and writer connections to verify connectivity.
// Use this for Kubernetes readiness probes or health checks.
//
// Returns nil if both connections are healthy, or an error describing the failure.
func (db *DB) Ready(ctx context.Context) error {
	// Check writer connection
	if err := db.writer.PingContext(ctx); err != nil {
		return fmt.Errorf("writer connection unhealthy: %w", err)
	}

	// Check reader connection
	if err := db.reader.PingContext(ctx); err != nil {
		return fmt.Errorf("reader connection unhealthy: %w", err)
	}

	return nil
}

// ExecBatchContext executes a batch of operations within a single transaction with context support.
// This is a generic helper that reduces lock contention during warmup.
// The execFn receives the prepared statement and should execute it for each item.
//
// Example:
//
//	err := db.ExecBatchContext(ctx, "INSERT INTO t (a,b) VALUES (?,?)", func(stmt *sql.Stmt) error {
//	    for _, item := range items {
//	        if _, err := stmt.ExecContext(ctx, item.A, item.B); err != nil {
//	            return err
//	        }
//	    }
//	    return nil
//	})
func (db *DB) ExecBatchContext(ctx context.Context, query string, execFn func(stmt *sql.Stmt) error) error {
	tx, err := db.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	if err := execFn(stmt); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true

	return nil
}
