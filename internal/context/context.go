// Package context provides type-safe context value management for the application.
// It uses private key types to prevent context key collisions and provides
// safe getter/setter functions following Go best practices.
package context

import (
	"context"
)

type contextKey string

const (
	userIDKey    contextKey = "context.userID"
	chatIDKey    contextKey = "context.chatID"
	requestIDKey contextKey = "context.requestID"
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

// MustGetRequestID retrieves the request ID from the context.
// Panics if the request ID is not found.
func MustGetRequestID(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDKey).(string)
	if !ok || requestID == "" {
		panic("context: requestID not found")
	}
	return requestID
}

// PreserveTracing creates a detached context that preserves tracing values.
// The new context is independent of the parent's cancellation and deadlines.
//
// Use cases:
// - Async webhook processing (parent HTTP request may close before completion)
// - Background jobs triggered by webhooks (DB writes, scraping, cache updates)
//
// This is safer than context.WithoutCancel for long-running operations because:
// 1. No parent context reference is held (prevents memory leaks)
// 2. Only essential tracing values are copied
// 3. Parent value mutations don't affect the new context
//
// Use for async operations that need tracing but must outlive the parent context
// (e.g., LINE webhook processing that continues after HTTP response is sent).
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
