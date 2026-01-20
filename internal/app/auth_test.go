package app

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestMetricsAuthMiddleware_NoPasswordBypass(t *testing.T) {
	// When disabled, middleware should pass through without auth
	router := gin.New()
	router.GET("/metrics", metricsAuthMiddleware(false, "prometheus", ""), func(c *gin.Context) {
		c.String(http.StatusOK, "metrics")
	})

	// Request without auth header should succeed
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "metrics", w.Body.String())
}

func TestMetricsAuthMiddleware_ValidCredentials(t *testing.T) {
	router := gin.New()
	router.GET("/metrics", metricsAuthMiddleware(true, "prometheus", "secret123"), func(c *gin.Context) {
		c.String(http.StatusOK, "metrics")
	})

	// Request with valid auth header
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("prometheus:secret123")))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "metrics", w.Body.String())
}

func TestMetricsAuthMiddleware_InvalidCredentials(t *testing.T) {
	router := gin.New()
	router.GET("/metrics", metricsAuthMiddleware(true, "prometheus", "secret123"), func(c *gin.Context) {
		c.String(http.StatusOK, "metrics")
	})

	tests := []struct {
		name     string
		username string
		password string
	}{
		{"wrong username", "wronguser", "secret123"},
		{"wrong password", "prometheus", "wrongpass"},
		{"both wrong", "wronguser", "wrongpass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(tt.username+":"+tt.password)))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic realm=")
		})
	}
}

func TestMetricsAuthMiddleware_NoAuthHeader(t *testing.T) {
	router := gin.New()
	router.GET("/metrics", metricsAuthMiddleware(true, "prometheus", "secret123"), func(c *gin.Context) {
		c.String(http.StatusOK, "metrics")
	})

	// Request without auth header when password is configured
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic realm=")
}

func TestMetricsAuthMiddleware_MalformedAuthHeader(t *testing.T) {
	router := gin.New()
	router.GET("/metrics", metricsAuthMiddleware(true, "prometheus", "secret123"), func(c *gin.Context) {
		c.String(http.StatusOK, "metrics")
	})

	tests := []struct {
		name   string
		header string
	}{
		{"empty", ""},
		{"only basic", "Basic"},
		{"invalid base64", "Basic notbase64!!!"},
		{"bearer token", "Bearer sometoken"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}
