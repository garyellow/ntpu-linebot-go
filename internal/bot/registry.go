package bot

import (
	"context"
	"strings"

	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// Registry manages bot handlers and dispatches messages/postbacks.
type Registry struct {
	handlers []Handler
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make([]Handler, 0),
	}
}

// Register adds a handler to the registry.
func (r *Registry) Register(h Handler) {
	r.handlers = append(r.handlers, h)
}

// DispatchMessage dispatches a text message to the first handler that can handle it.
func (r *Registry) DispatchMessage(ctx context.Context, text string) []messaging_api.MessageInterface {
	for _, h := range r.handlers {
		if h.CanHandle(text) {
			return h.HandleMessage(ctx, text)
		}
	}
	return nil
}

// DispatchPostback dispatches a postback event based on the prefix.
func (r *Registry) DispatchPostback(ctx context.Context, data string) []messaging_api.MessageInterface {
	for _, h := range r.handlers {
		prefix := h.PostbackPrefix()
		if prefix != "" && strings.HasPrefix(data, prefix) {
			return h.HandlePostback(ctx, strings.TrimPrefix(data, prefix))
		}
	}
	return nil
}

// GetHandler returns a handler by name.
func (r *Registry) GetHandler(name string) Handler {
	for _, h := range r.handlers {
		if h.Name() == name {
			return h
		}
	}
	return nil
}
