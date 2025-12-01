// Package sticker provides avatar sticker management for LINE bot messages.
// It handles scraping, caching, and weighted random selection of sticker URLs.
package sticker

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// Manager manages sticker URLs for LINE bot avatars with SQLite persistence
type Manager struct {
	stickers []string
	mu       sync.RWMutex
	loaded   bool
	db       *storage.DB
	client   *scraper.Client
	logger   *logger.Logger
}

// NewManager creates a new sticker manager with database persistence
func NewManager(db *storage.DB, client *scraper.Client, log *logger.Logger) *Manager {
	return &Manager{
		stickers: make([]string, 0),
		loaded:   false,
		db:       db,
		client:   client,
		logger:   log,
	}
}

// LoadStickers loads stickers from database first, then fetches from web if expired/missing
func (m *Manager) LoadStickers(ctx context.Context) error {
	// Step 1: Try loading from database
	dbStickers, err := m.db.GetAllStickers(ctx)
	if err != nil {
		m.logger.WithError(err).Warn("Failed to load stickers from database, will fetch from web")
	} else if len(dbStickers) > 0 {
		// Load stickers from database cache
		m.mu.Lock()
		m.stickers = make([]string, 0, len(dbStickers))
		for _, s := range dbStickers {
			m.stickers = append(m.stickers, s.URL)
		}
		m.loaded = true
		m.mu.Unlock()
		m.logger.WithField("count", len(dbStickers)).Info("Loaded stickers from database cache")
		return nil
	}

	// Step 2: No valid cache, fetch from web sources
	m.logger.Info("No cached stickers found, fetching from web sources")
	return m.FetchAndSaveStickers(ctx)
}

// FetchAndSaveStickers fetches stickers from web and saves to database
func (m *Manager) FetchAndSaveStickers(ctx context.Context) error {
	// Spy Family URLs (8 sources)
	spyFamilyURLs := []string{
		"https://spy-family.net/tvseries/special/special1_season1.php",
		"https://spy-family.net/tvseries/special/special2_season1.php",
		"https://spy-family.net/tvseries/special/special9_season1.php",
		"https://spy-family.net/tvseries/special/special13_season1.php",
		"https://spy-family.net/tvseries/special/special16_season1.php",
		"https://spy-family.net/tvseries/special/special17_season1.php",
		"https://spy-family.net/tvseries/special/special3_season2.php",
		"https://spy-family.net/tvseries/special/special10.php",
	}

	// Ichigo Production URL (1 source)
	ichigoURL := "https://ichigoproduction.com/Season1/special/present_icon.html"

	// Create channels for concurrent fetching with retry
	type result struct {
		stickers []string
		source   string
		err      error
	}

	results := make(chan result, len(spyFamilyURLs)+1)

	// Fetch Spy Family stickers concurrently with retry
	for _, url := range spyFamilyURLs {
		go func(u string) {
			defer func() {
				if r := recover(); r != nil {
					m.logger.WithField("panic", r).WithField("url", u).Error("Panic in sticker fetch")
					results <- result{source: "spy_family", err: fmt.Errorf("panic: %v", r)}
				}
			}()
			stickers, err := m.fetchWithRetry(ctx, u, "spy_family", m.fetchSpyFamilyStickers, 3)
			results <- result{stickers: stickers, source: "spy_family", err: err}
		}(url)
	}

	// Fetch Ichigo Production stickers with retry
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.WithField("panic", r).Error("Panic in ichigo sticker fetch")
				results <- result{source: "ichigo", err: fmt.Errorf("panic: %v", r)}
			}
		}()
		stickers, err := m.fetchWithRetry(ctx, ichigoURL, "ichigo", m.fetchIchigoStickers, 3)
		results <- result{stickers: stickers, source: "ichigo", err: err}
	}()

	// Collect results
	allStickers := make([]string, 0)
	successCount := 0
	errorCount := 0

	for i := 0; i < len(spyFamilyURLs)+1; i++ {
		res := <-results
		if res.err != nil {
			errorCount++
			m.logger.WithError(res.err).WithField("source", res.source).Warn("Failed to fetch from source")
			continue
		}
		successCount++
		allStickers = append(allStickers, res.stickers...)

		// Save each sticker to database
		for _, stickerURL := range res.stickers {
			sticker := &storage.Sticker{
				URL:          stickerURL,
				Source:       res.source,
				CachedAt:     time.Now().Unix(),
				SuccessCount: 1,
				FailureCount: 0,
			}
			if err := m.db.SaveSticker(ctx, sticker); err != nil {
				m.logger.WithError(err).WithField("url", stickerURL).Warn("Failed to save sticker")
			}
		}
	}

	// If no stickers fetched, generate fallback stickers
	if len(allStickers) == 0 {
		m.logger.Warn("All web sources failed, generating fallback stickers")
		fallbackStickers := m.generateFallbackStickers()
		allStickers = append(allStickers, fallbackStickers...)

		// Save fallback stickers to database
		for _, stickerURL := range fallbackStickers {
			sticker := &storage.Sticker{
				URL:          stickerURL,
				Source:       "fallback",
				CachedAt:     time.Now().Unix(),
				SuccessCount: 0,
				FailureCount: 0,
			}
			if err := m.db.SaveSticker(ctx, sticker); err != nil {
				m.logger.WithError(err).WithField("url", stickerURL).Warn("Failed to save fallback sticker")
			}
		}
	}

	// Update in-memory stickers list
	m.mu.Lock()
	m.stickers = allStickers
	m.loaded = true
	m.mu.Unlock()

	m.logger.WithFields(map[string]interface{}{
		"count":   len(allStickers),
		"success": successCount,
		"failed":  errorCount,
	}).Info("Loaded stickers")

	return nil
}

// fetchWithRetry retries fetching with exponential backoff
func (m *Manager) fetchWithRetry(ctx context.Context, url, _ string, fetchFunc func(context.Context, *scraper.Client, string) ([]string, error), maxRetries int) ([]string, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			m.logger.WithFields(map[string]interface{}{
				"attempt": attempt + 1,
				"url":     url,
				"backoff": backoff,
			}).Debug("Retrying sticker fetch")
			time.Sleep(backoff)
		}

		stickers, err := fetchFunc(ctx, m.client, url)
		if err == nil {
			return stickers, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// generateFallbackStickers creates 20 fallback sticker URLs using ui-avatars.com
func (m *Manager) generateFallbackStickers() []string {
	names := []string{
		"Anya", "Loid", "Yor", "Bond", "Damian",
		"Becky", "Fiona", "Franky", "Yuri", "Sylvia",
		"Ichigo", "Ai", "Kana", "Aqua", "Ruby",
		"Miyako", "Mem", "Akane", "Taiki", "Sarina",
	}

	backgrounds := []string{"FF6B6B", "4ECDC4", "45B7D1", "FFA07A", "98D8C8"}
	stickers := make([]string, 0, 20)

	for i, name := range names {
		bg := backgrounds[i%len(backgrounds)]
		url := fmt.Sprintf("https://ui-avatars.com/api/?name=%s&size=256&background=%s&color=fff", name, bg)
		stickers = append(stickers, url)
	}

	return stickers
}

// fetchSpyFamilyStickers fetches sticker URLs from Spy Family website
func (m *Manager) fetchSpyFamilyStickers(ctx context.Context, client *scraper.Client, url string) ([]string, error) {
	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Spy Family stickers from %s: %w", url, err)
	}

	stickers := make([]string, 0)
	baseURL := "https://spy-family.net/tvseries/"

	doc.Find("ul.icondlLists a[href$='.png']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		// Handle relative URLs (e.g., ../assets/img/special/01.png)
		if len(href) > 3 && href[:3] == "../" {
			// Remove ../ prefix and construct absolute URL
			relPath := href[3:] // Remove "../"
			absURL := baseURL + relPath
			stickers = append(stickers, absURL)
		} else if len(href) > 4 && href[:4] == "http" {
			// Already absolute URL
			stickers = append(stickers, href)
		} else if len(href) > 2 && href[:2] == "//" {
			// Protocol-relative URL
			stickers = append(stickers, "https:"+href)
		}
	})

	if len(stickers) == 0 {
		return nil, fmt.Errorf("no stickers found on %s", url)
	}

	return stickers, nil
}

// fetchIchigoStickers fetches sticker URLs from Ichigo Production website
func (m *Manager) fetchIchigoStickers(ctx context.Context, client *scraper.Client, url string) ([]string, error) {
	doc, err := client.GetDocument(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Ichigo stickers: %w", err)
	}

	stickers := make([]string, 0)
	baseURL := "https://ichigoproduction.com/Season1/"

	// Find all <img> tags with src attributes containing sticker content
	// (Ichigo uses img tags with query strings like .jpg?timestamp)
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists {
			return
		}
		// Skip non-sticker images (logo, icons, etc.)
		// Stickers are in core_sys/images/contents/ directory
		if !strings.Contains(src, "core_sys/images/contents/") {
			return
		}

		// Must contain .jpg (with or without query string)
		if !strings.Contains(src, ".jpg") {
			return
		}

		// Handle relative URLs (e.g., ../core_sys/images/...)
		if len(src) > 3 && src[:3] == "../" {
			// Remove ../ prefix and construct absolute URL
			relPath := src[3:]
			absURL := baseURL + relPath
			stickers = append(stickers, absURL)
		} else if len(src) > 4 && src[:4] == "http" {
			// Already absolute URL
			stickers = append(stickers, src)
		} else if len(src) > 2 && src[:2] == "//" {
			// Protocol-relative URL
			stickers = append(stickers, "https:"+src)
		}
	})

	if len(stickers) == 0 {
		return nil, fmt.Errorf("no stickers found on %s", url)
	}

	return stickers, nil
}

// RefreshStickers refreshes stickers from web sources (should be called periodically)
func (m *Manager) RefreshStickers(ctx context.Context) error {
	m.logger.Info("Starting periodic sticker refresh")
	if err := m.FetchAndSaveStickers(ctx); err != nil {
		m.logger.WithError(err).Error("Failed to refresh stickers")
		return err
	}
	m.logger.Info("Sticker refresh completed successfully")
	return nil
}

// CleanupExpiredStickers removes expired stickers from database
// Returns the number of deleted entries
func (m *Manager) CleanupExpiredStickers(ctx context.Context) (int64, error) {
	return m.db.CleanupExpiredStickers(ctx)
}

// GetRandomSticker returns a random sticker URL (guaranteed to never be empty)
// Uses a background goroutine for non-blocking counter updates.
//
//nolint:contextcheck // Internal goroutine intentionally uses context.Background() for fire-and-forget operation
func (m *Manager) GetRandomSticker() string {
	m.mu.RLock()
	stickers := m.stickers
	m.mu.RUnlock()

	// If stickers list is empty, use fallback immediately
	if len(stickers) == 0 {
		// Use a deterministic fallback based on current time
		names := []string{
			"Anya", "Loid", "Yor", "Bond", "Damian",
			"Becky", "Fiona", "Franky", "Yuri", "Sylvia",
		}
		backgrounds := []string{"FF6B6B", "4ECDC4", "45B7D1", "FFA07A", "98D8C8"}

		idx := int(time.Now().Unix()) % len(names)
		name := names[idx]
		bg := backgrounds[idx%len(backgrounds)]

		return fmt.Sprintf("https://ui-avatars.com/api/?name=%s&size=256&background=%s&color=fff", name, bg)
	}

	// Use crypto/rand for secure randomness
	var b [8]byte
	_, _ = rand.Read(b[:]) // Error ignored: crypto/rand.Read only fails on catastrophic system failures
	idx := int(binary.LittleEndian.Uint64(b[:])) % len(stickers)
	if idx < 0 {
		idx = -idx
	}
	selectedURL := stickers[idx]

	// Update success count in database (non-blocking)
	// Uses background context with timeout to prevent goroutine leaks on shutdown
	// Errors are intentionally ignored: sticker selection count is non-critical
	// and logging would spam logs on every message
	go func(url string) {
		//nolint:contextcheck // Intentionally using context.Background() for non-blocking fire-and-forget operation
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = m.db.UpdateStickerSuccess(ctx, url)
	}(selectedURL)

	return selectedURL
}

// IsLoaded returns whether stickers have been loaded
func (m *Manager) IsLoaded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loaded
}

// Count returns the number of loaded stickers
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.stickers)
}

// GetStats returns sticker statistics from database
func (m *Manager) GetStats(ctx context.Context) (map[string]int, error) {
	return m.db.GetStickerStats(ctx)
}
