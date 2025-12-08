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
// Writer uses a single connection to avoid SQLITE_BUSY errors.
// Reader uses multiple connections for parallel queries.
type DB struct {
	writer   *sql.DB
	reader   *sql.DB
	path     string
	cacheTTL time.Duration
}

// New creates a new database with read/write separation and initializes the schema.
func New(ctx context.Context, dbPath string, cacheTTL time.Duration) (*DB, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
	}

	isMemory := dbPath == ":memory:"

	var writerDSN, readerDSN string
	if isMemory {
		writerDSN = "file::memory:?cache=shared&_txlock=immediate"
		readerDSN = "file::memory:?cache=shared&mode=ro"
	} else {
		writerDSN = dbPath + "?_txlock=immediate"
		readerDSN = dbPath + "?mode=ro"
	}

	writer, err := sql.Open("sqlite", writerDSN)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}

	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(config.DatabaseConnMaxLifetime)

	if err := configureConnection(ctx, writer, false); err != nil {
		writer.Close()
		return nil, fmt.Errorf("configure writer: %w", err)
	}

	if err := writer.PingContext(ctx); err != nil {
		writer.Close()
		return nil, fmt.Errorf("ping writer: %w", err)
	}

	if err := InitSchema(ctx, writer); err != nil {
		writer.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	reader, err := sql.Open("sqlite", readerDSN)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}

	reader.SetMaxOpenConns(10)
	reader.SetMaxIdleConns(5)
	reader.SetConnMaxLifetime(config.DatabaseConnMaxLifetime)

	if err := configureConnection(ctx, reader, true); err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf("configure reader: %w", err)
	}

	if err := reader.PingContext(ctx); err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf("ping reader: %w", err)
	}

	db := &DB{
		writer:   writer,
		reader:   reader,
		path:     dbPath,
		cacheTTL: cacheTTL,
	}

	return db, nil
}

func configureConnection(ctx context.Context, conn *sql.DB, readOnly bool) error {
	if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("enable WAL: %w", err)
	}

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
// Returns all errors joined together.
func (db *DB) Close() error {
	var errs []error
	if db.reader != nil {
		if err := db.reader.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close reader: %w", err))
		}
	}
	if db.writer != nil {
		if err := db.writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close writer: %w", err))
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

// ExecContext executes a write query with context on the writer connection
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.writer.ExecContext(ctx, query, args...)
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

// Ping checks if the database is ready to serve requests.
// This performs a ping on both reader and writer connections to verify connectivity.
// Use this for Kubernetes readiness probes or health checks.
func (db *DB) Ping(ctx context.Context) error {
	return errors.Join(
		db.writer.PingContext(ctx),
		db.reader.PingContext(ctx),
	)
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
