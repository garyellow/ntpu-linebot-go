package course

import (
	"github.com/garyellow/ntpu-linebot-go/internal/genai"
	"github.com/garyellow/ntpu-linebot-go/internal/rag"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
)

// HandlerOption is a functional option for configuring the course handler.
type HandlerOption func(*Handler)

// WithBM25Index sets the BM25 index for smart search.
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

// WithLLMRateLimiter sets the LLM rate limiter for query expansion.
func WithLLMRateLimiter(limiter *ratelimit.LLMRateLimiter, maxPerHour float64) HandlerOption {
	return func(h *Handler) {
		h.llmRateLimiter = limiter
		h.llmRateLimitPerHour = maxPerHour
	}
}
