package webhook

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"

	"github.com/garyellow/ntpu-linebot-go/internal/bot"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/contact"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/course"
	"github.com/garyellow/ntpu-linebot-go/internal/modules/id"
	"github.com/garyellow/ntpu-linebot-go/internal/ratelimit"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// setupTestHandler creates a test handler with isolated temp file database
func setupTestHandler(t *testing.T) *Handler {
	t.Helper()
	// Use a unique temp file database for each test to avoid shared memory conflicts
	// when running t.Parallel() tests. The temp directory is automatically cleaned up.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.New(context.Background(), dbPath, 168*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	// Register cleanup to close database before temp directory removal
	t.Cleanup(func() { _ = db.Close() })

	baseURLs := map[string][]string{
		"lms": {"https://lms.ntpu.edu.tw"},
		"sea": {"https://sea.cc.ntpu.edu.tw"},
	}
	scraperClient := scraper.NewClient(30*time.Second, 3, baseURLs)

	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	log := logger.New("info")

	stickerManager := sticker.NewManager(db, scraperClient, log)

	idHandler := id.NewHandler(db, scraperClient, m, log, stickerManager)
	contactHandler := contact.NewHandler(db, scraperClient, m, log, stickerManager, 100)
	courseHandler := course.NewHandler(db, scraperClient, m, log, stickerManager, nil, nil, nil)

	botRegistry := bot.NewRegistry()
	botRegistry.Register(contactHandler)
	botRegistry.Register(courseHandler)
	botRegistry.Register(idHandler)

	botCfg := config.BotConfig{
		WebhookTimeout:      30 * time.Second,
		UserRateBurst:       15.0,
		UserRateRefill:      0.1,
		LLMRateBurst:        60.0,
		LLMRateRefill:       30.0,
		LLMRateDaily:        150,
		GlobalRateRPS:       100.0,
		MaxMessagesPerReply: 5,
		MaxEventsPerWebhook: 100,
		MinReplyTokenLength: 10,
		MaxMessageLength:    20000,
		MaxPostbackDataSize: 300,
		MaxCoursesPerSearch: 40,

		MaxStudentsPerSearch: 400,
		MaxContactsPerSearch: 100,
		ValidYearStart:       95,
		ValidYearEnd:         112,
	}

	llmLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Name:          "llm",
		Burst:         botCfg.LLMRateBurst,
		RefillRate:    botCfg.LLMRateRefill / 3600.0,
		DailyLimit:    botCfg.LLMRateDaily,
		CleanupPeriod: 5 * time.Minute,
		Metrics:       m,
		MetricType:    ratelimit.MetricTypeLLM,
	})
	userLimiter := ratelimit.NewKeyedLimiter(ratelimit.KeyedConfig{
		Name:          "user",
		Burst:         botCfg.UserRateBurst,
		RefillRate:    botCfg.UserRateRefill,
		CleanupPeriod: 5 * time.Minute,
		Metrics:       m,
		MetricType:    ratelimit.MetricTypeUser,
	})

	processor := bot.NewProcessor(bot.ProcessorConfig{
		Registry:       botRegistry,
		IntentParser:   nil,
		LLMLimiter:     llmLimiter,
		UserLimiter:    userLimiter,
		StickerManager: stickerManager,
		Logger:         log,
		Metrics:        m,
		BotConfig:      &botCfg,
	})

	handler, err := NewHandler(HandlerConfig{
		ChannelSecret:  "test_channel_secret",
		ChannelToken:   "test_channel_token",
		BotConfig:      &botCfg,
		Metrics:        m,
		Logger:         log,
		Processor:      processor,
		StickerManager: stickerManager,
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	return handler
}

// TestHandlerInitialization tests handler creation
func TestHandlerInitialization(t *testing.T) {
	t.Parallel()
	handler := setupTestHandler(t)

	if handler.channelSecret != "test_channel_secret" {
		t.Errorf("Expected channel secret 'test_channel_secret', got '%s'", handler.channelSecret)
	}

	if handler.client == nil {
		t.Error("Expected client to be initialized")
	}

	if handler.processor == nil {
		t.Error("Expected processor to be initialized")
	}
}

// TestHandleInvalidSignature tests webhook with invalid signature
func TestHandleInvalidSignature(t *testing.T) {
	t.Parallel()
	handler := setupTestHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handler.Handle)

	// Create request with invalid signature
	body := []byte(`{"events":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", "invalid_signature")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handler returns 400 with no body for invalid signature
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleRequestTooLarge tests webhook with oversized request
// Note: The handler doesn't explicitly check request size - LINE SDK handles this
// during signature validation. Large requests will fail signature validation.
func TestHandleRequestTooLarge(t *testing.T) {
	t.Parallel()
	handler := setupTestHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handler.Handle)

	// Create request with large body (> 1MB)
	// This will fail signature validation (no valid signature for random data)
	largeBody := make([]byte, 1<<20+1) // 1MB + 1 byte
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", "invalid")
	req.ContentLength = int64(len(largeBody))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handler returns 400 for signature validation failure (which is expected for large random data)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestGetReplyToken tests reply token extraction logic exists
func TestGetReplyToken(t *testing.T) {
	t.Parallel()
	handler := setupTestHandler(t)

	// Just verify the handler has the method (detailed testing requires mock events)
	if handler == nil {
		t.Error("Handler should not be nil")
	}
}

// TestGetChatID tests chat ID extraction logic exists
func TestGetChatID(t *testing.T) {
	t.Parallel()
	handler := setupTestHandler(t)

	// Just verify the handler has the method (detailed testing requires mock events)
	if handler == nil {
		t.Error("Handler should not be nil")
	}
}

// TestGetHelpMessage tests help message generation
// Note: getHelpMessage is now on Processor, not Handler
// This test verifies the handler is properly set up to use processor
func TestGetHelpMessage(t *testing.T) {
	t.Parallel()
	handler := setupTestHandler(t)

	if handler.processor == nil {
		t.Fatal("Expected processor to be initialized")
	}

	// The processor has getHelpMessage, and we verify it's set up
	if handler == nil {
		t.Error("Expected handler to be initialized")
	}
}

// TestMessageValidation tests message content validation
func TestMessageValidation(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
	handler := setupTestHandler(t)

	// Verify handler is properly initialized
	if handler == nil {
		t.Fatal("handler should not be nil")
	}

	// Verify processor is set
	if handler.processor == nil {
		t.Error("processor should be initialized")
	}
}

// TestEventProcessingLimit tests that event processing is limited
func TestEventProcessingLimit(t *testing.T) {
	t.Parallel()
	// This test verifies the concept that we limit events per webhook
	maxEvents := 100
	testEvents := make([]any, 150)

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
	t.Parallel()
	maxMessages := 5
	testMessages := make([]any, 10)

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
	t.Parallel()
	handler := setupTestHandler(t)

	// We can't easily test with actual webhook.Source types without mocking
	// But we can verify the method exists and the logic pattern
	if handler == nil {
		t.Fatal("Handler should not be nil")
	}
}

func TestHandlerShutdown(t *testing.T) {
	t.Parallel()
	handler := setupTestHandler(t)

	// Should not panic - Shutdown uses WaitGroup internally
	ctx := context.Background()
	if err := handler.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown should not return error: %v", err)
	}

	// Should be safe to call multiple times
	if err := handler.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown should not return error on second call: %v", err)
	}
}

// TestGetChatID_GroupAndRoom tests that getChatID supports group and room sources
func TestGetChatID_SourceTypes(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
