// Package snapshot provides SQLite snapshot management with S3-compatible storage.
// It handles snapshot upload/download, background polling for updates,
// and coordination with the leader lease for refresh/cleanup exclusivity.
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

	"github.com/garyellow/ntpu-linebot-go/internal/s3client"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// Config holds snapshot manager configuration.
type Config struct {
	SnapshotKey  string        // S3 object key for snapshot (e.g., "snapshots/cache.db.zst")
	LockKey      string        // S3 object key for the leader lease
	LockTTL      time.Duration // TTL for the leader lease
	PollInterval time.Duration // How often to check for new snapshots
	TempDir      string        // Directory for temporary files
	// RequestTimeout is the timeout for a single S3 request.
	// Set to 0 to disable per-request timeouts.
	RequestTimeout time.Duration
}

// Manager handles SQLite snapshot synchronization with S3-compatible storage.
type Manager struct {
	client       *s3client.Client
	config       Config
	currentETag  string
	mu           sync.RWMutex
	pollCancel   context.CancelFunc
	pollDone     chan struct{}
	leaderMu     sync.Mutex
	renewMu      sync.Mutex
	leaderLock   *s3client.LeaseLock
	leaderCtx    context.Context
	leaderCancel context.CancelFunc
	renewDone    chan struct{}
}

type snapshotUploadClient interface {
	HeadObject(ctx context.Context, key string) (string, error)
	PutObjectIfNotExists(ctx context.Context, key string, body io.Reader, contentType string) (bool, string, error)
	PutObjectIfMatch(ctx context.Context, key string, body io.Reader, etag string, contentType string) (bool, string, error)
}

// New creates a new snapshot manager.
func New(client *s3client.Client, cfg Config) *Manager {
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	return &Manager{
		client:   client,
		config:   cfg,
		pollDone: make(chan struct{}),
	}
}

// DownloadSnapshot downloads and decompresses the latest snapshot from S3-compatible storage.
// Returns the path to the decompressed database and its ETag.
// Returns ErrNotFound if no snapshot exists.
func (m *Manager) DownloadSnapshot(ctx context.Context, destDir string) (string, string, error) {
	if destDir == "" {
		destDir = m.config.TempDir
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", "", fmt.Errorf("create snapshot dir: %w", err)
	}

	// Download compressed snapshot
	downloadCtx := ctx
	cancel := func() {}
	if m.config.RequestTimeout > 0 {
		downloadCtx, cancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	body, etag, err := m.client.Download(downloadCtx, m.config.SnapshotKey)
	if err != nil {
		cancel()
		if errors.Is(err, s3client.ErrNotFound) {
			return "", "", ErrNotFound
		}
		return "", "", fmt.Errorf("download snapshot: %w", err)
	}
	defer cancel()
	defer func() {
		_ = body.Close()
	}()

	// Create temp file for compressed data
	compressedFile, err := os.CreateTemp(destDir, "snapshot_*.db.zst")
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}
	compressedPath := compressedFile.Name()
	defer func() {
		_ = os.Remove(compressedPath)
	}()

	// Stream download to temp file
	if _, err := io.Copy(compressedFile, body); err != nil {
		_ = compressedFile.Close()
		_ = os.Remove(compressedPath) //nolint:gosec // G703: path is constructed internally from temp dir
		return "", "", fmt.Errorf("write compressed data: %w", err)
	}
	if err := compressedFile.Close(); err != nil {
		return "", "", fmt.Errorf("close compressed file: %w", err)
	}

	// Decompress to a temporary destination first
	dbTempFile, err := os.CreateTemp(destDir, "cache_*.db")
	if err != nil {
		return "", "", fmt.Errorf("create temp db: %w", err)
	}
	dbTempPath := dbTempFile.Name()
	_ = dbTempFile.Close()
	defer func() {
		_ = os.Remove(dbTempPath)
	}()

	compressedReader, err := os.Open(compressedPath) //nolint:gosec // G703: path is constructed internally from temp dir
	if err != nil {
		return "", "", fmt.Errorf("open compressed file: %w", err)
	}
	defer func() {
		_ = compressedReader.Close()
	}()

	if err := s3client.DecompressStream(compressedReader, dbTempPath); err != nil {
		return "", "", fmt.Errorf("decompress snapshot: %w", err)
	}

	// Atomically replace the target database
	dbPath := filepath.Join(destDir, "cache.db")
	if err := replaceFile(dbTempPath, dbPath); err != nil {
		return "", "", fmt.Errorf("replace snapshot: %w", err)
	}

	m.mu.Lock()
	m.currentETag = etag
	m.mu.Unlock()

	return dbPath, etag, nil
}

// UploadSnapshot compresses and uploads the database as a new snapshot to S3-compatible storage.
// Returns the ETag of the uploaded snapshot.
func (m *Manager) UploadSnapshot(ctx context.Context, db *storage.DB) (string, error) {
	if err := os.MkdirAll(m.config.TempDir, 0o750); err != nil {
		return "", fmt.Errorf("create snapshot temp dir: %w", err)
	}

	// Create a consistent snapshot file first
	snapshotPath := filepath.Join(m.config.TempDir, fmt.Sprintf("snapshot_%d.db", time.Now().UnixNano()))
	if err := db.CreateSnapshot(ctx, snapshotPath); err != nil {
		return "", fmt.Errorf("create snapshot: %w", err)
	}
	defer func() {
		_ = os.Remove(snapshotPath)
	}()

	// Create temp file for compressed data
	compressedPath := snapshotPath + ".zst"

	// Compress the snapshot
	if err := s3client.CompressFile(snapshotPath, compressedPath); err != nil {
		return "", fmt.Errorf("compress database: %w", err)
	}
	defer func() {
		_ = os.Remove(compressedPath)
	}()

	// Upload compressed file
	compressedFile, err := os.Open(compressedPath)
	if err != nil {
		return "", fmt.Errorf("open compressed file: %w", err)
	}
	defer func() {
		_ = compressedFile.Close()
	}()

	etag, err := m.uploadSnapshotObject(ctx, compressedFile)
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	m.currentETag = etag
	m.mu.Unlock()

	return etag, nil
}

func (m *Manager) uploadSnapshotObject(ctx context.Context, body io.Reader) (string, error) {
	return uploadSnapshotObject(ctx, m.client, m.config.SnapshotKey, m.CurrentETag(), m.config.RequestTimeout, body)
}

func uploadSnapshotObject(ctx context.Context, client snapshotUploadClient, key, currentETag string, requestTimeout time.Duration, body io.Reader) (string, error) {
	expectedETag := currentETag
	if expectedETag == "" {
		headCtx := ctx
		headCancel := func() {}
		if requestTimeout > 0 {
			headCtx, headCancel = context.WithTimeout(ctx, requestTimeout)
		}
		remoteETag, err := client.HeadObject(headCtx, key)
		headCancel()
		if err != nil && !errors.Is(err, s3client.ErrNotFound) {
			return "", fmt.Errorf("head snapshot before upload: %w", err)
		}
		if err == nil {
			expectedETag = remoteETag
		}
	}

	uploadCtx := ctx
	cancel := func() {}
	if requestTimeout > 0 {
		uploadCtx, cancel = context.WithTimeout(ctx, requestTimeout)
	}
	defer cancel()

	if expectedETag == "" {
		created, etag, err := client.PutObjectIfNotExists(uploadCtx, key, body, "application/zstd")
		if err != nil {
			return "", fmt.Errorf("upload snapshot if absent: %w", err)
		}
		if !created {
			return "", s3client.ErrPreconditionFailed
		}
		return etag, nil
	}

	updated, etag, err := client.PutObjectIfMatch(uploadCtx, key, body, expectedETag, "application/zstd")
	if err != nil {
		return "", fmt.Errorf("upload snapshot if-match: %w", err)
	}
	if !updated {
		return "", s3client.ErrPreconditionFailed
	}

	return etag, nil
}

// AcquireLeaderLock attempts to acquire the leader lease lock.
// Returns true if this instance became the leader.
func (m *Manager) AcquireLeaderLock(ctx context.Context) (bool, error) {
	lock := s3client.NewLeaseLock(m.client, m.config.LockKey, m.config.LockTTL)
	lockCtx := ctx
	cancel := func() {}
	if m.config.RequestTimeout > 0 {
		lockCtx, cancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	acquired, err := lock.Acquire(lockCtx)
	cancel()
	if err != nil || !acquired {
		return acquired, err
	}

	m.leaderMu.Lock()
	if m.leaderCancel != nil {
		m.leaderCancel()
		if m.renewDone != nil {
			<-m.renewDone
		}
	}
	leaderCtx, leaderCancel := context.WithCancel(ctx)
	m.leaderLock = lock
	m.leaderCtx = leaderCtx
	m.leaderCancel = leaderCancel
	m.renewDone = make(chan struct{})
	go m.renewLoop(leaderCtx, lock, leaderCancel, m.renewDone)
	m.leaderMu.Unlock()

	return true, nil
}

// ReleaseLeaderLock releases the leader lease lock.
func (m *Manager) ReleaseLeaderLock(ctx context.Context) error {
	m.leaderMu.Lock()
	lock := m.leaderLock
	leaderCancel := m.leaderCancel
	done := m.renewDone
	m.leaderLock = nil
	m.leaderCtx = nil
	m.leaderCancel = nil
	m.renewDone = nil
	m.leaderMu.Unlock()

	if leaderCancel != nil {
		leaderCancel()
		if done != nil {
			<-done
		}
	}

	if lock == nil {
		return nil
	}
	releaseCtx := ctx
	cancel := func() {}
	if m.config.RequestTimeout > 0 {
		releaseCtx, cancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	defer cancel()
	return lock.Release(releaseCtx)
}

// RenewLeaderLock verifies this process still owns the leader lease and extends
// its TTL. It returns false when the lock has already been lost.
func (m *Manager) RenewLeaderLock(ctx context.Context) (bool, error) {
	m.leaderMu.Lock()
	lock := m.leaderLock
	leaderCancel := m.leaderCancel
	m.leaderMu.Unlock()
	if lock == nil {
		return false, nil
	}
	renewed, err := m.renewLock(ctx, lock)
	if err != nil || !renewed {
		if leaderCancel != nil {
			leaderCancel()
		}
	}
	return renewed, err
}

// LeaderContext returns the active leader lease context.
// It is canceled when the lease is released or a renew attempt fails, allowing
// long-running refresh/cleanup work to stop instead of continuing as a stale leader.
func (m *Manager) LeaderContext(parent context.Context) context.Context {
	m.leaderMu.Lock()
	leaderCtx := m.leaderCtx
	m.leaderMu.Unlock()
	if leaderCtx == nil {
		return parent
	}
	return leaderCtx
}

// StartPolling starts background polling for new snapshots.
// When a new snapshot is detected (via ETag change), it downloads and
// hot-swaps the database in the provided HotSwapDB.
// onHotSwap is called after a successful hot-swap with the new ETag.
func (m *Manager) StartPolling(ctx context.Context, hotSwapDB *storage.HotSwapDB, destDir string, onHotSwap func(string)) {
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
				m.pollOnce(pollCtx, hotSwapDB, destDir, onHotSwap)
			}
		}
	}()

	slog.Info("Snapshot polling started",
		"interval", m.config.PollInterval,
		"snapshot_key", m.config.SnapshotKey)
}

// pollOnce checks for a new snapshot and performs hot-swap if found.
// The snapshot is validated and integrity-checked before swapping to ensure safety.
func (m *Manager) pollOnce(ctx context.Context, hotSwapDB *storage.HotSwapDB, destDir string, onHotSwap func(string)) {
	// Check current ETag
	m.mu.RLock()
	currentETag := m.currentETag
	m.mu.RUnlock()

	// Get remote ETag
	headCtx := ctx
	headCancel := func() {}
	if m.config.RequestTimeout > 0 {
		headCtx, headCancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	remoteETag, err := m.client.HeadObject(headCtx, m.config.SnapshotKey)
	headCancel()
	if err != nil {
		if !errors.Is(err, s3client.ErrNotFound) {
			slog.Warn("Snapshot poll head object failed", "error", err)
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
	newDBPath := filepath.Join(destDir, fmt.Sprintf("cache_%d.db", time.Now().UnixNano()))
	var swapSuccess bool
	defer func() {
		// Clean up on error only (hot-swap will handle cleanup on success)
		if !swapSuccess {
			if _, err := os.Stat(newDBPath); err == nil {
				_ = os.Remove(newDBPath)
				_ = os.Remove(newDBPath + "-wal")
				_ = os.Remove(newDBPath + "-shm")
			}
		}
	}()

	// Download and decompress with ETag consistency
	downloadCtx := ctx
	downloadCancel := func() {}
	if m.config.RequestTimeout > 0 {
		downloadCtx, downloadCancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	body, downloadedETag, err := m.client.DownloadIfMatch(downloadCtx, m.config.SnapshotKey, remoteETag)
	if err != nil {
		downloadCancel()
		if errors.Is(err, s3client.ErrPreconditionFailed) {
			slog.Warn("Snapshot poll ETag changed during download, retrying later",
				"expected_etag", remoteETag)
			return
		}
		slog.Error("Snapshot poll download failed", "error", err)
		return
	}
	defer downloadCancel()
	defer func() {
		_ = body.Close()
	}()

	// Stream decompress directly
	if err := s3client.DecompressStream(body, newDBPath); err != nil {
		slog.Error("Snapshot poll decompress failed", "error", err)
		return
	}

	// Validate the downloaded snapshot before swapping (with timeout to prevent blocking)
	validateCtx := ctx
	validateCancel := func() {}
	if m.config.RequestTimeout > 0 {
		validateCtx, validateCancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	validateDB, err := storage.New(validateCtx, newDBPath, hotSwapDB.DB().GetCacheTTL())
	if err != nil {
		validateCancel()
		slog.Error("Snapshot poll validation failed: cannot open", "error", err)
		return
	}

	// Check integrity of the downloaded snapshot (with timeout)
	if err := validateDB.CheckIntegrity(validateCtx); err != nil {
		_ = validateDB.Close(validateCtx)
		validateCancel()
		slog.Error("Snapshot poll integrity check failed", "error", err)
		return
	}

	// Close validation connection before hot-swap
	_ = validateDB.Close(validateCtx)
	validateCancel()

	// Hot-swap the database (with timeout to prevent filesystem stalls)
	swapCtx := ctx
	swapCancel := func() {}
	if m.config.RequestTimeout > 0 {
		swapCtx, swapCancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	if err := hotSwapDB.Swap(swapCtx, newDBPath); err != nil {
		swapCancel()
		slog.Error("Snapshot poll hot-swap failed", "error", err)
		return
	}
	swapCancel()

	// Mark swap as successful to prevent defer cleanup
	swapSuccess = true

	m.mu.Lock()
	if downloadedETag != "" {
		m.currentETag = downloadedETag
	} else {
		m.currentETag = remoteETag
	}
	m.mu.Unlock()

	if onHotSwap != nil {
		onHotSwap(remoteETag)
	}

	slog.Info("Hot-swap completed successfully", "new_etag", remoteETag)
}

// StopPolling stops the background polling goroutine.
func (m *Manager) StopPolling() {
	if m.pollCancel != nil {
		m.pollCancel()
		<-m.pollDone
	}
}

func (m *Manager) renewLoop(ctx context.Context, lock *s3client.LeaseLock, cancelLeader context.CancelFunc, done chan struct{}) {
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
			renewed, err := m.renewLock(ctx, lock)
			if err != nil {
				if ctx.Err() == nil {
					slog.Warn("Leader lease renew failed", "error", err)
					cancelLeader()
				}
				return
			}
			if !renewed {
				slog.Warn("Leader lease lost during renew")
				cancelLeader()
				return
			}
		}
	}
}

func (m *Manager) renewLock(ctx context.Context, lock *s3client.LeaseLock) (bool, error) {
	renewCtx := ctx
	cancel := func() {}
	if m.config.RequestTimeout > 0 {
		renewCtx, cancel = context.WithTimeout(ctx, m.config.RequestTimeout)
	}
	defer cancel()

	m.renewMu.Lock()
	defer m.renewMu.Unlock()
	return lock.Renew(renewCtx)
}

// CurrentETag returns the ETag of the currently loaded snapshot.
func (m *Manager) CurrentETag() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentETag
}

// ErrNotFound indicates no snapshot exists in S3-compatible storage.
var ErrNotFound = errors.New("snapshot: not found")

func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil { //nolint:gosec // G703: paths are constructed internally
		return nil
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst) //nolint:gosec // G703: paths are constructed internally
}
