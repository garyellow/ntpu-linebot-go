package scraper

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/corpix/uarand"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// Client is an HTTP client for web scraping with retry and URL failover
type Client struct {
	httpClient *http.Client
	maxRetries int
	baseURLs   map[string][]string // Base URLs for failover by domain
	mu         sync.RWMutex
}

// NewClient creates a new scraper client with URL failover support
// timeout: HTTP request timeout (e.g., 60s)
// maxRetries: max retry attempts with exponential backoff (e.g., 3)
func NewClient(timeout time.Duration, maxRetries int) *Client {
	// Define failover URLs for NTPU services
	baseURLs := map[string][]string{
		"lms": {
			"http://120.126.197.52",
			"https://120.126.197.52",
			"https://lms.ntpu.edu.tw",
		},
		"sea": {
			"http://120.126.197.7",
			"https://120.126.197.7",
			"https://sea.cc.ntpu.edu.tw",
		},
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		maxRetries: maxRetries,
		baseURLs:   baseURLs,
	}
}

// SuccessDelay is the fixed delay after each successful request (anti-scraping)
const SuccessDelay = 2 * time.Second

// Get performs a GET request with retries on failure
// On success: waits SuccessDelay (2s) before returning (anti-scraping)
// On failure: retries with exponential backoff (4s initial, 5 retries)
// IMPORTANT: Caller must close the response body on success
func (c *Client) Get(ctx context.Context, reqURL string) (*http.Response, error) {
	var resp *http.Response
	var lastErr error

	// Retry with exponential backoff on failure (4s initial delay)
	err := RetryWithBackoff(ctx, c.maxRetries, 4*time.Second, func() error {
		// Create request
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, http.NoBody)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set random User-Agent
		req.Header.Set("User-Agent", c.randomUserAgent())
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "zh-TW,zh;q=0.9,en-US;q=0.8,en;q=0.7")
		req.Header.Set("Accept-Encoding", "gzip, deflate")

		// Perform request
		var httpErr error
		resp, httpErr = c.httpClient.Do(req)
		if httpErr != nil {
			lastErr = fmt.Errorf("request failed: %w", httpErr)
			return lastErr
		}

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// Close body for non-success responses since we won't return it
			_ = resp.Body.Close()

			switch resp.StatusCode {
			case 429: // Rate limited - retry with backoff, respect Retry-After if present
				if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
					if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
						_ = Sleep(ctx, time.Duration(seconds)*time.Second)
					}
				}
				lastErr = fmt.Errorf("rate limited for %s: status %d", reqURL, resp.StatusCode)
			case 503, 502, 504: // Server errors - retry
				lastErr = fmt.Errorf("server error for %s: status %d", reqURL, resp.StatusCode)
			case 404, 403, 401: // Client errors - don't retry
				return fmt.Errorf("client error for %s: status %d (not retrying)", reqURL, resp.StatusCode)
			default:
				lastErr = fmt.Errorf("unexpected status for %s: %d", reqURL, resp.StatusCode)
			}
			return lastErr
		}

		// Success - caller must close response body
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Anti-scraping delay after successful request
	_ = Sleep(ctx, SuccessDelay)

	return resp, nil
}

// GetDocument performs a GET request and parses the response as HTML
func (c *Client) GetDocument(ctx context.Context, reqURL string) (*goquery.Document, error) {
	resp, err := c.Get(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	return c.processResponseToDocument(resp)
}

// PostFormDocument performs a POST request with form data and parses the response as HTML
func (c *Client) PostFormDocument(ctx context.Context, postURL string, formData url.Values) (*goquery.Document, error) {
	return c.PostFormDocumentRaw(ctx, postURL, formData.Encode())
}

// PostFormDocumentRaw performs a POST request with raw form data string and parses the response as HTML
// Use this when you need custom encoding (e.g., Big5) instead of standard UTF-8 url.Values encoding
// On success: waits SuccessDelay (2s) before returning (anti-scraping)
// On failure: retries with exponential backoff (4s initial, 5 retries)
func (c *Client) PostFormDocumentRaw(ctx context.Context, postURL, formDataStr string) (*goquery.Document, error) {
	var resp *http.Response
	var lastErr error

	// Retry with exponential backoff on failure (4s initial delay)
	err := RetryWithBackoff(ctx, c.maxRetries, 4*time.Second, func() error {
		// Create POST request with form data
		req, err := http.NewRequestWithContext(ctx, "POST", postURL, strings.NewReader(formDataStr))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers for form POST
		req.Header.Set("User-Agent", c.randomUserAgent())
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "zh-TW,zh;q=0.9,en-US;q=0.8,en;q=0.7")
		req.Header.Set("Accept-Encoding", "gzip, deflate")

		// Perform request
		var httpErr error
		resp, httpErr = c.httpClient.Do(req)
		if httpErr != nil {
			lastErr = fmt.Errorf("request failed: %w", httpErr)
			return lastErr
		}

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()

			switch resp.StatusCode {
			case 429: // Rate limited - retry with backoff, respect Retry-After if present
				if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
					if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
						_ = Sleep(ctx, time.Duration(seconds)*time.Second)
					}
				}
				lastErr = fmt.Errorf("rate limited for %s: status %d", postURL, resp.StatusCode)
			case 503, 502, 504:
				lastErr = fmt.Errorf("server error for %s: status %d", postURL, resp.StatusCode)
			case 404, 403, 401:
				return fmt.Errorf("client error for %s: status %d (not retrying)", postURL, resp.StatusCode)
			default:
				lastErr = fmt.Errorf("unexpected status for %s: %d", postURL, resp.StatusCode)
			}
			return lastErr
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Anti-scraping delay after successful request
	_ = Sleep(ctx, SuccessDelay)

	return c.processResponseToDocument(resp)
}

// processResponseToDocument processes an HTTP response and parses it as HTML document.
// Handles gzip decompression and Big5 to UTF-8 encoding conversion.
// The response body is closed after processing.
func (c *Client) processResponseToDocument(resp *http.Response) (*goquery.Document, error) {
	defer func() { _ = resp.Body.Close() }()

	// Handle gzip encoding
	var reader io.Reader = resp.Body
	var gzipReader *gzip.Reader
	if resp.Header.Get("Content-Encoding") == "gzip" {
		var err error
		gzipReader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip: %w", err)
		}
		defer func() { _ = gzipReader.Close() }()
		reader = gzipReader
	}

	// Check if content is Big5 encoded (common for Taiwan websites)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToUpper(contentType), "BIG5") {
		// Wrap reader with Big5 to UTF-8 decoder
		reader = transform.NewReader(reader, traditionalchinese.Big5.NewDecoder())
	}

	// Parse HTML from reader
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return doc, nil
}

// randomUserAgent returns a random user agent string using uarand package
func (c *Client) randomUserAgent() string {
	return uarand.GetRandom()
}

// TryFailoverURLs attempts to use alternative base URLs when primary URL fails
// Returns the working URL or empty string if all URLs failed
func (c *Client) TryFailoverURLs(ctx context.Context, domain string) (string, error) {
	c.mu.RLock()
	urls, exists := c.baseURLs[domain]
	c.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("no failover URLs configured for domain: %s", domain)
	}

	// Try each URL
	for _, baseURL := range urls {
		// Simple HEAD request to check if URL is accessible
		req, err := http.NewRequestWithContext(ctx, "HEAD", baseURL, http.NoBody)
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", c.randomUserAgent())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode < 500 {
			// URL is accessible
			return baseURL, nil
		}
	}

	return "", fmt.Errorf("all failover URLs failed for domain: %s", domain)
}

// GetBaseURLs returns the list of base URLs for a domain
func (c *Client) GetBaseURLs(domain string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	urls, exists := c.baseURLs[domain]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]string, len(urls))
	copy(result, urls)
	return result
}
