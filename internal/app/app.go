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
	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
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
	semesterCache  *course.SemesterCache  // Shared cache for semester data (updated by warmup)
	readinessState *warmup.ReadinessState // Tracks initial warmup completion for readiness
	wg             sync.WaitGroup         // Track background goroutines for graceful shutdown
}

// Initialize creates and initializes a new application with all dependencies.
func Initialize(ctx context.Context, cfg *config.Config) (*Application, error) {
	log := logger.NewWithOptions(cfg.LogLevel, os.Stdout, logger.Options{
		BetterStackToken:    cfg.BetterStackToken,
		BetterStackEndpoint: cfg.BetterStackEndpoint,
	})

	log = log.WithField("service", "ntpu-linebot-go")
	if host, err := os.Hostname(); err == nil && host != "" {
		log = log.WithField("instance_id", host)
	}

	// Set as default logger to enable context value extraction (userID, chatID, requestID)
	// via ContextHandler in package-level slog.*Context() calls.
	slog.SetDefault(log.Logger)

	log.Info("Initializing application...")
	if cfg.BetterStackToken != "" {
		log.WithField("endpoint", cfg.BetterStackEndpoint).Info("Better Stack logging enabled")
	}

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
			// Get configured providers from LLM config
			providers := llmCfg.ConfiguredProviders()
			providerNames := make([]string, len(providers))
			for i, p := range providers {
				providerNames[i] = p.String()
			}
			log.WithField("providers", providerNames).Info("LLM features enabled")
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

	// Create shared semester cache for course and program handlers
	semesterCache := course.NewSemesterCache()
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerMgr, bm25Index, queryExpander, llmLimiter, semesterCache)
	contactHandler := contact.NewHandler(db, scraperClient, m, log, stickerMgr, cfg.Bot.MaxContactsPerSearch)
	programHandler := program.NewHandler(db, m, log, stickerMgr, semesterCache)
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
		semesterCache:  semesterCache,
		readinessState: warmup.NewReadinessState(cfg.WarmupGracePeriod),
	}

	router.GET("/", app.redirectToGitHub)
	router.GET("/livez", app.livenessCheck)
	router.HEAD("/livez", app.livenessCheck)
	router.GET("/readyz", app.readinessCheck)
	router.HEAD("/readyz", app.readinessCheck)
	router.POST("/webhook", app.readinessMiddleware(), webhookHandler.Handle)
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

	llmCfg.Gemini.APIKey = cfg.GeminiAPIKey
	llmCfg.Groq.APIKey = cfg.GroqAPIKey
	llmCfg.Cerebras.APIKey = cfg.CerebrasAPIKey

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

	// Check warmup state first (for initial startup) - only if waiting for warmup is enabled
	if a.cfg.WaitForWarmup && !a.readinessState.IsReady() {
		status := a.readinessState.Status()
		a.logger.WithField("elapsed_seconds", status.ElapsedSeconds).
			WithField("timeout_seconds", status.TimeoutSeconds).
			Debug("Readiness check: warmup in progress")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": status.Reason,
			"progress": gin.H{
				"elapsed_seconds": status.ElapsedSeconds,
				"timeout_seconds": status.TimeoutSeconds,
			},
		})
		return
	}

	if err := a.db.Ping(ctx); err != nil {
		a.logger.WithError(err).Warn("Readiness check failed: database unavailable")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "database unavailable",
		})
		return
	}

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
//
// Graceful shutdown sequence (critical for data integrity):
//  1. Receive shutdown signal (SIGINT/SIGTERM)
//  2. Cancel context → signal background jobs to stop
//  3. Wait for background jobs to complete (warmup, cleanup, etc.)
//  4. Close resources in order (HTTP server, webhook handler, API clients, database, rate limiters)
//
// This order prevents "sql: database is closed" errors during warmup/cleanup operations.
// Previous bug: Resources were closed before background jobs finished, causing transaction failures.
func (a *Application) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure context is always canceled

	a.startBackgroundJobs(ctx)
	a.startHTTPServer()

	// Wait for shutdown signal
	sig := a.waitForShutdownSignal()

	a.logger.WithField("signal", sig.String()).Info("Received shutdown signal")

	// Step 1: Cancel context to signal all background jobs to stop
	cancel()

	// Step 2: Wait for all background goroutines to finish
	a.logger.Info("Waiting for background jobs to finish...")
	start := time.Now()
	a.wg.Wait()
	a.logger.WithField("duration_ms", time.Since(start).Milliseconds()).
		Info("All background jobs completed")

	// Step 3: Perform graceful shutdown (HTTP server, resources)
	return a.shutdown()
}

// startBackgroundJobs starts all background goroutines tracked by WaitGroup.
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

// waitForShutdownSignal blocks until SIGINT/SIGTERM is received.
func (a *Application) waitForShutdownSignal() os.Signal {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	return <-quit
}

// shutdown performs graceful shutdown of HTTP server and resources.
// This method should be called AFTER background jobs have been stopped and completed.
// Shutdown order:
// 1. Stop accepting new HTTP requests
// 2. Wait for in-flight HTTP requests to complete
// 3. Close resources (DB, API clients, rate limiters)
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

	if err := a.logger.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Warn("Logger shutdown timed out")
	}

	a.logger.Info("Shutdown complete")
	return nil
}

// cacheCleanup runs daily at 4:00 AM Taiwan time, exits on context cancellation.
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

// refreshStickers loads stickers once on startup.
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
		Reset:         false,
		HasLLMKey:     a.cfg.HasLLMProvider(),
		WarmID:        warmID,
		Metrics:       a.metrics,
		BM25Index:     a.bm25Index,
		SemesterCache: a.semesterCache,
	}

	stats, err := warmup.Run(warmupCtx, a.db, a.scraperClient, a.logger, opts)

	if warmID {
		a.readinessState.MarkReady()
		a.logger.Info("Service marked as ready after initial warmup")
	}

	if err != nil {
		a.logger.WithError(err).Error("Proactive warmup failed")
		return
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

// updateCacheSizeMetrics periodically records cache size to Prometheus.
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

// readinessMiddleware rejects webhook requests with 503 until initial warmup completes.
// LINE Platform will automatically retry, ensuring eventual delivery.
func (a *Application) readinessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.cfg.WaitForWarmup && !a.readinessState.IsReady() {
			status := a.readinessState.Status()
			a.logger.WithField("elapsed_seconds", status.ElapsedSeconds).
				Debug("Webhook rejected: warmup in progress")
			c.Header("Retry-After", "60")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":       "service warming up",
				"retry_after": 60,
			})
			c.Abort()
			return
		}
		c.Next()
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

// loggingMiddleware logs HTTP requests with status-based log levels:
// 5xx=Error, 4xx=Warn, 404=Debug, 3xx/2xx=Debug.
func loggingMiddleware(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = c.GetHeader("X-Request-ID")
		}
		if requestID == "" {
			requestID = c.GetHeader("X-Correlation-Id")
		}
		if requestID == "" {
			requestID = c.GetHeader("X-Correlation-ID")
		}
		if requestID != "" {
			ctx := ctxutil.WithRequestID(c.Request.Context(), requestID)
			c.Request = c.Request.WithContext(ctx)
		}

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		entry := log.WithField("http_method", method).
			WithField("http_path", path).
			WithField("http_status", status).
			WithField("duration_ms", duration.Milliseconds()).
			WithField("client_ip", c.ClientIP())

		if requestID != "" {
			entry = entry.WithRequestID(requestID)
		}

		if status >= 500 {
			entry.Error("HTTP request failed")
		} else if status >= 400 && status != 404 {
			entry.Warn("HTTP request rejected")
		} else if status == 404 {
			entry.Debug("HTTP request not found")
		} else {
			entry.Debug("HTTP request completed")
		}
	}
}
