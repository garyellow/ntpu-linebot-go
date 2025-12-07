package bot

import (
	"context"
	"strings"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// HandlerFunc is a middleware function that wraps handler execution.
// It can be used for logging, metrics, recovery, etc.
// The 'next' parameter allows chaining middlewares.
type HandlerFunc func(ctx context.Context, handler Handler, text string, next HandlerFunc) []messaging_api.MessageInterface

// Registry manages bot handlers and dispatches messages/postbacks.
// It supports middleware for cross-cutting concerns.
type Registry struct {
	handlers    []Handler
	middlewares []HandlerFunc
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers:    make([]Handler, 0),
		middlewares: make([]HandlerFunc, 0),
	}
}

// Use adds a middleware to the registry.
// Middlewares are executed in the order they are added.
func (r *Registry) Use(mw HandlerFunc) {
	r.middlewares = append(r.middlewares, mw)
}

// Register adds a handler to the registry
func (r *Registry) Register(h Handler) {
	r.handlers = append(r.handlers, h)
}

// DispatchMessage dispatches a text message to the first handler that can handle it.
// Applies all registered middlewares in order before handler execution.
func (r *Registry) DispatchMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	for _, h := range r.handlers {
		if h.CanHandle(text) {
			// If no middlewares, call handler directly
			if len(r.middlewares) == 0 {
				return h.HandleMessage(ctx, text)
			}

			// Build middleware chain from bottom up
			// The last function in the chain calls the actual handler
			handler := h
			final := func(ctx context.Context, h Handler, text string, _ HandlerFunc) []messaging_api.MessageInterface {
				return handler.HandleMessage(ctx, text)
			}

			// Wrap with middlewares in reverse order
			// Create closure factory to avoid loop variable capture issues
			chain := final
			for i := len(r.middlewares) - 1; i >= 0; i-- {
				chain = r.wrapMiddleware(r.middlewares[i], chain)
			}

			return chain(ctx, handler, text, nil)
		}
	}
	return nil
}

// DispatchPostback dispatches a postback event based on the prefix
func (r *Registry) DispatchPostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	for _, h := range r.handlers {
		prefix := h.PostbackPrefix()
		if prefix != "" && strings.HasPrefix(data, prefix) {
			return h.HandlePostback(ctx, strings.TrimPrefix(data, prefix))
		}
	}
	return nil
}

// GetHandler returns a handler by name
func (r *Registry) GetHandler(name string) Handler {
	for _, h := range r.handlers {
		if h.Name() == name {
			return h
		}
	}
	return nil
}

// wrapMiddleware creates a closure that wraps the next handler with the middleware.
// This helper avoids loop variable capture issues in the middleware chain.
func (r *Registry) wrapMiddleware(mw HandlerFunc, next HandlerFunc) HandlerFunc {
	return func(ctx context.Context, h Handler, text string, _ HandlerFunc) []messaging_api.MessageInterface {
		return mw(ctx, h, text, next)
	}
}
