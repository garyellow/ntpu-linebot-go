// Package contact provides functional options for Handler configuration.
package contact

// HandlerOption is a functional option for configuring Handler.
// Used for truly optional parameters like limits and thresholds.
type HandlerOption func(*Handler)

// WithMaxContactsLimit sets the maximum contacts per search.
func WithMaxContactsLimit(limit int) HandlerOption {
	return func(h *Handler) {
		h.maxContactsLimit = limit
	}
}
