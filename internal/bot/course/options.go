// Package course provides functional options for Handler configuration.
package course

import (
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
)

// HandlerOption is a functional option for configuring Handler.
type HandlerOption func(*Handler)

// WithRepository sets the course repository.
func WithRepository(repo storage.CourseRepository) HandlerOption {
	return func(h *Handler) {
		h.repo = repo
	}
}

// WithSyllabusRepository sets the syllabus repository.
func WithSyllabusRepository(repo storage.SyllabusRepository) HandlerOption {
	return func(h *Handler) {
		h.syllabusRepo = repo
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

// WithBM25Index sets the BM25 search index for smart search.
func WithBM25Index(index *rag.BM25Index) HandlerOption {
	return func(h *Handler) {
		h.bm25Index = index
	}
}

// WithQueryExpander sets the query expander for smart search.
func WithQueryExpander(expander *genai.QueryExpander) HandlerOption {
	return func(h *Handler) {
		h.queryExpander = expander
	}
}

// WithLLMRateLimiter sets the LLM rate limiter.
func WithLLMRateLimiter(limiter *ratelimit.LLMRateLimiter, perHour float64) HandlerOption {
	return func(h *Handler) {
		h.llmRateLimiter = limiter
		h.llmRateLimitPerHour = perHour
	}
}
