package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/yourusername/vidly-backend/internal/api"
	"github.com/yourusername/vidly-backend/internal/config"
	"github.com/yourusername/vidly-backend/internal/downloader"
	"github.com/yourusername/vidly-backend/internal/ratelimit"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	limiter := ratelimit.NewLimiter(cfg.RateLimit, cfg.RateWindow)
	ytdlp := downloader.New(cfg)
	// Best-effort yt-dlp update on boot.
	if err := ytdlp.Update(); err != nil {
		fmt.Println("yt-dlp update skipped (kept previous version):", err)
	}

	// Sweep old temp dirs from previous crashes.
	if entries, err := os.ReadDir(cfg.TempDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "dl_") {
				os.RemoveAll(filepath.Join(cfg.TempDir, e.Name()))
			}
		}
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(cfg, limiter, ytdlp),
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Graceful shutdown: on SIGINT/SIGTERM, stop accepting new conns
	// and give in-flight downloads up to 30s to finish.
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()
	fmt.Println("vidly backend listening on :" + cfg.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}
