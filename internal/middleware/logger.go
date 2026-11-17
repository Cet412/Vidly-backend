package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger returns a structured JSON request logger.
// It never logs query strings or bodies, so no secrets/tokens leak.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"ip", c.ClientIP(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

// ErrorHandler converts panics into a generic 500 response and logs
// the real error server-side. Client never sees internal details.
func ErrorHandler() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		slog.Error("panic recovered", "error", recovered, "path", c.Request.URL.Path)
		c.AbortWithStatusJSON(500, gin.H{"error": "terjadi kesalahan, coba lagi nanti"})
	})
}
