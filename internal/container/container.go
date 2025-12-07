// Package container manages application dependencies and their lifecycle.
// It implements the Application Container pattern to provide centralized
// dependency injection with clear initialization order and error handling.
package container

import (
	"context"
	"fmt"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/course"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/id"
	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/webhook"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Container manages application dependencies and their lifecycle.
// Initialization order:
// 1. Core services (database, metrics, scraper, sticker manager)
// 2. Bot handlers (id, course, contact)
// 3. GenAI components (optional - BM25 index, intent parser, query expander)
// 4. Webhook handler (aggregates all dependencies)
type Container struct {
	cfg    *config.Config
	logger *logger.Logger

	// Core services (closed in reverse order)
	db             *storage.DB
	metrics        *metrics.Metrics
	registry       *prometheus.Registry
	scraperClient  *scraper.Client
	stickerManager *sticker.Manager

	// Bot handlers
	idHandler      *id.Handler
	courseHandler  *course.Handler
	contactHandler *contact.Handler

	// GenAI components (optional)
	bm25Index     *rag.BM25Index
	intentParser  genai.IntentParser
	queryExpander *genai.QueryExpander
}

// New creates a new dependency container.
// Only configuration is required; all other dependencies are lazy-initialized.
func New(cfg *config.Config) *Container {
	return &Container{
		cfg:    cfg,
		logger: logger.New(cfg.LogLevel),
	}
}

// Initialize performs full dependency initialization and returns a fully-configured Application.
// Initialization order: Core → GenAI (optional) → Bot Handlers → Webhook
// GenAI is initialized before handlers so they can receive GenAI options during construction.
func (c *Container) Initialize(ctx context.Context) (*Application, error) {
	c.logger.Info("Initializing application...")

	if err := c.initCoreServices(ctx); err != nil {
		return nil, fmt.Errorf("core services: %w", err)
	}

	if err := c.initGenAI(ctx); err != nil {
		c.logger.WithError(err).Warn("GenAI features disabled")
	}

	if err := c.initBotHandlers(ctx); err != nil {
		return nil, fmt.Errorf("bot handlers: %w", err)
	}

	webhookHandler, err := c.initWebhook(ctx)
	if err != nil {
		return nil, fmt.Errorf("webhook: %w", err)
	}

	c.logger.Info("Initialization complete")

	return NewApplication(
		c.cfg,
		c.logger,
		c.db,
		c.metrics,
		c.registry,
		c.scraperClient,
		c.stickerManager,
		webhookHandler,
		c.bm25Index,
		c.intentParser,
		c.queryExpander,
	), nil
}

// initCoreServices initializes database, metrics, scraper, and sticker manager.
func (c *Container) initCoreServices(ctx context.Context) error {
	// Database
	db, err := storage.New(c.cfg.SQLitePath(), c.cfg.CacheTTL)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	c.db = db
	c.logger.WithField("path", c.cfg.SQLitePath()).
		WithField("cache_ttl", c.cfg.CacheTTL).
		Info("Database connected")

	// Prometheus registry and metrics
	c.registry = prometheus.NewRegistry()
	c.registry.MustRegister(collectors.NewGoCollector())
	c.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	c.registry.MustRegister(collectors.NewBuildInfoCollector())
	c.metrics = metrics.New(c.registry)

	// Scraper client
	c.scraperClient = scraper.NewClient(c.cfg.ScraperTimeout, c.cfg.ScraperMaxRetries)

	// Sticker manager
	c.stickerManager = sticker.NewManager(c.db, c.scraperClient, c.logger)

	c.logger.Info("Core services initialized")
	return nil
}

// initBotHandlers creates all bot handlers with GenAI options if available.
func (c *Container) initBotHandlers(ctx context.Context) error {
	c.idHandler = id.NewHandler(
		c.db,
		c.scraperClient,
		c.metrics,
		c.logger,
		c.stickerManager,
	)

	// Build course handler options
	courseOpts := []course.HandlerOption{}
	if c.bm25Index != nil {
		courseOpts = append(courseOpts, course.WithBM25Index(c.bm25Index))
	}
	if c.queryExpander != nil {
		courseOpts = append(courseOpts, course.WithQueryExpander(c.queryExpander))
	}

	c.courseHandler = course.NewHandler(
		c.db,
		c.scraperClient,
		c.metrics,
		c.logger,
		c.stickerManager,
		courseOpts...,
	)

	c.contactHandler = contact.NewHandler(
		c.db,
		c.scraperClient,
		c.metrics,
		c.logger,
		c.stickerManager,
		contact.WithMaxContactsLimit(config.DefaultBotConfig().MaxContactsPerSearch),
	)

	c.logger.Info("Bot handlers initialized")
	return nil
}

// initGenAI initializes optional GenAI features (BM25 index, intent parser, query expander).
// BM25 index is always initialized if syllabi exist (independent of Gemini API).
// Intent parser and query expander require Gemini API key.
// Returns error if API key is configured but initialization fails.
func (c *Container) initGenAI(ctx context.Context) error {
	// Initialize BM25 Index (independent of Gemini API, uses local syllabi data)
	syllabi, err := c.db.GetAllSyllabi(ctx)
	if err != nil {
		return fmt.Errorf("load syllabi for BM25: %w", err)
	}

	c.bm25Index = rag.NewBM25Index(c.logger)
	if len(syllabi) > 0 {
		if err := c.bm25Index.Initialize(syllabi); err != nil {
			c.logger.WithError(err).Warn("BM25 index initialization failed")
		} else {
			c.logger.WithField("doc_count", c.bm25Index.Count()).Info("BM25 index initialized")
		}
	} else {
		c.logger.Warn("No syllabi found, BM25 search disabled")
	}

	// Initialize Gemini-based features (requires API key)
	if c.cfg.GeminiAPIKey == "" {
		c.logger.Info("Gemini API key not configured, NLU and query expansion disabled")
		return nil
	}

	// Intent Parser (NLU for unmatched messages)
	intentParser, err := genai.NewIntentParser(ctx, c.cfg.GeminiAPIKey)
	if err != nil {
		return fmt.Errorf("intent parser: %w", err)
	}
	c.intentParser = intentParser
	c.logger.Info("Intent parser enabled")

	// Query Expander (improves BM25 search results)
	queryExpander, err := genai.NewQueryExpander(ctx, c.cfg.GeminiAPIKey)
	if err != nil {
		return fmt.Errorf("query expander: %w", err)
	}
	c.queryExpander = queryExpander
	c.logger.Info("Query expander enabled")

	c.logger.Info("GenAI features enabled")
	return nil
}

// initWebhook creates and returns the webhook handler with all dependencies.
func (c *Container) initWebhook(ctx context.Context) (*webhook.Handler, error) {
	// Build registry and register handlers in priority order
	registry := bot.NewRegistry()
	registry.Register(c.contactHandler) // Priority 1
	registry.Register(c.courseHandler)  // Priority 2
	registry.Register(c.idHandler)      // Priority 3

	// Build bot config
	botCfg, err := config.LoadBotConfig()
	if err != nil {
		return nil, fmt.Errorf("load bot config: %w", err)
	}
	if c.cfg.WebhookTimeout != 0 {
		botCfg.WebhookTimeout = c.cfg.WebhookTimeout
	}
	if c.cfg.UserRateLimitTokens != 0 {
		botCfg.UserRateLimitTokens = c.cfg.UserRateLimitTokens
	}
	if c.cfg.UserRateLimitRefillRate != 0 {
		botCfg.UserRateLimitRefillRate = c.cfg.UserRateLimitRefillRate
	}
	if c.cfg.LLMRateLimitPerHour != 0 {
		botCfg.LLMRateLimitPerHour = c.cfg.LLMRateLimitPerHour
	}

	// Build handler options
	opts := []webhook.HandlerOption{
		webhook.WithBotConfig(botCfg),
		webhook.WithStickerManager(c.stickerManager),
	}
	if c.intentParser != nil {
		opts = append(opts, webhook.WithIntentParser(c.intentParser))
	}

	// Create webhook handler
	handler, err := webhook.NewHandler(
		c.cfg.LineChannelSecret,
		c.cfg.LineChannelToken,
		registry,
		c.metrics,
		c.logger,
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("create handler: %w", err)
	}

	// Add LLM rate limiter to course handler if query expander is available
	if c.queryExpander != nil {
		courseOpts := []course.HandlerOption{
			course.WithBM25Index(c.bm25Index),
			course.WithQueryExpander(c.queryExpander),
			course.WithLLMRateLimiter(handler.GetLLMRateLimiter(), c.cfg.LLMRateLimitPerHour),
		}

		c.courseHandler = course.NewHandler(
			c.db,
			c.scraperClient,
			c.metrics,
			c.logger,
			c.stickerManager,
			courseOpts...,
		)

		// Recreate registry with updated course handler
		registry = bot.NewRegistry()
		registry.Register(c.contactHandler)
		registry.Register(c.courseHandler)
		registry.Register(c.idHandler)

		// Recreate webhook handler with updated registry
		handler, err = webhook.NewHandler(
			c.cfg.LineChannelSecret,
			c.cfg.LineChannelToken,
			registry,
			c.metrics,
			c.logger,
			opts...,
		)
		if err != nil {
			return nil, fmt.Errorf("recreate handler: %w", err)
		}
	}

	c.logger.Info("Webhook handler initialized")
	return handler, nil
}

// Close gracefully shuts down all services in reverse initialization order.
func (c *Container) Close() error {
	c.logger.Info("Closing services...")
	var errs []error

	// Close GenAI components
	if c.queryExpander != nil {
		if err := c.queryExpander.Close(); err != nil {
			c.logger.WithError(err).Error("Failed to close query expander")
			errs = append(errs, fmt.Errorf("query expander: %w", err))
		}
	}
	if c.intentParser != nil {
		if err := c.intentParser.Close(); err != nil {
			c.logger.WithError(err).Error("Failed to close intent parser")
			errs = append(errs, fmt.Errorf("intent parser: %w", err))
		}
	}

	// Close database
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			c.logger.WithError(err).Error("Failed to close database")
			errs = append(errs, fmt.Errorf("database: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	c.logger.Info("Services closed")
	return nil
}
