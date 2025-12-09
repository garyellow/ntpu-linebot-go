// Package app provides application initialization and lifecycle management.
package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/course"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/id"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
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

// Application manages the application lifecycle and dependencies.
type Application struct {
	cfg            *config.Config
	logger         *logger.Logger
	db             *storage.DB
	metrics        *metrics.Metrics
	registry       *prometheus.Registry
	scraperClient  *scraper.Client
	stickerManager *sticker.Manager
	webhookHandler *webhook.Handler
	server         *http.Server
	bm25Index      *rag.BM25Index
	intentParser   *genai.GeminiIntentParser
	queryExpander  *genai.QueryExpander
	llmRateLimiter *ratelimit.LLMRateLimiter
	userLimiter    *ratelimit.UserRateLimiter
}

// Initialize creates and initializes a new application with all dependencies.
func Initialize(ctx context.Context, cfg *config.Config) (*Application, error) {
	log := logger.New(cfg.LogLevel)
	log.Info("Initializing application...")

	// === Core Infrastructure ===
	db, err := storage.New(ctx, cfg.SQLitePath(), cfg.CacheTTL)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	log.WithField("path", cfg.SQLitePath()).WithField("cache_ttl", cfg.CacheTTL).Info("Database connected")

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)
	m := metrics.New(registry)

	scraperClient := scraper.NewClient(cfg.ScraperTimeout, cfg.ScraperMaxRetries, cfg.ScraperBaseURLs)
	stickerMgr := sticker.NewManager(db, scraperClient, log)

	// === RAG / BM25 Index ===
	syllabi, err := db.GetAllSyllabi(ctx)
	if err != nil {
		log.WithError(err).Warn("Failed to load syllabi for BM25")
		syllabi = nil
	}

	bm25Index := rag.NewBM25Index(log)
	if len(syllabi) > 0 {
		if err := bm25Index.Initialize(syllabi); err != nil {
			log.WithError(err).Warn("BM25 initialization failed")
		} else {
			log.WithField("doc_count", bm25Index.Count()).Info("BM25 index initialized")
		}
	} else {
		log.Warn("No syllabi found, BM25 search disabled")
	}

	// === GenAI Features ===
	var intentParser *genai.GeminiIntentParser
	var queryExpander *genai.QueryExpander
	if cfg.GeminiAPIKey != "" {
		if intentParser, err = genai.NewIntentParser(ctx, cfg.GeminiAPIKey); err != nil {
			log.WithError(err).Warn("Intent parser initialization failed")
		}
		if queryExpander, err = genai.NewQueryExpander(ctx, cfg.GeminiAPIKey); err != nil {
			log.WithError(err).Warn("Query expander initialization failed")
		}
		if intentParser != nil || queryExpander != nil {
			log.Info("GenAI features enabled")
		}
	} else {
		log.Info("Gemini API key not configured, NLU and query expansion disabled")
	}

	// === Rate Limiters ===
	llmRateLimiter := ratelimit.NewLLMRateLimiter(cfg.Bot.LLMRateLimitPerHour, config.RateLimiterCleanupInterval, m)
	userLimiter := ratelimit.NewUserRateLimiter(cfg.Bot.UserRateLimitTokens, cfg.Bot.UserRateLimitRefillRate, config.RateLimiterCleanupInterval, m)

	// === Bot Handlers ===
	idHandler := id.NewHandler(db, scraperClient, m, log, stickerMgr)
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerMgr, bm25Index, queryExpander, llmRateLimiter)
	contactHandler := contact.NewHandler(db, scraperClient, m, log, stickerMgr, cfg.Bot.MaxContactsPerSearch)

	// === Bot Registry ===
	botRegistry := bot.NewRegistry()
	botRegistry.Register(contactHandler)
	botRegistry.Register(courseHandler)
	botRegistry.Register(idHandler)

	// === Processor ===
	processor := bot.NewProcessor(bot.ProcessorConfig{
		Registry:       botRegistry,
		IntentParser:   intentParser,
		LLMRateLimiter: llmRateLimiter,
		UserLimiter:    userLimiter,
		StickerManager: stickerMgr,
		Logger:         log,
		Metrics:        m,
		BotConfig:      &cfg.Bot,
	})

	// === Webhook Handler ===
	webhookHandler, err := webhook.NewHandler(webhook.HandlerConfig{
		ChannelSecret:  cfg.LineChannelSecret,
		ChannelToken:   cfg.LineChannelToken,
		BotConfig:      &cfg.Bot,
		Metrics:        m,
		Logger:         log,
		Processor:      processor,
		StickerManager: stickerMgr,
	})
	if err != nil {
		return nil, fmt.Errorf("webhook: %w", err)
	}

	// === HTTP Router ===
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(securityHeadersMiddleware())
	router.Use(loggingMiddleware(log))

	app := &Application{
		cfg:            cfg,
		logger:         log,
		db:             db,
		metrics:        m,
		registry:       registry,
		scraperClient:  scraperClient,
		stickerManager: stickerMgr,
		webhookHandler: webhookHandler,
		bm25Index:      bm25Index,
		intentParser:   intentParser,
		queryExpander:  queryExpander,
		llmRateLimiter: llmRateLimiter,
		userLimiter:    userLimiter,
	}

	router.GET("/livez", app.livenessCheck)
	router.HEAD("/livez", app.livenessCheck)
	router.GET("/readyz", app.readinessCheck)
	router.HEAD("/readyz", app.readinessCheck)
	router.POST("/webhook", webhookHandler.Handle)
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

	app.server = &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: config.WebhookHTTPRead,
		ReadTimeout:       config.WebhookHTTPRead,
		WriteTimeout:      config.WebhookHTTPWrite,
		IdleTimeout:       config.WebhookHTTPIdle,
	}

	log.Info("Initialization complete")
	return app, nil
}

func (a *Application) livenessCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
	})
}

func (a *Application) getFeatures() map[string]bool {
	return map[string]bool{
		"bm25_search":     a.bm25Index != nil && a.bm25Index.IsEnabled(),
		"nlu":             a.intentParser != nil && a.intentParser.IsEnabled(),
		"query_expansion": a.queryExpander != nil,
	}
}

func (a *Application) readinessCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), config.ReadinessCheckTimeout)
	defer cancel()

	// Check database connectivity
	if err := a.db.Ping(ctx); err != nil {
		a.logger.WithError(err).Warn("Readiness check failed: database unavailable")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "database unavailable",
		})
		return
	}

	// Get cache statistics
	cacheStats := a.getCacheStats(ctx)

	c.JSON(http.StatusOK, gin.H{
		"status":   "ready",
		"database": "connected",
		"cache":    cacheStats,
		"features": a.getFeatures(),
	})
}

func (a *Application) getCacheStats(ctx context.Context) map[string]int {
	stats := make(map[string]int)

	// Query each cache table count, log errors for observability
	if count, err := a.db.CountStudents(ctx); err == nil {
		stats["students"] = count
	} else {
		a.logger.WithError(err).Warn("Failed to count students in cache stats")
	}
	if count, err := a.db.CountContacts(ctx); err == nil {
		stats["contacts"] = count
	} else {
		a.logger.WithError(err).Warn("Failed to count contacts in cache stats")
	}
	if count, err := a.db.CountCourses(ctx); err == nil {
		stats["courses"] = count
	} else {
		a.logger.WithError(err).Warn("Failed to count courses in cache stats")
	}
	if count, err := a.db.CountStickers(ctx); err == nil {
		stats["stickers"] = count
	} else {
		a.logger.WithError(err).Warn("Failed to count stickers in cache stats")
	}

	return stats
}

// Run starts the HTTP server and background jobs.
func (a *Application) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.startBackgroundJobs(ctx)
	a.startHTTPServer()

	return a.waitForShutdown()
}

// startBackgroundJobs starts all background goroutines.
func (a *Application) startBackgroundJobs(ctx context.Context) {
	go a.performCacheCleanup(ctx)
	go a.refreshStickers(ctx)
	go a.proactiveWarmup(ctx)
	go a.updateCacheSizeMetrics(ctx)
}

// startHTTPServer starts the HTTP server in a goroutine.
func (a *Application) startHTTPServer() {
	go func() {
		a.logger.WithField("port", a.cfg.Port).Info("Starting HTTP server")
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.WithError(err).Error("HTTP server error")
		}
	}()
}

// waitForShutdown blocks until shutdown signal is received.
func (a *Application) waitForShutdown() error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	a.logger.Info("Shutting down server...")
	return a.shutdown()
}

// shutdown performs graceful shutdown.
func (a *Application) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	// Wait for webhook handler to finish processing pending events
	if err := a.webhookHandler.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Warn("Webhook handler shutdown timeout")
	}

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Error("Server shutdown error")
	}

	// Close components in reverse initialization order
	if a.queryExpander != nil {
		if err := a.queryExpander.Close(); err != nil {
			a.logger.WithError(err).WithField("component", "query_expander").Error("Component close error")
		}
	}

	if a.intentParser != nil {
		if err := a.intentParser.Close(); err != nil {
			a.logger.WithError(err).WithField("component", "intent_parser").Error("Component close error")
		}
	}

	if err := a.db.Close(); err != nil {
		a.logger.WithError(err).WithField("component", "database").Error("Component close error")
	}

	// Stop rate limiters
	if a.llmRateLimiter != nil {
		a.llmRateLimiter.Stop()
	}
	if a.userLimiter != nil {
		a.userLimiter.Stop()
	}

	a.logger.Info("Shutdown complete")
	return nil
}

// performCacheCleanup runs periodic cache cleanup.
func (a *Application) performCacheCleanup(ctx context.Context) {
	// Initial delay to let server stabilize
	select {
	case <-ctx.Done():
		return
	case <-time.After(config.CacheCleanupInitialDelay):
	}

	ticker := time.NewTicker(config.CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runCacheCleanup(ctx)
		}
	}
}

// runCacheCleanup performs the actual cache cleanup operation.
func (a *Application) runCacheCleanup(ctx context.Context) {
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

	// VACUUM to reclaim space
	if _, err := a.db.Writer().Exec("VACUUM"); err != nil {
		a.logger.WithError(err).Warn("Failed to VACUUM database")
	}

	duration := time.Since(startTime)
	a.logger.WithField("deleted", totalDeleted).
		WithField("duration_ms", duration.Milliseconds()).
		Info("Cache cleanup completed")

	if a.metrics != nil {
		a.metrics.RecordJob("cache_cleanup", "all", duration.Seconds())
	}
}

// refreshStickers runs periodic sticker refresh.
func (a *Application) refreshStickers(ctx context.Context) {
	// Initial delay
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

// performStickerRefresh performs the actual sticker refresh operation.
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

// proactiveWarmup runs daily warmup at 3:00 AM.
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

// performProactiveWarmup performs the actual warmup operation.
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
		a.logger.WithField("students", stats.Students.Load()).
			WithField("contacts", stats.Contacts.Load()).
			WithField("courses", stats.Courses.Load()).
			WithField("syllabi", stats.Syllabi.Load()).
			WithField("stickers", stats.Stickers.Load()).
			WithField("duration_ms", time.Since(startTime).Milliseconds()).
			Info("Proactive warmup completed")
	}
}

// updateCacheSizeMetrics periodically updates cache size metrics.
func (a *Application) updateCacheSizeMetrics(ctx context.Context) {
	ticker := time.NewTicker(config.MetricsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.recordCacheSizeMetrics(ctx)
		}
	}
}

// recordCacheSizeMetrics records current cache sizes to metrics.
func (a *Application) recordCacheSizeMetrics(ctx context.Context) {
	studentCount, _ := a.db.CountStudents(ctx)
	contactCount, _ := a.db.CountContacts(ctx)
	courseCount, _ := a.db.CountCourses(ctx)
	syllabiCount, _ := a.db.CountSyllabi(ctx)
	stickerCount := a.stickerManager.Count()

	a.logger.WithField("students", studentCount).
		WithField("contacts", contactCount).
		WithField("courses", courseCount).
		WithField("syllabi", syllabiCount).
		WithField("stickers", stickerCount).
		Debug("Cache size metrics updated")

	if a.bm25Index != nil && a.bm25Index.IsEnabled() {
		a.logger.WithField("bm25_docs", a.bm25Index.Count()).Debug("BM25 index size")
	}

	// Update Prometheus gauges
	if a.metrics != nil {
		a.metrics.SetCacheSize("students", studentCount)
		a.metrics.SetCacheSize("contacts", contactCount)
		a.metrics.SetCacheSize("courses", courseCount)
		a.metrics.SetCacheSize("syllabi", syllabiCount)
		a.metrics.SetCacheSize("stickers", stickerCount)
		if a.bm25Index != nil {
			a.metrics.SetIndexSize("bm25", a.bm25Index.Count())
		}
	}
}

// securityHeadersMiddleware adds security headers to responses.
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
			entry.Debug("Request")
		}
	}
}
