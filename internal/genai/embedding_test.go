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

func TestNewRateLimiter(t *testing.T) {
	rl := newRateLimiter(1000) // 1000 RPM
	if rl == nil {
		t.Fatal("newRateLimiter returned nil")
	}

	// Check initial state
	expectedRefillRate := 1000.0 / 60.0 // ~16.67 tokens/sec
	if rl.refillRate != expectedRefillRate {
		t.Errorf("refillRate = %v, want %v", rl.refillRate, expectedRefillRate)
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	// Create a rate limiter with high rate so it doesn't block
	rl := newRateLimiter(6000) // 100 tokens/sec
	ctx := context.Background()

	// First call should succeed immediately
	start := time.Now()
	err := rl.wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("wait() error = %v", err)
	}
	// Should be nearly instant (less than 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("wait() took too long: %v", elapsed)
	}
}

func TestRateLimiter_Wait_ContextCanceled(t *testing.T) {
	// Create a rate limiter with very low rate
	rl := newRateLimiter(1) // 1 request per minute
	rl.tokens = 0           // Exhaust tokens

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := rl.wait(ctx)
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
