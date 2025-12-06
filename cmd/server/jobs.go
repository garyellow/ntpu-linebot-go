// Package main provides the LINE bot server entry point.
package main

import (
	"context"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/warmup"
)

// cleanupExpiredCache periodically removes expired cache entries from database
func cleanupExpiredCache(ctx context.Context, db *storage.DB, ttl time.Duration, m *metrics.Metrics, log *logger.Logger) {
	// Run initial cleanup after configured delay to let server stabilize
	select {
	case <-ctx.Done():
		return
	case <-time.After(config.CacheCleanupInitialDelay):
		performCacheCleanup(ctx, db, ttl, m, log)
	}

	// Then run cleanup at configured interval
	ticker := time.NewTicker(config.CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			performCacheCleanup(ctx, db, ttl, m, log)
		}
	}
}

// performCacheCleanup executes cache cleanup operation
func performCacheCleanup(ctx context.Context, db *storage.DB, ttl time.Duration, m *metrics.Metrics, log *logger.Logger) {
	startTime := time.Now()
	log.Info("Starting cache cleanup...")

	var totalDeleted int64

	// Cleanup students
	if deleted, err := db.DeleteExpiredStudents(ctx, ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired students")
	} else {
		totalDeleted += deleted
		count, _ := db.CountStudents(ctx)
		log.WithFields(map[string]interface{}{
			"deleted":   deleted,
			"remaining": count,
		}).Debug("Students cleanup complete")
	}

	// Cleanup contacts
	if deleted, err := db.DeleteExpiredContacts(ctx, ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired contacts")
	} else {
		totalDeleted += deleted
		count, _ := db.CountContacts(ctx)
		log.WithFields(map[string]interface{}{
			"deleted":   deleted,
			"remaining": count,
		}).Debug("Contacts cleanup complete")
	}

	// Cleanup courses
	if deleted, err := db.DeleteExpiredCourses(ctx, ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired courses")
	} else {
		totalDeleted += deleted
		count, _ := db.CountCourses(ctx)
		log.WithFields(map[string]interface{}{
			"deleted":   deleted,
			"remaining": count,
		}).Debug("Courses cleanup complete")
	}

	// Cleanup historical courses (uses same TTL as regular courses)
	if deleted, err := db.DeleteExpiredHistoricalCourses(ctx, ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired historical courses")
	} else {
		totalDeleted += deleted
		count, _ := db.CountHistoricalCourses(ctx)
		log.WithFields(map[string]interface{}{
			"deleted":   deleted,
			"remaining": count,
		}).Debug("Historical courses cleanup complete")
	}

	// Cleanup stickers
	if deleted, err := db.CleanupExpiredStickers(ctx); err != nil {
		log.WithError(err).Error("Failed to cleanup expired stickers")
	} else {
		totalDeleted += deleted
		count, _ := db.CountStickers(ctx)
		log.WithFields(map[string]interface{}{
			"deleted":   deleted,
			"remaining": count,
		}).Debug("Stickers cleanup complete")
	}

	// Cleanup syllabi
	if deleted, err := db.DeleteExpiredSyllabi(ctx, ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired syllabi")
	} else {
		totalDeleted += deleted
		count, _ := db.CountSyllabi(ctx)
		log.WithFields(map[string]interface{}{
			"deleted":   deleted,
			"remaining": count,
		}).Debug("Syllabi cleanup complete")
	}

	// Run SQLite VACUUM to reclaim space (optional, may be slow)
	if _, err := db.Writer().Exec("VACUUM"); err != nil {
		log.WithError(err).Warn("Failed to vacuum database")
	} else {
		log.Debug("Database vacuumed successfully")
	}

	// Record job metrics
	if m != nil {
		m.RecordJob("cleanup", "all", time.Since(startTime).Seconds())
	}

	log.WithField("total_deleted", totalDeleted).Info("Cache cleanup complete")
}

// refreshStickers periodically refreshes stickers from web sources
func refreshStickers(ctx context.Context, stickerManager *sticker.Manager, m *metrics.Metrics, log *logger.Logger) {
	// Run initial refresh after configured delay to let server stabilize
	select {
	case <-ctx.Done():
		return
	case <-time.After(config.StickerRefreshInitialDelay):
		performStickerRefresh(ctx, stickerManager, m, log)
	}

	// Then refresh at configured interval
	ticker := time.NewTicker(config.StickerRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			performStickerRefresh(ctx, stickerManager, m, log)
		}
	}
}

// performStickerRefresh executes sticker refresh operation
func performStickerRefresh(ctx context.Context, stickerManager *sticker.Manager, m *metrics.Metrics, log *logger.Logger) {
	startTime := time.Now()
	log.Info("Starting periodic sticker refresh...")

	refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := stickerManager.RefreshStickers(refreshCtx); err != nil {
		log.WithError(err).Error("Failed to refresh stickers")
	} else {
		count := stickerManager.Count()
		stats, _ := stickerManager.GetStats(refreshCtx)
		log.WithField("count", count).
			WithField("stats", stats).
			Info("Sticker refresh complete")
	}

	// Record job metrics (record even on error to track duration)
	if m != nil {
		m.RecordJob("sticker_refresh", "all", time.Since(startTime).Seconds())
	}
}

// proactiveWarmup runs daily cache warmup to ensure data freshness
// Refreshes all modules unconditionally every day at 3:00 AM
// Data not updated within 7 days (Hard TTL) will be deleted by cleanup job
func proactiveWarmup(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, cfg *config.Config, bm25Index *rag.BM25Index) {
	const targetHour = 3 // 3:00 AM

	for {
		// Recalculate wait time on each iteration to ensure accurate scheduling
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), targetHour, 0, 0, 0, now.Location())
		if now.After(next) {
			// Already past today's target time, schedule for tomorrow
			next = next.Add(24 * time.Hour)
		}
		waitDuration := next.Sub(now)

		log.WithField("next_run", next.Format("2006-01-02 15:04:05")).
			Debug("Daily warmup scheduled")

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			performProactiveWarmup(ctx, db, client, stickerMgr, log, cfg, bm25Index)
		}
	}
}

// performProactiveWarmup executes daily cache warmup for all modules
// Runs unconditionally every day to ensure data freshness
// Uses configured modules from WARMUP_MODULES (excluding sticker, which is handled separately)
func performProactiveWarmup(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, cfg *config.Config, bm25Index *rag.BM25Index) {
	log.Info("Starting daily proactive cache warmup...")

	// Parse configured modules, but exclude sticker (handled separately by refreshStickers)
	allModules := warmup.ParseModules(cfg.WarmupModules)
	var modules []string
	for _, m := range allModules {
		if m != "sticker" {
			modules = append(modules, m)
		}
	}

	// If no modules configured (empty string), use default data modules
	if len(modules) == 0 {
		modules = []string{"id", "contact", "course"}
	}

	log.WithField("modules", modules).Info("Running daily warmup for configured modules")

	// Run warmup (logs progress internally, runs until completion)
	stats, err := warmup.Run(ctx, db, client, stickerMgr, log, warmup.Options{
		Modules:   modules,
		Reset:     false,     // Never reset existing data
		BM25Index: bm25Index, // Pass BM25Index for syllabus module
	})

	if err != nil {
		log.WithError(err).Warn("Daily proactive warmup finished with errors")
	} else {
		log.WithFields(map[string]interface{}{
			"students_refreshed": stats.Students.Load(),
			"courses_refreshed":  stats.Courses.Load(),
			"contacts_refreshed": stats.Contacts.Load(),
			"syllabi_refreshed":  stats.Syllabi.Load(),
		}).Info("Daily proactive warmup completed successfully")
	}
}

// updateCacheSizeMetrics periodically updates cache size gauge metrics
func updateCacheSizeMetrics(ctx context.Context, db *storage.DB, stickerManager *sticker.Manager, bm25Index *rag.BM25Index, m *metrics.Metrics, log *logger.Logger) {
	// Update metrics at configured interval
	ticker := time.NewTicker(config.MetricsUpdateInterval)
	defer ticker.Stop()

	// Run initial update immediately
	performCacheSizeUpdate(ctx, db, stickerManager, bm25Index, m, log)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			performCacheSizeUpdate(ctx, db, stickerManager, bm25Index, m, log)
		}
	}
}

// performCacheSizeUpdate updates cache size metrics
func performCacheSizeUpdate(ctx context.Context, db *storage.DB, stickerManager *sticker.Manager, bm25Index *rag.BM25Index, m *metrics.Metrics, log *logger.Logger) {
	if studentCount, err := db.CountStudents(ctx); err == nil {
		m.SetCacheSize("students", studentCount)
	} else {
		log.WithError(err).Debug("Failed to count students for metrics")
	}
	if contactCount, err := db.CountContacts(ctx); err == nil {
		m.SetCacheSize("contacts", contactCount)
	} else {
		log.WithError(err).Debug("Failed to count contacts for metrics")
	}
	if courseCount, err := db.CountCourses(ctx); err == nil {
		m.SetCacheSize("courses", courseCount)
	} else {
		log.WithError(err).Debug("Failed to count courses for metrics")
	}
	if historicalCount, err := db.CountHistoricalCourses(ctx); err == nil {
		m.SetCacheSize("historical_courses", historicalCount)
	} else {
		log.WithError(err).Debug("Failed to count historical courses for metrics")
	}
	if syllabiCount, err := db.CountSyllabi(ctx); err == nil {
		m.SetCacheSize("syllabi", syllabiCount)
	} else {
		log.WithError(err).Debug("Failed to count syllabi for metrics")
	}
	m.SetCacheSize("stickers", stickerManager.Count())

	// Update BM25 index size if enabled
	if bm25Index != nil && bm25Index.IsEnabled() {
		m.SetIndexSize("bm25", bm25Index.Count())
	}
}
