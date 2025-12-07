// Package id provides functional options for Handler configuration.
package id

import (
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// HandlerOption is a functional option for configuring Handler.
// Using the functional options pattern provides:
// - Explicit optional parameters
// - Backward-compatible API evolution
// - Self-documenting configuration
// - Reduced constructor parameter count
type HandlerOption func(*Handler)

// WithRepository sets the student repository.
func WithRepository(repo storage.StudentRepository) HandlerOption {
	return func(h *Handler) {
		h.repo = repo
	}
}

// WithScraper sets the scraper client.
func WithScraper(scraper *scraper.Client) HandlerOption {
	return func(h *Handler) {
		h.scraper = scraper
	}
}

// WithMetrics sets the metrics collector.
func WithMetrics(metrics *metrics.Metrics) HandlerOption {
	return func(h *Handler) {
		h.metrics = metrics
	}
}

// WithLogger sets the logger.
func WithLogger(logger *logger.Logger) HandlerOption {
	return func(h *Handler) {
		h.logger = logger
	}
}

// WithStickerManager sets the sticker manager.
func WithStickerManager(stickerManager *sticker.Manager) HandlerOption {
	return func(h *Handler) {
		h.stickerManager = stickerManager
	}
}
