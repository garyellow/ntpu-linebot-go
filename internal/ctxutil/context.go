// Package ctxutil provides type-safe context value management.
// Uses private key types to prevent collisions.
package ctxutil

import (
	"context"
)

type contextKey string

const (
	userIDKey     contextKey = "ctxutil.userID"
	chatIDKey     contextKey = "ctxutil.chatID"
	requestIDKey  contextKey = "ctxutil.requestID"
	eventIDKey    contextKey = "ctxutil.eventID"
	messageIDKey  contextKey = "ctxutil.messageID"
	quoteTokenKey contextKey = "ctxutil.quoteToken" //nolint:gosec // G101: False positive - this is a context key name, not a credential
)

// WithUserID adds a user ID to the context.
// User ID typically comes from LINE webhook events and is used for
// rate limiting and user-specific operations.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID retrieves the user ID from the context.
// Returns the user ID if found, empty string otherwise.
func GetUserID(ctx context.Context) string {
	if v := ctx.Value(userIDKey); v != nil {
		if userID, ok := v.(string); ok && userID != "" {
			return userID
		}
	}
	return ""
}

// MustGetUserID retrieves the user ID from the context.
// Panics if the user ID is not found. Use this in contexts where
// the user ID is guaranteed to exist (e.g., after authentication middleware).
func MustGetUserID(ctx context.Context) string {
	userID, ok := ctx.Value(userIDKey).(string)
	if !ok || userID == "" {
		panic("ctxutil: userID not found")
	}
	return userID
}

// WithChatID adds a chat ID to the context.
// Chat ID identifies the conversation (user, group, or room) in LINE.
func WithChatID(ctx context.Context, chatID string) context.Context {
	return context.WithValue(ctx, chatIDKey, chatID)
}

// GetChatID retrieves the chat ID from the context.
// Returns the chat ID if found, empty string otherwise.
func GetChatID(ctx context.Context) string {
	if v := ctx.Value(chatIDKey); v != nil {
		if chatID, ok := v.(string); ok && chatID != "" {
			return chatID
		}
	}
	return ""
}

// MustGetChatID retrieves the chat ID from the context.
// Panics if the chat ID is not found.
func MustGetChatID(ctx context.Context) string {
	chatID, ok := ctx.Value(chatIDKey).(string)
	if !ok || chatID == "" {
		panic("ctxutil: chatID not found")
	}
	return chatID
}

// WithRequestID adds a request ID to the context for tracing.
// Request ID is typically generated per webhook request for log correlation.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context.
// Returns the request ID and true if found, empty string and false otherwise.
func GetRequestID(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(requestIDKey).(string)
	return requestID, ok
}

// MustGetRequestID retrieves the request ID from the context.
// Panics if the request ID is not found.
func MustGetRequestID(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDKey).(string)
	if !ok || requestID == "" {
		panic("ctxutil: requestID not found")
	}
	return requestID
}

// WithEventID adds a LINE webhook event ID to the context.
// This is the webhookEventId from LINE, useful for correlation.
func WithEventID(ctx context.Context, eventID string) context.Context {
	return context.WithValue(ctx, eventIDKey, eventID)
}

// GetEventID retrieves the LINE webhook event ID from the context.
// Returns the event ID if found, empty string otherwise.
func GetEventID(ctx context.Context) string {
	if v := ctx.Value(eventIDKey); v != nil {
		if eventID, ok := v.(string); ok && eventID != "" {
			return eventID
		}
	}
	return ""
}

// WithMessageID adds a LINE message ID to the context.
// Applies to message events; used for tracing message content fetches.
func WithMessageID(ctx context.Context, messageID string) context.Context {
	return context.WithValue(ctx, messageIDKey, messageID)
}

// GetMessageID retrieves the LINE message ID from the context.
// Returns the message ID if found, empty string otherwise.
func GetMessageID(ctx context.Context) string {
	if v := ctx.Value(messageIDKey); v != nil {
		if messageID, ok := v.(string); ok && messageID != "" {
			return messageID
		}
	}
	return ""
}

// PreserveTracing creates a detached context that preserves tracing values.
// The new context is independent of the parent's cancellation and deadlines.
//
// This function creates a fresh context.Background() and copies only tracing values,
// avoiding memory leaks from retaining parent context references (Go issue #64478).
//
// Use for async operations that need tracing but must outlive the parent context,
// such as LINE webhook processing that continues after HTTP response is sent.
func PreserveTracing(ctx context.Context) context.Context {
	newCtx := context.Background()

	if userID := GetUserID(ctx); userID != "" {
		newCtx = WithUserID(newCtx, userID)
	}
	if chatID := GetChatID(ctx); chatID != "" {
		newCtx = WithChatID(newCtx, chatID)
	}
	if requestID, ok := GetRequestID(ctx); ok && requestID != "" {
		newCtx = WithRequestID(newCtx, requestID)
	}
	if eventID := GetEventID(ctx); eventID != "" {
		newCtx = WithEventID(newCtx, eventID)
	}
	if messageID := GetMessageID(ctx); messageID != "" {
		newCtx = WithMessageID(newCtx, messageID)
	}
	if quoteToken := GetQuoteToken(ctx); quoteToken != "" {
		newCtx = WithQuoteToken(newCtx, quoteToken)
	}

	return newCtx
}

// WithQuoteToken adds a quote token to the context.
// Quote token is used to quote a specific message when replying in LINE.
// This enables Quote Reply functionality for better conversation context.
func WithQuoteToken(ctx context.Context, quoteToken string) context.Context {
	return context.WithValue(ctx, quoteTokenKey, quoteToken)
}

// GetQuoteToken retrieves the quote token from the context.
// Returns the quote token if found, empty string otherwise.
// An empty token means no message should be quoted in the reply.
func GetQuoteToken(ctx context.Context) string {
	if v := ctx.Value(quoteTokenKey); v != nil {
		if quoteToken, ok := v.(string); ok && quoteToken != "" {
			return quoteToken
		}
	}
	return ""
}
