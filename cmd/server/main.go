// Package main provides the LINE bot server entry point.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
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
	"github.com/prometheus/client_golang/prometheus/collectors"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.LogLevel)
	log.Info("Starting NTPU LineBot Server")

	// Connect to database with configured TTL
	db, err := storage.New(cfg.SQLitePath(), cfg.CacheTTL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer func() { _ = db.Close() }()
	log.WithField("path", cfg.SQLitePath()).
		WithField("cache_ttl", cfg.CacheTTL).
		Info("Database connected")

	// Create Prometheus registry
	registry := prometheus.NewRegistry()

	// Register Go and process collectors
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewBuildInfoCollector())

	// Create metrics
	m := metrics.New(registry)
	log.Info("Metrics initialized")

	// Set metrics recorder for database integrity checks
	db.SetMetrics(m)

	// Create scraper client
	scraperClient := scraper.NewClient(
		cfg.ScraperTimeout,
		cfg.ScraperMaxRetries,
	)
	log.Info("Scraper client created")

	// Create sticker manager with database and scraper client
	stickerManager := sticker.NewManager(db, scraperClient, log)
	log.Info("Sticker manager created")

	// Create vector database for semantic search (optional - requires Gemini API key)
	var vectorDB *rag.VectorDB
	var bm25Index *rag.BM25Index
	var hybridSearcher *rag.HybridSearcher

	if cfg.GeminiAPIKey != "" {
		var err error
		vectorDB, err = rag.NewVectorDB(cfg.DataDir, cfg.GeminiAPIKey, log)
		if err != nil {
			log.WithError(err).Warn("Failed to create vector database, semantic search disabled")
		} else if vectorDB != nil {
			// Initialize vector store with existing syllabi from database
			syllabi, err := db.GetAllSyllabi(context.Background())
			if err != nil {
				log.WithError(err).Warn("Failed to load syllabi for vector store initialization")
			} else {
				// Initialize vector database
				if err := vectorDB.Initialize(context.Background(), syllabi); err != nil {
					log.WithError(err).Warn("Failed to initialize vector store")
				} else {
					log.WithField("syllabi_count", len(syllabi)).Info("Vector database initialized for semantic search")
				}

				// Initialize BM25 index for hybrid search
				bm25Index = rag.NewBM25Index(log)
				if err := bm25Index.Initialize(syllabi); err != nil {
					log.WithError(err).Warn("Failed to initialize BM25 index")
					log.Warn("Hybrid search will use vector-only fallback (degraded search quality)")
					bm25Index = nil
				} else {
					log.WithField("doc_count", bm25Index.Count()).Info("BM25 index initialized for hybrid search")
				}
			}
		}
	} else {
		log.Info("Gemini API key not configured, semantic search disabled")
	}

	// Create hybrid searcher if either component is available
	if vectorDB != nil || bm25Index != nil {
		hybridSearcher = rag.NewHybridSearcher(vectorDB, bm25Index, log)
		log.WithFields(map[string]any{
			"vector_enabled": vectorDB != nil && vectorDB.IsEnabled(),
			"bm25_enabled":   bm25Index != nil && bm25Index.IsEnabled(),
		}).Info("Hybrid searcher created")
	}

	// Start background cache warming (non-blocking)
	// Warmup runs concurrently with server startup until completion
	warmup.RunInBackground(context.Background(), db, scraperClient, stickerManager, log, warmup.Options{
		Modules:  warmup.ParseModules(cfg.WarmupModules),
		Reset:    false,    // Never reset in production
		Metrics:  m,        // Pass metrics for monitoring
		VectorDB: vectorDB, // Pass vector database for syllabus indexing
	})
	log.Info("Background cache warming started")

	// Create webhook handler
	webhookHandler, err := webhook.NewHandler(
		cfg.LineChannelSecret,
		cfg.LineChannelToken,
		db,
		scraperClient,
		m,
		log,
		stickerManager,
		cfg.WebhookTimeout,
		cfg.UserRateLimitTokens,
		cfg.UserRateLimitRefillRate,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create webhook handler")
	}

	// Set hybrid searcher for course semantic search (includes BM25 + vector)
	if hybridSearcher != nil {
		webhookHandler.GetCourseHandler().SetHybridSearcher(hybridSearcher)
		log.Info("Hybrid search enabled for course module")
	} else if vectorDB != nil {
		// Fallback to vector-only if hybrid not available
		webhookHandler.GetCourseHandler().SetVectorDB(vectorDB) //nolint:staticcheck // Intentional fallback for backward compatibility
		log.Info("Vector-only semantic search enabled for course module")
	}

	// Create query expander for semantic search (optional - requires Gemini API key)
	if cfg.GeminiAPIKey != "" {
		expander, err := genai.NewQueryExpander(context.Background(), cfg.GeminiAPIKey)
		if err != nil {
			log.WithError(err).Warn("Failed to create query expander")
		} else if expander != nil {
			webhookHandler.GetCourseHandler().SetQueryExpander(expander)
			log.Info("Query expander enabled for semantic search")
		}
	}

	// Create NLU intent parser (optional - requires Gemini API key)
	var intentParser genai.IntentParser
	if cfg.GeminiAPIKey != "" {
		parser, err := genai.NewIntentParser(context.Background(), cfg.GeminiAPIKey)
		if err != nil {
			log.WithError(err).Warn("Failed to create intent parser, NLU disabled")
		} else if parser != nil {
			intentParser = parser
			webhookHandler.SetIntentParser(intentParser)
			log.Info("NLU intent parser enabled")
		}
	}
	log.Info("Webhook handler created")

	// Set Gin mode based on log level
	if cfg.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create Gin router
	router := gin.New()

	// Add middleware
	router.Use(gin.Recovery())
	router.Use(securityHeadersMiddleware())
	router.Use(loggingMiddleware(log))

	// Setup routes
	setupRoutes(router, webhookHandler, db, registry, scraperClient, stickerManager)

	// Create HTTP server with timeouts optimized for LINE webhook handling
	// See internal/config/timeouts.go for detailed explanations
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  config.WebhookHTTPRead,
		WriteTimeout: config.WebhookHTTPWrite,
		IdleTimeout:  config.WebhookHTTPIdle,
	}

	// Start background goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Cache cleanup goroutine (every 12 hours)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("Panic in cache cleanup goroutine")
			}
		}()
		cleanupExpiredCache(ctx, db, cfg.CacheTTL, log)
	}()

	// Sticker refresh goroutine (every 24 hours)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("Panic in sticker refresh goroutine")
			}
		}()
		refreshStickers(ctx, stickerManager, log)
	}()

	// Daily cache warmup goroutine (daily at 3:00 AM)
	// Refreshes all data modules unconditionally to ensure freshness
	// Data not updated within 7 days (Hard TTL) will be deleted by cleanup job
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("Panic in proactive warmup goroutine")
			}
		}()
		proactiveWarmup(ctx, db, scraperClient, stickerManager, log, cfg, vectorDB)
	}()

	// Cache size metrics updater goroutine (every 5 minutes)
	// Updates Prometheus gauge metrics with current cache entry counts
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.WithField("panic", r).Error("Panic in cache metrics goroutine")
			}
		}()
		updateCacheSizeMetrics(ctx, db, stickerManager, vectorDB, m, log)
	}()

	// Start server in goroutine
	go func() {
		log.WithField("port", cfg.Port).Info("Server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Failed to start server")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Stop webhook handler background goroutines
	webhookHandler.Stop()

	// Cancel context to stop metrics updater
	cancel()

	// Wait for goroutines to finish (with timeout)
	goDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(goDone)
	}()

	select {
	case <-goDone:
		log.Info("All background goroutines stopped")
	case <-time.After(5 * time.Second):
		log.Warn("Timeout waiting for goroutines to stop")
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	// Shutdown server gracefully
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server forced to shutdown")
	}

	// Close intent parser (if enabled)
	if intentParser != nil {
		if err := intentParser.Close(); err != nil {
			log.WithError(err).Error("Failed to close intent parser")
		}
	}

	// Close database connection
	if err := db.Close(); err != nil {
		log.WithError(err).Error("Failed to close database")
	}

	log.Info("Server stopped")
}
