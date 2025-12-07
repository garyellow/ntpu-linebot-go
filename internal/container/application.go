// Package container provides the Application type that represents the fully-configured app.
package container

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/warmup"
	"github.com/garyellow/ntpu-linebot-go/internal/webhook"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Application represents a fully-configured application with all dependencies injected.
type Application struct {
	cfg    *config.Config
	logger *logger.Logger

	// Core services (for lifecycle management)
	db             *storage.DB
	metrics        *metrics.Metrics
	registry       *prometheus.Registry
	scraperClient  *scraper.Client
	stickerManager *sticker.Manager

	// Webhook handler
	webhookHandler *webhook.Handler

	// HTTP server
	router *gin.Engine
	server *http.Server

	// GenAI components (for lifecycle management)
	bm25Index     *rag.BM25Index
	intentParser  genai.IntentParser
	queryExpander *genai.QueryExpander
}

// NewApplication creates a new Application with all dependencies injected.
func NewApplication(
	cfg *config.Config,
	logger *logger.Logger,
	db *storage.DB,
	metrics *metrics.Metrics,
	registry *prometheus.Registry,
	scraperClient *scraper.Client,
	stickerManager *sticker.Manager,
	webhookHandler *webhook.Handler,
	bm25Index *rag.BM25Index,
	intentParser genai.IntentParser,
	queryExpander *genai.QueryExpander,
) *Application {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(securityHeadersMiddleware())
	router.Use(loggingMiddleware(logger))

	app := &Application{
		cfg:            cfg,
		logger:         logger,
		db:             db,
		metrics:        metrics,
		registry:       registry,
		scraperClient:  scraperClient,
		stickerManager: stickerManager,
		webhookHandler: webhookHandler,
		router:         router,
		bm25Index:      bm25Index,
		intentParser:   intentParser,
		queryExpander:  queryExpander,
	}

	app.setupRoutes()
	return app
}

// Run starts the HTTP server and handles graceful shutdown.
func (a *Application) Run() error {
	log := a.logger

	// Create context for background jobs
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background jobs
	go a.startBackgroundJobs(ctx)

	// Create HTTP server
	a.server = &http.Server{
		Addr:         fmt.Sprintf(":%s", a.cfg.Port),
		Handler:      a.router,
		ReadTimeout:  config.WebhookHTTPRead,
		WriteTimeout: config.WebhookHTTPWrite,
		IdleTimeout:  config.WebhookHTTPIdle,
	}

	// Start server in goroutine
	go func() {
		log.WithField("port", a.cfg.Port).Info("Server starting")
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Server failed")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	// Cancel background jobs
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer shutdownCancel()

	// Shutdown HTTP server
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server forced to shutdown")
		return fmt.Errorf("server shutdown: %w", err)
	}

	// Close GenAI components
	if a.queryExpander != nil {
		_ = a.queryExpander.Close()
	}
	if a.intentParser != nil {
		_ = a.intentParser.Close()
	}

	// Close database
	if err := a.db.Close(); err != nil {
		log.WithError(err).Error("Failed to close database")
	}

	log.Info("Server exited")
	return nil
}

// setupRoutes configures all HTTP routes (using direct dependency access).
func (a *Application) setupRoutes() {
	// Root endpoint - redirect to GitHub
	rootHandler := func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "https://github.com/garyellow/ntpu-linebot-go")
	}
	a.router.GET("/", rootHandler)
	a.router.HEAD("/", rootHandler)

	// Health check endpoints
	healthHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
	a.router.GET("/healthz", healthHandler)
	a.router.HEAD("/healthz", healthHandler)

	// Readiness Probe
	readyHandler := func(ctx *gin.Context) {
		// Check database
		if err := a.db.Ready(ctx.Request.Context()); err != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": err.Error(),
			})
			return
		}

		// Check scraper URLs (quick check)
		checkCtx, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
		defer cancel()

		seaAvailable := false
		seaURLs := a.scraperClient.GetBaseURLs("sea")
		if len(seaURLs) > 0 {
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", seaURLs[0], http.NoBody)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					seaAvailable = true
				}
			}
		}

		lmsAvailable := false
		lmsURLs := a.scraperClient.GetBaseURLs("lms")
		if len(lmsURLs) > 0 {
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", lmsURLs[0], http.NoBody)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					lmsAvailable = true
				}
			}
		}

		// Check cache data
		studentCount, _ := a.db.CountStudents(ctx.Request.Context())
		contactCount, _ := a.db.CountContacts(ctx.Request.Context())
		courseCount, _ := a.db.CountCourses(ctx.Request.Context())
		stickerCount := a.stickerManager.Count()

		ctx.JSON(http.StatusOK, gin.H{
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
	a.router.GET("/ready", readyHandler)
	a.router.HEAD("/ready", readyHandler)

	// LINE webhook
	a.router.POST("/callback", a.webhookHandler.Handle)

	// Prometheus metrics
	a.router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(a.registry, promhttp.HandlerOpts{})))
}

// startBackgroundJobs starts all background jobs.
func (a *Application) startBackgroundJobs(ctx context.Context) {
	log := a.logger

	log.Info("Starting background jobs...")

	// Cache cleanup (every 12 hours)
	go a.cleanupExpiredCache(ctx)

	// Sticker refresh (every 24 hours)
	go a.refreshStickers(ctx)

	// Proactive warmup (daily at 3:00 AM)
	go a.proactiveWarmup(ctx)

	// Cache size metrics (every 5 minutes)
	go a.updateCacheSizeMetrics(ctx)
}

// cleanupExpiredCache periodically removes expired cache entries.
func (a *Application) cleanupExpiredCache(ctx context.Context) {
	// Initial delay
	select {
	case <-ctx.Done():
		return
	case <-time.After(config.CacheCleanupInitialDelay):
		a.performCacheCleanup(ctx)
	}

	// Periodic cleanup
	ticker := time.NewTicker(config.CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.performCacheCleanup(ctx)
		}
	}
}

// performCacheCleanup executes cache cleanup.
func (a *Application) performCacheCleanup(ctx context.Context) {
	startTime := time.Now()
	a.logger.Info("Starting cache cleanup...")

	var totalDeleted int64

	// Cleanup each cache type
	if deleted, err := a.db.DeleteExpiredStudents(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired students")
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredContacts(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired contacts")
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredCourses(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired courses")
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredSyllabi(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired syllabi")
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.CleanupExpiredStickers(ctx); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired stickers")
	} else {
		totalDeleted += deleted
	}

	// VACUUM database to reclaim space
	if _, err := a.db.Writer().Exec("VACUUM"); err != nil {
		a.logger.WithError(err).Warn("Failed to VACUUM database")
	} else {
		a.logger.Debug("Database vacuumed successfully")
	}

	a.logger.WithField("deleted", totalDeleted).
		WithField("duration_ms", time.Since(startTime).Milliseconds()).
		Info("Cache cleanup completed")
}

// refreshStickers periodically refreshes sticker cache.
func (a *Application) refreshStickers(ctx context.Context) {
	// Initial delay
	select {
	case <-ctx.Done():
		return
	case <-time.After(config.StickerRefreshInitialDelay):
		a.performStickerRefresh(ctx)
	}

	// Periodic refresh
	ticker := time.NewTicker(config.StickerRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.performStickerRefresh(ctx)
		}
	}
}

// performStickerRefresh executes sticker refresh.
func (a *Application) performStickerRefresh(ctx context.Context) {
	a.logger.Info("Starting periodic sticker refresh...")
	startTime := time.Now()

	refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := a.stickerManager.RefreshStickers(refreshCtx); err != nil {
		a.logger.WithError(err).Error("Failed to refresh stickers")
	} else {
		count := a.stickerManager.Count()
		stats, _ := a.stickerManager.GetStats(refreshCtx)
		a.logger.WithField("count", count).
			WithField("stats", stats).
			Info("Sticker refresh complete")
	}

	// Record job metrics
	if a.metrics != nil {
		a.metrics.RecordJob("sticker_refresh", "all", time.Since(startTime).Seconds())
	}
}

// proactiveWarmup performs daily cache warming at 3:00 AM.
func (a *Application) proactiveWarmup(ctx context.Context) {
	for {
		// Calculate next 3:00 AM
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		// Wait until 3:00 AM or context cancellation
		waitDuration := time.Until(next)
		a.logger.WithField("next_run", next.Format(time.RFC3339)).
			Info("Scheduled next proactive warmup")

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			a.performProactiveWarmup(ctx)
		}
	}
}

// performProactiveWarmup executes proactive warmup.
func (a *Application) performProactiveWarmup(ctx context.Context) {
	a.logger.Info("Starting proactive warmup...")
	startTime := time.Now()

	// Warmup all modules unconditionally
	opts := warmup.Options{
		Modules:   warmup.ParseModules(a.cfg.WarmupModules),
		Reset:     false,
		Metrics:   a.metrics,
		BM25Index: a.bm25Index,
	}

	stats, err := warmup.Run(ctx, a.db, a.scraperClient, a.stickerManager, a.logger, opts)
	if err != nil {
		a.logger.WithError(err).Error("Proactive warmup failed")
	} else {
		a.logger.WithField("stats", stats).
			WithField("duration_ms", time.Since(startTime).Milliseconds()).
			Info("Proactive warmup completed")
	}
}

// updateCacheSizeMetrics periodically updates cache size metrics.
func (a *Application) updateCacheSizeMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Update every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.performCacheSizeMetricsUpdate(ctx)
		}
	}
}

// performCacheSizeMetricsUpdate updates cache size metrics.
// Note: Cache size metrics are currently tracked via Count methods in each module.
// If you want explicit Prometheus gauges, add UpdateCacheSize to metrics.Metrics.
func (a *Application) performCacheSizeMetricsUpdate(ctx context.Context) {
	studentCount, _ := a.db.CountStudents(ctx)
	contactCount, _ := a.db.CountContacts(ctx)
	courseCount, _ := a.db.CountCourses(ctx)
	syllabiCount, _ := a.db.CountSyllabi(ctx)
	stickerCount := a.stickerManager.Count()

	// Log cache sizes for monitoring
	a.logger.WithFields(map[string]interface{}{
		"students": studentCount,
		"contacts": contactCount,
		"courses":  courseCount,
		"syllabi":  syllabiCount,
		"stickers": stickerCount,
	}).Debug("Cache size metrics updated")

	// Log BM25 index size if available
	if a.bm25Index != nil && a.bm25Index.IsEnabled() {
		a.logger.WithField("bm25_docs", a.bm25Index.Count()).Debug("BM25 index size")
	}
}

// securityHeadersMiddleware adds security headers to all responses.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// loggingMiddleware logs HTTP requests.
func loggingMiddleware(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		entry := log.WithField("method", method).
			WithField("path", path).
			WithField("status", status).
			WithField("duration_ms", duration.Milliseconds()).
			WithField("ip", c.ClientIP())

		if status >= 500 {
			entry.Error("Server error")
		} else if status >= 400 {
			entry.Warn("Client error")
		} else {
			entry.Info("Request")
		}
	}
}
