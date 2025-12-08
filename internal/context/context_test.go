package context

import (
	"context"
	"testing"
)

func TestUserIDContext(t *testing.T) {
	t.Parallel()

	t.Run("empty context", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		if userID := GetUserID(ctx); userID != "" {
			t.Errorf("Expected empty string, got %s", userID)
		}
	})

	t.Run("with user ID", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		expectedUserID := "U1234567890"
		ctx = WithUserID(ctx, expectedUserID)
		userID := GetUserID(ctx)
		if userID != expectedUserID {
			t.Errorf("Expected userID %s, got %s", expectedUserID, userID)
		}
	})

	t.Run("must get user ID", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		expectedUserID := "U1234567890"
		ctx = WithUserID(ctx, expectedUserID)
		userID := MustGetUserID(ctx)
		if userID != expectedUserID {
			t.Errorf("Expected userID %s, got %s", expectedUserID, userID)
		}
	})
}

func TestMustGetUserID_Panic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustGetUserID to panic on empty context")
		}
	}()

	MustGetUserID(ctx)
}

func TestChatIDContext(t *testing.T) {
	t.Parallel()

	t.Run("empty context", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		if chatID := GetChatID(ctx); chatID != "" {
			t.Errorf("Expected empty string, got %s", chatID)
		}
	})

	t.Run("with chat ID", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		expectedChatID := "C1234567890"
		ctx = WithChatID(ctx, expectedChatID)
		chatID := GetChatID(ctx)
		if chatID != expectedChatID {
			t.Errorf("Expected chatID %s, got %s", expectedChatID, chatID)
		}
	})

	t.Run("must get chat ID", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		expectedChatID := "C1234567890"
		ctx = WithChatID(ctx, expectedChatID)
		chatID := MustGetChatID(ctx)
		if chatID != expectedChatID {
			t.Errorf("Expected chatID %s, got %s", expectedChatID, chatID)
		}
	})
}

func TestMustGetChatID_Panic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustGetChatID to panic on empty context")
		}
	}()

	MustGetChatID(ctx)
}

func TestRequestIDContext(t *testing.T) {
	t.Parallel()

	t.Run("empty context", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		if requestID, ok := GetRequestID(ctx); ok || requestID != "" {
			t.Error("Expected GetRequestID to return empty string and false for empty context")
		}
	})

	t.Run("with request ID", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		expectedRequestID := "req-12345"
		ctx = WithRequestID(ctx, expectedRequestID)
		requestID, ok := GetRequestID(ctx)
		if !ok {
			t.Error("Expected GetRequestID to return true")
		}
		if requestID != expectedRequestID {
			t.Errorf("Expected requestID %s, got %s", expectedRequestID, requestID)
		}
	})
}

func TestModuleContext(t *testing.T) {
	// Module context value has been removed - module name is now only in structured logging
	// This simplifies context management and follows Go best practices:
	// context.Value should only be used for request-scoped data, not configuration
	t.Skip("Module context value removed - module tracking now done via structured logging")
}

func TestContextChaining(t *testing.T) {
	ctx := context.Background()

	// Chain multiple context values
	ctx = WithUserID(ctx, "U123")
	ctx = WithChatID(ctx, "C456")
	ctx = WithRequestID(ctx, "req-789")

	// Verify all values are preserved
	if userID := GetUserID(ctx); userID != "U123" {
		t.Error("UserID not preserved in chained context")
	}
	if chatID := GetChatID(ctx); chatID != "C456" {
		t.Error("ChatID not preserved in chained context")
	}
	if requestID, ok := GetRequestID(ctx); !ok || requestID != "req-789" {
		t.Error("RequestID not preserved in chained context")
	}
}

func TestPreserveTracing(t *testing.T) {
	t.Run("preserves all tracing values", func(t *testing.T) {
		parentCtx := context.Background()
		parentCtx = WithUserID(parentCtx, "user123")
		parentCtx = WithChatID(parentCtx, "chat456")
		parentCtx = WithRequestID(parentCtx, "req789")

		detachedCtx := PreserveTracing(parentCtx)

		if userID := GetUserID(detachedCtx); userID != "user123" {
			t.Errorf("Expected userID 'user123', got %q", userID)
		}
		if chatID := GetChatID(detachedCtx); chatID != "chat456" {
			t.Errorf("Expected chatID 'chat456', got %q", chatID)
		}
		if requestID, ok := GetRequestID(detachedCtx); !ok || requestID != "req789" {
			t.Errorf("Expected requestID 'req789', got %q (ok=%v)", requestID, ok)
		}
	})

	t.Run("handles partial values", func(t *testing.T) {
		partialCtx := context.Background()
		partialCtx = WithUserID(partialCtx, "user_only")
		detachedPartial := PreserveTracing(partialCtx)

		if userID := GetUserID(detachedPartial); userID != "user_only" {
			t.Errorf("Expected userID 'user_only', got %q", userID)
		}
		if chatID := GetChatID(detachedPartial); chatID != "" {
			t.Errorf("Expected empty chatID, got %q", chatID)
		}
	})

	t.Run("handles empty context", func(t *testing.T) {
		emptyDetached := PreserveTracing(context.Background())

		if userID := GetUserID(emptyDetached); userID != "" {
			t.Errorf("Expected empty userID, got %q", userID)
		}
		if chatID := GetChatID(emptyDetached); chatID != "" {
			t.Errorf("Expected empty chatID, got %q", chatID)
		}
		if requestID, ok := GetRequestID(emptyDetached); ok || requestID != "" {
			t.Errorf("Expected empty requestID, got %q (ok=%v)", requestID, ok)
		}
	})

	t.Run("creates independent context (cancellation)", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(WithUserID(context.Background(), "user_cancel"))
		detachedCancel := PreserveTracing(cancelCtx)

		cancel() // Cancel parent

		// Parent should be canceled
		if err := cancelCtx.Err(); err == nil {
			t.Error("Expected parent context to be canceled")
		}

		// Detached child should NOT be canceled
		if err := detachedCancel.Err(); err != nil {
			t.Errorf("Expected detached context to be active, got error: %v", err)
		}

		// But values should still be preserved
		if userID := GetUserID(detachedCancel); userID != "user_cancel" {
			t.Errorf("Expected userID 'user_cancel', got %q", userID)
		}
	})
}
