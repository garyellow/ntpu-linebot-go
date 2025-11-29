// Package genai provides integration with Google's Generative AI APIs,
// including embedding generation for semantic search functionality.
package genai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	chromem "github.com/philippgille/chromem-go"
)

const (
	// GeminiEmbeddingModel is the model used for generating embeddings
	GeminiEmbeddingModel = "gemini-embedding-001"

	// GeminiEmbeddingDimensions is the output dimension (768 default, supports MRL truncation)
	GeminiEmbeddingDimensions = 768

	// GeminiAPIRateLimit is the requests per minute limit (1000 RPM for embedding API)
	GeminiAPIRateLimit = 1000

	// geminiAPIBaseURL is the base URL for Gemini API
	geminiAPIBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

	// Retry configuration for transient errors (similar to scraper)
	defaultMaxRetries    = 5
	defaultInitialDelay  = 2 * time.Second
	defaultMaxDelay      = 60 * time.Second
	defaultBackoffFactor = 2.0
	defaultJitterFactor  = 0.25
)

// EmbeddingClient provides embedding generation using Gemini API
type EmbeddingClient struct {
	apiKey      string
	httpClient  *http.Client
	rateLimiter *rateLimiter
}

// rateLimiter implements a simple token bucket rate limiter
type rateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// newRateLimiter creates a new rate limiter with given rate (requests per minute)
func newRateLimiter(requestsPerMinute float64) *rateLimiter {
	return &rateLimiter{
		tokens:     requestsPerMinute / 60,     // Start with some tokens
		maxTokens:  requestsPerMinute / 60 * 2, // Allow small burst
		refillRate: requestsPerMinute / 60,     // Convert to per-second
		lastRefill: time.Now(),
	}
}

// wait blocks until a token is available
func (r *rateLimiter) wait(ctx context.Context) error {
	for {
		r.mu.Lock()

		// Refill tokens based on time elapsed
		now := time.Now()
		elapsed := now.Sub(r.lastRefill).Seconds()
		r.tokens += elapsed * r.refillRate
		if r.tokens > r.maxTokens {
			r.tokens = r.maxTokens
		}
		r.lastRefill = now

		// Check if we have a token
		if r.tokens >= 1 {
			r.tokens--
			r.mu.Unlock()
			return nil
		}

		// Calculate wait time
		waitTime := time.Duration((1 - r.tokens) / r.refillRate * float64(time.Second))
		r.mu.Unlock()

		// Wait outside the lock
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Retry the loop to acquire token
		}
	}
}

// NewEmbeddingClient creates a new Gemini embedding client
func NewEmbeddingClient(apiKey string) *EmbeddingClient {
	return &EmbeddingClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimiter: newRateLimiter(GeminiAPIRateLimit),
	}
}

// embeddingRequest represents the request body for Gemini embedding API
type embeddingRequest struct {
	Model   string           `json:"model"`
	Content embeddingContent `json:"content"`
}

type embeddingContent struct {
	Parts []embeddingPart `json:"parts"`
}

type embeddingPart struct {
	Text string `json:"text"`
}

// embeddingResponse represents the response from Gemini embedding API
type embeddingResponse struct {
	Embedding struct {
		Values []float32 `json:"values"`
	} `json:"embedding"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// Embed generates an embedding vector for the given text
// Uses exponential backoff with jitter for transient errors (429, 500+)
func (c *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini API key not configured")
	}

	if text == "" {
		return nil, fmt.Errorf("empty text cannot be embedded")
	}

	var lastErr error
	delay := defaultInitialDelay

	for attempt := 0; attempt <= defaultMaxRetries; attempt++ {
		// Wait for rate limit
		if err := c.rateLimiter.wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}

		result, retryable, err := c.embedOnce(ctx, text)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry non-retryable errors
		if !retryable {
			return nil, err
		}

		// Don't retry if this was the last attempt
		if attempt == defaultMaxRetries {
			break
		}

		// Apply jitter to delay (±25%)
		jitteredDelay := c.applyJitter(delay)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(jitteredDelay):
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * defaultBackoffFactor)
		if delay > defaultMaxDelay {
			delay = defaultMaxDelay
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// embedOnce performs a single embedding request
// Returns (result, retryable, error) - error is last per Go convention
func (c *EmbeddingClient) embedOnce(ctx context.Context, text string) ([]float32, bool, error) {
	// Build request
	url := fmt.Sprintf("%s/%s:embedContent?key=%s", geminiAPIBaseURL, GeminiEmbeddingModel, c.apiKey)

	reqBody := embeddingRequest{
		Model: fmt.Sprintf("models/%s", GeminiEmbeddingModel),
		Content: embeddingContent{
			Parts: []embeddingPart{{Text: text}},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, false, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network errors are retryable
		return nil, true, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status for retryable errors
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("HTTP %d: server error or rate limited", resp.StatusCode)
	}

	// Parse response
	var embeddingResp embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, false, fmt.Errorf("decode response: %w", err)
	}

	// Check for API error
	if embeddingResp.Error != nil {
		// 429 RESOURCE_EXHAUSTED is retryable
		retryable := embeddingResp.Error.Code == 429 ||
			embeddingResp.Error.Status == "RESOURCE_EXHAUSTED" ||
			embeddingResp.Error.Code >= 500

		return nil, retryable, fmt.Errorf("API error %d: %s - %s",
			embeddingResp.Error.Code,
			embeddingResp.Error.Status,
			embeddingResp.Error.Message)
	}

	if len(embeddingResp.Embedding.Values) == 0 {
		return nil, false, fmt.Errorf("empty embedding returned")
	}

	return embeddingResp.Embedding.Values, false, nil
}

// applyJitter adds random jitter to delay (±25%)
func (c *EmbeddingClient) applyJitter(delay time.Duration) time.Duration {
	// Use current time nanoseconds for simple randomness (no need for crypto/rand)
	jitter := float64(time.Now().UnixNano()%1000) / 1000.0 // 0.0 to 0.999
	jitter = (jitter - 0.5) * 2 * defaultJitterFactor      // -0.25 to +0.25
	return time.Duration(float64(delay) * (1 + jitter))
}

// NewEmbeddingFunc creates a chromem-go compatible EmbeddingFunc
// that uses the Gemini embedding API
func NewEmbeddingFunc(apiKey string) chromem.EmbeddingFunc {
	client := NewEmbeddingClient(apiKey)
	return func(ctx context.Context, text string) ([]float32, error) {
		return client.Embed(ctx, text)
	}
}

// IsConfigured returns true if the API key is set
func (c *EmbeddingClient) IsConfigured() bool {
	return c.apiKey != ""
}
