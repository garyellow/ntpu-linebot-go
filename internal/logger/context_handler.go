// Package logger provides structured logging utilities for the application.
package logger

import (
	"context"
	"log/slog"

	"github.com/garyellow/ntpu-linebot-go/internal/ctxutil"
)

// ContextHandler is a custom slog.Handler that automatically extracts
// tracing values (userID, chatID, requestID) from the context and adds
// them as attributes to log records.
//
// This handler wraps another handler and intercepts all logging calls
// to enrich log entries with context values, eliminating the need to
// manually extract and pass these values at every logging call site.
//
// Design pattern: Handler wrapper (decorator pattern)
// Reference: https://betterstack.com/community/guides/logging/golang-contextual-logging/
type ContextHandler struct {
	handler slog.Handler
}

// NewContextHandler creates a new ContextHandler that wraps the provided handler.
func NewContextHandler(handler slog.Handler) *ContextHandler {
	return &ContextHandler{handler: handler}
}

// Enabled reports whether the handler handles records at the given level.
// This delegates to the wrapped handler.
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle processes the log record by extracting context values and adding them
// as attributes before delegating to the wrapped handler.
//
// Context values extracted:
// - user_id: LINE user ID for user-specific operations and rate limiting
// - chat_id: LINE chat ID (user, group, or room conversation)
// - request_id: Request ID for log correlation and tracing
// - event_id: LINE webhook event ID
// - message_id: LINE message ID
//
// Note: The context parameter is provided solely to access context values.
// Canceling the context does not affect record processing (per slog.Handler contract).
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract userID from context
	if userID := ctxutil.GetUserID(ctx); userID != "" {
		r.AddAttrs(slog.String("user_id", userID))
	}

	// Extract chatID from context
	if chatID := ctxutil.GetChatID(ctx); chatID != "" {
		r.AddAttrs(slog.String("chat_id", chatID))
	}

	// Extract requestID from context
	if requestID, ok := ctxutil.GetRequestID(ctx); ok && requestID != "" {
		r.AddAttrs(slog.String("request_id", requestID))
	}

	// Extract LINE webhook event ID from context
	if eventID := ctxutil.GetEventID(ctx); eventID != "" {
		r.AddAttrs(slog.String("event_id", eventID))
	}

	// Extract LINE message ID from context
	if messageID := ctxutil.GetMessageID(ctx); messageID != "" {
		r.AddAttrs(slog.String("message_id", messageID))
	}

	// Delegate to wrapped handler
	return h.handler.Handle(ctx, r)
}

// WithAttrs returns a new ContextHandler whose attributes consist of
// both the receiver's attributes and the arguments.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{handler: h.handler.WithAttrs(attrs)}
}

// WithGroup returns a new ContextHandler with the given group name prepended
// to the current group name.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{handler: h.handler.WithGroup(name)}
}
