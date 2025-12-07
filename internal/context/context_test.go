package context

import (
	"context"
	"testing"
)

func TestUserIDContext(t *testing.T) {
	ctx := context.Background()

	// Test empty context with GetUserIDOk
	if _, ok := GetUserIDOk(ctx); ok {
		t.Error("Expected GetUserIDOk to return false for empty context")
	}

	// Test empty context with GetUserID (convenience function)
	if userID := GetUserID(ctx); userID != "" {
		t.Errorf("Expected empty string, got %s", userID)
	}

	// Test WithUserID and GetUserIDOk
	expectedUserID := "U1234567890"
	ctx = WithUserID(ctx, expectedUserID)
	userID, ok := GetUserIDOk(ctx)
	if !ok {
		t.Error("Expected GetUserIDOk to return true")
	}
	if userID != expectedUserID {
		t.Errorf("Expected userID %s, got %s", expectedUserID, userID)
	}

	// Test GetUserID (convenience function)
	userID = GetUserID(ctx)
	if userID != expectedUserID {
		t.Errorf("Expected userID %s, got %s", expectedUserID, userID)
	}

	// Test MustGetUserID
	userID = MustGetUserID(ctx)
	if userID != expectedUserID {
		t.Errorf("Expected userID %s, got %s", expectedUserID, userID)
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

	// Test empty context with GetChatIDOk
	if _, ok := GetChatIDOk(ctx); ok {
		t.Error("Expected GetChatIDOk to return false for empty context")
	}

	// Test empty context with GetChatID (convenience function)
	if chatID := GetChatID(ctx); chatID != "" {
		t.Errorf("Expected empty string, got %s", chatID)
	}

	// Test WithChatID and GetChatIDOk
	expectedChatID := "C1234567890"
	ctx = WithChatID(ctx, expectedChatID)
	chatID, ok := GetChatIDOk(ctx)
	if !ok {
		t.Error("Expected GetChatIDOk to return true")
	}
	if chatID != expectedChatID {
		t.Errorf("Expected chatID %s, got %s", expectedChatID, chatID)
	}

	// Test GetChatID (convenience function)
	chatID = GetChatID(ctx)
	if chatID != expectedChatID {
		t.Errorf("Expected chatID %s, got %s", expectedChatID, chatID)
	}

	// Test MustGetChatID
	chatID = MustGetChatID(ctx)
	if chatID != expectedChatID {
		t.Errorf("Expected chatID %s, got %s", expectedChatID, chatID)
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
	ctx := context.Background()

	// Test empty context
	if module, ok := GetModule(ctx); ok || module != "" {
		t.Error("Expected GetModule to return empty string and false for empty context")
	}

	// Test WithModule and GetModule
	expectedModule := "course"
	ctx = WithModule(ctx, expectedModule)
	module, ok := GetModule(ctx)
	if !ok {
		t.Error("Expected GetModule to return true")
	}
	if module != expectedModule {
		t.Errorf("Expected module %s, got %s", expectedModule, module)
	}
}

func TestContextChaining(t *testing.T) {
	ctx := context.Background()

	// Chain multiple context values
	ctx = WithUserID(ctx, "U123")
	ctx = WithChatID(ctx, "C456")
	ctx = WithRequestID(ctx, "req-789")
	ctx = WithModule(ctx, "id")

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
	if module, ok := GetModule(ctx); !ok || module != "id" {
		t.Error("Module not preserved in chained context")
	}
}
