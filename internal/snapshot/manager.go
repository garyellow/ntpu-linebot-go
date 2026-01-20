// Package snapshot provides SQLite snapshot management with R2 storage.
// It handles snapshot upload/download, background polling for updates,
// and coordination with the distributed lock for leader election.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/r2client"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// Config holds snapshot manager configuration.
type Config struct {
	SnapshotKey  string        // R2 object key for snapshot (e.g., "snapshots/cache.db.zst")
	LockKey      string        // R2 object key for distributed lock
	LockTTL      time.Duration // TTL for the distributed lock
	PollInterval time.Duration // How often to check for new snapshots
	TempDir      string        // Directory for temporary files
}

// Manager handles SQLite snapshot synchronization with R2.
type Manager struct {
	client      *r2client.Client
	config      Config
	currentETag string
	mu          sync.RWMutex
	pollCancel  context.CancelFunc
	pollDone    chan struct{}
	leaderMu    sync.Mutex
	leaderLock  *r2client.DistributedLock
	renewCancel context.CancelFunc
	renewDone   chan struct{}
}

// New creates a new snapshot manager.
func New(client *r2client.Client, cfg Config) *Manager {
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	return &Manager{
		client:   client,
		config:   cfg,
		pollDone: make(chan struct{}),
	}
}

// DownloadSnapshot downloads and decompresses the latest snapshot from R2.
// Returns the path to the decompressed database and its ETag.
// Returns ErrNotFound if no snapshot exists.
func (m *Manager) DownloadSnapshot(ctx context.Context, destDir string) (string, string, error) {
	// Download compressed snapshot
	body, etag, err := m.client.Download(ctx, m.config.SnapshotKey)
	if err != nil {
		if errors.Is(err, r2client.ErrNotFound) {
			return "", "", ErrNotFound
		}
		return "", "", fmt.Errorf("download snapshot: %w", err)
	}
	defer body.Close()

	// Create temp file for compressed data
	compressedPath := filepath.Join(destDir, "snapshot_download.db.zst")
	compressedFile, err := os.Create(compressedPath)
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}

	// Stream download to temp file
	if _, err := io.Copy(compressedFile, body); err != nil {
		compressedFile.Close()
		os.Remove(compressedPath)
		return "", "", fmt.Errorf("write compressed data: %w", err)
	}
	compressedFile.Close()

	// Decompress to final destination
	dbPath := filepath.Join(destDir, "cache.db")
	compressedReader, err := os.Open(compressedPath)
	if err != nil {
		os.Remove(compressedPath)
		return "", "", fmt.Errorf("open compressed file: %w", err)
	}
	defer compressedReader.Close()

	if err := r2client.DecompressStream(compressedReader, dbPath); err != nil {
		os.Remove(compressedPath)
		return "", "", fmt.Errorf("decompress snapshot: %w", err)
	}

	// Clean up compressed file
	os.Remove(compressedPath)

	m.mu.Lock()
	m.currentETag = etag
	m.mu.Unlock()

	return dbPath, etag, nil
}

// UploadSnapshot compresses and uploads the database as a new snapshot to R2.
// Returns the ETag of the uploaded snapshot.
func (m *Manager) UploadSnapshot(ctx context.Context, db *storage.DB) (string, error) {
	// Create a consistent snapshot file first
	snapshotPath := filepath.Join(m.config.TempDir, fmt.Sprintf("snapshot_%d.db", time.Now().UnixNano()))
	if err := db.CreateSnapshot(ctx, snapshotPath); err != nil {
		return "", fmt.Errorf("create snapshot: %w", err)
	}
	defer os.Remove(snapshotPath)

	// Create temp file for compressed data
	compressedPath := snapshotPath + ".zst"

	// Compress the snapshot
	if err := r2client.CompressFile(snapshotPath, compressedPath); err != nil {
		return "", fmt.Errorf("compress database: %w", err)
	}
	defer os.Remove(compressedPath)

	// Upload compressed file
	compressedFile, err := os.Open(compressedPath)
	if err != nil {
		return "", fmt.Errorf("open compressed file: %w", err)
	}
	defer compressedFile.Close()

	etag, err := m.client.Upload(ctx, m.config.SnapshotKey, compressedFile, "application/zstd")
	if err != nil {
		return "", fmt.Errorf("upload snapshot: %w", err)
	}

	m.mu.Lock()
	m.currentETag = etag
	m.mu.Unlock()

	return etag, nil
}

// AcquireLeaderLock attempts to acquire the distributed leader lock.
// Returns true if this instance became the leader.
func (m *Manager) AcquireLeaderLock(ctx context.Context) (bool, error) {
	lock := r2client.NewDistributedLock(m.client, m.config.LockKey, m.config.LockTTL)
	acquired, err := lock.Acquire(ctx)
	if err != nil || !acquired {
		return acquired, err
	}

	m.leaderMu.Lock()
	if m.renewCancel != nil {
		m.renewCancel()
		if m.renewDone != nil {
			<-m.renewDone
		}
	}
	m.leaderLock = lock
	ctx, cancel := context.WithCancel(ctx)
	m.renewCancel = cancel
	m.renewDone = make(chan struct{})
	go m.renewLoop(ctx, lock, m.renewDone)
	m.leaderMu.Unlock()

	return true, nil
}

// ReleaseLeaderLock releases the distributed leader lock.
func (m *Manager) ReleaseLeaderLock(ctx context.Context) error {
	m.leaderMu.Lock()
	lock := m.leaderLock
	cancel := m.renewCancel
	done := m.renewDone
	m.leaderLock = nil
	m.renewCancel = nil
	m.renewDone = nil
	m.leaderMu.Unlock()

	if cancel != nil {
		cancel()
		if done != nil {
			<-done
		}
	}

	if lock == nil {
		return nil
	}
	return lock.Release(ctx)
}

// StartPolling starts background polling for new snapshots.
// When a new snapshot is detected (via ETag change), it downloads and
// hot-swaps the database in the provided HotSwapDB.
func (m *Manager) StartPolling(ctx context.Context, hotSwapDB *storage.HotSwapDB, destDir string) {
	pollCtx, cancel := context.WithCancel(ctx)
	m.pollCancel = cancel

	go func() {
		defer close(m.pollDone)

		ticker := time.NewTicker(m.config.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-pollCtx.Done():
				slog.Info("Snapshot polling stopped")
				return
			case <-ticker.C:
				m.pollOnce(pollCtx, hotSwapDB, destDir)
			}
		}
	}()

	slog.Info("Snapshot polling started",
		"interval", m.config.PollInterval,
		"snapshot_key", m.config.SnapshotKey)
}

// pollOnce checks for a new snapshot and performs hot-swap if found.
func (m *Manager) pollOnce(ctx context.Context, hotSwapDB *storage.HotSwapDB, destDir string) {
	// Check current ETag
	m.mu.RLock()
	currentETag := m.currentETag
	m.mu.RUnlock()

	// Get remote ETag
	remoteETag, err := m.client.HeadObject(ctx, m.config.SnapshotKey)
	if err != nil {
		if !errors.Is(err, r2client.ErrNotFound) {
			slog.Warn("Snapshot poll: head object failed", "error", err)
		}
		return
	}

	// No change
	if remoteETag == currentETag {
		return
	}

	slog.Info("New snapshot detected, initiating hot-swap",
		"old_etag", currentETag,
		"new_etag", remoteETag)

	// Download new snapshot to a unique path to avoid conflicts
	newDbPath := filepath.Join(destDir, fmt.Sprintf("cache_%d.db", time.Now().UnixNano()))

	// Download and decompress
	body, _, err := m.client.Download(ctx, m.config.SnapshotKey)
	if err != nil {
		slog.Error("Snapshot poll: download failed", "error", err)
		return
	}
	defer body.Close()

	// Stream decompress directly
	if err := r2client.DecompressStream(body, newDbPath); err != nil {
		slog.Error("Snapshot poll: decompress failed", "error", err)
		os.Remove(newDbPath)
		return
	}

	// Hot-swap the database
	if err := hotSwapDB.Swap(ctx, newDbPath); err != nil {
		slog.Error("Snapshot poll: hot-swap failed", "error", err)
		os.Remove(newDbPath)
		os.Remove(newDbPath + "-wal")
		os.Remove(newDbPath + "-shm")
		return
	}

	m.mu.Lock()
	m.currentETag = remoteETag
	m.mu.Unlock()

	slog.Info("Hot-swap completed successfully", "new_etag", remoteETag)
}

// StopPolling stops the background polling goroutine.
func (m *Manager) StopPolling() {
	if m.pollCancel != nil {
		m.pollCancel()
		<-m.pollDone
	}
}

func (m *Manager) renewLoop(ctx context.Context, lock *r2client.DistributedLock, done chan struct{}) {
	defer close(done)

	interval := m.config.LockTTL / 3
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewed, err := lock.Renew(ctx)
			if err != nil {
				slog.Warn("Leader lock renew failed", "error", err)
				return
			}
			if !renewed {
				slog.Warn("Leader lock lost during renew")
				return
			}
		}
	}
}

// CurrentETag returns the ETag of the currently loaded snapshot.
func (m *Manager) CurrentETag() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentETag
}

// SetCurrentETag sets the current ETag (used when loading from local DB).
func (m *Manager) SetCurrentETag(etag string) {
	m.mu.Lock()
	m.currentETag = etag
	m.mu.Unlock()
}

// ErrNotFound indicates no snapshot exists in R2.
var ErrNotFound = errors.New("snapshot: not found")
