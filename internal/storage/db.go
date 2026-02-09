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
	"strings"
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	_ "modernc.org/sqlite" // SQLite driver for database/sql
)

// DB wraps SQLite database connections with read/write separation.
// Writer uses a single connection to avoid SQLITE_BUSY errors.
// Reader uses multiple connections for parallel queries.
type DB struct {
	mu       sync.RWMutex
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
		baseDSN := "file:ntpu_cache?mode=memory&cache=shared"
		writerDSN = baseDSN + "&_txlock=immediate"
		readerDSN = baseDSN
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
		_ = writer.Close() // Best effort cleanup
		return nil, fmt.Errorf("configure writer: %w", err)
	}

	if err := writer.PingContext(ctx); err != nil {
		_ = writer.Close() // Best effort cleanup
		return nil, fmt.Errorf("ping writer: %w", err)
	}

	if err := InitSchema(ctx, writer); err != nil {
		_ = writer.Close() // Best effort cleanup
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	reader, err := sql.Open("sqlite", readerDSN)
	if err != nil {
		_ = writer.Close() // Best effort cleanup
		return nil, fmt.Errorf("open reader: %w", err)
	}

	reader.SetMaxOpenConns(10)
	reader.SetMaxIdleConns(5)
	reader.SetConnMaxLifetime(config.DatabaseConnMaxLifetime)

	if err := configureConnection(ctx, reader, true); err != nil {
		_ = writer.Close() // Best effort cleanup
		_ = reader.Close() // Best effort cleanup
		return nil, fmt.Errorf("configure reader: %w", err)
	}

	if err := reader.PingContext(ctx); err != nil {
		_ = writer.Close() // Best effort cleanup
		_ = reader.Close() // Best effort cleanup
		return nil, fmt.Errorf("ping reader: %w", err)
	}

	return &DB{
		writer:   writer,
		reader:   reader,
		path:     dbPath,
		cacheTTL: cacheTTL,
	}, nil
}

func configureConnection(ctx context.Context, conn *sql.DB, readOnly bool) error {
	if !readOnly {
		if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
			return fmt.Errorf("enable WAL: %w", err)
		}
	}

	busyTimeoutMs := int(config.DatabaseBusyTimeout.Milliseconds())
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMs)); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Store temporary tables in memory for faster queries
	if _, err := conn.ExecContext(ctx, "PRAGMA temp_store=MEMORY"); err != nil {
		return fmt.Errorf("failed to set temp store: %w", err)
	}

	// Set synchronous mode to NORMAL for better write performance
	// (WAL mode makes this safe - data is still durable)
	if !readOnly {
		if _, err := conn.ExecContext(ctx, "PRAGMA synchronous=NORMAL"); err != nil {
			return fmt.Errorf("failed to set synchronous mode: %w", err)
		}
	} else {
		if _, err := conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
			return fmt.Errorf("failed to set query-only mode: %w", err)
		}
	}

	return nil
}

// Close closes both reader and writer database connections.
// Returns all errors joined together.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

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
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.writer
}

// Reader returns the reader connection pool for read operations.
// Use this for SELECT queries.
func (db *DB) Reader() *sql.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.reader
}

// Path returns the database file path
func (db *DB) Path() string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.path
}

// ExecContext executes a write query with context on the writer connection
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.mu.RLock()
	writer := db.writer
	db.mu.RUnlock()
	return writer.ExecContext(ctx, query, args...)
}

// GetCacheTTL returns the configured cache TTL
func (db *DB) GetCacheTTL() time.Duration {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.cacheTTL
}

// getTTLTimestamp returns the Unix timestamp for TTL cutoff (entries older than this are expired)
// This is a helper method to avoid repeating the same calculation across repository methods
func (db *DB) getTTLTimestamp() int64 {
	db.mu.RLock()
	cacheTTL := db.cacheTTL
	db.mu.RUnlock()
	return time.Now().Unix() - int64(cacheTTL.Seconds())
}

// Ping verifies the database connections are alive by pinging both writer and reader connections.
func (db *DB) Ping(ctx context.Context) error {
	db.mu.RLock()
	writer := db.writer
	reader := db.reader
	db.mu.RUnlock()
	return errors.Join(
		writer.PingContext(ctx),
		reader.PingContext(ctx),
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
	db.mu.RLock()
	writer := db.writer
	db.mu.RUnlock()

	tx, err := writer.BeginTx(ctx, nil)
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

// SwapConnections replaces the underlying reader/writer connections and path.
// Returns the old connections and path for cleanup by the caller.
func (db *DB) SwapConnections(newDB *DB) (oldWriter, oldReader *sql.DB, oldPath string) {
	db.mu.Lock()
	defer db.mu.Unlock()

	oldWriter = db.writer
	oldReader = db.reader
	oldPath = db.path

	db.writer = newDB.writer
	db.reader = newDB.reader
	db.path = newDB.path
	db.cacheTTL = newDB.cacheTTL

	newDB.writer = nil
	newDB.reader = nil

	return oldWriter, oldReader, oldPath
}

// CreateSnapshot creates a consistent snapshot of the database at destPath.
// It uses VACUUM INTO to produce a compact, consistent copy.
func (db *DB) CreateSnapshot(ctx context.Context, destPath string) error {
	if destPath == "" {
		return errors.New("snapshot path is required")
	}
	_ = os.Remove(destPath)

	query := fmt.Sprintf("VACUUM INTO '%s'", escapeSQLiteString(destPath))
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("create snapshot: wal checkpoint: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return fmt.Errorf("create snapshot: optimize: %w", err)
	}
	return nil
}

// CheckIntegrity runs PRAGMA integrity_check on the database.
// Returns nil if the database is OK, or an error describing the corruption.
func (db *DB) CheckIntegrity(ctx context.Context) error {
	db.mu.RLock()
	reader := db.reader
	db.mu.RUnlock()

	var result string
	err := reader.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return fmt.Errorf("integrity check query failed: %w", err)
	}

	if result != "ok" {
		return fmt.Errorf("database integrity check failed: %s", result)
	}

	return nil
}

func escapeSQLiteString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
