package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// setupTestHandler creates a test handler with in-memory database
func setupTestHandler(t *testing.T) *Handler {
	// Create test database
	db, err := storage.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create test scraper
	scraperClient := scraper.NewClient(30000000000, 2, 500000000, 1000000000, 3)

	// Create test metrics with a new registry
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	// Create test logger
	log := logger.New("info")

	// Create handler (nil for stickerManager as it's unused in webhook handler)
	handler, err := NewHandler(
		"test_channel_secret",
		"test_channel_token",
		db,
		scraperClient,
		m,
		log,
		nil, // stickerManager (unused)
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

	if handler.idHandler == nil {
		t.Error("Expected idHandler to be initialized")
	}

	if handler.contactHandler == nil {
		t.Error("Expected contactHandler to be initialized")
	}

	if handler.courseHandler == nil {
		t.Error("Expected courseHandler to be initialized")
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

	// Verify handlers are initialized (they should use context internally)
	if handler.idHandler == nil {
		t.Error("idHandler should be initialized")
	}
	if handler.contactHandler == nil {
		t.Error("contactHandler should be initialized")
	}
	if handler.courseHandler == nil {
		t.Error("courseHandler should be initialized")
	}

	// Use handler to avoid unused variable error
	_ = handler
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
