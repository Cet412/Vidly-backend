package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yourusername/vidly-backend/internal/config"
	"github.com/yourusername/vidly-backend/internal/downloader"
	"github.com/yourusername/vidly-backend/internal/ratelimit"
)

// allowedHosts is an exact allowlist of supported platform hosts.
// Using a map (not regex) prevents bypass via userinfo, ports, unicode, etc.
var allowedHosts = map[string]bool{
	"youtube.com":     true,
	"www.youtube.com": true,
	"m.youtube.com":   true,
	"youtu.be":        true,
	"tiktok.com":      true,
	"www.tiktok.com":  true,
	"vm.tiktok.com":   true,
	"vt.tiktok.com":   true,
	"instagram.com":   true,
	"www.instagram.com": true,
	"instagr.am":      true,
}

type Handler struct {
	cfg      *config.Config
	limiter  *ratelimit.Limiter
	ytdlp    *downloader.YtDlp
	sem      chan struct{} // global concurrency cap
}

func NewHandler(cfg *config.Config, limiter *ratelimit.Limiter, ytdlp *downloader.YtDlp) *Handler {
	return &Handler{
		cfg:     cfg,
		limiter: limiter,
		ytdlp:   ytdlp,
		sem:     make(chan struct{}, 2), // max 2 concurrent downloads
	}
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
}

type downloadRequest struct {
	URL string `json:"url"`
}

// Download handles POST /api/v1/download.
func (h *Handler) Download(c *gin.Context) {
	// Limit request body size to prevent abuse.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20) // 1 MB max

	var req downloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "format request tidak valid")
		return
	}

	if len(req.URL) == 0 || len(req.URL) > h.cfg.MaxURLLength {
		respondError(c, http.StatusBadRequest, "URL tidak valid")
		return
	}

	safeURL, err := validateURL(req.URL)
	if err != nil {
		respondError(c, http.StatusBadRequest, "platform belum didukung")
		return
	}

	ip := c.ClientIP()
	if !h.limiter.Allow(ip) {
		respondError(c, http.StatusTooManyRequests, "batas unduhan tercapai, coba lagi dalam 1 jam")
		return
	}

	// Global concurrency cap.
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	case <-c.Request.Context().Done():
		h.limiter.Refund(ip)
		return
	default:
		h.limiter.Refund(ip)
		respondError(c, http.StatusServiceUnavailable, "server sibuk, coba lagi nanti")
		return
	}

	if err := os.MkdirAll(h.cfg.TempDir, 0o750); err != nil {
		h.limiter.Refund(ip)
		respondError(c, http.StatusInternalServerError, "terjadi kesalahan")
		return
	}

	// Per-request temp directory for isolation.
	tempDir, err := os.MkdirTemp(h.cfg.TempDir, "dl_*")
	if err != nil {
		h.limiter.Refund(ip)
		respondError(c, http.StatusInternalServerError, "terjadi kesalahan")
		return
	}
	defer os.RemoveAll(tempDir)

	id, err := randomID()
	if err != nil {
		h.limiter.Refund(ip)
		respondError(c, http.StatusInternalServerError, "terjadi kesalahan")
		return
	}

	// Use fixed output template (never from user input).
	tmpl := filepath.Join(tempDir, id+".%(ext)s")

	// Timeout: 90 seconds max per download.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	out, err := h.ytdlp.Download(ctx, safeURL, tmpl)
	if err != nil {
		h.limiter.Refund(ip)
		// Log detail server-side only; never expose to client.
		fmt.Printf("ytdlp failed: %s\n", redactURL(string(out)))
		// Detect photo-only content.
		if strings.Contains(string(out), "There is no video in this post") {
			respondError(c, http.StatusBadRequest, "Konten ini adalah foto. Hanya video yang bisa diunduh.")
			return
		}
		respondError(c, http.StatusBadGateway, "gagal mengunduh dari sumber, coba lagi nanti")
		return
	}

	path, err := findOutputFile(tempDir, id)
	if err != nil {
		h.limiter.Refund(ip)
		fmt.Printf("output file not found: %v\n", err)
		respondError(c, http.StatusBadGateway, "gagal mengunduh dari sumber, coba lagi nanti")
		return
	}

	// Validate the resolved path is inside tempDir (prevent traversal).
	absPath, _ := filepath.Abs(path)
	absTemp, _ := filepath.Abs(tempDir)
	if !strings.HasPrefix(absPath, absTemp) {
		h.limiter.Refund(ip)
		respondError(c, http.StatusInternalServerError, "terjadi kesalahan")
		return
	}

	// Post-download size check (max 100 MB).
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > 100*1024*1024 {
		h.limiter.Refund(ip)
		os.Remove(path)
		respondError(c, http.StatusBadRequest, "file terlalu besar")
		return
	}

	defer os.Remove(path)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	c.Header("Content-Length", fmt.Sprintf("%d", fi.Size()))
	c.File(path)
}

// validateURL parses and validates the URL against an exact host allowlist.
func validateURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", errors.New("scheme must be https")
	}
	if u.User != nil {
		return "", errors.New("userinfo not allowed")
	}
	if u.Port() != "" {
		return "", errors.New("port not allowed")
	}
	host := strings.ToLower(u.Hostname())
	if !allowedHosts[host] {
		return "", errors.New("host not allowed")
	}
	// Return canonical URL string.
	return u.String(), nil
}

// findOutputFile locates the file yt-dlp wrote.
func findOutputFile(dir, id string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, id+".*"))
	if err != nil || len(matches) == 0 {
		return "", errors.New("output file not found")
	}
	return matches[0], nil
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func respondError(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

// redactURL removes query parameters from URLs in log output.
func redactURL(s string) string {
	// Simple redaction: replace query strings.
	if idx := strings.Index(s, "?"); idx >= 0 {
		return s[:idx] + "?[REDACTED]"
	}
	return s
}
