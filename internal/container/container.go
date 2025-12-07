// Package container manages application dependencies and their lifecycle.
// It implements the Application Container pattern to provide centralized
// dependency injection with clear initialization order and error handling.
package container

import (
	"context"
	"errors"
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

// Container manages application dependencies using Pure DI pattern.
//
// Initialization phases:
//  1. Core services: database, metrics, scraper, sticker manager
//  2. GenAI components: BM25 index, intent parser, query expander (optional)
//  3. Bot handlers: id, course, contact (with GenAI features injected)
//  4. Webhook handler: aggregates all handlers and provides unified entry point
//
// Lifecycle management:
//   - Resources are closed in reverse initialization order
//   - GenAI components implement Close() for proper cleanup
//   - Database connection pooling handled by modernc.org/sqlite
type Container struct {
	cfg    *config.Config
	logger *logger.Logger

	// Core services
	db             *storage.DB
	metrics        *metrics.Metrics
	registry       *prometheus.Registry
	scraperClient  *scraper.Client
	stickerManager *sticker.Manager

	// Bot handlers
	idHandler      *id.Handler
	courseHandler  *course.Handler
	contactHandler *contact.Handler

	// GenAI components (optional, nil if disabled)
	bm25Index     *rag.BM25Index
	intentParser  genai.IntentParser
	queryExpander *genai.QueryExpander
}

// New creates a dependency container with configuration.
// Logger is initialized immediately for early error reporting.
// All other dependencies are initialized via Initialize().
func New(cfg *config.Config) *Container {
	return &Container{
		cfg:    cfg,
		logger: logger.New(cfg.LogLevel),
	}
}

// Initialize performs dependency initialization and returns a configured Application.
//
// Initialization sequence ensures proper dependency flow:
//  1. Core services (required for all modules)
//  2. GenAI components (optional, error is logged but not fatal)
//  3. Bot handlers (receive GenAI features via constructor options)
//  4. Webhook handler (aggregates handlers and owns rate limiters)
//
// Returns error only for critical failures. GenAI initialization errors
// are logged as warnings and the application continues without those features.
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

// initCoreServices initializes foundational services required by all modules.
//
// Initialization sequence:
//  1. Database - SQLite with WAL mode (modernc.org/sqlite)
//  2. Metrics - Prometheus registry with standard collectors
//  3. Scraper - HTTP client with retry logic and failover URLs
//  4. Sticker manager - Avatar URL provider for LINE messages
//
// All services are mandatory; initialization failure is fatal.
func (c *Container) initCoreServices(ctx context.Context) error {
	db, err := storage.New(c.cfg.SQLitePath(), c.cfg.CacheTTL)
	if err != nil {
		return fmt.Errorf("database connection: %w", err)
	}
	c.db = db
	c.logger.WithField("path", c.cfg.SQLitePath()).
		WithField("cache_ttl", c.cfg.CacheTTL).
		Info("Database connected")

	c.registry = prometheus.NewRegistry()
	c.registry.MustRegister(collectors.NewGoCollector())
	c.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	c.registry.MustRegister(collectors.NewBuildInfoCollector())
	c.metrics = metrics.New(c.registry)

	c.scraperClient = scraper.NewClient(c.cfg.ScraperTimeout, c.cfg.ScraperMaxRetries)

	c.stickerManager = sticker.NewManager(c.db, c.scraperClient, c.logger)

	c.logger.Info("Core services initialized")
	return nil
}

// initBotHandlers creates bot handlers with their dependencies.
//
// Handler construction patterns:
//   - ID handler: Direct parameter injection (no optional features)
//   - Contact handler: Direct parameter injection with static config
//   - Course handler: Functional options pattern for GenAI features
//
// GenAI features (BM25, QueryExpander) are injected as options
// to avoid nil checks in handler logic.
func (c *Container) initBotHandlers(ctx context.Context) error {
	c.idHandler = id.NewHandler(
		c.db,
		c.scraperClient,
		c.metrics,
		c.logger,
		c.stickerManager,
	)

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

	botCfg := config.DefaultBotConfig()
	c.contactHandler = contact.NewHandler(
		c.db,
		c.scraperClient,
		c.metrics,
		c.logger,
		c.stickerManager,
		botCfg.MaxContactsPerSearch,
	)

	c.logger.Info("Bot handlers initialized")
	return nil
}

// initGenAI initializes optional GenAI features.
//
// Features initialized:
//  1. BM25 Index - Local syllabus search (no API key required)
//  2. Intent Parser - NLU for unmatched messages (requires Gemini API)
//  3. Query Expander - Enhances BM25 results (requires Gemini API)
//
// BM25 operates independently of Gemini API. If no API key is configured,
// only keyword-based features remain available.
//
// Returns error if API key is configured but initialization fails.
// Missing API key or empty syllabi are logged as info/warning, not errors.
func (c *Container) initGenAI(ctx context.Context) error {
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

	if c.cfg.GeminiAPIKey == "" {
		c.logger.Info("Gemini API key not configured, NLU and query expansion disabled")
		return nil
	}

	intentParser, err := genai.NewIntentParser(ctx, c.cfg.GeminiAPIKey)
	if err != nil {
		return fmt.Errorf("intent parser: %w", err)
	}
	c.intentParser = intentParser
	c.logger.Info("Intent parser enabled")

	queryExpander, err := genai.NewQueryExpander(ctx, c.cfg.GeminiAPIKey)
	if err != nil {
		return fmt.Errorf("query expander: %w", err)
	}
	c.queryExpander = queryExpander
	c.logger.Info("Query expander enabled")

	c.logger.Info("GenAI features enabled")
	return nil
}

// initWebhook creates the webhook handler and wires it to bot handlers.
//
// Registry priority determines handler matching order:
//  1. Contact - Most specific patterns (emergency, org names)
//  2. Course - Medium specificity (course codes, keywords)
//  3. ID - Broader patterns (student IDs, departments)
//
// Configuration cascade:
//   - Loads default BotConfig
//   - Overrides with environment-specific values
//   - Applies via functional options
//
// Post-construction injection:
//   - LLM rate limiter is owned by webhook (shared across NLU + query expansion)
//   - Injected into course handler after webhook creation
//   - Ensures unified quota management for all LLM features
func (c *Container) initWebhook(ctx context.Context) (*webhook.Handler, error) {
	registry := bot.NewRegistry()
	registry.Register(c.contactHandler)
	registry.Register(c.courseHandler)
	registry.Register(c.idHandler)

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

	opts := []webhook.HandlerOption{
		webhook.WithBotConfig(botCfg),
		webhook.WithStickerManager(c.stickerManager),
	}
	if c.intentParser != nil {
		opts = append(opts, webhook.WithIntentParser(c.intentParser))
	}

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

	if c.queryExpander != nil && c.courseHandler != nil {
		c.courseHandler.SetLLMRateLimiter(handler.GetLLMRateLimiter())
	}

	c.logger.Info("Webhook handler initialized")
	return handler, nil
}

// Close gracefully shuts down services in reverse initialization order.
// Errors are logged individually but all cleanup operations are attempted.
// Returns combined error if any closures fail.
func (c *Container) Close() error {
	c.logger.Info("Closing services...")
	var errs []error

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

	if c.db != nil {
		if err := c.db.Close(); err != nil {
			c.logger.WithError(err).Error("Failed to close database")
			errs = append(errs, fmt.Errorf("database: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	c.logger.Info("Services closed")
	return nil
}
