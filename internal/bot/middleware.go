// Package bot provides standard middlewares for bot handlers.
package bot

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
)

// LoggingMiddleware logs handler execution with timing and result info.
func LoggingMiddleware(log *logger.Logger) HandlerFunc {
	return func(ctx context.Context, h Handler, text string, next HandlerFunc) []messaging_api.MessageInterface {
		start := time.Now()

		log.WithField("module", h.Name()).
			WithField("text_length", len(text)).
			Debug("Handler started")

		msgs := next(ctx, h, text, nil)

		log.WithField("module", h.Name()).
			WithField("duration_ms", time.Since(start).Milliseconds()).
			WithField("msg_count", len(msgs)).
			Debug("Handler completed")

		return msgs
	}
}

// MetricsMiddleware records handler execution metrics.
func MetricsMiddleware(m *metrics.Metrics) HandlerFunc {
	return func(ctx context.Context, h Handler, text string, next HandlerFunc) []messaging_api.MessageInterface {
		start := time.Now()

		msgs := next(ctx, h, text, nil)

		// Record handler execution time
		duration := time.Since(start).Seconds()
		if m != nil {
			m.RecordScraperRequest(h.Name(), "success", duration)
		}

		return msgs
	}
}

// RecoveryMiddleware recovers from panics in handlers and returns error message.
func RecoveryMiddleware(log *logger.Logger, stickerManager interface{}) HandlerFunc {
	return func(ctx context.Context, h Handler, text string, next HandlerFunc) []messaging_api.MessageInterface {
		defer func() {
			if r := recover(); r != nil {
				// Log the panic with stack trace
				log.WithField("module", h.Name()).
					WithField("panic", r).
					WithField("stack", string(debug.Stack())).
					Error("Handler panicked")
			}
		}()

		return next(ctx, h, text, nil)
	}
}
