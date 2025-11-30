package genai

import (
	"context"
	"testing"
	"time"
)

func TestNewEmbeddingClient(t *testing.T) {
	client := NewEmbeddingClient("test-api-key")
	if client == nil {
		t.Fatal("NewEmbeddingClient returned nil")
	}
	if client.apiKey != "test-api-key" {
		t.Errorf("apiKey = %q, want %q", client.apiKey, "test-api-key")
	}
	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}
	if client.rateLimiter == nil {
		t.Error("rateLimiter is nil")
	}
}

func TestEmbeddingClient_IsConfigured(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   bool
	}{
		{
			name:   "configured",
			apiKey: "valid-key",
			want:   true,
		},
		{
			name:   "empty key",
			apiKey: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewEmbeddingClient(tt.apiKey)
			if got := client.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmbeddingClient_Embed_EmptyKey(t *testing.T) {
	client := NewEmbeddingClient("")
	ctx := context.Background()

	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Error("Expected error for empty API key, got nil")
	}
	if err.Error() != "gemini API key not configured" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestEmbeddingClient_Embed_EmptyText(t *testing.T) {
	client := NewEmbeddingClient("test-key")
	ctx := context.Background()

	_, err := client.Embed(ctx, "")
	if err == nil {
		t.Error("Expected error for empty text, got nil")
	}
	if err.Error() != "empty text cannot be embedded" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestEmbeddingClient_Embed_ContextCanceled(t *testing.T) {
	client := NewEmbeddingClient("test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Embed(ctx, "test text")
	if err == nil {
		t.Error("Expected error for canceled context, got nil")
	}
}

func TestRateLimiter_Integration(t *testing.T) {
	// Create an embedding client and verify rate limiter is set up
	client := NewEmbeddingClient("test-key")
	if client.rateLimiter == nil {
		t.Fatal("rateLimiter is nil")
	}

	// Verify rate limiter works - should allow first request
	if !client.rateLimiter.Allow() {
		t.Error("First Allow() should return true")
	}
}

func TestRateLimiter_Wait_WithContext(t *testing.T) {
	// Create an embedding client
	client := NewEmbeddingClient("test-key")
	ctx := context.Background()

	// First call should succeed immediately
	start := time.Now()
	err := client.rateLimiter.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Wait() error = %v", err)
	}
	// Should be nearly instant (less than 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Wait() took too long: %v", elapsed)
	}
}

func TestRateLimiter_Wait_ContextCanceled(t *testing.T) {
	// Create an embedding client with exhausted tokens
	client := NewEmbeddingClient("test-key")
	// Consume available tokens
	for client.rateLimiter.Allow() {
		// Keep consuming until rate limited
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := client.rateLimiter.Wait(ctx)
	if err == nil {
		t.Error("Expected error for canceled context, got nil")
	}
}

func TestNewEmbeddingFunc(t *testing.T) {
	fn := NewEmbeddingFunc("test-key")
	if fn == nil {
		t.Error("NewEmbeddingFunc returned nil")
	}
}

// Constants tests
func TestConstants(t *testing.T) {
	if GeminiEmbeddingModel != "gemini-embedding-001" {
		t.Errorf("GeminiEmbeddingModel = %q, want %q", GeminiEmbeddingModel, "gemini-embedding-001")
	}
	if GeminiEmbeddingDimensions != 768 {
		t.Errorf("GeminiEmbeddingDimensions = %d, want %d", GeminiEmbeddingDimensions, 768)
	}
	if GeminiAPIRateLimit != 1000 {
		t.Errorf("GeminiAPIRateLimit = %d, want %d", GeminiAPIRateLimit, 1000)
	}
}

// TestRetryConstants tests retry configuration constants
func TestRetryConstants(t *testing.T) {
	if defaultMaxRetries != 5 {
		t.Errorf("defaultMaxRetries = %d, want 5", defaultMaxRetries)
	}
	if defaultInitialDelay != 2*time.Second {
		t.Errorf("defaultInitialDelay = %v, want 2s", defaultInitialDelay)
	}
	if defaultBackoffFactor != 2.0 {
		t.Errorf("defaultBackoffFactor = %v, want 2.0", defaultBackoffFactor)
	}
	if defaultJitterFactor != 0.25 {
		t.Errorf("defaultJitterFactor = %v, want 0.25", defaultJitterFactor)
	}
}

// TestApplyJitter tests the jitter application
func TestApplyJitter(t *testing.T) {
	client := NewEmbeddingClient("test-key")
	baseDelay := 2 * time.Second

	// Run multiple times to ensure jitter is within bounds
	for i := 0; i < 100; i++ {
		jittered := client.applyJitter(baseDelay)

		// Jitter should be within ±25%
		minExpected := time.Duration(float64(baseDelay) * 0.75)
		maxExpected := time.Duration(float64(baseDelay) * 1.25)

		if jittered < minExpected || jittered > maxExpected {
			t.Errorf("applyJitter(%v) = %v, expected between %v and %v",
				baseDelay, jittered, minExpected, maxExpected)
		}
	}
}
