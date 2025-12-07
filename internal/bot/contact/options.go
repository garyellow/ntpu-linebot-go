// Package contact provides functional options for Handler configuration.
package contact

import (
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// HandlerOption is a functional option for configuring Handler.
type HandlerOption func(*Handler)

// WithRepository sets the contact repository.
func WithRepository(repo storage.ContactRepository) HandlerOption {
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

// WithMaxContactsLimit sets the maximum contacts per search.
func WithMaxContactsLimit(limit int) HandlerOption {
	return func(h *Handler) {
		h.maxContactsLimit = limit
	}
}
