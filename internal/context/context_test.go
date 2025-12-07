package context

import (
	"context"
	"testing"
)

func TestUserIDContext(t *testing.T) {
	ctx := context.Background()

	// Test empty context with GetUserID
	if userID := GetUserID(ctx); userID != "" {
		t.Errorf("Expected empty string, got %s", userID)
	}

	// Test WithUserID and GetUserID
	expectedUserID := "U1234567890"
	ctx = WithUserID(ctx, expectedUserID)
	userID := GetUserID(ctx)
	if userID != expectedUserID {
		t.Errorf("Expected userID %s, got %s", expectedUserID, userID)
	}

	// Test MustGetUserID
	userID2 := MustGetUserID(ctx)
	if userID2 != expectedUserID {
		t.Errorf("Expected userID %s, got %s", expectedUserID, userID2)
	}
}

func TestMustGetUserID_Panic(t *testing.T) {
	ctx := context.Background()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustGetUserID to panic on empty context")
		}
	}()

	MustGetUserID(ctx)
}

func TestChatIDContext(t *testing.T) {
	ctx := context.Background()

	// Test empty context with GetChatID
	if chatID := GetChatID(ctx); chatID != "" {
		t.Errorf("Expected empty string, got %s", chatID)
	}

	// Test WithChatID and GetChatID
	expectedChatID := "C1234567890"
	ctx = WithChatID(ctx, expectedChatID)
	chatID := GetChatID(ctx)
	if chatID != expectedChatID {
		t.Errorf("Expected chatID %s, got %s", expectedChatID, chatID)
	}

	// Test MustGetChatID
	chatID2 := MustGetChatID(ctx)
	if chatID2 != expectedChatID {
		t.Errorf("Expected chatID %s, got %s", expectedChatID, chatID2)
	}
}

func TestMustGetChatID_Panic(t *testing.T) {
	ctx := context.Background()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustGetChatID to panic on empty context")
		}
	}()

	MustGetChatID(ctx)
}

func TestRequestIDContext(t *testing.T) {
	ctx := context.Background()

	// Test empty context
	if requestID, ok := GetRequestID(ctx); ok || requestID != "" {
		t.Error("Expected GetRequestID to return empty string and false for empty context")
	}

	// Test WithRequestID and GetRequestID
	expectedRequestID := "req-12345"
	ctx = WithRequestID(ctx, expectedRequestID)
	requestID, ok := GetRequestID(ctx)
	if !ok {
		t.Error("Expected GetRequestID to return true")
	}
	if requestID != expectedRequestID {
		t.Errorf("Expected requestID %s, got %s", expectedRequestID, requestID)
	}
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
