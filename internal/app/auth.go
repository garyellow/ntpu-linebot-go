package app

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// metricsAuthMiddleware returns a Gin middleware that enforces Basic Auth for /metrics.
// If password is empty, authentication is disabled (pass-through)
// and internal Docker network deployments.
func metricsAuthMiddleware(username, password string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth if password is not configured
		if password == "" {
			c.Next()
			return
		}

		user, pass, hasAuth := c.Request.BasicAuth()
		if !hasAuth {
			c.Header("WWW-Authenticate", `Basic realm="metrics"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// Constant-time comparison to prevent timing attacks
		userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1
		passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1

		if !userMatch || !passMatch {
			c.Header("WWW-Authenticate", `Basic realm="metrics"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}
