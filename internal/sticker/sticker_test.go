package sticker

import (
	"context"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*storage.DB, func()) {
	// Use a unique temporary file for each test to ensure complete isolation
	// This avoids the shared cache issue with in-memory databases
	tmpFile := t.TempDir() + "/test.db"
	db, err := storage.New(tmpFile, 168*time.Hour)
	require.NoError(t, err)

	cleanup := func() {
		_ = db.Close()
	}

	return db, cleanup
}

func TestNewManager(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	client := scraper.NewClient(30*time.Second, 2)
	log := logger.New("info")
	manager := NewManager(db, client, log)

	assert.NotNil(t, manager)
	assert.False(t, manager.IsLoaded())
	assert.Equal(t, 0, manager.Count())
}

func TestGetRandomStickerWithFallback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	client := scraper.NewClient(30*time.Second, 2)
	log := logger.New("info")
	manager := NewManager(db, client, log)

	// Test fallback when no stickers loaded
	sticker := manager.GetRandomSticker()
	assert.NotEmpty(t, sticker)
	assert.Contains(t, sticker, "ui-avatars.com")
}

func TestGetRandomStickerWithLoadedStickers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	client := scraper.NewClient(30*time.Second, 2)
	log := logger.New("info")
	manager := NewManager(db, client, log)

	// Mock some stickers in database
	testStickers := []storage.Sticker{
		{URL: "https://spy-family.net/sticker1.png", Source: "spy_family", CachedAt: time.Now().Unix(), SuccessCount: 0, FailureCount: 0},
		{URL: "https://spy-family.net/sticker2.png", Source: "spy_family", CachedAt: time.Now().Unix(), SuccessCount: 0, FailureCount: 0},
		{URL: "https://ichigoproduction.com/sticker1.png", Source: "ichigo", CachedAt: time.Now().Unix(), SuccessCount: 0, FailureCount: 0},
	}

	for _, s := range testStickers {
		err := db.SaveSticker(ctx, &s)
		require.NoError(t, err)
	}

	// Load stickers from database
	err := manager.LoadStickers(ctx)
	require.NoError(t, err)

	assert.True(t, manager.IsLoaded())
	assert.Equal(t, 3, manager.Count())

	// Test random selection
	sticker := manager.GetRandomSticker()
	assert.NotEmpty(t, sticker)
	assert.True(t,
		sticker == "https://spy-family.net/sticker1.png" ||
			sticker == "https://spy-family.net/sticker2.png" ||
			sticker == "https://ichigoproduction.com/sticker1.png",
	)
}

func TestLoadStickersFromDatabase(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	client := scraper.NewClient(30*time.Second, 2)
	log := logger.New("info")
	manager := NewManager(db, client, log)

	// Save stickers to database
	testStickers := []storage.Sticker{
		{URL: "https://spy-family.net/test1.png", Source: "spy_family", CachedAt: time.Now().Unix(), SuccessCount: 5, FailureCount: 0},
		{URL: "https://spy-family.net/test2.png", Source: "spy_family", CachedAt: time.Now().Unix(), SuccessCount: 3, FailureCount: 1},
	}

	for _, s := range testStickers {
		err := db.SaveSticker(ctx, &s)
		require.NoError(t, err)
	}

	// Load stickers
	err := manager.LoadStickers(ctx)
	require.NoError(t, err)

	assert.True(t, manager.IsLoaded())
	assert.Equal(t, 2, manager.Count())
}

func TestGenerateFallbackStickers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	client := scraper.NewClient(30*time.Second, 2)
	log := logger.New("info")
	manager := NewManager(db, client, log)

	fallbacks := manager.generateFallbackStickers()

	assert.Equal(t, 20, len(fallbacks))
	for _, url := range fallbacks {
		assert.Contains(t, url, "ui-avatars.com")
		assert.Contains(t, url, "name=")
		assert.Contains(t, url, "size=256")
		assert.Contains(t, url, "background=")
	}
}

// TestFetchAndSaveStickers is an integration test that hits real websites
// Removed in favor of unit tests only. Run manual tests with `task warmup` if needed.
// Integration tests should be run separately, not as part of standard unit test suite.

func TestGetStats(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	client := scraper.NewClient(30*time.Second, 2)
	log := logger.New("info")
	manager := NewManager(db, client, log)

	// Save stickers with different sources
	testStickers := []storage.Sticker{
		{URL: "https://spy-family.net/s1.png", Source: "spy_family", CachedAt: time.Now().Unix()},
		{URL: "https://spy-family.net/s2.png", Source: "spy_family", CachedAt: time.Now().Unix()},
		{URL: "https://spy-family.net/s3.png", Source: "spy_family", CachedAt: time.Now().Unix()},
		{URL: "https://ichigoproduction.com/i1.png", Source: "ichigo", CachedAt: time.Now().Unix()},
		{URL: "https://ui-avatars.com/f1", Source: "fallback", CachedAt: time.Now().Unix()},
	}

	for _, s := range testStickers {
		err := db.SaveSticker(ctx, &s)
		require.NoError(t, err)
	}

	stats, err := manager.GetStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, 3, stats["spy_family"])
	assert.Equal(t, 1, stats["ichigo"])
	assert.Equal(t, 1, stats["fallback"])
}
