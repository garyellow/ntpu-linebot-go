package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/webhook"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.LogLevel)
	log.Info("Starting NTPU LineBot Server")

	// Connect to database
	db, err := storage.New(cfg.SQLitePath)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer db.Close()
	log.WithField("path", cfg.SQLitePath).Info("Database connected")

	// Create Prometheus registry
	registry := prometheus.NewRegistry()

	// Register Go and process collectors
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Create metrics
	m := metrics.New(registry)
	log.Info("Metrics initialized")

	// Create scraper client
	scraperClient := scraper.NewClient(
		cfg.ScraperTimeout,
		cfg.ScraperWorkers,
		cfg.ScraperMinDelay,
		cfg.ScraperMaxDelay,
		cfg.ScraperMaxRetries,
	)
	log.Info("Scraper client created")

	// Create webhook handler
	webhookHandler, err := webhook.NewHandler(
		cfg.LineChannelSecret,
		cfg.LineChannelToken,
		db,
		scraperClient,
		m,
		log,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create webhook handler")
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
	router.Use(loggingMiddleware(log))

	// Setup routes
	setupRoutes(router, webhookHandler, db, registry)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start system metrics updater goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go updateSystemMetrics(ctx, m, log)

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

	// Cancel context to stop metrics updater
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	// Shutdown server gracefully
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server forced to shutdown")
	}

	// Close database connection
	if err := db.Close(); err != nil {
		log.WithError(err).Error("Failed to close database")
	}

	log.Info("Server stopped")
}

// setupRoutes configures all HTTP routes
func setupRoutes(router *gin.Engine, webhookHandler *webhook.Handler, db *storage.DB, registry *prometheus.Registry) {
	// Root endpoint - redirect to GitHub
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "https://github.com/garyellow/ntpu-linebot-go")
	})

	// Health check endpoints
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/healthy", func(c *gin.Context) {
		// Check database connection
		if err := db.Conn().Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  "database connection failed",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"database": "connected",
		})
	})

	// LINE webhook callback endpoint
	router.POST("/callback", webhookHandler.Handle)

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Process request
		c.Next()

		// Log request
		duration := time.Since(start)
		status := c.Writer.Status()

		entry := log.WithField("method", method).
			WithField("path", path).
			WithField("status", status).
			WithField("duration_ms", duration.Milliseconds()).
			WithField("ip", c.ClientIP())

		if len(c.Errors) > 0 {
			entry.WithField("errors", c.Errors.String()).Error("Request completed with errors")
		} else if status >= 500 {
			entry.Error("Request failed")
		} else if status >= 400 {
			entry.Warn("Request completed with client error")
		} else {
			entry.Debug("Request completed")
		}
	}
}

// updateSystemMetrics periodically updates system metrics
func updateSystemMetrics(ctx context.Context, m *metrics.Metrics, log *logger.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			goroutines := runtime.NumGoroutine()
			memoryBytes := memStats.Alloc

			m.UpdateSystemMetrics(goroutines, memoryBytes)

			log.WithField("goroutines", goroutines).
				WithField("memory_mb", memoryBytes/1024/1024).
				Debug("Updated system metrics")
		}
	}
}
