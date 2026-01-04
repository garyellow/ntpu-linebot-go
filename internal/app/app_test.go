package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/config"
	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/garyellow/ntpu-linebot-go/internal/metrics"
	"github.com/garyellow/ntpu-linebot-go/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// setupTestApp creates a minimal Application for testing endpoints
func setupTestApp(t *testing.T) *Application {
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

	// Create test metrics with a new registry
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)

	// Create test logger
	log := logger.New("info")

	return &Application{
		db:      db,
		metrics: m,
		logger:  log,
	}
}

func TestLivenessCheckHealthy(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)
	defer func() { _ = app.db.Close() }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/livez", app.livenessCheck)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify JSON structure
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check required fields for liveness (minimal response)
	if status, ok := response["status"].(string); !ok || status != "alive" {
		t.Errorf("Expected status='alive', got %v", response["status"])
	}
}

func TestLivenessCheckAlwaysSucceeds(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)

	_ = app.db.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/livez", app.livenessCheck)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 even with database down, got %d", w.Code)
	}
}

func TestReadinessCheckHealthy(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)
	defer func() { _ = app.db.Close() }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/readyz", app.readinessCheck)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify JSON structure
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check required fields
	if status, ok := response["status"].(string); !ok || status != "ready" {
		t.Errorf("Expected status='ready', got %v", response["status"])
	}

	if database, ok := response["database"].(string); !ok || database != "connected" {
		t.Errorf("Expected database='connected', got %v", response["database"])
	}

	if _, ok := response["cache"].(map[string]any); !ok {
		t.Error("Expected cache statistics in response")
	}

	if _, ok := response["features"].(map[string]any); !ok {
		t.Error("Expected features in response")
	}
}

// TestReadinessCheckDatabaseFailure verifies /readyz returns 503 when database ping fails
func TestReadinessCheckDatabaseFailure(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)

	// Close database to simulate failure
	if err := app.db.Close(); err != nil {
		t.Fatalf("Failed to close database: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/readyz", app.readinessCheck)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	// Verify JSON structure
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if status, ok := response["status"].(string); !ok || status != "not ready" {
		t.Errorf("Expected status='not ready', got %v", response["status"])
	}

	if reason, ok := response["reason"].(string); !ok || reason != "database unavailable" {
		t.Errorf("Expected reason='database unavailable', got %v", response["reason"])
	}
}

// TestReadinessCheckContextTimeout verifies context timeout is respected
func TestReadinessCheckContextTimeout(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)
	defer func() { _ = app.db.Close() }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/readyz", app.readinessCheck)

	// Create request with a context that will be canceled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	// The handler should complete quickly (< 100ms) since SQLite operations are fast,
	// completing before either the request context timeout or ReadinessCheckTimeout fires.
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
		// Handler completed successfully
		if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status 200 or 503, got %d", w.Code)
		}
	case <-time.After(config.ReadinessCheckTimeout + 1*time.Second):
		t.Fatal("Handler did not complete within expected timeout")
	}
}

// TestReadinessCheckCacheStats verifies cache statistics are correctly populated
func TestReadinessCheckCacheStats(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)
	defer func() { _ = app.db.Close() }()
	ctx := context.Background()

	// Insert test data
	now := time.Now().Unix()
	_ = app.db.SaveStudent(ctx, &storage.Student{
		ID: "411234567", Name: "Test Student", Year: 111, Department: "CS", CachedAt: now,
	})
	_ = app.db.SaveContact(ctx, &storage.Contact{
		UID: "test-1", Type: "individual", Name: "Test Contact", Phone: "0212345678", Email: "test@ntpu.edu.tw", CachedAt: now,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/readyz", app.readinessCheck)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify cache statistics
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	cache, ok := response["cache"].(map[string]any)
	if !ok {
		t.Fatal("Expected cache statistics in response")
	}

	// Check that students count is present (should be 1)
	if students, ok := cache["students"].(float64); !ok || students != 1 {
		t.Errorf("Expected students=1, got %v", cache["students"])
	}

	// Check that contacts count is present (should be 1)
	if contacts, ok := cache["contacts"].(float64); !ok || contacts != 1 {
		t.Errorf("Expected contacts=1, got %v", cache["contacts"])
	}
}

// TestGetCacheStats verifies getCacheStats handles errors gracefully
func TestGetCacheStats(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)
	defer func() { _ = app.db.Close() }()
	ctx := context.Background()

	// With healthy database, should return counts (even if zero)
	stats := app.getCacheStats(ctx)
	if stats == nil {
		t.Fatal("Expected non-nil stats map")
	}

	// Should have all cache tables in stats (values may be 0)
	if _, ok := stats["students"]; !ok {
		t.Error("Expected 'students' in stats")
	}
	if _, ok := stats["contacts"]; !ok {
		t.Error("Expected 'contacts' in stats")
	}
	if _, ok := stats["courses"]; !ok {
		t.Error("Expected 'courses' in stats")
	}
	if _, ok := stats["stickers"]; !ok {
		t.Error("Expected 'stickers' in stats")
	}
}

// TestGetCacheStatsWithDatabaseError verifies getCacheStats logs errors but continues
func TestGetCacheStatsWithDatabaseError(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)

	// Close database to simulate failure
	if err := app.db.Close(); err != nil {
		t.Fatalf("Failed to close database: %v", err)
	}

	ctx := context.Background()

	// Should return empty stats map (all queries will fail)
	stats := app.getCacheStats(ctx)
	if stats == nil {
		t.Fatal("Expected non-nil stats map")
	}

	// Stats should be empty since all queries failed
	if len(stats) != 0 {
		t.Errorf("Expected empty stats map, got %d entries", len(stats))
	}
}

// TestGetFeatures verifies feature flags are correctly reported
func TestGetFeatures(t *testing.T) {
	t.Parallel()
	app := setupTestApp(t)
	defer func() { _ = app.db.Close() }()

	features := app.getFeatures()
	if features == nil {
		t.Fatal("Expected non-nil features map")
	}

	// All features should be false in minimal test setup
	if bm25 := features["bm25_search"]; bm25 {
		t.Errorf("Expected bm25_search=false, got %v", bm25)
	}

	if nlu := features["nlu"]; nlu {
		t.Errorf("Expected nlu=false, got %v", nlu)
	}

	if queryExpansion := features["query_expansion"]; queryExpansion {
		t.Errorf("Expected query_expansion=false, got %v", queryExpansion)
	}
}
