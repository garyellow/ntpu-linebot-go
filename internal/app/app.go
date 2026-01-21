// Package app provides application initialization and lifecycle management.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/buildinfo"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
	"github.com/garyellow/ntpu-linebot-go/internal/delta"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/maintenance"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/course"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/id"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/program"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/usage"
	"github.com/garyellow/ntpu-linebot-go/internal/r2client"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	internalSentry "github.com/garyellow/ntpu-linebot-go/internal/sentry"
	"github.com/garyellow/ntpu-linebot-go/internal/snapshot"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/warmup"
	"github.com/garyellow/ntpu-linebot-go/internal/webhook"
	sentrygin "github.com/getsentry/sentry-go/gin"
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
	hotSwapDB      *storage.HotSwapDB // Used when R2 is enabled
	snapshotMgr    *snapshot.Manager  // R2 snapshot manager (nil if R2 disabled)
	snapshotReady  *atomic.Bool       // True if a snapshot was successfully downloaded/applied
	deltaLog       *delta.R2Log       // R2 delta log (nil if R2 disabled)
	scheduleStore  *maintenance.R2ScheduleStore
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
	semesterCache  *course.SemesterCache  // Shared cache for semester data (updated by refresh task)
	readinessState *warmup.ReadinessState // Tracks initial refresh completion for readiness
	wg             sync.WaitGroup         // Track background goroutines for graceful shutdown
}

// Initialize creates and initializes a new application with all dependencies.
func Initialize(ctx context.Context, cfg *config.Config) (*Application, error) {
	version := resolveLogVersion(cfg)
	log := logger.NewWithOptions(cfg.LogLevel, os.Stdout, logger.Options{
		BetterStackToken:    cfg.BetterStackToken,
		BetterStackEndpoint: cfg.BetterStackEndpoint,
		Version:             version,
	})

	readinessState := warmup.NewReadinessState(cfg.WarmupGracePeriod)
	host, _ := os.Hostname()
	serverName, instanceID := resolveServerIdentity(cfg, host, time.Now)
	log = log.WithField("service", "ntpu-linebot-go")
	log = log.WithField("server_name", serverName)
	log = log.WithField("instance_id", instanceID)

	// Set as default logger to enable context value extraction (userID, chatID, requestID)
	// via ContextHandler in package-level slog.*Context() calls.
	slog.SetDefault(log.Logger)

	log.Info("Initializing application")

	// Log status of Optional Features
	log.WithField("sentry", cfg.IsSentryEnabled()).
		WithField("betterstack", cfg.IsBetterStackEnabled()).
		WithField("r2_snapshot", cfg.IsR2Enabled()).
		WithField("llm_features", cfg.IsLLMEnabled()).
		WithField("metrics_auth", cfg.IsMetricsAuthEnabled()).
		Info("Feature status")

	// Warn on ignored credentials when feature flags are disabled
	if !cfg.IsLLMEnabled() && (cfg.GeminiAPIKey != "" || cfg.GroqAPIKey != "" || cfg.CerebrasAPIKey != "") {
		log.Warn("LLM credentials provided but NTPU_LLM_ENABLED=false, LLM features are disabled")
	}
	if !cfg.IsSentryEnabled() && cfg.SentryDSN != "" {
		log.Warn("Sentry DSN provided but NTPU_SENTRY_ENABLED=false, Sentry is disabled")
	}
	if !cfg.IsBetterStackEnabled() && cfg.BetterStackToken != "" {
		log.Warn("Better Stack token provided but NTPU_BETTERSTACK_ENABLED=false, Better Stack is disabled")
	}
	if !cfg.IsR2Enabled() && (cfg.R2AccountID != "" || cfg.R2AccessKeyID != "" || cfg.R2SecretKey != "" || cfg.R2BucketName != "") {
		log.Warn("R2 credentials provided but NTPU_R2_ENABLED=false, R2 snapshot sync is disabled")
	}
	if !cfg.IsMetricsAuthEnabled() && cfg.MetricsPassword != "" {
		log.Warn("Metrics password provided but NTPU_METRICS_AUTH_ENABLED=false, metrics auth is disabled")
	}

	// 1. Better Stack Logging
	if cfg.IsBetterStackEnabled() {
		log.WithField("endpoint", cfg.BetterStackEndpoint).Info("Better Stack logging enabled")
	}

	// 2. Sentry Error Tracking
	if cfg.IsSentryEnabled() {
		release := cfg.SentryRelease
		env := resolveSentryEnvironment(cfg.SentryEnvironment, cfg.LogLevel)
		if err := internalSentry.Initialize(internalSentry.Config{
			DSN:              cfg.SentryDSN,
			Environment:      env,
			Release:          release,
			ServerName:       serverName,
			SampleRate:       cfg.SentrySampleRate,
			TracesSampleRate: cfg.SentryTracesSampleRate,
			HTTPTimeout:      config.SentryHTTPTimeout,
			Debug:            cfg.LogLevel == "debug",
			ServiceName:      "ntpu-linebot-go",
		}); err != nil {
			log.WithError(err).Warn("Sentry initialization failed")
		} else {
			log.WithField("environment", env).
				WithField("traces_sample_rate", cfg.SentryTracesSampleRate).
				Info("Sentry error tracking enabled")
		}
	}

	// 3. Database & R2 Initialization
	var db *storage.DB
	var hotSwapDB *storage.HotSwapDB
	var snapshotMgr *snapshot.Manager
	var deltaLog *delta.R2Log
	var scheduleStore *maintenance.R2ScheduleStore
	useLocalDB := true // Flag to track if we should use local DB

	snapshotReady := &atomic.Bool{}
	if cfg.IsR2Enabled() {
		// R2 mode: try to download snapshot for fast startup
		log.Info("R2 snapshot sync enabled, downloading latest snapshot")

		r2Client, r2Err := r2client.New(ctx, r2client.Config{
			Endpoint:    cfg.R2Endpoint(),
			AccessKeyID: cfg.R2AccessKeyID,
			SecretKey:   cfg.R2SecretKey,
			BucketName:  cfg.R2BucketName,
		})
		if r2Err != nil {
			log.WithError(r2Err).Warn("R2 client initialization failed, falling back to local database")
		} else {
			snapshotMgr = snapshot.New(r2Client, snapshot.Config{
				SnapshotKey:    cfg.R2SnapshotKey,
				LockKey:        cfg.R2LockKey,
				LockTTL:        cfg.R2LockTTL,
				PollInterval:   cfg.R2SnapshotPollInterval,
				TempDir:        cfg.DataDir,
				RequestTimeout: config.R2RequestTimeout,
			})

			// Default to local database path; may be replaced by snapshot download
			dbPath := cfg.SQLitePath()

			// Try to download latest snapshot
			snapshotPath, etag, dlErr := snapshotMgr.DownloadSnapshot(ctx, cfg.DataDir)
			if dlErr != nil {
				if errors.Is(dlErr, snapshot.ErrNotFound) {
					log.Info("No R2 snapshot found, starting with local database")
				} else {
					log.WithError(dlErr).Warn("R2 snapshot download failed, starting with local database")
				}
			} else {
				log.WithField("etag", etag).Info("Downloaded snapshot from R2")
				dbPath = snapshotPath
				snapshotReady.Store(true)
			}

			// Create HotSwapDB for runtime updates (even if snapshot download failed)
			var hsErr error
			hotSwapDB, hsErr = storage.NewHotSwapDB(ctx, dbPath, cfg.CacheTTL)
			if hsErr != nil {
				log.WithError(hsErr).Warn("HotSwapDB creation failed, falling back to regular DB")
				snapshotMgr = nil
			} else {
				db = hotSwapDB.DB()
				useLocalDB = false
				log.WithField("path", dbPath).
					WithField("cache_ttl", cfg.CacheTTL).
					WithField("db_mode", "r2_snapshot").
					Info("Database connected")

				var deltaErr error
				deltaLog, deltaErr = delta.NewR2Log(r2Client, cfg.R2DeltaPrefix, instanceID)
				if deltaErr != nil {
					log.WithError(deltaErr).Warn("Delta log initialization failed, missed results will not be preserved")
				}

				var scheduleErr error
				scheduleStore, scheduleErr = maintenance.NewR2ScheduleStore(r2Client, cfg.R2ScheduleKey, config.R2RequestTimeout)
				if scheduleErr != nil {
					log.WithError(scheduleErr).Warn("Schedule store initialization failed, maintenance will fall back to local scheduling")
				}

				// Start background polling for new snapshots
				snapshotMgr.StartPolling(ctx, hotSwapDB, cfg.DataDir, func(etag string) {
					snapshotReady.Store(true)
					readinessState.MarkReady()
					log.WithField("etag", etag).Info("Snapshot hot-swap applied and service marked ready")
				})
			}
		}
	}

	// Fallback to local database if R2 is disabled or failed
	if useLocalDB {
		var dbErr error
		db, dbErr = storage.New(ctx, cfg.SQLitePath(), cfg.CacheTTL)
		if dbErr != nil {
			return nil, fmt.Errorf("database: %w", dbErr)
		}
		log.WithField("path", cfg.SQLitePath()).
			WithField("cache_ttl", cfg.CacheTTL).
			WithField("db_mode", "local").
			Info("Database connected")
	}

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

	// 4. LLM Initialization
	var intentParser genai.IntentParser
	var queryExpander genai.QueryExpander
	if cfg.IsLLMEnabled() {
		llmCfg := buildLLMConfig(cfg)

		var ipErr, qeErr error
		intentParser, ipErr = genai.CreateIntentParser(ctx, llmCfg)
		if ipErr != nil {
			log.WithError(ipErr).Warn("Intent parser initialization failed")
		}
		queryExpander, qeErr = genai.CreateQueryExpander(ctx, llmCfg)
		if qeErr != nil {
			log.WithError(qeErr).Warn("Query expander initialization failed")
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

	idHandler := id.NewHandler(db, scraperClient, m, log, stickerMgr, deltaLog)

	// Create shared semester cache for course and program handlers
	semesterCache := course.NewSemesterCache()
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerMgr, deltaLog, bm25Index, queryExpander, llmLimiter, semesterCache)
	contactHandler := contact.NewHandler(db, scraperClient, m, log, stickerMgr, cfg.Bot.MaxContactsPerSearch, deltaLog)
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

	// Sentry middleware must be first to capture panics before gin.Recovery()
	if internalSentry.IsEnabled() {
		router.Use(sentrygin.New(sentrygin.Options{
			Repanic:         true,  // Re-panic after capture for gin.Recovery()
			WaitForDelivery: false, // Async sending
			Timeout:         config.SentryHTTPTimeout,
		}))
	}

	router.Use(gin.Recovery())
	router.Use(securityHeadersMiddleware())
	router.Use(loggingMiddleware(ctx, log))

	app := &Application{
		cfg:            cfg,
		logger:         log,
		db:             db,
		hotSwapDB:      hotSwapDB,
		snapshotMgr:    snapshotMgr,
		snapshotReady:  snapshotReady,
		deltaLog:       deltaLog,
		scheduleStore:  scheduleStore,
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
		readinessState: readinessState,
	}

	router.GET("/", app.redirectToGitHub)
	router.GET("/livez", app.livenessCheck)
	router.HEAD("/livez", app.livenessCheck)
	router.GET("/readyz", app.readinessCheck)
	router.HEAD("/readyz", app.readinessCheck)
	router.POST("/webhook", app.readinessMiddleware(), webhookHandler.Handle)
	router.GET("/metrics",
		// 5. Metrics Authentication
		metricsAuthMiddleware(cfg.IsMetricsAuthEnabled(), cfg.MetricsUsername, cfg.MetricsPassword),
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

func resolveServerIdentity(cfg *config.Config, hostname string, now func() time.Time) (string, string) {
	serverName := strings.TrimSpace(cfg.ServerName)
	instanceID := strings.TrimSpace(cfg.InstanceID)
	hostname = strings.TrimSpace(hostname)

	if serverName == "" {
		serverName = firstNonEmpty(
			getEnvTrim("NODE_NAME"),
			getEnvTrim("K8S_NODE_NAME"),
			getEnvTrim("KUBE_NODE_NAME"),
			getEnvTrim("MY_NODE_NAME"),
		)
		if serverName == "" {
			serverName = hostname
		}
	}

	if instanceID == "" {
		instanceID = firstNonEmpty(
			getEnvTrim("POD_UID"),
			getEnvTrim("MY_POD_UID"),
			getEnvTrim("POD_NAME"),
			getEnvTrim("MY_POD_NAME"),
			getEnvTrim("HOSTNAME"),
		)
		if instanceID == "" {
			instanceID = hostname
		}
	}

	if serverName == "" && instanceID != "" {
		serverName = instanceID
	}
	if instanceID == "" && serverName != "" {
		instanceID = serverName
	}
	if instanceID == "" {
		instanceID = fmt.Sprintf("instance-%d", now().UnixNano())
		if serverName == "" {
			serverName = instanceID
		}
	}
	return serverName, instanceID
}

func getEnvTrim(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveLogVersion(cfg *config.Config) string {
	if buildinfo.Version != "" {
		return buildinfo.Version
	}
	if buildinfo.Commit != "" {
		return buildinfo.Commit
	}

	if cfg.SentryRelease != "" {
		return cfg.SentryRelease
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	revision := ""
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if revision != "" {
		if modified {
			revision += "-dirty"
		}
		return revision
	}

	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	return ""
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

	// Check refresh state first (for initial startup)
	if !a.readinessState.IsReady() {
		status := a.readinessState.Status()
		a.logger.WithField("elapsed_seconds", status.ElapsedSeconds).
			WithField("timeout_seconds", status.TimeoutSeconds).
			Debug("Readiness check: refresh in progress")
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
		a.logger.WithError(err).Warn("Readiness check failed, database unavailable")
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
//  3. Wait for background jobs to complete (refresh, cleanup, etc.)
//  4. Close resources in order (HTTP server, webhook handler, API clients, database, rate limiters)
//
// This order prevents "sql: database is closed" errors during refresh/cleanup operations.
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
	a.logger.Info("Waiting for background jobs to finish")
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
		a.maintenanceLoop(ctx)
	})
	a.wg.Go(func() {
		a.updateCacheSizeMetrics(ctx)
	})
	a.wg.Go(func() {
		a.refreshStickers(ctx)
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

	a.logger.Info("Stopping HTTP server")
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Error("HTTP server shutdown error")
	}

	a.logger.Info("Waiting for webhook events to complete")
	if err := a.webhookHandler.Shutdown(shutdownCtx); err != nil {
		a.logger.WithError(err).Warn("Webhook handler shutdown timeout")
	}

	a.logger.Info("Closing resources")

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

	// Stop R2 snapshot polling if enabled
	if a.snapshotMgr != nil {
		a.snapshotMgr.StopPolling()
		a.logger.Info("Stopped R2 snapshot polling")
	}

	// Close database (use HotSwapDB if R2 is enabled)
	if a.hotSwapDB != nil {
		if err := a.hotSwapDB.Close(); err != nil {
			a.logger.WithError(err).WithField("component", "hotswap_database").Error("Component close error")
		}
	} else if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.logger.WithError(err).WithField("component", "database").Error("Component close error")
		}
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

	// Flush Sentry events
	if internalSentry.IsEnabled() {
		if !internalSentry.Flush(config.SentryFlushTimeout) {
			a.logger.Warn("Sentry flush timed out")
		}
	}

	a.logger.Info("Shutdown complete")
	return nil
}

// runDataCleanup performs the actual cleanup operation.
// Returns (true, nil) when cleanup ran successfully.
func (a *Application) runDataCleanup(ctx context.Context) (bool, error) {
	startTime := time.Now()
	a.logger.Info("Starting data cleanup")

	isLeader := true
	var lockAcquired bool
	if a.snapshotMgr != nil {
		var lockErr error
		lockAcquired, lockErr = a.snapshotMgr.AcquireLeaderLock(ctx)
		if lockErr != nil {
			a.logger.WithError(lockErr).Warn("Failed to acquire leader lock for cleanup, skipping this run")
			return false, lockErr
		} else if !lockAcquired {
			a.logger.Info("Another instance is leader for cleanup, skipping")
			return false, nil
		}
		if lockAcquired {
			defer func() {
				if err := a.snapshotMgr.ReleaseLeaderLock(ctx); err != nil {
					a.logger.WithError(err).Warn("Failed to release leader lock")
				}
			}()
		}
	}

	var totalDeleted int64
	var cleanupErr error

	if deleted, err := a.db.DeleteExpiredContacts(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired contacts")
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredCourses(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired courses")
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredHistoricalCourses(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired historical courses")
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredCoursePrograms(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired course programs")
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredPrograms(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired programs")
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		totalDeleted += deleted
	}

	if deleted, err := a.db.DeleteExpiredSyllabi(ctx, a.cfg.CacheTTL); err != nil {
		a.logger.WithError(err).Error("Failed to cleanup expired syllabi")
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		totalDeleted += deleted
	}

	if _, err := a.db.Writer().ExecContext(ctx, "VACUUM"); err != nil {
		a.logger.WithError(err).Warn("Failed to VACUUM database")
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if _, err := a.db.Writer().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		a.logger.WithError(err).Warn("Failed to checkpoint WAL after VACUUM")
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if _, err := a.db.Writer().ExecContext(ctx, "PRAGMA optimize"); err != nil {
		a.logger.WithError(err).Warn("Failed to optimize database")
		cleanupErr = errors.Join(cleanupErr, err)
	}

	duration := time.Since(startTime)
	a.logger.WithField("deleted", totalDeleted).
		WithField("duration_ms", duration.Milliseconds()).
		Info("Data cleanup completed")

	if a.metrics != nil {
		a.metrics.RecordJob("data_cleanup", "all", duration.Seconds())
	}

	if a.snapshotMgr != nil && isLeader {
		if a.deltaLog != nil {
			stats, err := a.deltaLog.MergeIntoDB(ctx, a.db)
			if err != nil {
				a.logger.WithError(err).Warn("Failed to merge delta logs before cleanup snapshot upload")
				cleanupErr = errors.Join(cleanupErr, err)
			} else {
				a.logger.WithField("processed", stats.ObjectsProcessed).
					WithField("merged", stats.ObjectsMerged).
					WithField("skipped", stats.ObjectsSkipped).
					Info("Delta logs merged before cleanup snapshot upload")
			}
		}

		a.logger.Info("Uploading cleanup snapshot to R2")
		etag, uploadErr := a.snapshotMgr.UploadSnapshot(ctx, a.db)
		if uploadErr != nil {
			a.logger.WithError(uploadErr).Error("Failed to upload cleanup snapshot to R2")
			cleanupErr = errors.Join(cleanupErr, uploadErr)
		} else {
			a.logger.WithField("etag", etag).Info("Cleanup snapshot uploaded to R2 successfully")
		}
	}

	if cleanupErr != nil {
		return true, cleanupErr
	}
	return true, nil
}

// refreshStickers loads sticker cache on startup (DB first, fetch if missing).
func (a *Application) refreshStickers(ctx context.Context) {
	a.logger.Debug("Sticker load job started")
	defer a.logger.Debug("Sticker load job stopped")

	initialCtx, initialCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer initialCancel()
	a.performStickerRefresh(initialCtx)

	<-ctx.Done()
	a.logger.Debug("Sticker load received shutdown signal")
}

func (a *Application) performStickerRefresh(ctx context.Context) {
	a.logger.Info("Starting sticker load")
	startTime := time.Now()

	refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := a.stickerManager.LoadStickers(refreshCtx); err != nil {
		a.logger.WithError(err).Error("Failed to load stickers")
	} else {
		count := a.stickerManager.Count()
		stats, _ := a.stickerManager.GetStats(refreshCtx)
		a.logger.WithField("count", count).
			WithField("stats", stats).
			Info("Sticker load complete")
	}

	if a.metrics != nil {
		a.metrics.RecordJob("sticker_refresh", "all", time.Since(startTime).Seconds())
	}
}

func (a *Application) runDataRefresh(ctx context.Context, includeID bool) (bool, bool, error) {
	a.logger.Info("Starting data refresh")
	startTime := time.Now()

	warmupCtx, cancel := context.WithTimeout(ctx, config.WarmupProactive)
	defer cancel()

	if includeID && a.snapshotMgr != nil && a.snapshotReady.Load() {
		a.logger.Info("Snapshot already loaded from R2, initial refresh not needed")
		a.readinessState.MarkReady()
		return false, true, nil
	}

	// R2 distributed lock: only one instance should run refresh at a time
	isLeader := true
	var lockAcquired bool
	if a.snapshotMgr != nil {
		var lockErr error
		lockAcquired, lockErr = a.snapshotMgr.AcquireLeaderLock(warmupCtx)
		if lockErr != nil {
			a.logger.WithError(lockErr).Warn("Failed to acquire leader lock, skipping this run")
			return false, false, lockErr
		} else if !lockAcquired {
			if includeID {
				a.logger.Info("Another instance is leader for initial refresh, waiting for new snapshot via polling")
			} else {
				a.logger.Info("Another instance is leader for refresh, waiting for new snapshot via polling")
			}
			return false, false, nil
		}

		a.logger.Info("Acquired leader lock, this instance will run refresh")
		if lockAcquired {
			defer func() {
				if err := a.snapshotMgr.ReleaseLeaderLock(warmupCtx); err != nil {
					a.logger.WithError(err).Warn("Failed to release leader lock")
				}
			}()
		}
	}

	opts := warmup.Options{
		Reset:         false,
		HasLLMKey:     a.cfg.IsLLMEnabled(), // Use unified check
		WarmID:        includeID,
		Metrics:       a.metrics,
		BM25Index:     a.bm25Index,
		SemesterCache: a.semesterCache,
	}

	stats, err := warmup.Run(warmupCtx, a.db, a.scraperClient, a.logger, opts)

	if err != nil {
		a.logger.WithError(err).Error("Data refresh failed")
		return true, false, err
	}

	if includeID {
		a.readinessState.MarkReady()
		a.logger.Info("Service marked as ready after initial refresh")
	}

	logEntry := a.logger.WithField("contacts", stats.Contacts.Load()).
		WithField("courses", stats.Courses.Load()).
		WithField("syllabi", stats.Syllabi.Load()).
		WithField("duration_ms", time.Since(startTime).Milliseconds()).
		WithField("initial_refresh", includeID)

	logEntry.Info("Data refresh completed")

	if a.bm25Index != nil && a.bm25Index.IsEnabled() {
		a.logger.WithField("doc_count", a.bm25Index.Count()).Info("BM25 smart search enabled")
	}

	// R2: Leader merges delta logs then uploads new snapshot after successful refresh
	if a.snapshotMgr != nil && isLeader {
		if a.deltaLog != nil {
			stats, err := a.deltaLog.MergeIntoDB(warmupCtx, a.db)
			if err != nil {
				a.logger.WithError(err).Warn("Failed to merge delta logs before snapshot upload")
				return true, false, err
			}
			a.logger.WithField("processed", stats.ObjectsProcessed).
				WithField("merged", stats.ObjectsMerged).
				WithField("skipped", stats.ObjectsSkipped).
				Info("Delta logs merged before snapshot upload")
		}

		a.logger.Info("Uploading new snapshot to R2")

		etag, uploadErr := a.snapshotMgr.UploadSnapshot(warmupCtx, a.db)
		if uploadErr != nil {
			a.logger.WithError(uploadErr).Error("Failed to upload snapshot to R2")
			return true, false, uploadErr
		}
		a.logger.WithField("etag", etag).Info("Snapshot uploaded to R2 successfully")
	}

	return true, includeID, nil
}

// maintenanceLoop coordinates refresh/cleanup based on shared schedule state.
// Startup behavior: executes initial refresh immediately if due (or snapshot missing).
// Readiness is marked only after initial refresh completes (or grace period elapses).
// Tasks are executed when due time is reached based on last execution timestamp from R2/local state.
func (a *Application) maintenanceLoop(ctx context.Context) {
	a.logger.Debug("Maintenance scheduler started")
	defer a.logger.Debug("Maintenance scheduler stopped")

	refreshInterval := a.cfg.MaintenanceRefreshInterval
	cleanupInterval := a.cfg.MaintenanceCleanupInterval
	if refreshInterval <= 0 && cleanupInterval <= 0 {
		a.logger.Warn("Maintenance scheduling disabled (refresh/cleanup intervals invalid)")
		if !a.readinessState.IsReady() {
			a.readinessState.MarkReady()
		}
		<-ctx.Done()
		return
	}

	logger := a.logger.WithField("refresh_interval", refreshInterval.String())
	if cleanupInterval > 0 {
		logger = logger.WithField("cleanup_interval", cleanupInterval.String())
	}
	logger.Info("Maintenance scheduler initialized")

	var refreshTicker *time.Ticker
	var cleanupTicker *time.Ticker
	if refreshInterval > 0 {
		refreshTicker = time.NewTicker(refreshInterval)
		defer refreshTicker.Stop()
	}
	if cleanupInterval > 0 {
		cleanupTicker = time.NewTicker(cleanupInterval)
		defer cleanupTicker.Stop()
	}

	localState := maintenance.State{}
	resolveState := func(ctx context.Context) (maintenance.State, bool) {
		state := localState
		useLocal := a.scheduleStore == nil
		if a.scheduleStore != nil {
			loaded, _, err := a.scheduleStore.Ensure(ctx)
			if err != nil {
				a.logger.WithError(err).Warn("Failed to load maintenance schedule state, using local fallback")
				useLocal = true
			} else {
				state = loaded
			}
		}
		return state, useLocal
	}
	updateLocal := func(update func(*maintenance.State)) {
		update(&localState)
		localState.UpdatedAt = time.Now().UTC().Unix()
	}
	updateRemote := func(ctx context.Context, update func(*maintenance.State)) bool {
		if a.scheduleStore == nil {
			return false
		}
		if err := a.scheduleStore.Update(ctx, update); err != nil {
			a.logger.WithError(err).Warn("Failed to update maintenance schedule state, falling back to local")
			return false
		}
		return true
	}

	runRefreshIfDue := func(now time.Time) {
		if refreshInterval <= 0 {
			if !a.readinessState.WarmupCompleted() {
				a.readinessState.MarkReady()
				a.logger.Info("Maintenance refresh disabled; service marked ready")
			}
			return
		}

		state, useLocal := resolveState(ctx)
		snapshotReady := a.snapshotMgr != nil && a.snapshotReady.Load()
		if snapshotReady && state.LastRefresh == 0 {
			completedAt := time.Now().UTC()
			if useLocal || !updateRemote(ctx, func(s *maintenance.State) {
				s.LastRefresh = completedAt.Unix()
			}) {
				updateLocal(func(s *maintenance.State) {
					s.LastRefresh = completedAt.Unix()
				})
			}
			if !a.readinessState.WarmupCompleted() {
				a.readinessState.MarkReady()
				a.logger.Info("Initial refresh skipped due to snapshot; service marked ready")
			}
			return
		}

		forceInitial := a.snapshotMgr != nil && !a.snapshotReady.Load()
		shouldRun := isMaintenanceDue(state.LastRefresh, refreshInterval, now) || forceInitial
		if !shouldRun {
			if !a.readinessState.WarmupCompleted() {
				a.readinessState.MarkReady()
				a.logger.Info("Initial refresh not required; service marked ready")
			}
			return
		}

		isInitialRefresh := state.LastRefresh == 0 || forceInitial
		ran, _, err := a.runDataRefresh(ctx, isInitialRefresh)
		if err != nil {
			a.logger.WithError(err).Warn("Data refresh run failed")
			if isInitialRefresh && !a.readinessState.WarmupCompleted() {
				a.readinessState.MarkReady()
				a.logger.Warn("Initial data refresh failed; service marked ready in degraded mode")
			}
			return
		}
		if ran {
			completedAt := time.Now().UTC()
			if useLocal || !updateRemote(ctx, func(s *maintenance.State) {
				s.LastRefresh = completedAt.Unix()
			}) {
				updateLocal(func(s *maintenance.State) {
					s.LastRefresh = completedAt.Unix()
				})
			}
		}
	}

	runCleanupIfDue := func(now time.Time) {
		if cleanupInterval <= 0 {
			return
		}
		state, useLocal := resolveState(ctx)
		if !isMaintenanceDue(state.LastCleanup, cleanupInterval, now) {
			return
		}
		ran, err := a.runDataCleanup(ctx)
		if err != nil {
			a.logger.WithError(err).Warn("Data cleanup run failed")
			return
		}
		if ran {
			completedAt := time.Now().UTC()
			if useLocal || !updateRemote(ctx, func(s *maintenance.State) {
				s.LastCleanup = completedAt.Unix()
			}) {
				updateLocal(func(s *maintenance.State) {
					s.LastCleanup = completedAt.Unix()
				})
			}
		}
	}

	// Run once immediately on startup
	now := time.Now().UTC()
	runRefreshIfDue(now)
	runCleanupIfDue(now)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tickerChannel(refreshTicker):
			runRefreshIfDue(time.Now().UTC())
		case <-tickerChannel(cleanupTicker):
			runCleanupIfDue(time.Now().UTC())
		}
	}
}

func tickerChannel(ticker *time.Ticker) <-chan time.Time {
	if ticker == nil {
		return nil
	}
	return ticker.C
}

func isMaintenanceDue(lastUnix int64, interval time.Duration, now time.Time) bool {
	if interval <= 0 {
		return false
	}
	if lastUnix == 0 {
		return true
	}
	last := time.Unix(lastUnix, 0).UTC()
	return now.Sub(last) >= interval
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
	a.metrics.SetCacheSize("program", programCount)
	a.metrics.SetCacheSize("stickers", stickerCount)

	if a.bm25Index != nil {
		a.metrics.SetIndexSize("bm25", a.bm25Index.Count())
	}
}

// readinessMiddleware rejects webhook requests with 503 when warmup wait is enabled
// and initial refresh is not ready. LINE Platform will automatically retry.
func (a *Application) readinessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.cfg.WaitForWarmup && !a.readinessState.IsReady() {
			status := a.readinessState.Status()
			a.logger.WithField("elapsed_seconds", status.ElapsedSeconds).
				Debug("Webhook rejected: refresh in progress")
			c.Header("Retry-After", "60")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":       "service refreshing",
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
func loggingMiddleware(baseCtx context.Context, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		//nolint:contextcheck // Use request-scoped context for cancellation and tracing.
		reqCtx := c.Request.Context()
		if reqCtx == nil {
			reqCtx = baseCtx
		}

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
			reqCtx = ctxutil.WithRequestID(reqCtx, requestID)
			c.Request = c.Request.WithContext(reqCtx)
			if hub := sentrygin.GetHubFromContext(c); hub != nil {
				hub.Scope().SetTag("request_id", requestID)
			}
		}

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()
		entry := log.WithField("http_method", method).
			WithField("http_path", path).
			WithField("http_status", status).
			WithField("duration_ms", duration.Milliseconds()).
			WithField("client_ip", c.ClientIP())

		if status >= 500 {
			entry.ErrorContext(reqCtx, "HTTP request failed")
		} else if status >= 400 && status != 404 {
			entry.WarnContext(reqCtx, "HTTP request rejected")
		} else if status == 404 {
			entry.DebugContext(reqCtx, "HTTP request not found")
		} else {
			entry.DebugContext(reqCtx, "HTTP request completed")
		}
	}
}

func resolveSentryEnvironment(explicit string, logLevel string) string {
	if explicit != "" {
		return explicit
	}
	if logLevel == "debug" {
		return "development"
	}
	return "production"
}
