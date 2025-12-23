// Package scraper provides HTTP client utilities for web scraping.
// It includes retry logic, rate limiting, URL failover, and encoding conversion
// for scraping NTPU websites.
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
// maxRetries: max retry attempts with exponential backoff (e.g., 10)
// baseURLs: map of domain to list of base URLs for failover
func NewClient(timeout time.Duration, maxRetries int, baseURLs map[string][]string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
		maxRetries: maxRetries,
		baseURLs:   baseURLs,
	}
}

// GetDocument performs a GET request and parses the response as HTML.
// Includes retry with exponential backoff, gzip decompression, and Big5 encoding conversion.
// No fixed delay between requests - relies on retry backoff for rate limiting.
func (c *Client) GetDocument(ctx context.Context, reqURL string) (*goquery.Document, error) {
	resp, err := c.doRequest(ctx, "GET", reqURL, "")
	if err != nil {
		return nil, err
	}

	// Process document and return immediately on success
	doc, err := c.processResponseToDocument(resp)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// PostFormDocument performs a POST request with form data and parses the response as HTML.
func (c *Client) PostFormDocument(ctx context.Context, postURL string, formData url.Values) (*goquery.Document, error) {
	return c.PostFormDocumentRaw(ctx, postURL, formData.Encode())
}

// PostFormDocumentRaw performs a POST request with raw form data string and parses the response as HTML.
// Use this when you need custom encoding (e.g., Big5) instead of standard UTF-8 url.Values encoding.
// No fixed delay between requests - relies on retry backoff for rate limiting.
func (c *Client) PostFormDocumentRaw(ctx context.Context, postURL, formDataStr string) (*goquery.Document, error) {
	resp, err := c.doRequest(ctx, "POST", postURL, formDataStr)
	if err != nil {
		return nil, err
	}

	// Process document and return immediately on success
	doc, err := c.processResponseToDocument(resp)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// doRequest performs an HTTP request with retry logic and status code handling.
// This is the core request method used by GetDocument and PostFormDocumentRaw.
// Returns the response on success; caller is responsible for closing the body.
// Retry starts from 1 second with exponential backoff up to maxRetries (default: 10).
func (c *Client) doRequest(ctx context.Context, method, reqURL, body string) (*http.Response, error) {
	var resp *http.Response
	var lastErr error

	err := RetryWithBackoff(ctx, c.maxRetries, 1*time.Second, func() error {
		// Create request with optional body
		var bodyReader io.Reader = http.NoBody
		if body != "" {
			bodyReader = strings.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set common headers
		req.Header.Set("User-Agent", c.randomUserAgent())
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "zh-TW,zh;q=0.9,en-US;q=0.8,en;q=0.7")
		req.Header.Set("Accept-Encoding", "gzip, deflate")

		// Set Content-Type for POST requests
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		// Perform request
		var httpErr error
		resp, httpErr = c.httpClient.Do(req)
		if httpErr != nil {
			lastErr = fmt.Errorf("request failed: %w", httpErr)
			return lastErr
		}

		// Handle non-success status codes
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			lastErr = c.handleErrorStatus(ctx, reqURL, resp.StatusCode, resp.Header)
			return lastErr
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// handleErrorStatus processes HTTP error status codes and returns appropriate errors.
// For client errors (4xx except 429), returns a permanentError that won't be retried.
// For server errors (5xx) and rate limits (429), returns a regular error for retry.
func (c *Client) handleErrorStatus(ctx context.Context, reqURL string, statusCode int, header http.Header) error {
	switch statusCode {
	case 429: // Rate limited - retry with backoff, respect Retry-After if present
		if retryAfter := header.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				_ = Sleep(ctx, time.Duration(seconds)*time.Second)
			}
		}
		return fmt.Errorf("rate limited for %s: status %d", reqURL, statusCode)
	case 503, 502, 504: // Server errors - retry
		return fmt.Errorf("server error for %s: status %d", reqURL, statusCode)
	case 404, 403, 401: // Client errors - don't retry
		return &permanentError{fmt.Errorf("client error for %s: status %d", reqURL, statusCode)}
	default:
		return fmt.Errorf("unexpected status for %s: %d", reqURL, statusCode)
	}
}

// permanentError wraps an error to indicate it should not be retried.
type permanentError struct {
	err error
}

func (e *permanentError) Error() string {
	return e.err.Error()
}

func (e *permanentError) Unwrap() error {
	return e.err
}

// ClearURLCache clears the URL cache for a specific domain.
// This triggers re-detection of working URLs on the next request.
// Domain must be one of: "lms", "sea"
func (c *Client) ClearURLCache(domain string) {
	NewURLCache(c, domain).Clear()
}

// ClearAllURLCaches clears URL caches for all configured domains.
// Use this when widespread connectivity issues are detected.
func (c *Client) ClearAllURLCaches() {
	c.mu.RLock()
	domains := make([]string, 0, len(c.baseURLs))
	for domain := range c.baseURLs {
		domains = append(domains, domain)
	}
	c.mu.RUnlock()

	for _, domain := range domains {
		c.ClearURLCache(domain)
	}
}

// processResponseToDocument processes an HTTP response and parses it as HTML document.
// Handles gzip decompression and Big5 to UTF-8 encoding conversion.
// The response body is closed after processing.
func (c *Client) processResponseToDocument(resp *http.Response) (*goquery.Document, error) {
	defer func() { _ = resp.Body.Close() }()

	var reader io.Reader = resp.Body

	// Handle gzip encoding
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip: %w", err)
		}
		defer func() { _ = gzipReader.Close() }()
		reader = gzipReader
	}

	// Handle Big5 encoding (common for Taiwan websites)
	if strings.Contains(strings.ToUpper(resp.Header.Get("Content-Type")), "BIG5") {
		reader = transform.NewReader(reader, traditionalchinese.Big5.NewDecoder())
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return doc, nil
}

// randomUserAgent returns a random user agent string
func (c *Client) randomUserAgent() string {
	return uarand.GetRandom()
}

// TryFailoverURLs attempts to use alternative base URLs when primary URL fails.
// Returns the working URL or error if all URLs failed.
// Uses HEAD requests for quick availability checks.
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
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL, http.NoBody)
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
