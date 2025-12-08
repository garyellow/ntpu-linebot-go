// Package context provides type-safe context value management for the application.
// It uses private key types to prevent context key collisions and provides
// safe getter/setter functions following Go best practices.
package context

import (
	"context"
)

// contextKey is a private type to prevent context key collisions.
// Using an unexported type ensures that keys defined in this package
// cannot conflict with keys defined in other packages.
type contextKey int

const (
	userIDKey contextKey = iota
	chatIDKey
	requestIDKey
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
		panic("context: userID not found")
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
		panic("context: chatID not found")
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

// PreserveTracing creates a new detached context that preserves tracing values
// from the parent context. This is safe for asynchronous operations where the
// parent context may be canceled or its values may become invalid.
//
// Unlike context.WithoutCancel (Go 1.21+), this function:
//  1. Creates a truly independent context (no reference to parent)
//  2. Only copies necessary tracing values (userID, chatID, requestID)
//  3. Prevents memory leaks from holding parent context references
//  4. Avoids issues with parent values being flushed/closed
//
// Use this when spawning goroutines that need tracing but should not be
// affected by parent context cancellation (e.g., LINE webhook async processing).
//
// Reference: https://github.com/golang/go/issues/64478
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

	return newCtx
}
