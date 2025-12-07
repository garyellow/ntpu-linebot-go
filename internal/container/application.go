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

// Application represents a fully-configured application managing the complete lifecycle
// from HTTP server startup through background jobs to graceful shutdown.
//
// Lifecycle phases:
//  1. Run() - Starts HTTP server and background jobs
//  2. Signal handling - Waits for SIGINT/SIGTERM
//  3. Graceful shutdown - Stops server, closes connections, releases resources
//
// Background jobs:
//   - Cache cleanup (every 12h) - Removes expired entries and VACUUMs database
//   - Sticker refresh (every 24h) - Updates avatar URL cache
//   - Daily warmup (3:00 AM) - Proactively refreshes all data
//   - Metrics update (every 5m) - Updates cache size gauges
type Application struct {
	cfg    *config.Config
	logger *logger.Logger

	// Core services
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

// NewApplication creates an Application with dependencies and configures HTTP routing.
// Gin is set to release mode and configured with standard middleware:
//   - Recovery - Panic recovery to prevent crashes
//   - Security headers - OWASP recommended headers
//   - Logging - Request/response logging with duration tracking
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

// Run starts the application and blocks until shutdown signal is received.
//
// Startup sequence:
//  1. Launch background jobs (cache cleanup, warmup, metrics)
//  2. Start HTTP server (non-blocking via goroutine)
//  3. Wait for OS signal (SIGINT/SIGTERM)
//
// Shutdown sequence:
//  1. Cancel background jobs context
//  2. Gracefully shutdown HTTP server (waits for active connections)
//  3. Close GenAI components (release API clients)
//  4. Close database (flush pending writes, close connections)
//
// Returns error only if server or database shutdown fails.
// GenAI component errors are logged but don't prevent shutdown.
func (a *Application) Run() error {
	log := a.logger

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.startBackgroundJobs(ctx)

	a.server = &http.Server{
		Addr:         fmt.Sprintf(":%s", a.cfg.Port),
		Handler:      a.router,
		ReadTimeout:  config.WebhookHTTPRead,
		WriteTimeout: config.WebhookHTTPWrite,
		IdleTimeout:  config.WebhookHTTPIdle,
	}

	go func() {
		log.WithField("port", a.cfg.Port).Info("Server starting")
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server forced to shutdown")
		return fmt.Errorf("server shutdown: %w", err)
	}

	if a.queryExpander != nil {
		_ = a.queryExpander.Close()
	}
	if a.intentParser != nil {
		_ = a.intentParser.Close()
	}

	if err := a.db.Close(); err != nil {
		log.WithError(err).Error("Failed to close database")
	}

	log.Info("Server exited")
	return nil
}

// setupRoutes configures HTTP endpoints following RESTful conventions.
//
// Endpoint structure:
//   - GET  /        - Redirect to project repository
//   - GET  /healthz - Liveness probe (always returns 200)
//   - GET  /ready   - Readiness probe (checks database + cache state)
//   - POST /callback - LINE webhook entry point
//   - GET  /metrics - Prometheus metrics endpoint
//
// All GET endpoints support HEAD for efficient health checking.
func (a *Application) setupRoutes() {
	rootHandler := func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "https://github.com/garyellow/ntpu-linebot-go")
	}
	a.router.GET("/", rootHandler)
	a.router.HEAD("/", rootHandler)

	healthHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
	a.router.GET("/healthz", healthHandler)
	a.router.HEAD("/healthz", healthHandler)

	readyHandler := func(ctx *gin.Context) {
		if err := a.db.Ready(ctx.Request.Context()); err != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": err.Error(),
			})
			return
		}

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

	a.router.POST("/callback", a.webhookHandler.Handle)

	a.router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(a.registry, promhttp.HandlerOpts{})))
}

// startBackgroundJobs launches all periodic maintenance tasks.
// Each job runs independently in its own goroutine and respects context cancellation.
func (a *Application) startBackgroundJobs(ctx context.Context) {
	log := a.logger
	log.Info("Starting background jobs...")

	go a.cleanupExpiredCache(ctx)
	go a.refreshStickers(ctx)
	go a.proactiveWarmup(ctx)
	go a.updateCacheSizeMetrics(ctx)
}

// cleanupExpiredCache removes expired cache entries every 12 hours.
// Includes VACUUM operation to reclaim disk space after deletions.
func (a *Application) cleanupExpiredCache(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(config.CacheCleanupInitialDelay):
		a.performCacheCleanup(ctx)
	}

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

// performCacheCleanup deletes expired cache entries and reclaims disk space.
// Each cache type is cleaned individually with error logging.
// VACUUM compacts the database file after deletions.
func (a *Application) performCacheCleanup(ctx context.Context) {
	startTime := time.Now()
	a.logger.Info("Starting cache cleanup...")

	var totalDeleted int64

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

	if _, err := a.db.Writer().Exec("VACUUM"); err != nil {
		a.logger.WithError(err).Warn("Failed to VACUUM database")
	} else {
		a.logger.Debug("Database vacuumed successfully")
	}

	a.logger.WithField("deleted", totalDeleted).
		WithField("duration_ms", time.Since(startTime).Milliseconds()).
		Info("Cache cleanup completed")
}

// refreshStickers updates avatar URL cache every 24 hours.
// Fetches latest sticker metadata from LINE API for consistent sender display.
func (a *Application) refreshStickers(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(config.StickerRefreshInitialDelay):
		a.performStickerRefresh(ctx)
	}

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

// performStickerRefresh fetches latest sticker metadata with timeout.
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

	if a.metrics != nil {
		a.metrics.RecordJob("sticker_refresh", "all", time.Since(startTime).Seconds())
	}
}

// proactiveWarmup performs daily cache warming at 3:00 AM.
// proactiveWarmup refreshes all data daily at 3:00 AM.
// Reduces cold cache hits during peak usage hours by pre-fetching data.
func (a *Application) proactiveWarmup(ctx context.Context) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

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

// performProactiveWarmup executes cache warmup for configured modules.
// Refreshes data from source systems and rebuilds BM25 index if enabled.
func (a *Application) performProactiveWarmup(ctx context.Context) {
	a.logger.Info("Starting proactive warmup...")
	startTime := time.Now()

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

// updateCacheSizeMetrics logs cache statistics every 5 minutes for monitoring.
func (a *Application) updateCacheSizeMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
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

// performCacheSizeMetricsUpdate logs current cache state for observability.
func (a *Application) performCacheSizeMetricsUpdate(ctx context.Context) {
	studentCount, _ := a.db.CountStudents(ctx)
	contactCount, _ := a.db.CountContacts(ctx)
	courseCount, _ := a.db.CountCourses(ctx)
	syllabiCount, _ := a.db.CountSyllabi(ctx)
	stickerCount := a.stickerManager.Count()

	a.logger.WithFields(map[string]interface{}{
		"students": studentCount,
		"contacts": contactCount,
		"courses":  courseCount,
		"syllabi":  syllabiCount,
		"stickers": stickerCount,
	}).Debug("Cache size metrics updated")

	if a.bm25Index != nil && a.bm25Index.IsEnabled() {
		a.logger.WithField("bm25_docs", a.bm25Index.Count()).Debug("BM25 index size")
	}
}

// securityHeadersMiddleware applies OWASP recommended security headers.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// loggingMiddleware logs requests with structured fields for observability.
// Log level varies by status code: info (2xx/3xx), warn (4xx), error (5xx).
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
