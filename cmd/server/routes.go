// Package main provides the LINE bot server entry point.
package main

import (
	"context"
	"net/http"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/garyellow/ntpu-linebot-go/internal/webhook"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// setupRoutes configures all HTTP routes
func setupRoutes(router *gin.Engine, webhookHandler *webhook.Handler, db *storage.DB, registry *prometheus.Registry, scraperClient *scraper.Client, stickerManager *sticker.Manager) {
	// Root endpoint - redirect to GitHub
	rootHandler := func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "https://github.com/garyellow/ntpu-linebot-go")
	}
	router.GET("/", rootHandler)
	router.HEAD("/", rootHandler)

	// Health check endpoints
	// Liveness Probe - checks if the application is alive (minimal check)
	// This should NEVER check dependencies - only that the process is running
	healthHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
	router.GET("/healthz", healthHandler)
	router.HEAD("/healthz", healthHandler)

	// Readiness Probe - checks if the application is ready to serve traffic (full dependency check)
	readyHandler := func(c *gin.Context) {
		// Check database connections (both reader and writer)
		if err := db.Ready(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": err.Error(),
			})
			return
		}

		// Check scraper URLs availability (quick check, just try first URL)
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		seaAvailable := false
		lmsAvailable := false

		// Only check first URL in failover list (for speed)
		seaURLs := scraperClient.GetBaseURLs("sea")
		if len(seaURLs) > 0 {
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", seaURLs[0], http.NoBody)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					seaAvailable = true
				}
			}
		}

		lmsURLs := scraperClient.GetBaseURLs("lms")
		if len(lmsURLs) > 0 {
			req, _ := http.NewRequestWithContext(checkCtx, "HEAD", lmsURLs[0], http.NoBody)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode < 500 {
					lmsAvailable = true
				}
			}
		}

		// Check cache data availability
		studentCount, _ := db.CountStudents(c.Request.Context())
		contactCount, _ := db.CountContacts(c.Request.Context())
		courseCount, _ := db.CountCourses(c.Request.Context())
		stickerCount := stickerManager.Count()

		c.JSON(http.StatusOK, gin.H{
			"status":   "ready",
			"database": "connected",
			"scrapers": gin.H{
				"sea": seaAvailable,
				"lms": lmsAvailable,
			},
			"cache": gin.H{
				"students": studentCount,
				"contacts": contactCount,
				"courses":  courseCount,
				"stickers": stickerCount,
			},
		})
	}
	router.GET("/ready", readyHandler)
	router.HEAD("/ready", readyHandler)

	// LINE webhook callback endpoint
	router.POST("/callback", webhookHandler.Handle)

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))
}
