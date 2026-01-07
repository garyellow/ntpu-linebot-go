// Package app provides application initialization and lifecycle management.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/lineutil"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/course"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/id"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/program"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/usage"
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
	intentParser   genai.IntentParser  // Interface type for multi-provider support
	queryExpander  genai.QueryExpander // Interface type for multi-provider support
	llmLimiter     *ratelimit.KeyedLimiter
	userLimiter    *ratelimit.KeyedLimiter
	courseHandler  *course.Handler // For refreshing semester data after warmup
	wg             sync.WaitGroup  // Track background goroutines for graceful shutdown
}

// Initialize creates and initializes a new application with all dependencies.
func Initialize(ctx context.Context, cfg *config.Config) (*Application, error) {
	log := logger.New(cfg.LogLevel)

	// Set the custom logger as the default slog logger.
	// This allows package-level slog.*Context() calls to automatically extract
	// context values (userID, chatID, requestID) via the ContextHandler.
	// Reference: https://betterstack.com/community/guides/logging/golang-contextual-logging/
	slog.SetDefault(log.Logger)

	log.Info("Initializing application...")

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

	// Initialize global metrics for genai package
	metrics.InitGlobal(m)

	scraperClient := scraper.NewClient(cfg.ScraperTimeout, cfg.ScraperMaxRetries, cfg.ScraperBaseURLs)
	stickerMgr := sticker.NewManager(db, scraperClient, log)

	bm25Index := rag.NewBM25Index(log)
	if err := bm25Index.Initialize(ctx, db); err != nil {
		log.WithError(err).Warn("BM25 initialization failed")
	}

	var intentParser genai.IntentParser
	var queryExpander genai.QueryExpander
	if cfg.HasLLMProvider() {
		llmCfg := buildLLMConfig(cfg)

		if intentParser, err = genai.CreateIntentParser(ctx, llmCfg); err != nil {
			log.WithError(err).Warn("Intent parser initialization failed")
		}
		if queryExpander, err = genai.CreateQueryExpander(ctx, llmCfg); err != nil {
			log.WithError(err).Warn("Query expander initialization failed")
		}
		if intentParser != nil || queryExpander != nil {
			providers := []string{}
			if cfg.GeminiAPIKey != "" {
				providers = append(providers, "gemini")
			}
			if cfg.GroqAPIKey != "" {
				providers = append(providers, "groq")
			}
			log.WithField("providers", providers).Info("LLM features enabled")
		}
	}

	llmLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Name:          "llm",
		Burst:         cfg.Bot.LLMRateBurst,
		RefillRate:    cfg.Bot.LLMRateRefill / 3600.0, // Convert hourly to per-second
		DailyLimit:    cfg.Bot.LLMRateDaily,
		CleanupPeriod: config.RateLimiterCleanupInterval,
		Metrics:       m,
		MetricType:    ratelimit.MetricTypeLLM,
	})
	userLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Name:          "user",
		Burst:         cfg.Bot.UserRateBurst,
		RefillRate:    cfg.Bot.UserRateRefill,
		CleanupPeriod: config.RateLimiterCleanupInterval,
		Metrics:       m,
		MetricType:    ratelimit.MetricTypeUser,
	})

	idHandler := id.NewHandler(db, scraperClient, m, log, stickerMgr)
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerMgr, bm25Index, queryExpander, llmLimiter)
	contactHandler := contact.NewHandler(db, scraperClient, m, log, stickerMgr, cfg.Bot.MaxContactsPerSearch)
	programHandler := program.NewHandler(db, m, log, stickerMgr, courseHandler.GetSemesterDetector())
	usageHandler := usage.NewHandler(userLimiter, llmLimiter, log, stickerMgr)

	botRegistry := bot.NewRegistry()
	botRegistry.Register(contactHandler)
	botRegistry.Register(courseHandler)
	botRegistry.Register(idHandler)
	botRegistry.Register(programHandler)
	botRegistry.Register(usageHandler)

	processor := bot.NewProcessor(bot.ProcessorConfig{
		Registry:       botRegistry,
		IntentParser:   intentParser,
		LLMLimiter:     llmLimiter,
		UserLimiter:    userLimiter,
		StickerManager: stickerMgr,
		Logger:         log,
		Metrics:        m,
		BotConfig:      &cfg.Bot,
	})

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
		llmLimiter:     llmLimiter,
		userLimiter:    userLimiter,
		courseHandler:  courseHandler,
	}

	router.GET("/", app.redirectToGitHub)
	router.GET("/livez", app.livenessCheck)
	router.HEAD("/livez", app.livenessCheck)
	router.GET("/readyz", app.readinessCheck)
	router.HEAD("/readyz", app.readinessCheck)
	router.POST("/webhook", webhookHandler.Handle)
	router.GET("/metrics",
		metricsAuthMiddleware(cfg.MetricsUsername, cfg.MetricsPassword),
		gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

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

// buildLLMConfig creates an LLMConfig from the application config.
func buildLLMConfig(cfg *config.Config) genai.LLMConfig {
	llmCfg := genai.DefaultLLMConfig()

	// Set API keys
	llmCfg.Gemini.APIKey = cfg.GeminiAPIKey
	llmCfg.Groq.APIKey = cfg.GroqAPIKey
	llmCfg.Cerebras.APIKey = cfg.CerebrasAPIKey

	// Override model chains if provided in config (supports comma-separated lists)
	if len(cfg.GeminiIntentModels) > 0 {
		llmCfg.Gemini.IntentModels = cfg.GeminiIntentModels
	}
	if len(cfg.GeminiExpanderModels) > 0 {
		llmCfg.Gemini.ExpanderModels = cfg.GeminiExpanderModels
	}
	if len(cfg.GroqIntentModels) > 0 {
		llmCfg.Groq.IntentModels = cfg.GroqIntentModels
	}
	if len(cfg.GroqExpanderModels) > 0 {
		llmCfg.Groq.ExpanderModels = cfg.GroqExpanderModels
	}
	if len(cfg.CerebrasIntentModels) > 0 {
		llmCfg.Cerebras.IntentModels = cfg.CerebrasIntentModels
	}
	if len(cfg.CerebrasExpanderModels) > 0 {
		llmCfg.Cerebras.ExpanderModels = cfg.CerebrasExpanderModels
	}

	// Set provider order from config
	if len(cfg.LLMProviders) > 0 {
		providers := make([]genai.Provider, 0, len(cfg.LLMProviders))
		for _, p := range cfg.LLMProviders {
			switch p {
			case "gemini":
				providers = append(providers, genai.ProviderGemini)
			case "groq":
				providers = append(providers, genai.ProviderGroq)
			case "cerebras":
				providers = append(providers, genai.ProviderCerebras)
			default:
				slog.Warn("ignoring unknown provider", "name", p)
			}
		}
		if len(providers) > 0 {
			llmCfg.Providers = providers
		}
	}

	return llmCfg
}

func (a *Application) redirectToGitHub(c *gin.Context) {
	c.Redirect(http.StatusTemporaryRedirect, "https://github.com/garyellow/ntpu-linebot-go")
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

	a.startBackgroundJobs(ctx)
	a.startHTTPServer()

	// Wait for shutdown signal
	err := a.waitForShutdown()

	// Cancel context to stop all background jobs
	cancel()

	// Wait for all background goroutines to finish
	a.logger.Info("Waiting for background jobs to finish...")
	a.wg.Wait()
	a.logger.Info("All background jobs completed")

	return err
}

// startBackgroundJobs starts all background goroutines.
// Each goroutine is tracked via WaitGroup for graceful shutdown.
func (a *Application) startBackgroundJobs(ctx context.Context) {
	a.wg.Go(func() {
		a.cacheCleanup(ctx)
	})
	a.wg.Go(func() {
		a.refreshStickers(ctx)
	})
	a.wg.Go(func() {
		a.proactiveWarmup(ctx)
	})
	a.wg.Go(func() {
		a.updateCacheSizeMetrics(ctx)
	})
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
	sig := <-quit

	a.logger.WithField("signal", sig.String()).Info("Received shutdown signal")
	return a.shutdown()
}

// shutdown performs graceful shutdown in the following order:
// 1. Stop accepting new HTTP requests
// 2. Wait for in-flight HTTP requests to complete
// 3. Signal background jobs to stop (via context cancellation in Run())
// 4. Wait for background jobs to finish (via WaitGroup in Run())
// 5. Close resources (DB, API clients, rate limiters)
func (a *Application) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	a.logger.Info("Stopping HTTP server...")
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Error("HTTP server shutdown error")
	}

	a.logger.Info("Waiting for webhook events to complete...")
	if err := a.webhookHandler.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Warn("Webhook handler shutdown timeout")
	}

	a.logger.Info("Closing resources...")

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

	if a.llmLimiter != nil {
		a.llmLimiter.Stop()
	}
	if a.userLimiter != nil {
		a.userLimiter.Stop()
	}

	a.logger.Info("Shutdown complete")
	return nil
}

// cacheCleanup runs daily cache cleanup at 4:00 AM Taiwan time.
// Exits cleanly when context is canceled during shutdown.
func (a *Application) cacheCleanup(ctx context.Context) {
	a.logger.Debug("Cache cleanup job started")
	defer a.logger.Debug("Cache cleanup job stopped")

	// Run initial cleanup on startup with independent context
	initialCtx, initialCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	//nolint:contextcheck // Intentionally using independent context
	a.runCacheCleanup(initialCtx)
	initialCancel()

	// Schedule daily cleanup at fixed time (4:00 AM Taiwan time)
	taipeiTZ := lineutil.GetTaipeiLocation()
	for {
		now := time.Now().In(taipeiTZ)
		next := time.Date(now.Year(), now.Month(), now.Day(), config.CacheCleanupHour, 0, 0, 0, taipeiTZ)
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		waitDuration := time.Until(next)
		a.logger.WithField("next_run", next.Format(time.RFC3339)).
			Info("Scheduled next cache cleanup (Taiwan time)")

		select {
		case <-ctx.Done():
			a.logger.Debug("Cache cleanup received shutdown signal")
			return
		case <-time.After(waitDuration):
			a.runCacheCleanup(ctx)
		}
	}
}

// runCacheCleanup performs the actual cache cleanup operation.
func (a *Application) runCacheCleanup(ctx context.Context) {
	startTime := time.Now()
	a.logger.Info("Starting cache cleanup...")

	var totalDeleted int64

	// Note: Students and stickers don't expire - they are only loaded once on startup,
	// so no cleanup needed for them

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

	if deleted, err := a.db.DeleteExpiredCoursePrograms(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired course programs")
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredPrograms(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired programs")
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredSyllabi(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired syllabi")
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

// refreshStickers loads stickers once on startup (no periodic refresh).
func (a *Application) refreshStickers(ctx context.Context) {
	a.logger.Debug("Sticker refresh job started")
	defer a.logger.Debug("Sticker refresh job stopped")

	initialCtx, initialCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer initialCancel()
	//nolint:contextcheck // Intentionally using independent context
	a.performStickerRefresh(initialCtx)

	<-ctx.Done()
	a.logger.Debug("Sticker refresh received shutdown signal")
}

func (a *Application) performStickerRefresh(ctx context.Context) {
	a.logger.Info("Starting sticker refresh...")
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

// proactiveWarmup runs initial warmup on startup, then daily at 3:00 AM Taiwan time.
func (a *Application) proactiveWarmup(ctx context.Context) {
	a.logger.Debug("Proactive warmup job started")
	defer a.logger.Debug("Proactive warmup job stopped")

	initialCtx, initialCancel := context.WithTimeout(context.Background(), config.WarmupProactive)
	//nolint:contextcheck // Intentionally using independent context
	a.performProactiveWarmup(initialCtx, true)
	initialCancel()

	taipeiTZ := lineutil.GetTaipeiLocation()
	for {
		now := time.Now().In(taipeiTZ)
		next := time.Date(now.Year(), now.Month(), now.Day(), config.WarmupHour, 0, 0, 0, taipeiTZ)
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		waitDuration := time.Until(next)
		a.logger.WithField("next_run", next.Format(time.RFC3339)).
			Info("Scheduled next proactive warmup (Taiwan time)")

		select {
		case <-ctx.Done():
			a.logger.Debug("Proactive warmup received shutdown signal")
			return
		case <-time.After(waitDuration):
			a.performProactiveWarmup(ctx, false)
		}
	}
}

func (a *Application) performProactiveWarmup(ctx context.Context, warmID bool) {
	a.logger.Info("Starting proactive warmup...")
	startTime := time.Now()

	warmupCtx, cancel := context.WithTimeout(ctx, config.WarmupProactive)
	defer cancel()

	opts := warmup.Options{
		Reset:     false,
		HasLLMKey: a.cfg.HasLLMProvider(),
		WarmID:    warmID,
		Metrics:   a.metrics,
		BM25Index: a.bm25Index,
	}

	stats, err := warmup.Run(warmupCtx, a.db, a.scraperClient, a.logger, opts)
	if err != nil {
		a.logger.WithError(err).Error("Proactive warmup failed")
		return
	}

	// Refresh semester data in course handler after warmup completes
	// This ensures user queries use data-driven semester detection
	if a.courseHandler != nil {
		a.courseHandler.RefreshSemesters(ctx)
	}

	logEntry := a.logger.WithField("contacts", stats.Contacts.Load()).
		WithField("courses", stats.Courses.Load()).
		WithField("syllabi", stats.Syllabi.Load()).
		WithField("duration_ms", time.Since(startTime).Milliseconds())

	if warmID {
		logEntry.Info("Proactive warmup completed (startup: includes ID data)")
	} else {
		logEntry.Info("Proactive warmup completed (daily refresh)")
	}

	if a.bm25Index != nil && a.bm25Index.IsEnabled() {
		a.logger.WithField("doc_count", a.bm25Index.Count()).Info("BM25 smart search enabled")
	}
}

// updateCacheSizeMetrics periodically updates cache size metrics.
func (a *Application) updateCacheSizeMetrics(ctx context.Context) {
	a.logger.Debug("Cache metrics job started")
	defer a.logger.Debug("Cache metrics job stopped")

	ticker := time.NewTicker(config.MetricsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Debug("Cache metrics received shutdown signal")
			return
		case <-ticker.C:
			a.recordCacheSizeMetrics(ctx)
		}
	}
}

func (a *Application) recordCacheSizeMetrics(ctx context.Context) {
	if a.metrics == nil {
		return
	}

	studentCount, _ := a.db.CountStudents(ctx)
	contactCount, _ := a.db.CountContacts(ctx)
	courseCount, _ := a.db.CountCourses(ctx)
	syllabiCount, _ := a.db.CountSyllabi(ctx)
	programCount, _ := a.db.CountPrograms(ctx)
	stickerCount := a.stickerManager.Count()

	a.metrics.SetCacheSize("students", studentCount)
	a.metrics.SetCacheSize("contacts", contactCount)
	a.metrics.SetCacheSize("courses", courseCount)
	a.metrics.SetCacheSize("syllabi", syllabiCount)
	a.metrics.SetCacheSize("programs", programCount)
	a.metrics.SetCacheSize("stickers", stickerCount)

	if a.bm25Index != nil {
		a.metrics.SetIndexSize("bm25", a.bm25Index.Count())
	}
}

// securityHeadersMiddleware adds security headers to responses.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'none'")
		c.Header("X-Permitted-Cross-Domain-Policies", "none")
		c.Next()
	}
}

// loggingMiddleware logs HTTP requests with appropriate log levels.
// Follows industry for HTTP status code logging:
// - 5xx: Error (server issues requiring immediate attention)
// - 4xx: Warn (client errors, except 404 which is common noise)
// - 404: Debug (common probes: robots.txt, favicon.ico, security.txt)
// - 3xx: Debug (redirects are normal behavior)
// - 2xx: Debug (successful requests)
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

		// Log level selection based on status code
		if status >= 500 {
			entry.Error("Server error")
		} else if status >= 400 && status != 404 {
			// 400, 401, 403, etc. - potential security issues or misconfigurations
			entry.Warn("Client error")
		} else if status == 404 {
			// 404s are common from health checkers, security scanners, and bots
			// Use Prometheus metrics for monitoring 404 patterns instead
			entry.Debug("Not found")
		} else {
			// 2xx success and 3xx redirects
			entry.Debug("Request")
		}
	}
}
