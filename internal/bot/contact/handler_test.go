package contact

import (
	"context"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
	"github.com/prometheus/client_golang/prometheus"
)

func setupTestHandler(t *testing.T) *Handler {
	// Setup test database
	db, err := storage.New(":memory:", 168*time.Hour) // 7 days for tests
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create test scraper
	scraperClient := scraper.NewClient(30*time.Second, 3)

	// Create test metrics
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	// Create test logger
	log := logger.New("info")

	// Create sticker manager for tests (uses fallback stickers)
	testLogger := logger.New("info")
	stickerMgr := sticker.NewManager(db, scraperClient, testLogger)

	return NewHandler(db, scraperClient, m, log, stickerMgr)
}

func TestCanHandle(t *testing.T) {
	h := setupTestHandler(t)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Emergency keywords
		{"Emergency query", "緊急電話", true},
		{"Emergency without space", "緊急", true},

		// Contact keywords (English)
		{"Contact keyword", "contact info", true},
		{"Touch keyword", "touch base", true},
		{"Connect keyword", "how to connect", true},

		// Contact keywords (Chinese)
		{"聯繫 keyword", "聯繫方式", true},
		{"聯絡 keyword", "聯絡電話", true},
		{"連絡 keyword", "連絡方式", true},
		{"電話 keyword", "電話分機", true},
		{"分機 keyword", "查詢分機", true},
		{"Email keyword", "email信箱", true},
		{"信箱 keyword", "電子信箱", true},

		// Invalid queries
		{"Random text", "hello world", false},
		{"Empty string", "", false},
		{"Spaces only", "   ", false},
		{"Student related", "學號", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.CanHandle(tt.input)
			if got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleEmergencyPhones(t *testing.T) {
	h := setupTestHandler(t)

	messages := h.handleEmergencyPhones()

	if len(messages) == 0 {
		t.Error("Expected emergency phone messages, got none")
	}

	// Should contain at least the flex message
	if len(messages) < 1 {
		t.Errorf("Expected at least 1 message, got %d", len(messages))
	}

	// Verify the message structure
	flexMsg, ok := messages[0].(*messaging_api.FlexMessage)
	if !ok {
		t.Fatal("Expected first message to be *messaging_api.FlexMessage")
	}

	if flexMsg.AltText != "緊急聯絡電話" {
		t.Errorf("Expected AltText '緊急聯絡電話', got %q", flexMsg.AltText)
	}
}

func TestHandleMessage_Emergency(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	messages := h.HandleMessage(ctx, "緊急電話")

	if len(messages) == 0 {
		t.Error("Expected messages for emergency query, got none")
	}
}

func TestHandleMessage_Contact(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	// This will likely return no results from empty cache
	messages := h.HandleMessage(ctx, "聯絡陳教授")

	// Should return at least some response
	if len(messages) == 0 {
		t.Error("Expected some response messages, got none")
	}
}

// buildRegex is tested indirectly via CanHandle - no need for separate test

func TestFormatContactResults_Empty(t *testing.T) {
	h := setupTestHandler(t)

	messages := h.formatContactResults([]storage.Contact{})

	if len(messages) != 1 {
		t.Errorf("Expected 1 message for empty results, got %d", len(messages))
	}
}

func TestFormatContactResults_Organizations(t *testing.T) {
	h := setupTestHandler(t)

	contacts := []storage.Contact{
		{
			UID:      "org1",
			Type:     "organization",
			Name:     "資訊工程學系",
			Superior: "工學院",
			Location: "資訊大樓",
			Website:  "https://www.csie.ntpu.edu.tw",
		},
	}

	messages := h.formatContactResults(contacts)

	if len(messages) == 0 {
		t.Error("Expected messages for organization results, got none")
	}
}

func TestFormatContactResults_Individuals(t *testing.T) {
	h := setupTestHandler(t)

	contacts := []storage.Contact{
		{
			UID:          "person1",
			Type:         "individual",
			Name:         "陳教授",
			Organization: "資訊工程學系",
			Title:        "教授",
			Extension:    "88888",
			Phone:        "02-1234-5678",
			Email:        "chen@gm.ntpu.edu.tw",
		},
	}

	messages := h.formatContactResults(contacts)

	if len(messages) == 0 {
		t.Error("Expected messages for individual results, got none")
	}
}

func TestFormatContactResults_LargeList(t *testing.T) {
	h := setupTestHandler(t)

	// Create 60 contacts to test pagination
	contacts := make([]storage.Contact, 60)
	for i := 0; i < 60; i++ {
		contacts[i] = storage.Contact{
			UID:  "contact" + string(rune(i)),
			Type: "individual",
			Name: "Contact " + string(rune(i)),
		}
	}

	messages := h.formatContactResults(contacts)

	// Should split into multiple messages
	if len(messages) < 2 {
		t.Errorf("Expected multiple messages for large list, got %d", len(messages))
	}
}

func TestHandlePostback_ViewMore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	// Test "查看更多" postback
	messages := h.HandlePostback(ctx, "查看更多$陳教授")

	if len(messages) == 0 {
		t.Error("Expected messages for postback, got none")
	}
}

func TestHandlePostback_ViewInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	h := setupTestHandler(t)
	ctx := context.Background()

	// Test "查看資訊" postback
	messages := h.HandlePostback(ctx, "查看資訊$資工系")

	if len(messages) == 0 {
		t.Error("Expected messages for postback, got none")
	}
}

func TestHandlePostback_InvalidData(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test invalid postback data
	messages := h.HandlePostback(ctx, "invalid")

	// Should return empty slice for invalid data
	if len(messages) != 0 {
		t.Errorf("Expected no messages for invalid postback, got %d", len(messages))
	}
}
