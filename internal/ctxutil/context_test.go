package ctxutil

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

func TestContextChaining(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Chain multiple context values
	ctx = WithUserID(ctx, "U123")
	ctx = WithChatID(ctx, "C456")
	ctx = WithRequestID(ctx, "req-789")
	ctx = WithQuoteToken(ctx, "quote-abc")

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
	if quoteToken := GetQuoteToken(ctx); quoteToken != "quote-abc" {
		t.Error("QuoteToken not preserved in chained context")
	}
}

func TestPreserveTracing(t *testing.T) {
	t.Parallel()
	t.Run("preserves all tracing values", func(t *testing.T) {
		t.Parallel()
		parentCtx := context.Background()
		parentCtx = WithUserID(parentCtx, "user123")
		parentCtx = WithChatID(parentCtx, "chat456")
		parentCtx = WithRequestID(parentCtx, "req789")
		parentCtx = WithQuoteToken(parentCtx, "quote-xyz")

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
		if quoteToken := GetQuoteToken(detachedCtx); quoteToken != "quote-xyz" {
			t.Errorf("Expected quoteToken 'quote-xyz', got %q", quoteToken)
		}
	})

	t.Run("handles partial values", func(t *testing.T) {
		t.Parallel()
		partialCtx := context.Background()
		partialCtx = WithUserID(partialCtx, "user_only")
		detachedPartial := PreserveTracing(partialCtx)

		if userID := GetUserID(detachedPartial); userID != "user_only" {
			t.Errorf("Expected userID 'user_only', got %q", userID)
		}
		if chatID := GetChatID(detachedPartial); chatID != "" {
			t.Errorf("Expected empty chatID, got %q", chatID)
		}
		if quoteToken := GetQuoteToken(detachedPartial); quoteToken != "" {
			t.Errorf("Expected empty quoteToken, got %q", quoteToken)
		}
	})

	t.Run("handles empty context", func(t *testing.T) {
		t.Parallel()
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
		if quoteToken := GetQuoteToken(emptyDetached); quoteToken != "" {
			t.Errorf("Expected empty quoteToken, got %q", quoteToken)
		}
	})

	t.Run("creates independent context (cancellation)", func(t *testing.T) {
		t.Parallel()
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

func TestQuoteTokenContext(t *testing.T) {
	t.Parallel()

	t.Run("empty context", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		if quoteToken := GetQuoteToken(ctx); quoteToken != "" {
			t.Errorf("Expected empty string, got %s", quoteToken)
		}
	})

	t.Run("with quote token", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		expectedToken := "quote-token-12345"
		ctx = WithQuoteToken(ctx, expectedToken)
		quoteToken := GetQuoteToken(ctx)
		if quoteToken != expectedToken {
			t.Errorf("Expected quoteToken %s, got %s", expectedToken, quoteToken)
		}
	})

	t.Run("empty token returns empty", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		ctx = WithQuoteToken(ctx, "")
		if quoteToken := GetQuoteToken(ctx); quoteToken != "" {
			t.Errorf("Expected empty quoteToken for empty input, got %s", quoteToken)
		}
	})
}
