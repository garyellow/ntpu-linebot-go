package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
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

	// Create metrics
	m := metrics.New(registry)
	log.Info("Metrics initialized")

	// Create scraper client
	scraperClient := scraper.NewClient(
		cfg.ScraperTimeout,
		cfg.ScraperWorkers,
		cfg.ScraperMinDelay,
		cfg.ScraperMaxDelay,
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
		Workers: cfg.ScraperWorkers,
		Timeout: cfg.WarmupTimeout,
		Reset:   false, // Never reset in production
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
	router.Use(loggingMiddleware(log))

	// Setup routes
	setupRoutes(router, webhookHandler, db, registry, scraperClient, stickerManager)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start background goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// System metrics updater
	wg.Add(1)
	go func() {
		defer wg.Done()
		updateSystemMetrics(ctx, m, log)
	}()

	// Cache cleanup goroutine
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
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "https://github.com/garyellow/ntpu-linebot-go")
	})

	// Health check endpoints
	// Liveness Probe - checks if the application is alive (minimal check)
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Readiness Probe - checks if the application is ready to serve traffic (full dependency check)
	router.GET("/ready", func(c *gin.Context) {
		// Check database connection
		if err := db.Conn().Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": "database unavailable",
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
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", seaURLs[0], nil)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					seaAvailable = true
				}
			}
		}

		lmsURLs := scraperClient.GetBaseURLs("lms")
		if len(lmsURLs) > 0 {
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", lmsURLs[0], nil)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					lmsAvailable = true
				}
			}
		}

		// Check cache data availability
		studentCount, _ := db.CountStudents()
		contactCount, _ := db.CountContacts()
		courseCount, _ := db.CountCourses()
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
	})

	// LINE webhook callback endpoint
	router.POST("/callback", webhookHandler.Handle)

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
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
		} else if status >= 500 {
			entry.Error("Request failed")
		} else if status >= 400 {
			entry.Warn("Request completed with client error")
		} else {
			entry.Debug("Request completed")
		}
	}
}

// updateSystemMetrics periodically updates system metrics
func updateSystemMetrics(ctx context.Context, m *metrics.Metrics, log *logger.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			goroutines := runtime.NumGoroutine()
			memoryBytes := memStats.Alloc

			m.UpdateSystemMetrics(goroutines, memoryBytes)

			log.WithField("goroutines", goroutines).
				WithField("memory_mb", memoryBytes/1024/1024).
				Debug("Updated system metrics")
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
			performCacheCleanup(db, ttl, log)
		case <-ticker.C:
			performCacheCleanup(db, ttl, log)
		}
	}
}

// performCacheCleanup executes cache cleanup operation
func performCacheCleanup(db *storage.DB, ttl time.Duration, log *logger.Logger) {
	log.Info("Starting cache cleanup...")

	var totalDeleted int

	// Cleanup students
	if err := db.DeleteExpiredStudents(ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired students")
	} else {
		count, _ := db.CountStudents()
		log.WithField("remaining", count).Debug("Students cleanup complete")
	}

	// Cleanup contacts
	if err := db.DeleteExpiredContacts(ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired contacts")
	} else {
		count, _ := db.CountContacts()
		log.WithField("remaining", count).Debug("Contacts cleanup complete")
	}

	// Cleanup courses
	if err := db.DeleteExpiredCourses(ttl); err != nil {
		log.WithError(err).Error("Failed to cleanup expired courses")
	} else {
		count, _ := db.CountCourses()
		log.WithField("remaining", count).Debug("Courses cleanup complete")
	}

	// Cleanup stickers
	if err := db.CleanupExpiredStickers(); err != nil {
		log.WithError(err).Error("Failed to cleanup expired stickers")
	} else {
		count, _ := db.CountStickers()
		log.WithField("remaining", count).Debug("Stickers cleanup complete")
	}

	// Run SQLite VACUUM to reclaim space (optional, may be slow)
	if _, err := db.Conn().Exec("VACUUM"); err != nil {
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
		stats, _ := stickerManager.GetStats()
		log.WithField("count", count).
			WithField("stats", stats).
			Info("Sticker refresh complete")
	}
}
