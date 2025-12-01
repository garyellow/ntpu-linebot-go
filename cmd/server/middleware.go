// Package main provides the LINE bot server entry point.
package main

import (
	"time"

	"github.com/garyellow/ntpu-linebot-go/internal/logger"
	"github.com/gin-gonic/gin"
)

// securityHeadersMiddleware adds security headers to all responses
// Reference: https://gin-gonic.com/en/docs/examples/security-headers
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")
		// Enable XSS filter in browsers
		c.Header("X-XSS-Protection", "1; mode=block")
		// Strict referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		// Restrict permissions
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// Content Security Policy - prevent XSS attacks
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// loggingMiddleware logs HTTP requests
func loggingMiddleware(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Process request
		c.Next()

		// Log request
		duration := time.Since(start)
		status := c.Writer.Status()

		entry := log.WithField("method", method).
			WithField("path", path).
			WithField("status", status).
			WithField("duration_ms", duration.Milliseconds()).
			WithField("ip", c.ClientIP())

		if len(c.Errors) > 0 {
			entry.WithField("errors", c.Errors.String()).Error("Request completed with errors")
		} else {
			switch {
			case status >= 500:
				entry.Error("Request failed")
			case status >= 400:
				entry.Warn("Request completed with client error")
			default:
				entry.Debug("Request completed")
			}
		}
	}
}
