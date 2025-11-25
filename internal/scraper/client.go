package scraper

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/corpix/uarand"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// Client is an HTTP client for web scraping with rate limiting and URL failover
type Client struct {
	httpClient  *http.Client
	rateLimiter *RateLimiter
	userAgents  []string
	maxRetries  int
	baseURLs    map[string][]string // Base URLs for failover by domain
	mu          sync.RWMutex
}

// NewClient creates a new scraper client with URL failover support
func NewClient(timeout time.Duration, workers int, minDelay, maxDelay time.Duration, maxRetries int) *Client {
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
		rateLimiter: NewRateLimiter(workers, minDelay, maxDelay),
		userAgents:  generateUserAgents(),
		maxRetries:  maxRetries,
		baseURLs:    baseURLs,
	}
}

// Get performs a GET request with rate limiting and retries
// Caller is responsible for closing the response body
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	var resp *http.Response
	var lastErr error

	// Retry with exponential backoff
	err := RetryWithBackoff(ctx, c.maxRetries, 1*time.Second, 30*time.Second, func() error {
		// Wait for rate limiter
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return err
		}

		// Create request
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set random User-Agent
		req.Header.Set("User-Agent", c.randomUserAgent())
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "zh-TW,zh;q=0.9,en-US;q=0.8,en;q=0.7")
		req.Header.Set("Accept-Encoding", "gzip, deflate")

		// Perform request
		resp, err = c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			return lastErr
		}

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// Close body for non-success responses since we won't return it
			_ = resp.Body.Close()

			switch resp.StatusCode {
			case 429: // Rate limited - retry with backoff
				lastErr = fmt.Errorf("rate limited for %s: status %d", url, resp.StatusCode)
			case 503, 502, 504: // Server errors - retry
				lastErr = fmt.Errorf("server error for %s: status %d", url, resp.StatusCode)
			case 404, 403, 401: // Client errors - don't retry
				return fmt.Errorf("client error for %s: status %d (not retrying)", url, resp.StatusCode)
			default:
				lastErr = fmt.Errorf("unexpected status for %s: %d", url, resp.StatusCode)
			}
			return lastErr
		}

		// Success - caller must close response body
		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetDocument performs a GET request and parses the response as HTML
func (c *Client) GetDocument(ctx context.Context, url string) (*goquery.Document, error) {
	resp, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle gzip encoding
	var reader io.Reader
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip: %w", err)
		}
		defer func() { _ = gzipReader.Close() }()
		reader = gzipReader
	} else {
		reader = resp.Body
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

// PostFormDocument performs a POST request with form data and parses the response as HTML
func (c *Client) PostFormDocument(ctx context.Context, postURL string, formData url.Values) (*goquery.Document, error) {
	return c.PostFormDocumentRaw(ctx, postURL, formData.Encode())
}

// PostFormDocumentRaw performs a POST request with raw form data string and parses the response as HTML
// Use this when you need custom encoding (e.g., Big5) instead of standard UTF-8 url.Values encoding
func (c *Client) PostFormDocumentRaw(ctx context.Context, postURL string, formDataStr string) (*goquery.Document, error) {
	var resp *http.Response
	var lastErr error

	// Retry with exponential backoff
	err := RetryWithBackoff(ctx, c.maxRetries, 1*time.Second, 30*time.Second, func() error {
		// Wait for rate limiter
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return err
		}

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
		resp, err = c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			return lastErr
		}

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()

			switch resp.StatusCode {
			case 429:
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
	defer func() { _ = resp.Body.Close() }()

	// Handle gzip encoding
	var reader io.Reader
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip: %w", err)
		}
		defer func() { _ = gzipReader.Close() }()
		reader = gzipReader
	} else {
		reader = resp.Body
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

// randomUserAgent returns a random user agent string
func (c *Client) randomUserAgent() string {
	if len(c.userAgents) == 0 {
		return uarand.GetRandom()
	}
	return c.userAgents[time.Now().UnixNano()%int64(len(c.userAgents))]
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
		req, err := http.NewRequestWithContext(ctx, "HEAD", baseURL, nil)
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

// generateUserAgents returns a list of common user agent strings
func generateUserAgents() []string {
	return []string{
		// Chrome on Windows
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",

		// Chrome on macOS
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",

		// Firefox on Windows
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",

		// Firefox on macOS
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0",

		// Safari on macOS
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",

		// Edge on Windows
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",

		// Chrome on Linux
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}
}
