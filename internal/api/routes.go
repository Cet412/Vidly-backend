package api

import (
	"github.com/gin-gonic/gin"

	"github.com/yourusername/vidly-backend/internal/config"
	"github.com/yourusername/vidly-backend/internal/downloader"
	"github.com/yourusername/vidly-backend/internal/middleware"
	"github.com/yourusername/vidly-backend/internal/ratelimit"
)

// NewRouter builds the Gin engine with all middleware and routes.
// All routes are versioned under /api/v1 so future breaking changes
// can ship as /api/v2 without breaking installed Android apps.
func NewRouter(cfg *config.Config, limiter *ratelimit.Limiter, ytdlp *downloader.YtDlp) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middleware.Logger(), middleware.ErrorHandler())
	r.MaxMultipartMemory = 1 << 20 // tiny; API is JSON-only

	h := NewHandler(cfg, limiter, ytdlp)

	r.GET("/health", h.Health)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/download", h.Download)
	}

	return r
}
