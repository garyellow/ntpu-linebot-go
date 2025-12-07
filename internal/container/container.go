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
// It follows pure Dependency Injection pattern - dependencies are initialized
// during container creation and injected into dependent components.
// NO getter methods - prevents service locator anti-pattern.
//
// Initialization order:
// 1. Core services (database, metrics, scraper)
// 2. Bot handlers (id, course, contact)
// 3. GenAI components (optional - intent parser, query expander, BM25)
// 4. Webhook handler (aggregates all dependencies)
type Container struct {
	// Configuration and logging (needed for lifecycle management)
	cfg    *config.Config
	logger *logger.Logger

	// Core services
	db             *storage.DB
	metrics        *metrics.Metrics
	registry       *prometheus.Registry
	scraperClient  *scraper.Client
	stickerManager *sticker.Manager

	// Bot components
	botHandlers    *BotHandlers
	botRegistry    *bot.Registry
	webhookHandler *webhook.Handler

	// GenAI components (optional)
	genaiComponents *GenAIComponents
}

// BotHandlers groups all bot handler instances.
type BotHandlers struct {
	ID      *id.Handler
	Course  *course.Handler
	Contact *contact.Handler
}

// GenAIComponents groups all GenAI-related components.
type GenAIComponents struct {
	IntentParser  genai.IntentParser
	QueryExpander *genai.QueryExpander
	BM25Index     *rag.BM25Index
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
// This implements Pure Dependency Injection - all dependencies are assembled here,
// and the Application receives everything it needs via constructor injection.
//
// Initialization order:
//  1. Core services (database, metrics, scraper, sticker manager)
//  2. Bot handlers (id, course, contact) and registry
//  3. GenAI components (optional - requires GEMINI_API_KEY)
//  4. Webhook handler (aggregates all dependencies)
//  5. Application (HTTP server with all dependencies injected)
//
// GenAI initialization failure is non-fatal and only logs a warning.
func (c *Container) Initialize(ctx context.Context) (*Application, error) {
	c.logger.Info("Initializing application container...")

	if err := c.initCoreServices(ctx); err != nil {
		return nil, fmt.Errorf("core services: %w", err)
	}

	if err := c.initBotHandlers(ctx); err != nil {
		return nil, fmt.Errorf("bot handlers: %w", err)
	}

	if err := c.initGenAI(ctx); err != nil {
		// GenAI is optional, log warning but don't fail
		c.logger.WithError(err).Warn("GenAI initialization failed, features disabled")
	}

	if err := c.initWebhook(ctx); err != nil {
		return nil, fmt.Errorf("webhook handler: %w", err)
	}

	c.logger.Info("Container initialized successfully")

	// Create fully-configured Application with all dependencies injected
	app := NewApplication(
		c.cfg,
		c.logger,
		c.db,
		c.metrics,
		c.registry,
		c.scraperClient,
		c.stickerManager,
		c.webhookHandler,
		c.genaiComponents,
		c, // Container itself for lifecycle management
	)

	return app, nil
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

// initBotHandlers creates all bot handlers and registry.
func (c *Container) initBotHandlers(ctx context.Context) error {
	c.botHandlers = &BotHandlers{
		ID: id.NewHandler(
			id.WithRepository(c.db),
			id.WithScraper(c.scraperClient),
			id.WithMetrics(c.metrics),
			id.WithLogger(c.logger),
			id.WithStickerManager(c.stickerManager),
		),
		Course: course.NewHandler(
			course.WithRepository(c.db),
			course.WithSyllabusRepository(c.db),
			course.WithScraper(c.scraperClient),
			course.WithMetrics(c.metrics),
			course.WithLogger(c.logger),
			course.WithStickerManager(c.stickerManager),
		),
		Contact: contact.NewHandler(
			contact.WithRepository(c.db),
			contact.WithScraper(c.scraperClient),
			contact.WithMetrics(c.metrics),
			contact.WithLogger(c.logger),
			contact.WithStickerManager(c.stickerManager),
			contact.WithMaxContactsLimit(config.DefaultBotConfig().MaxContactsPerSearch),
		),
	}

	// Create registry and register handlers in priority order
	c.botRegistry = bot.NewRegistry()
	c.botRegistry.Register(c.botHandlers.Contact) // Priority 1
	c.botRegistry.Register(c.botHandlers.Course)  // Priority 2
	c.botRegistry.Register(c.botHandlers.ID)      // Priority 3

	c.logger.Info("Bot handlers initialized and registered")
	return nil
}

// initGenAI initializes optional GenAI features (intent parser, query expander, BM25 index).
// Returns error if API key is configured but initialization fails.
// Returns nil if API key is not configured (features disabled).
func (c *Container) initGenAI(ctx context.Context) error {
	if c.cfg.GeminiAPIKey == "" {
		c.logger.Info("Gemini API key not configured, GenAI features disabled")
		return nil
	}

	c.genaiComponents = &GenAIComponents{}

	// Intent Parser
	intentParser, err := genai.NewIntentParser(ctx, c.cfg.GeminiAPIKey)
	if err != nil {
		return fmt.Errorf("intent parser: %w", err)
	}
	c.genaiComponents.IntentParser = intentParser
	c.logger.Info("Intent parser enabled")

	// Query Expander
	queryExpander, err := genai.NewQueryExpander(ctx, c.cfg.GeminiAPIKey)
	if err != nil {
		return fmt.Errorf("query expander: %w", err)
	}
	c.genaiComponents.QueryExpander = queryExpander
	c.logger.Info("Query expander enabled")

	// BM25 Index (requires syllabi from database)
	syllabi, err := c.db.GetAllSyllabi(ctx)
	if err != nil {
		return fmt.Errorf("load syllabi for BM25: %w", err)
	}

	bm25Index := rag.NewBM25Index(c.logger)
	if len(syllabi) > 0 {
		if err := bm25Index.Initialize(syllabi); err != nil {
			c.logger.WithError(err).Warn("BM25 index initialization failed")
		} else {
			c.logger.WithField("doc_count", bm25Index.Count()).Info("BM25 index initialized")
		}
	} else {
		c.logger.Warn("No syllabi found, BM25 index disabled")
	}
	c.genaiComponents.BM25Index = bm25Index

	// Configure course handler with GenAI features
	c.botHandlers.Course.SetBM25Index(bm25Index)
	if queryExpander != nil {
		c.botHandlers.Course.SetQueryExpander(queryExpander)
		c.logger.Info("Course handler configured with query expander")
	}

	c.logger.Info("GenAI features enabled")
	return nil
}

// initWebhook creates the webhook handler with all dependencies.
func (c *Container) initWebhook(ctx context.Context) error {
	// Build bot config from app config
	botCfg := config.DefaultBotConfig()
	botCfg.WebhookTimeout = c.cfg.WebhookTimeout
	botCfg.UserRateLimitTokens = c.cfg.UserRateLimitTokens
	botCfg.UserRateLimitRefillRate = c.cfg.UserRateLimitRefillRate
	botCfg.LLMRateLimitPerHour = c.cfg.LLMRateLimitPerHour

	// Build handler options
	opts := []webhook.HandlerOption{
		webhook.WithBotConfig(botCfg),
		webhook.WithStickerManager(c.stickerManager),
	}

	// Add intent parser if available
	if c.genaiComponents != nil && c.genaiComponents.IntentParser != nil {
		opts = append(opts, webhook.WithIntentParser(c.genaiComponents.IntentParser))
	}

	// Create webhook handler
	handler, err := webhook.NewHandler(
		c.cfg.LineChannelSecret,
		c.cfg.LineChannelToken,
		c.botRegistry,
		c.metrics,
		c.logger,
		opts...,
	)
	if err != nil {
		return fmt.Errorf("webhook handler creation: %w", err)
	}

	// Configure course handler with LLM rate limiter (for query expansion)
	if c.genaiComponents != nil && c.genaiComponents.QueryExpander != nil {
		c.botHandlers.Course.SetLLMRateLimiter(
			handler.GetLLMRateLimiter(),
			c.cfg.LLMRateLimitPerHour,
		)
		c.logger.Info("Course handler configured with LLM rate limiter")
	}

	c.webhookHandler = handler
	c.logger.Info("Webhook handler initialized")
	return nil
}

// Close gracefully shuts down all services in reverse initialization order.
// Returns aggregated errors if multiple components fail to close.
func (c *Container) Close() error {
	c.logger.Info("Closing container...")
	var errs []error

	// Stop webhook background tasks
	if c.webhookHandler != nil {
		c.webhookHandler.Stop()
	}

	// Close GenAI components
	if c.genaiComponents != nil {
		if c.genaiComponents.IntentParser != nil {
			if err := c.genaiComponents.IntentParser.Close(); err != nil {
				c.logger.WithError(err).Error("Failed to close intent parser")
				errs = append(errs, fmt.Errorf("intent parser: %w", err))
			}
		}
		if c.genaiComponents.QueryExpander != nil {
			if err := c.genaiComponents.QueryExpander.Close(); err != nil {
				c.logger.WithError(err).Error("Failed to close query expander")
				errs = append(errs, fmt.Errorf("query expander: %w", err))
			}
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

	c.logger.Info("Container closed successfully")
	return nil
}
