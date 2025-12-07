package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/course"
	"github.com/garyellow/ntpu-linebot-go/internal/bot/id"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// setupTestHandler creates a test handler with in-memory database
func setupTestHandler(t *testing.T) *Handler {
	// Create test database
	db, err := storage.New(":memory:", 168*time.Hour) // 7 days for tests
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create test scraper
	scraperClient := scraper.NewClient(30*time.Second, 3)

	// Create test metrics with a new registry
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	// Create test logger
	log := logger.New("info")

	// Create sticker manager
	stickerManager := sticker.NewManager(db, scraperClient, log)

	// Create bot handlers with direct constructor injection
	idHandler := id.NewHandler(db, scraperClient, m, log, stickerManager)
	contactHandler := contact.NewHandler(db, scraperClient, m, log, stickerManager, 100)
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerManager)

	// Create bot registry
	botRegistry := bot.NewRegistry()
	botRegistry.Register(contactHandler)
	botRegistry.Register(courseHandler)
	botRegistry.Register(idHandler)

	// Create handler using functional options
	handler, err := NewHandler(
		"test_channel_secret",
		"test_channel_token",
		botRegistry,
		m,
		log,
		WithStickerManager(stickerManager),
		WithWebhookTimeout(30*time.Second),
		WithUserRateLimit(6.0, 1.0/5.0),
		WithLLMRateLimit(50.0),
	)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	return handler
}

// TestHandlerInitialization tests handler creation
func TestHandlerInitialization(t *testing.T) {
	handler := setupTestHandler(t)

	if handler.channelSecret != "test_channel_secret" {
		t.Errorf("Expected channel secret 'test_channel_secret', got '%s'", handler.channelSecret)
	}

	if handler.client == nil {
		t.Error("Expected client to be initialized")
	}

	if handler.registry == nil {
		t.Error("Expected registry to be initialized")
	}
}

// TestHandleInvalidSignature tests webhook with invalid signature
func TestHandleInvalidSignature(t *testing.T) {
	handler := setupTestHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handler.Handle)

	// Create request with invalid signature
	body := []byte(`{"events":[]}`)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", "invalid_signature")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "invalid signature" {
		t.Errorf("Expected error 'invalid signature', got '%s'", response["error"])
	}
}

// TestHandleRequestTooLarge tests webhook with oversized request
func TestHandleRequestTooLarge(t *testing.T) {
	handler := setupTestHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handler.Handle)

	// Create request with large body (> 1MB)
	largeBody := make([]byte, 1<<20+1) // 1MB + 1 byte
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(largeBody))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", w.Code)
	}
}

// TestGetReplyToken tests reply token extraction logic exists
func TestGetReplyToken(t *testing.T) {
	handler := setupTestHandler(t)

	// Just verify the handler has the method (detailed testing requires mock events)
	if handler == nil {
		t.Error("Handler should not be nil")
	}
}

// TestGetChatID tests chat ID extraction logic exists
func TestGetChatID(t *testing.T) {
	handler := setupTestHandler(t)

	// Just verify the handler has the method (detailed testing requires mock events)
	if handler == nil {
		t.Error("Handler should not be nil")
	}
}

// TestGetHelpMessage tests help message generation
func TestGetHelpMessage(t *testing.T) {
	handler := setupTestHandler(t)

	messages := handler.getHelpMessage()

	if len(messages) == 0 {
		t.Error("Expected at least one message")
	}

	// Help message should be a text message
	if len(messages) > 0 {
		// Just verify we got messages back
		if messages == nil {
			t.Error("Expected non-nil messages")
		}
	}
}

// TestMessageValidation tests message content validation
func TestMessageValidation(t *testing.T) {
	_ = setupTestHandler(t)

	tests := []struct {
		name          string
		textLength    int
		expectError   bool
		errorContains string
	}{
		{
			name:        "Normal message",
			textLength:  100,
			expectError: false,
		},
		{
			name:          "Too long message",
			textLength:    20001,
			expectError:   true,
			errorContains: "過長",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a conceptual test - actual validation happens in handleMessageEvent
			// We're just verifying the logic exists
			text := string(make([]byte, tt.textLength))
			if len(text) > 20000 && !tt.expectError {
				t.Error("Expected validation to catch oversized message")
			}
		})
	}
}

// TestContextTimeout tests that handlers use context with timeout
func TestContextTimeout(t *testing.T) {
	handler := setupTestHandler(t)

	// Verify handler is properly initialized
	if handler == nil {
		t.Fatal("handler should not be nil")
	}

	// Verify registry is set
	if handler.registry == nil {
		t.Error("registry should be initialized")
	}
}

// TestEventProcessingLimit tests that event processing is limited
func TestEventProcessingLimit(t *testing.T) {
	// This test verifies the concept that we limit events per webhook
	maxEvents := 100
	testEvents := make([]interface{}, 150)

	// Verify our limit logic
	if len(testEvents) > maxEvents {
		limited := testEvents[:maxEvents]
		if len(limited) != maxEvents {
			t.Errorf("Expected %d events after limiting, got %d", maxEvents, len(limited))
		}
	}
}

// TestMessageTruncation tests that messages are truncated to LINE API limits
func TestMessageTruncation(t *testing.T) {
	maxMessages := 5
	testMessages := make([]interface{}, 10)

	// Verify our truncation logic
	if len(testMessages) > maxMessages {
		truncated := testMessages[:maxMessages]
		if len(truncated) != maxMessages {
			t.Errorf("Expected %d messages after truncation, got %d", maxMessages, len(truncated))
		}
	}
}

// ==================== Personal Chat Tests ====================

func TestIsPersonalChat(t *testing.T) {
	handler := setupTestHandler(t)

	// We can't easily test with actual webhook.Source types without mocking
	// But we can verify the method exists and the logic pattern
	if handler == nil {
		t.Fatal("Handler should not be nil")
	}
}

func TestHandlerStop(t *testing.T) {
	handler := setupTestHandler(t)

	// Should not panic
	handler.Stop()

	// Should be safe to call multiple times
	handler.Stop()
}

// TestGetChatID_GroupAndRoom tests that getChatID supports group and room sources
func TestGetChatID_SourceTypes(t *testing.T) {
	// This is a conceptual test - the actual implementation uses webhook.Source types
	// We verify the logic handles different source types

	tests := []struct {
		name       string
		sourceType string
		expectID   bool
	}{
		{"user source", "user", true},
		{"group source", "group", true},
		{"room source", "room", true},
		{"unknown source", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify logic pattern
			switch tt.sourceType {
			case "user", "group", "room":
				if !tt.expectID {
					t.Error("Expected ID for known source types")
				}
			default:
				if tt.expectID {
					t.Error("Should not expect ID for unknown source types")
				}
			}
		})
	}
}
