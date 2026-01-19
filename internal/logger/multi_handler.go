package logger

import (
	"context"
	"errors"
	"log/slog"
)

// MultiHandler fan-outs log records to multiple handlers.
// It preserves slog.Handler semantics by cloning records per handler.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a MultiHandler with the provided handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	filtered := make([]slog.Handler, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			filtered = append(filtered, h)
		}
	}
	return &MultiHandler{handlers: filtered}
}

// Enabled reports whether any underlying handler is enabled for the given level.
func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle dispatches the record to all enabled handlers.
func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, r.Level) {
			continue
		}
		record := r.Clone()
		if err := handler.Handle(ctx, record); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// WithAttrs returns a new MultiHandler with the attributes applied to all handlers.
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithAttrs(attrs))
	}
	return &MultiHandler{handlers: next}
}

// WithGroup returns a new MultiHandler with the group applied to all handlers.
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithGroup(name))
	}
	return &MultiHandler{handlers: next}
}
