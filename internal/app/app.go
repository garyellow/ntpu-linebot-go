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
	"github.com/garyellow/ntpu-linebot-go/internal/bot/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/course"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/id"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
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

// Application represents a fully-configured application managing the complete lifecycle.
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
	bm25Index      *rag.BM25Index
	intentParser   *genai.GeminiIntentParser
	queryExpander  *genai.QueryExpander
	llmRateLimiter *ratelimit.LLMRateLimiter
}

// Initialize creates and initializes a new application with all dependencies.
// This replaces the Container pattern with a single initialization function.
func Initialize(ctx context.Context, cfg *config.Config) (*Application, error) {
	log := logger.New(cfg.LogLevel)
	log.Info("Initializing application...")

	// Initialize core services
	db, registry, m, scraperClient, stickerMgr, err := initCore(ctx, cfg, log)
	if err != nil {
		return nil, fmt.Errorf("core services: %w", err)
	}

	// Initialize GenAI features (optional)
	bm25Index, intentParser, queryExpander := initGenAI(ctx, cfg, db, log)

	// Initialize LLM rate limiter for bot handlers
	botCfg, err := config.LoadBotConfig()
	if err != nil {
		return nil, fmt.Errorf("load bot config: %w", err)
	}
	if cfg.LLMRateLimitPerHour != 0 {
		botCfg.LLMRateLimitPerHour = cfg.LLMRateLimitPerHour
	}
	llmRateLimiter := ratelimit.NewLLMRateLimiter(botCfg.LLMRateLimitPerHour, 5*time.Minute, m)

	// Initialize bot handlers
	handlers := initHandlers(db, scraperClient, m, log, stickerMgr, bm25Index, queryExpander, llmRateLimiter)

	// Initialize webhook
	webhookHandler, err := initWebhook(ctx, cfg, handlers, m, log, stickerMgr, intentParser, llmRateLimiter)
	if err != nil {
		return nil, fmt.Errorf("webhook: %w", err)
	}

	// Setup HTTP server
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
		router:         router,
		bm25Index:      bm25Index,
		intentParser:   intentParser,
		queryExpander:  queryExpander,
		llmRateLimiter: llmRateLimiter,
	}

	app.setupRoutes()
	app.server = &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	log.Info("Initialization complete")
	return app, nil
}

// initCore initializes core services required by all modules.
func initCore(ctx context.Context, cfg *config.Config, log *logger.Logger) (
	*storage.DB,
	*prometheus.Registry,
	*metrics.Metrics,
	*scraper.Client,
	*sticker.Manager,
	error,
) {
	db, err := storage.New(ctx, cfg.SQLitePath(), cfg.CacheTTL)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("database: %w", err)
	}
	log.WithField("path", cfg.SQLitePath()).
		WithField("cache_ttl", cfg.CacheTTL).
		Info("Database connected")

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)
	m := metrics.New(registry)

	scraperClient := scraper.NewClient(cfg.ScraperTimeout, cfg.ScraperMaxRetries)
	stickerMgr := sticker.NewManager(db, scraperClient, log)

	log.Info("Core services initialized")
	return db, registry, m, scraperClient, stickerMgr, nil
}

// initGenAI initializes optional GenAI features.
// Returns nil values if features are disabled (no error).
func initGenAI(
	ctx context.Context,
	cfg *config.Config,
	db *storage.DB,
	log *logger.Logger,
) (*rag.BM25Index, *genai.GeminiIntentParser, *genai.QueryExpander) {
	// Initialize BM25 index (independent of Gemini API)
	syllabi, err := db.GetAllSyllabi(ctx)
	if err != nil {
		log.WithError(err).Warn("Failed to load syllabi for BM25")
		return nil, nil, nil
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

	// Initialize Gemini-powered features
	if cfg.GeminiAPIKey == "" {
		log.Info("Gemini API key not configured, NLU and query expansion disabled")
		return bm25Index, nil, nil
	}

	intentParser, err := genai.NewIntentParser(ctx, cfg.GeminiAPIKey)
	if err != nil {
		log.WithError(err).Warn("Intent parser initialization failed")
		return bm25Index, nil, nil
	}
	log.Info("Intent parser enabled")

	queryExpander, err := genai.NewQueryExpander(ctx, cfg.GeminiAPIKey)
	if err != nil {
		log.WithError(err).Warn("Query expander initialization failed")
		return bm25Index, intentParser, nil
	}
	log.Info("Query expander enabled")

	log.Info("GenAI features enabled")
	return bm25Index, intentParser, queryExpander
}

// initHandlers creates all bot handlers.
func initHandlers(
	db *storage.DB,
	scraperClient *scraper.Client,
	m *metrics.Metrics,
	log *logger.Logger,
	stickerMgr *sticker.Manager,
	bm25Index *rag.BM25Index,
	queryExpander *genai.QueryExpander,
	llmRateLimiter *ratelimit.LLMRateLimiter,
) []bot.Handler {
	idHandler := id.NewHandler(db, scraperClient, m, log, stickerMgr)
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerMgr, bm25Index, queryExpander, llmRateLimiter)

	botCfg := config.DefaultBotConfig()
	contactHandler := contact.NewHandler(
		db, scraperClient, m, log, stickerMgr,
		botCfg.MaxContactsPerSearch,
	)

	log.Info("Bot handlers initialized")
	return []bot.Handler{contactHandler, courseHandler, idHandler}
}

// initWebhook creates the webhook handler.
func initWebhook(
	ctx context.Context,
	cfg *config.Config,
	handlers []bot.Handler,
	m *metrics.Metrics,
	log *logger.Logger,
	stickerMgr *sticker.Manager,
	intentParser *genai.GeminiIntentParser,
	llmRateLimiter *ratelimit.LLMRateLimiter,
) (*webhook.Handler, error) {
	registry := bot.NewRegistry()
	for _, h := range handlers {
		registry.Register(h)
	}

	botCfg, err := config.LoadBotConfig()
	if err != nil {
		return nil, fmt.Errorf("load bot config: %w", err)
	}

	// Apply environment overrides
	if cfg.WebhookTimeout != 0 {
		botCfg.WebhookTimeout = cfg.WebhookTimeout
	}
	if cfg.UserRateLimitTokens != 0 {
		botCfg.UserRateLimitTokens = cfg.UserRateLimitTokens
	}
	if cfg.UserRateLimitRefillRate != 0 {
		botCfg.UserRateLimitRefillRate = cfg.UserRateLimitRefillRate
	}

	handler, err := webhook.NewHandler(
		cfg.LineChannelSecret,
		cfg.LineChannelToken,
		registry,
		botCfg,
		m,
		log,
		stickerMgr,
		intentParser,
		llmRateLimiter,
	)
	if err != nil {
		return nil, fmt.Errorf("create handler: %w", err)
	}

	log.Info("Webhook handler initialized")
	return handler, nil
}

// setupRoutes configures HTTP routes.
func (a *Application) setupRoutes() {
	a.router.GET("/health", a.healthCheck)
	a.router.POST("/webhook", a.webhookHandler.Handle)
	a.router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(a.registry, promhttp.HandlerOpts{})))
}

// healthCheck returns service health status.
func (a *Application) healthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := a.db.Ping(ctx); err != nil {
		a.logger.WithError(err).Error("Health check failed")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "database unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "healthy",
		"db_path":  a.cfg.SQLitePath(),
		"features": a.getFeatures(),
	})
}

// getFeatures returns enabled features.
func (a *Application) getFeatures() map[string]bool {
	return map[string]bool{
		"bm25_search":     a.bm25Index != nil && a.bm25Index.IsEnabled(),
		"nlu":             a.intentParser != nil && a.intentParser.IsEnabled(),
		"query_expansion": a.queryExpander != nil,
	}
}

// Run starts the HTTP server and background jobs.
func (a *Application) Run() error {
	// Start background jobs
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.performCacheCleanup(ctx)
	go a.refreshStickers(ctx)
	go a.proactiveWarmup(ctx)
	go a.updateCacheSizeMetrics(ctx)

	// Start HTTP server
	go func() {
		a.logger.WithField("port", a.cfg.Port).Info("Starting HTTP server")
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.WithError(err).Fatal("HTTP server error")
		}
	}()

	// Wait for interrupt signal
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

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Error("Server shutdown error")
	}

	if a.queryExpander != nil {
		if err := a.queryExpander.Close(); err != nil {
			a.logger.WithError(err).Error("Query expander close error")
		}
	}

	if a.intentParser != nil {
		if err := a.intentParser.Close(); err != nil {
			a.logger.WithError(err).Error("Intent parser close error")
		}
	}

	if a.llmRateLimiter != nil {
		a.llmRateLimiter.Stop()
	}

	if err := a.db.Close(); err != nil {
		a.logger.WithError(err).Error("Database close error")
		return err
	}

	a.logger.Info("Shutdown complete")
	return nil
}

// Background job methods (cache cleanup, sticker refresh, warmup, metrics)
// These remain unchanged from your current implementation

func (a *Application) performCacheCleanup(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
			}

			a.logger.WithField("deleted", totalDeleted).
				WithField("duration_ms", time.Since(startTime).Milliseconds()).
				Info("Cache cleanup completed")
		}
	}
}

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

func (a *Application) updateCacheSizeMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
	}
}

func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

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
