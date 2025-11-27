// Package main provides the LINE bot server entry point.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/timeouts"
	"github.com/garyellow/ntpu-linebot-go/internal/warmup"
	"github.com/garyellow/ntpu-linebot-go/internal/webhook"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.LogLevel)
	log.Info("Starting NTPU LineBot Server")

	// Connect to database with configured TTL
	db, err := storage.New(cfg.SQLitePath, cfg.CacheTTL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer func() { _ = db.Close() }()
	log.WithField("path", cfg.SQLitePath).
		WithField("cache_ttl", cfg.CacheTTL).
		Info("Database connected")

	// Create Prometheus registry
	registry := prometheus.NewRegistry()

	// Register Go and process collectors
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewBuildInfoCollector())

	// Create metrics
	m := metrics.New(registry)
	log.Info("Metrics initialized")

	// Set metrics recorder for database integrity checks
	db.SetMetrics(m)

	// Create scraper client
	scraperClient := scraper.NewClient(
		cfg.ScraperTimeout,
		cfg.ScraperMaxRetries,
	)
	log.Info("Scraper client created")

	// Create sticker manager with database and scraper client
	stickerManager := sticker.NewManager(db, scraperClient, log)
	log.Info("Sticker manager created")

	// Start background cache warming (non-blocking)
	// Warmup runs concurrently with server startup
	warmupCtx, warmupCancel := context.WithTimeout(context.Background(), cfg.WarmupTimeout)
	defer warmupCancel()

	warmup.RunInBackground(warmupCtx, db, scraperClient, stickerManager, log, warmup.Options{
		Modules: warmup.ParseModules(cfg.WarmupModules),
		Timeout: cfg.WarmupTimeout,
		Reset:   false, // Never reset in production
		Metrics: m,     // Pass metrics for monitoring
	})
	log.Info("Background cache warming started")

	// Create webhook handler
	webhookHandler, err := webhook.NewHandler(
		cfg.LineChannelSecret,
		cfg.LineChannelToken,
		db,
		scraperClient,
		m,
		log,
		stickerManager,
		cfg.WebhookTimeout,
		cfg.UserRateLimitTokens,
		cfg.UserRateLimitRefillRate,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create webhook handler")
	}
	log.Info("Webhook handler created")

	// Set Gin mode based on log level
	if cfg.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.New()

	// Add middleware
	router.Use(gin.Recovery())
	router.Use(securityHeadersMiddleware())
	router.Use(loggingMiddleware(log))

	// Setup routes
	setupRoutes(router, webhookHandler, db, registry, scraperClient, stickerManager)

	// Create HTTP server with timeouts optimized for LINE webhook handling
	// See internal/timeouts/timeouts.go for detailed explanations
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  timeouts.WebhookHTTPRead,
		WriteTimeout: timeouts.WebhookHTTPWrite,
		IdleTimeout:  timeouts.WebhookHTTPIdle,
	}

	// Start background goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Cache cleanup goroutine (every 12 hours)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cleanupExpiredCache(ctx, db, cfg.CacheTTL, log)
	}()

	// Sticker refresh goroutine (every 24 hours)
	wg.Add(1)
	go func() {
		defer wg.Done()
		refreshStickers(ctx, stickerManager, log)
	}()

	// Proactive cache warmup goroutine (daily at 3:00 AM)
	// Refreshes data approaching Soft TTL to prevent user-triggered scraping
	wg.Add(1)
	go func() {
		defer wg.Done()
		proactiveWarmup(ctx, db, scraperClient, stickerManager, log, cfg)
	}()

	// Cache size metrics updater goroutine (every 5 minutes)
	// Updates Prometheus gauge metrics with current cache entry counts
	wg.Add(1)
	go func() {
		defer wg.Done()
		updateCacheSizeMetrics(ctx, db, stickerManager, m, log)
	}()

	// Start server in goroutine
	go func() {
		log.WithField("port", cfg.Port).Info("Server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Failed to start server")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Cancel context to stop metrics updater
	cancel()

	// Wait for goroutines to finish (with timeout)
	goDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(goDone)
	}()

	select {
	case <-goDone:
		log.Info("All background goroutines stopped")
	case <-time.After(5 * time.Second):
		log.Warn("Timeout waiting for goroutines to stop")
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	// Shutdown server gracefully
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server forced to shutdown")
	}

	// Close database connection
	if err := db.Close(); err != nil {
		log.WithError(err).Error("Failed to close database")
	}

	log.Info("Server stopped")
}

// setupRoutes configures all HTTP routes
func setupRoutes(router *gin.Engine, webhookHandler *webhook.Handler, db *storage.DB, registry *prometheus.Registry, scraperClient *scraper.Client, stickerManager *sticker.Manager) {
	// Root endpoint - redirect to GitHub
	rootHandler := func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "https://github.com/garyellow/ntpu-linebot-go")
	}
	router.GET("/", rootHandler)
	router.HEAD("/", rootHandler)

	// Health check endpoints
	// Liveness Probe - checks if the application is alive (minimal check)
	// This should NEVER check dependencies - only that the process is running
	healthHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
	router.GET("/healthz", healthHandler)
	router.HEAD("/healthz", healthHandler)

	// Readiness Probe - checks if the application is ready to serve traffic (full dependency check)
	readyHandler := func(c *gin.Context) {
		// Check database connections (both reader and writer)
		if err := db.Reader().Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": "database reader unavailable",
			})
			return
		}
		if err := db.Writer().Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": "database writer unavailable",
			})
			return
		}

		// Check scraper URLs availability (quick check, just try first URL)
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		seaAvailable := false
		lmsAvailable := false

		// Only check first URL in failover list (for speed)
		seaURLs := scraperClient.GetBaseURLs("sea")
		if len(seaURLs) > 0 {
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", seaURLs[0], http.NoBody)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					seaAvailable = true
				}
			}
		}

		lmsURLs := scraperClient.GetBaseURLs("lms")
		if len(lmsURLs) > 0 {
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", lmsURLs[0], http.NoBody)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					lmsAvailable = true
				}
			}
		}

		// Check cache data availability
		studentCount, _ := db.CountStudents(c.Request.Context())
		contactCount, _ := db.CountContacts(c.Request.Context())
		courseCount, _ := db.CountCourses(c.Request.Context())
		stickerCount := stickerManager.Count()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ready",
			"database": "connected",
			"scrapers": gin.H{
				"sea": seaAvailable,
				"lms": lmsAvailable,
			},
			"cache": gin.H{
				"students": studentCount,
				"contacts": contactCount,
				"courses":  courseCount,
				"stickers": stickerCount,
			},
		})
	}
	router.GET("/ready", readyHandler)
	router.HEAD("/ready", readyHandler)

	// LINE webhook callback endpoint
	router.POST("/callback", webhookHandler.Handle)

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
}

// securityHeadersMiddleware adds security headers to all responses
// Reference: https://gin-gonic.com/en/docs/examples/security-headers
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")
		// Enable XSS filter in browsers
		c.Header("X-XSS-Protection", "1; mode=block")
		// Strict referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		// Restrict permissions
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// Content Security Policy - prevent XSS attacks
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Process request
		c.Next()

		// Log request
		duration := time.Since(start)
		status := c.Writer.Status()

		entry := log.WithField("method", method).
			WithField("path", path).
			WithField("status", status).
			WithField("duration_ms", duration.Milliseconds()).
			WithField("ip", c.ClientIP())

		if len(c.Errors) > 0 {
			entry.WithField("errors", c.Errors.String()).Error("Request completed with errors")
		} else {
			switch {
			case status >= 500:
				entry.Error("Request failed")
			case status >= 400:
				entry.Warn("Request completed with client error")
			default:
				entry.Debug("Request completed")
			}
		}
	}
}

// cleanupExpiredCache periodically removes expired cache entries from database
func cleanupExpiredCache(ctx context.Context, db *storage.DB, ttl time.Duration, log *logger.Logger) {
	// Run cleanup every 12 hours
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	// Run initial cleanup after 5 minutes
	initialDelay := time.NewTimer(5 * time.Minute)
	defer initialDelay.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-initialDelay.C:
			performCacheCleanup(ctx, db, ttl, log)
		case <-ticker.C:
			performCacheCleanup(ctx, db, ttl, log)
		}
	}
}

// performCacheCleanup executes cache cleanup operation
func performCacheCleanup(ctx context.Context, db *storage.DB, ttl time.Duration, log *logger.Logger) {
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

	// Run SQLite VACUUM to reclaim space (optional, may be slow)
	if _, err := db.Writer().Exec("VACUUM"); err != nil {
		log.WithError(err).Warn("Failed to vacuum database")
	} else {
		log.Debug("Database vacuumed successfully")
	}

	log.WithField("total_deleted", totalDeleted).Info("Cache cleanup complete")
}

// refreshStickers periodically refreshes stickers from web sources
func refreshStickers(ctx context.Context, stickerManager *sticker.Manager, log *logger.Logger) {
	// Refresh stickers every 24 hours
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run initial refresh after 1 hour (to let server stabilize)
	initialDelay := time.NewTimer(1 * time.Hour)
	defer initialDelay.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-initialDelay.C:
			performStickerRefresh(ctx, stickerManager, log)
		case <-ticker.C:
			performStickerRefresh(ctx, stickerManager, log)
		}
	}
}

// performStickerRefresh executes sticker refresh operation
func performStickerRefresh(ctx context.Context, stickerManager *sticker.Manager, log *logger.Logger) {
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
}

// proactiveWarmup runs cache warmup proactively to prevent user-triggered scraping
// Uses Soft TTL strategy: refresh data before it expires to ensure users always get cached data
// Runs daily at 3:00 AM to minimize impact on system resources
func proactiveWarmup(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, cfg *config.Config) {
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
			Debug("Proactive warmup scheduled")

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			performProactiveWarmup(ctx, db, client, stickerMgr, log, cfg)
		}
	}
}

// performProactiveWarmup checks for expiring data and triggers warmup if needed
func performProactiveWarmup(ctx context.Context, db *storage.DB, client *scraper.Client, stickerMgr *sticker.Manager, log *logger.Logger, cfg *config.Config) {
	log.Info("Starting proactive cache warmup check...")

	// Check how much data is approaching expiration (past Soft TTL but not Hard TTL)
	expiringStudents, _ := db.CountExpiringStudents(ctx, cfg.SoftTTL)
	expiringCourses, _ := db.CountExpiringCourses(ctx, cfg.SoftTTL)
	expiringContacts, _ := db.CountExpiringContacts(ctx, cfg.SoftTTL)

	totalExpiring := expiringStudents + expiringCourses + expiringContacts

	log.WithFields(map[string]interface{}{
		"expiring_students": expiringStudents,
		"expiring_courses":  expiringCourses,
		"expiring_contacts": expiringContacts,
		"total_expiring":    totalExpiring,
		"soft_ttl_hours":    cfg.SoftTTL.Hours(),
	}).Info("Checked expiring cache entries")

	// Only run warmup if there's data approaching expiration
	// This prevents unnecessary scraping when cache is fresh
	if totalExpiring == 0 {
		log.Info("No expiring data found, skipping proactive warmup")
		return
	}

	// Determine which modules need warming based on expiring data
	var modules []string
	if expiringStudents > 0 {
		modules = append(modules, "id")
	}
	if expiringCourses > 0 {
		modules = append(modules, "course")
	}
	if expiringContacts > 0 {
		modules = append(modules, "contact")
	}

	log.WithField("modules", modules).Info("Starting proactive warmup for expiring modules")

	// Create warmup context with timeout
	warmupCtx, cancel := context.WithTimeout(ctx, cfg.WarmupTimeout)
	defer cancel()

	// Run warmup (non-blocking, logs progress internally)
	stats, err := warmup.Run(warmupCtx, db, client, stickerMgr, log, warmup.Options{
		Modules: modules,
		Timeout: cfg.WarmupTimeout,
		Reset:   false, // Never reset existing data
	})

	if err != nil {
		log.WithError(err).Warn("Proactive warmup finished with errors")
	} else {
		log.WithFields(map[string]interface{}{
			"students_refreshed": stats.Students.Load(),
			"courses_refreshed":  stats.Courses.Load(),
			"contacts_refreshed": stats.Contacts.Load(),
		}).Info("Proactive warmup completed successfully")
	}
}

// updateCacheSizeMetrics periodically updates cache size gauge metrics
func updateCacheSizeMetrics(ctx context.Context, db *storage.DB, stickerManager *sticker.Manager, m *metrics.Metrics, log *logger.Logger) {
	// Update metrics every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Run initial update immediately
	performCacheSizeUpdate(ctx, db, stickerManager, m, log)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			performCacheSizeUpdate(ctx, db, stickerManager, m, log)
		}
	}
}

// performCacheSizeUpdate updates cache size metrics
func performCacheSizeUpdate(ctx context.Context, db *storage.DB, stickerManager *sticker.Manager, m *metrics.Metrics, log *logger.Logger) {
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
	m.SetCacheSize("stickers", stickerManager.Count())
}
