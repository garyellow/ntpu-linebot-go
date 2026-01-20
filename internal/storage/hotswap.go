package storage

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// HotSwapDB wraps a DB with thread-safe hot-swap capability.
// All read operations acquire a read lock, allowing concurrent queries.
// The Swap operation acquires a write lock, blocking new queries while
// atomically replacing the underlying database connection.
type HotSwapDB struct {
	mu       sync.RWMutex
	current  *DB
	cacheTTL time.Duration
}

// NewHotSwapDB creates a new HotSwapDB with the given initial database path.
func NewHotSwapDB(ctx context.Context, dbPath string, cacheTTL time.Duration) (*HotSwapDB, error) {
	db, err := New(ctx, dbPath, cacheTTL)
	if err != nil {
		return nil, fmt.Errorf("hotswap: create initial db: %w", err)
	}

	return &HotSwapDB{
		current:  db,
		cacheTTL: cacheTTL,
	}, nil
}

// DB returns the current database handle.
// The handle is stable, but callers should still fetch fresh reader/writer
// connections per operation via DB methods to respect hot-swap boundaries.
func (h *HotSwapDB) DB() *DB {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// Swap atomically replaces the current database with a new one.
// The old database is closed asynchronously after a grace period.
//
// Swap process:
//  1. Open and validate the new database
//  2. Acquire write lock (blocks new read operations)
//  3. Swap the database pointer
//  4. Release write lock
//  5. Close old database asynchronously (with grace period for in-flight queries)
func (h *HotSwapDB) Swap(ctx context.Context, newDbPath string) error {
	// Open and validate new database before acquiring lock
	newDB, err := New(ctx, newDbPath, h.cacheTTL)
	if err != nil {
		return fmt.Errorf("hotswap: open new db: %w", err)
	}

	// Validate new database is accessible
	if err := newDB.Ping(ctx); err != nil {
		_ = newDB.Close()
		return fmt.Errorf("hotswap: ping new db: %w", err)
	}

	// Acquire write lock and swap connections in-place
	h.mu.Lock()
	oldWriter, oldReader, oldPath := h.current.SwapConnections(newDB)
	h.mu.Unlock()

	// Close old database asynchronously with grace period
	// This allows in-flight queries to complete
	go func() {
		if oldReader != nil {
			_ = oldReader.Close()
		}
		if oldWriter != nil {
			_ = oldWriter.Close()
		}

		// Clean up old database file if it's different from the new one
		currentPath := h.current.Path()
		if oldPath != currentPath && oldPath != ":memory:" {
			// Remove old .db, .db-wal, and .db-shm files
			_ = os.Remove(oldPath)
			_ = os.Remove(oldPath + "-wal")
			_ = os.Remove(oldPath + "-shm")
		}
	}()

	return nil
}

// Path returns the current database file path.
func (h *HotSwapDB) Path() string {
	h.mu.RLock()
	current := h.current
	h.mu.RUnlock()
	return current.Path()
}

// Close closes the current database connection.
func (h *HotSwapDB) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.current != nil {
		return h.current.Close()
	}
	return nil
}

// Ping checks if the current database is accessible.
func (h *HotSwapDB) Ping(ctx context.Context) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current.Ping(ctx)
}

// Reader returns the reader connection pool for read operations.
func (h *HotSwapDB) Reader() interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current.Reader()
}

// Writer returns the writer connection for write operations.
func (h *HotSwapDB) Writer() interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current.Writer()
}
