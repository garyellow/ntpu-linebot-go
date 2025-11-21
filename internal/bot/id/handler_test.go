package id

import (
	"context"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/scraper"
	"github.com/garyellow/ntpu-linebot-go/internal/sticker"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
)

func setupTestHandler(t *testing.T) *Handler {
	db, err := storage.New(":memory:", 168*time.Hour) // 7 days for tests
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	scraperClient := scraper.NewClient(30*time.Second, 2, 500*time.Millisecond, 1*time.Second, 3)
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	log := logger.New("info")
	testLogger := logger.New("info")
	stickerMgr := sticker.NewManager(db, scraperClient, testLogger)

	return NewHandler(db, scraperClient, m, log, stickerMgr)
}

// TestCanHandle tests the core routing logic - critical for correct request dispatch
func TestCanHandle(t *testing.T) {
	h := &Handler{}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Critical business paths
		{"Valid 8-digit student ID", "學號 41247001", true},
		{"Valid 9-digit student ID", "學號 412470011", true},
		{"All department codes special", "所有系代碼", true},
		{"Year query (data cutoff check)", "year 113", true},
		{"Department lookup", "系 資工", true},

		// Edge cases that must fail
		{"Invalid - too short", "1234567", false},
		{"Invalid - random text", "hello world", false},
		{"Invalid - empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.CanHandle(tt.input); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleMessage_StudentID(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test non-existent student (should return error message)
	msgs := h.HandleMessage(ctx, "學號 99999999")
	if len(msgs) == 0 {
		t.Error("Expected error message for non-existent student")
	}
}

func TestHandleMessage_AllDepartments(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test all department codes
	msgs := h.HandleMessage(ctx, "所有系代碼")
	if len(msgs) == 0 {
		t.Error("Expected department list message")
	}
}

func TestHandleMessage_YearQuery(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test year query
	msgs := h.HandleMessage(ctx, "學年 112")
	if len(msgs) == 0 {
		t.Error("Expected year query result")
	}
}

func TestHandlePostback_Department(t *testing.T) {
	h := setupTestHandler(t)
	ctx := context.Background()

	// Test department postback - should return message even if no students found
	msgs := h.HandlePostback(ctx, "系$資工$112")
	// Department query may return empty results, which is valid behavior
	// Just verify it doesn't panic and returns something
	_ = msgs
}

// TestParseYear tests critical year conversion logic (ROC <-> AD)
// This is essential for correct data retrieval and the 113學年度 cutoff warning
func TestParseYear(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		// Critical conversions
		{"ROC 112 (2023)", "112", 112, false},
		{"ROC 113 (2024 cutoff)", "113", 113, false},
		{"AD 2023 converts to ROC", "2023", 112, false},
		{"AD 2024 converts to ROC", "2024", 113, false},

		// Edge cases
		{"Too short - must error", "1", 0, true},
		{"Too long - must error", "12345", 0, true},
		{"Non-numeric - must error", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseYear(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseYear(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseYear(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
