package bot

import (
	"context"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Registry manages bot handlers and provides dispatching functionality.
// Handlers are matched in registration order (first match wins).
type Registry struct {
	handlers   []Handler
	handlerMap map[string]Handler // Quick lookup by name
}

// NewRegistry creates a new handler registry with pre-allocated capacity.
func NewRegistry() *Registry {
	return &Registry{
		handlers:   make([]Handler, 0, 3), // Pre-allocate for typical handler count
		handlerMap: make(map[string]Handler, 3),
	}
}

// Register adds a handler to the registry.
// Handlers are matched in registration order for message/postback dispatch.
func (r *Registry) Register(h Handler) {
	r.handlers = append(r.handlers, h)
	r.handlerMap[h.Name()] = h
}

// DispatchMessage dispatches a text message to the first handler that can handle it.
// Returns nil if no handler matches.
func (r *Registry) DispatchMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	for _, h := range r.handlers {
		if h.CanHandle(text) {
			return h.HandleMessage(ctx, text)
		}
	}
	return nil
}

// DispatchPostback dispatches a postback event using structured data.
// Parses PostbackData and routes to appropriate handler by module name.
func (r *Registry) DispatchPostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	pb, err := ParsePostback(data)
	if err != nil {
		return nil
	}

	h, ok := r.handlerMap[pb.Module]
	if !ok {
		return nil
	}

	return h.HandlePostback(ctx, data)
}

// GetHandler returns a handler by name.
// Returns nil if handler not found.
func (r *Registry) GetHandler(name string) Handler {
	return r.handlerMap[name]
}

// Handlers returns all registered handlers in registration order.
func (r *Registry) Handlers() []Handler {
	return r.handlers
}
